package cst

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type ExecIdentity struct {
	ExecCWD                       string   `json:"exec_cwd,omitempty"`
	GitAvailable                  bool     `json:"git_available"`
	GitRoot                       string   `json:"git_root,omitempty"`
	GitHead                       string   `json:"git_head,omitempty"`
	GitBranch                     string   `json:"git_branch,omitempty"`
	GitStatusShort                string   `json:"git_status_short,omitempty"`
	StagedDiffSHA256              string   `json:"staged_diff_sha256,omitempty"`
	UnstagedDiffSHA256            string   `json:"unstaged_diff_sha256,omitempty"`
	UntrackedManifestSHA256       string   `json:"untracked_manifest_sha256,omitempty"`
	GitIdentityDigest             string   `json:"git_identity_digest,omitempty"`
	ExecSurface                   string   `json:"exec_surface,omitempty"`
	OwnedPaths                    []string `json:"owned_paths,omitempty"`
	ScopedGitStatusShort          string   `json:"scoped_git_status_short,omitempty"`
	ScopedStagedDiffSHA256        string   `json:"scoped_staged_diff_sha256,omitempty"`
	ScopedUnstagedDiffSHA256      string   `json:"scoped_unstaged_diff_sha256,omitempty"`
	ScopedUntrackedManifestSHA256 string   `json:"scoped_untracked_manifest_sha256,omitempty"`
	ScopedDigest                  string   `json:"scoped_digest,omitempty"`
	OutOfScopeGitStatusShort      string   `json:"out_of_scope_git_status_short,omitempty"`
	OutOfScopeDeltaCount          int      `json:"out_of_scope_delta_count,omitempty"`
	OutOfScopeDigest              string   `json:"out_of_scope_digest,omitempty"`
	WholeRepoDigest               string   `json:"whole_repo_digest,omitempty"`
	ParallelWorktree              string   `json:"parallel_worktree,omitempty"`
	ExecContextDigest             string   `json:"exec_context_digest,omitempty"`
}

func CaptureExecIdentity(execCWD string, envelope ExecutionEnvelope) ExecIdentity {
	abs := execCWD
	if abs == "" {
		if cwd, err := os.Getwd(); err == nil {
			abs = cwd
		}
	}
	if resolved, err := filepath.Abs(abs); err == nil {
		abs = resolved
	}
	id := ExecIdentity{
		ExecCWD:          abs,
		ExecSurface:      envelope.ExecSurface,
		OwnedPaths:       normalizeOwnedPaths(envelope.OwnedPaths),
		ParallelWorktree: os.Getenv("PARALLEL_WORKTREE"),
	}
	if id.ExecSurface == "" {
		id.ExecSurface = ExecSurfaceShared
	}
	root, ok := gitOutput(abs, "rev-parse", "--show-toplevel")
	if !ok {
		id.ExecContextDigest = execContextDigest("", id)
		return id
	}
	id.GitAvailable = true
	id.GitRoot = strings.TrimSpace(root)
	id.GitHead = gitOutputString(abs, "rev-parse", "--verify", "HEAD")
	id.GitBranch = gitOutputString(abs, "rev-parse", "--abbrev-ref", "HEAD")
	id.GitStatusShort = gitOutputString(abs, "status", "--short")
	id.StagedDiffSHA256 = gitOutputHash(abs, "diff", "--cached", "--binary", "--full-index", "--no-ext-diff")
	id.UnstagedDiffSHA256 = gitOutputHash(abs, "diff", "--binary", "--full-index", "--no-ext-diff")
	id.UntrackedManifestSHA256 = untrackedManifestHash(abs, id.GitRoot)
	id.GitIdentityDigest = gitIdentityDigest(id)
	id.WholeRepoDigest = id.GitIdentityDigest
	id.populateScopedIdentity(abs)
	id.ExecContextDigest = execContextDigest("", id)
	return id
}

