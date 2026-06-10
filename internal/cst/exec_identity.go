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
	ExecCWD                 string `json:"exec_cwd,omitempty"`
	GitAvailable            bool   `json:"git_available"`
	GitRoot                 string `json:"git_root,omitempty"`
	GitHead                 string `json:"git_head,omitempty"`
	GitBranch               string `json:"git_branch,omitempty"`
	GitStatusShort          string `json:"git_status_short,omitempty"`
	StagedDiffSHA256        string `json:"staged_diff_sha256,omitempty"`
	UnstagedDiffSHA256      string `json:"unstaged_diff_sha256,omitempty"`
	UntrackedManifestSHA256 string `json:"untracked_manifest_sha256,omitempty"`
	GitIdentityDigest       string `json:"git_identity_digest,omitempty"`
	ParallelWorktree        string `json:"parallel_worktree,omitempty"`
	ExecContextDigest       string `json:"exec_context_digest,omitempty"`
}

func CaptureExecIdentity(execCWD string) ExecIdentity {
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
		ParallelWorktree: os.Getenv("PARALLEL_WORKTREE"),
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
	id.ExecContextDigest = execContextDigest("", id)
	return id
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

func execContextDigest(storeID string, id ExecIdentity) string {
	b, _ := json.Marshal(struct {
		StoreID           string `json:"store_id,omitempty"`
		ExecCWD           string `json:"exec_cwd,omitempty"`
		GitIdentityDigest string `json:"git_identity_digest,omitempty"`
		ParallelWorktree  string `json:"parallel_worktree,omitempty"`
		GitAvailable      bool   `json:"git_available"`
	}{
		StoreID:           storeID,
		ExecCWD:           id.ExecCWD,
		GitIdentityDigest: id.GitIdentityDigest,
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
		ParallelWorktree:  run.ParallelWorktree,
	})
}
