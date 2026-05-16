package cst

import (
	"os"
	"path/filepath"
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