func (id *ExecIdentity) populateScopedIdentity(abs string) {
	if len(id.OwnedPaths) == 0 {
		return
	}
	id.ScopedGitStatusShort = statusSummary(statusEntries(abs), func(path string) bool {
		return pathInOwnedPaths(id.OwnedPaths, path)
	})
	id.OutOfScopeGitStatusShort = statusSummary(statusEntries(abs), func(path string) bool {
		return !pathInOwnedPaths(id.OwnedPaths, path)
	})
	id.OutOfScopeDeltaCount = countStatusEntries(statusEntries(abs), func(path string) bool {
		return !pathInOwnedPaths(id.OwnedPaths, path)
	})
	pathspec := append([]string{"diff", "--cached", "--binary", "--full-index", "--no-ext-diff", "--"}, id.OwnedPaths...)
	id.ScopedStagedDiffSHA256 = gitOutputHash(abs, pathspec...)
	pathspec = append([]string{"diff", "--binary", "--full-index", "--no-ext-diff", "--"}, id.OwnedPaths...)
	id.ScopedUnstagedDiffSHA256 = gitOutputHash(abs, pathspec...)
	id.ScopedUntrackedManifestSHA256 = untrackedManifestHashForPaths(abs, id.GitRoot, id.OwnedPaths, true)
	id.ScopedDigest = scopedDigest(*id)
	id.OutOfScopeDigest = outOfScopeDigest(*id)
}

func gitOutput(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

func gitOutputString(dir string, args ...string) string {
	out, ok := gitOutput(dir, args...)
	if !ok {
		return ""
	}
	return strings.TrimSpace(out)
}

func gitOutputHash(dir string, args ...string) string {
	out, ok := gitOutput(dir, args...)
	if !ok {
		return ""
	}
	return sha256Hex([]byte(out))
}

type untrackedManifestEntry struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func untrackedManifestHash(dir string, gitRoot string) string {
	return untrackedManifestHashForPaths(dir, gitRoot, nil, true)
}

func untrackedManifestHashForPaths(dir string, gitRoot string, ownedPaths []string, wantOwned bool) string {
	raw, ok := gitOutput(dir, "ls-files", "--others", "--exclude-standard", "-z")
	if !ok {
		return ""
	}
	parts := strings.Split(raw, "\x00")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	entries := make([]untrackedManifestEntry, 0, len(paths))
	for _, p := range paths {
		if len(ownedPaths) > 0 && pathInOwnedPaths(ownedPaths, p) != wantOwned {
			continue
		}
		full := filepath.Join(gitRoot, filepath.FromSlash(p))
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		entries = append(entries, untrackedManifestEntry{
			Path:   p,
			Mode:   info.Mode().String(),
			Size:   info.Size(),
			SHA256: sha256Hex(data),
		})
	}
	b, _ := json.Marshal(entries)
	return sha256Hex(b)
}

type gitStatusEntry struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

func statusEntries(dir string) []gitStatusEntry {
	raw, ok := gitOutput(dir, "status", "--porcelain=v1", "--untracked-files=all", "-z")
	if !ok || raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\x00")
	entries := make([]gitStatusEntry, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		if len(part) < 4 {
			continue
		}
		code := part[:2]
		path := part[3:]
		if code[0] == 'R' || code[1] == 'R' || code[0] == 'C' || code[1] == 'C' {
			if i+1 < len(parts) && parts[i+1] != "" {
				entries = append(entries, gitStatusEntry{Code: code, Path: parts[i+1]})
				i++
			}
		}
		entries = append(entries, gitStatusEntry{Code: code, Path: path})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path == entries[j].Path {
			return entries[i].Code < entries[j].Code
		}
		return entries[i].Path < entries[j].Path
	})
	return entries
}

