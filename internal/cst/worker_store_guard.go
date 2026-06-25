package cst

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const workerBindingFile = "cst-worker.json"
const workerBindingFallbackFile = "worker.json"

type WorkerStoreBinding struct {
	StoreRoot   string   `json:"store_root"`
	StoreID     string   `json:"store_id,omitempty"`
	ExecCWD     string   `json:"exec_cwd"`
	ExecSurface string   `json:"exec_surface,omitempty"`
	OwnedPaths  []string `json:"owned_paths,omitempty"`
}

func GuardImplicitWorkerStoreMutation(recovery string) error {
	if StoreRootExplicit() {
		return nil
	}
	binding, ok, err := DetectWorkerStoreBinding("")
	if err != nil || !ok {
		return err
	}
	if recovery == "" {
		recovery = fmt.Sprintf("cst --store %s ...", shellQuote(binding.StoreRoot))
	}
	return WorkerStoreGuardError(binding, recovery)
}

func WorkerStoreGuardError(binding WorkerStoreBinding, recovery string) error {
	return herr(ExitInvariantBroken,
		"worker checkout %s is bound to central CST store %s; mutating commands require explicit --store; recovery: %s",
		binding.ExecCWD, binding.StoreRoot, recovery)
}

func DetectWorkerStoreBinding(start string) (WorkerStoreBinding, bool, error) {
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return WorkerStoreBinding{}, false, err
		}
		start = cwd
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return WorkerStoreBinding{}, false, err
	}
	if gitDir, _, ok := gitMetadata(abs); ok {
		binding, found, err := readWorkerBinding(filepath.Join(gitDir, workerBindingFile))
		if err != nil || found {
			return binding, found, err
		}
	}
	for dir := abs; ; dir = filepath.Dir(dir) {
		binding, found, err := readWorkerBinding(filepath.Join(dir, StoreDirName, workerBindingFallbackFile))
		if err != nil || found {
			return binding, found, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return WorkerStoreBinding{}, false, nil
}

func WorkerRecoveryCommand(cmd string, args []string, binding WorkerStoreBinding) string {
	parts := []string{"cst", "--store", binding.StoreRoot, cmd}
	parts = append(parts, args...)
	if cmd == "take" && binding.ExecCWD != "" && !hasFlag(args, "exec-cwd") {
		parts = append(parts, "--exec-cwd", binding.ExecCWD)
		if binding.ExecSurface == ExecSurfacePrivate && !hasFlag(args, "private-exec-cwd") {
			parts = append(parts, "--private-exec-cwd")
		}
	}
	return shellJoin(parts)
}

func recordWorkerStoreBindings(paths StorePaths, storeID string, events []*Event) error {
	if !StoreRootExplicit() {
		return nil
	}
	for _, ev := range events {
		if ev == nil || ev.Envelope == nil || ev.Envelope.ExecCWD == "" {
			continue
		}
		if ev.Type != EvNodeCreated && ev.Type != EvNodeRevised {
			continue
		}
		if err := writeWorkerStoreBinding(paths, storeID, *ev.Envelope); err != nil {
			return err
		}
	}
	return nil
}

func writeWorkerStoreBinding(paths StorePaths, storeID string, env ExecutionEnvelope) error {
	execCWD, err := filepath.Abs(env.ExecCWD)
	if err != nil {
		return err
	}
	if pathInsideOrEqual(execCWD, paths.Root) {
		return nil
	}
	info, err := os.Stat(execCWD)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	binding := WorkerStoreBinding{
		StoreRoot:   paths.Root,
		StoreID:     storeID,
		ExecCWD:     execCWD,
		ExecSurface: firstNonEmpty(env.ExecSurface, ExecSurfaceShared),
		OwnedPaths:  normalizeOwnedPaths(env.OwnedPaths),
	}
	path, err := workerBindingWritePath(execCWD)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func workerBindingWritePath(execCWD string) (string, error) {
	if gitDir, _, ok := gitMetadata(execCWD); ok {
		return filepath.Join(gitDir, workerBindingFile), nil
	}
	return filepath.Join(execCWD, StoreDirName, workerBindingFallbackFile), nil
}

func readWorkerBinding(path string) (WorkerStoreBinding, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkerStoreBinding{}, false, nil
		}
		return WorkerStoreBinding{}, false, err
	}
	var binding WorkerStoreBinding
	if err := json.Unmarshal(data, &binding); err != nil {
		return WorkerStoreBinding{}, false, fmt.Errorf("%s: %w", path, err)
	}
	if err := normalizeWorkerBinding(&binding); err != nil {
		return WorkerStoreBinding{}, false, fmt.Errorf("%s: %w", path, err)
	}
	if err := validateWorkerBindingStore(binding); err != nil {
		return WorkerStoreBinding{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return binding, true, nil
}

func normalizeWorkerBinding(binding *WorkerStoreBinding) error {
	binding.StoreRoot = strings.TrimSpace(binding.StoreRoot)
	binding.StoreID = strings.TrimSpace(binding.StoreID)
	binding.ExecCWD = strings.TrimSpace(binding.ExecCWD)
	if binding.StoreRoot == "" {
		return fmt.Errorf("store_root is required")
	}
	if binding.StoreID == "" {
		return fmt.Errorf("store_id is required")
	}
	if binding.ExecCWD == "" {
		return fmt.Errorf("exec_cwd is required")
	}
	storeRoot, err := filepath.Abs(binding.StoreRoot)
	if err != nil {
		return err
	}
	execCWD, err := filepath.Abs(binding.ExecCWD)
	if err != nil {
		return err
	}
	binding.StoreRoot = storeRoot
	binding.ExecCWD = execCWD
	binding.ExecSurface = firstNonEmpty(binding.ExecSurface, ExecSurfaceShared)
	if binding.ExecSurface != ExecSurfaceShared && binding.ExecSurface != ExecSurfacePrivate {
		return fmt.Errorf("exec_surface must be %s or %s", ExecSurfaceShared, ExecSurfacePrivate)
	}
	binding.OwnedPaths = normalizeOwnedPaths(binding.OwnedPaths)
	return nil
}

func validateWorkerBindingStore(binding WorkerStoreBinding) error {
	paths, err := ResolveStorePaths(binding.StoreRoot)
	if err != nil {
		return err
	}
	events, err := ReplayAt(paths)
	if err != nil {
		return err
	}
	state, err := Apply(events)
	if err != nil {
		return err
	}
	if got := state.StoreID(); got != binding.StoreID {
		return fmt.Errorf("store_id %q does not match ledger root %q", binding.StoreID, got)
	}
	return nil
}

func gitMetadata(path string) (string, string, bool) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", "", false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 || lines[0] == "" || lines[1] == "" {
		return "", "", false
	}
	gitDir := strings.TrimSpace(lines[0])
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(path, gitDir)
	}
	gitDir, err = filepath.Abs(gitDir)
	if err != nil {
		return "", "", false
	}
	top, err := filepath.Abs(strings.TrimSpace(lines[1]))
	if err != nil {
		return "", "", false
	}
	return gitDir, top, true
}

func hasFlag(args []string, name string) bool {
	long := "--" + name
	prefix := long + "="
	for _, arg := range args {
		if arg == long || strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func shellJoin(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, shellQuote(part))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(part string) string {
	if part == "" {
		return `""`
	}
	if strings.IndexFunc(part, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '/' || r == '.' || r == ':' ||
			r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
	}) < 0 {
		return part
	}
	return strconv.Quote(part)
}

func sameCleanPath(a string, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func pathInsideOrEqual(child string, parent string) bool {
	childAbs, err := filepath.Abs(child)
	if err == nil {
		child = childAbs
	}
	parentAbs, err := filepath.Abs(parent)
	if err == nil {
		parent = parentAbs
	}
	child = filepath.Clean(child)
	parent = filepath.Clean(parent)
	if child == parent {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
