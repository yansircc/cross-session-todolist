package cst

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigStrict — unknown keys, bad values, and inverted lease/renew
// timing all fail loudly.
func TestConfigStrict(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"unknown key", "[brief]\nmax_taks = 5\n"},
		{"unknown section", "[unknown]\nx = 1\n"},
		{"bad int", "[brief]\nmax_tasks = abc\n"},
		{"renew >= ttl", "[claim]\nlease_ttl_seconds = 60\nrenew_every_seconds = 60\n"},
		{"orphan kv", "max_tasks = 5\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := withTempStore(t)
			if err := os.MkdirAll(filepath.Join(dir, ".cst"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".cst", "config.toml"), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(filepath.Join(dir, ".cst")); err == nil {
				t.Fatal("expected strict config error")
			}
		})
	}
}

func TestDefaultStoreRootUsesExistingAncestorStore(t *testing.T) {
	if err := SetStoreRoot(""); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, StoreDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	paths, err := CurrentStorePaths()
	if err != nil {
		t.Fatal(err)
	}
	if !sameExistingPath(t, paths.Root, root) {
		t.Fatalf("default store root=%q want %q", paths.Root, root)
	}
	if StoreRootExplicit() {
		t.Fatal("ancestor store discovery must not mark store root explicit")
	}
}

func TestDefaultStoreRootUsesGitRootBeforeCWD(t *testing.T) {
	if err := SetStoreRoot(""); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	initGitRepo(t, root)
	nested := filepath.Join(root, "pkg", "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	paths, err := CurrentStorePaths()
	if err != nil {
		t.Fatal(err)
	}
	if !sameExistingPath(t, paths.Root, root) {
		t.Fatalf("default store root=%q want git root %q", paths.Root, root)
	}
}

func TestExecutionScopePathsAreExecRelative(t *testing.T) {
	if _, err := normalizeExecutionEnvelope(&ExecutionEnvelope{OwnedPaths: []string{"/tmp/cst"}}); err == nil || !strings.Contains(err.Error(), "relative to exec checkout") {
		t.Fatalf("expected absolute scope rejection, got %v", err)
	}
	if _, err := normalizeExecutionEnvelope(&ExecutionEnvelope{OwnedPaths: []string{"../outside"}}); err == nil || !strings.Contains(err.Error(), "escapes the exec checkout") {
		t.Fatalf("expected escaping scope rejection, got %v", err)
	}
}

func sameExistingPath(t *testing.T, a string, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	bi, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(ai, bi)
}