func statusSummary(entries []gitStatusEntry, keep func(string) bool) string {
	filtered := make([]gitStatusEntry, 0, len(entries))
	for _, entry := range entries {
		if keep(entry.Path) {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	var b strings.Builder
	for _, entry := range filtered {
		b.WriteString(entry.Code)
		b.WriteByte(' ')
		b.WriteString(entry.Path)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func countStatusEntries(entries []gitStatusEntry, keep func(string) bool) int {
	count := 0
	for _, entry := range entries {
		if keep(entry.Path) {
			count++
		}
	}
	return count
}

func pathInOwnedPaths(ownedPaths []string, path string) bool {
	for _, scope := range ownedPaths {
		if ownedPathContains(scope, path) {
			return true
		}
	}
	return false
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func gitIdentityDigest(id ExecIdentity) string {
	b, _ := json.Marshal(struct {
		GitAvailable            bool   `json:"git_available"`
		GitRoot                 string `json:"git_root,omitempty"`
		GitHead                 string `json:"git_head,omitempty"`
		GitBranch               string `json:"git_branch,omitempty"`
		GitStatusShort          string `json:"git_status_short,omitempty"`
		StagedDiffSHA256        string `json:"staged_diff_sha256,omitempty"`
		UnstagedDiffSHA256      string `json:"unstaged_diff_sha256,omitempty"`
		UntrackedManifestSHA256 string `json:"untracked_manifest_sha256,omitempty"`
	}{
		GitAvailable:            id.GitAvailable,
		GitRoot:                 id.GitRoot,
		GitHead:                 id.GitHead,
		GitBranch:               id.GitBranch,
		GitStatusShort:          id.GitStatusShort,
		StagedDiffSHA256:        id.StagedDiffSHA256,
		UnstagedDiffSHA256:      id.UnstagedDiffSHA256,
		UntrackedManifestSHA256: id.UntrackedManifestSHA256,
	})
	return sha256Hex(b)
}

func scopedDigest(id ExecIdentity) string {
	b, _ := json.Marshal(struct {
		OwnedPaths                    []string `json:"owned_paths,omitempty"`
		ScopedGitStatusShort          string   `json:"scoped_git_status_short,omitempty"`
		ScopedStagedDiffSHA256        string   `json:"scoped_staged_diff_sha256,omitempty"`
		ScopedUnstagedDiffSHA256      string   `json:"scoped_unstaged_diff_sha256,omitempty"`
		ScopedUntrackedManifestSHA256 string   `json:"scoped_untracked_manifest_sha256,omitempty"`
	}{
		OwnedPaths:                    append([]string(nil), id.OwnedPaths...),
		ScopedGitStatusShort:          id.ScopedGitStatusShort,
		ScopedStagedDiffSHA256:        id.ScopedStagedDiffSHA256,
		ScopedUnstagedDiffSHA256:      id.ScopedUnstagedDiffSHA256,
		ScopedUntrackedManifestSHA256: id.ScopedUntrackedManifestSHA256,
	})
	return sha256Hex(b)
}

func outOfScopeDigest(id ExecIdentity) string {
	b, _ := json.Marshal(struct {
		OutOfScopeGitStatusShort string `json:"out_of_scope_git_status_short,omitempty"`
		OutOfScopeDeltaCount     int    `json:"out_of_scope_delta_count"`
		WholeRepoDigest          string `json:"whole_repo_digest,omitempty"`
	}{
		OutOfScopeGitStatusShort: id.OutOfScopeGitStatusShort,
		OutOfScopeDeltaCount:     id.OutOfScopeDeltaCount,
		WholeRepoDigest:          id.WholeRepoDigest,
	})
	return sha256Hex(b)
}

func execContextDigest(storeID string, id ExecIdentity) string {
	b, _ := json.Marshal(struct {
		StoreID           string   `json:"store_id,omitempty"`
		ExecCWD           string   `json:"exec_cwd,omitempty"`
		ExecSurface       string   `json:"exec_surface,omitempty"`
		OwnedPaths        []string `json:"owned_paths,omitempty"`
		GitIdentityDigest string   `json:"git_identity_digest,omitempty"`
		ScopedDigest      string   `json:"scoped_digest,omitempty"`
		OutOfScopeDigest  string   `json:"out_of_scope_digest,omitempty"`
		WholeRepoDigest   string   `json:"whole_repo_digest,omitempty"`
		ParallelWorktree  string   `json:"parallel_worktree,omitempty"`
		GitAvailable      bool     `json:"git_available"`
	}{
		StoreID:           storeID,
		ExecCWD:           id.ExecCWD,
		ExecSurface:       id.ExecSurface,
		OwnedPaths:        append([]string(nil), id.OwnedPaths...),
		GitIdentityDigest: id.GitIdentityDigest,
		ScopedDigest:      id.ScopedDigest,
		OutOfScopeDigest:  id.OutOfScopeDigest,
		WholeRepoDigest:   id.WholeRepoDigest,
		ParallelWorktree:  id.ParallelWorktree,
		GitAvailable:      id.GitAvailable,
	})
	return sha256Hex(b)
}

func execContextDigestFromRun(run ScriptRunRecord) string {
	if run.ExecContextDigest != "" {
		return run.ExecContextDigest
	}
	return execContextDigest(run.StoreID, ExecIdentity{
		ExecCWD:           run.ExecCWD,
		GitAvailable:      run.GitAvailable,
		GitIdentityDigest: run.GitIdentityDigest,
		ExecSurface:       run.ExecSurface,
		OwnedPaths:        append([]string(nil), run.OwnedPaths...),
		ScopedDigest:      run.ScopedDigest,
		OutOfScopeDigest:  run.OutOfScopeDigest,
		WholeRepoDigest:   run.WholeRepoDigest,
		ParallelWorktree:  run.ParallelWorktree,
	})
}
