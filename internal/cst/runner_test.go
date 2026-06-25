package cst

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunnerCapturesExitCode(t *testing.T) {
	res := Run(RunOpts{Cmd: "true", Trigger: TriggerProbe, Timeout: 5 * time.Second})
	if res.ExitCode != 0 {
		t.Fatalf("true should exit 0, got %d", res.ExitCode)
	}
	res = Run(RunOpts{Cmd: "exit 7", Trigger: TriggerProbe, Timeout: 5 * time.Second})
	if res.ExitCode != 7 {
		t.Fatalf("exit 7 expected, got %d", res.ExitCode)
	}
}

func TestRunnerStdoutTruncates(t *testing.T) {
	res := Run(RunOpts{
		Cmd:            `printf '%0.sX' {1..2000}`,
		Trigger:        TriggerProbe,
		Timeout:        5 * time.Second,
		StdoutMaxBytes: 64,
	})
	if !res.Truncated {
		t.Fatal("expected Truncated=true with small stdout cap")
	}
	if len(res.StdoutHead) > 64 {
		t.Fatalf("stdout head exceeded cap: %d", len(res.StdoutHead))
	}
}

func TestRunnerRejectsUnsafeArtifactEventID(t *testing.T) {
	dir := t.TempDir()
	res := Run(RunOpts{
		EventID:     "../escape",
		Cmd:         "printf artifact",
		Trigger:     TriggerProbe,
		Timeout:     5 * time.Second,
		ArtifactDir: dir,
	})
	if res.ArtifactError == nil {
		t.Fatal("expected unsafe artifact event id to fail")
	}
	if res.StdoutArtifact != nil || res.StderrArtifact != nil {
		t.Fatalf("unsafe artifact write must not publish refs: stdout=%+v stderr=%+v", res.StdoutArtifact, res.StderrArtifact)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "escape.stdout")); !os.IsNotExist(err) {
		t.Fatalf("unsafe artifact escaped dir, stat err=%v", err)
	}
}

func TestWriteRunArtifactPublishesVerifiedRef(t *testing.T) {
	dir := t.TempDir()
	ref, err := writeRunArtifact(dir, "event", "stdout", []byte("artifact"))
	if err != nil {
		t.Fatal(err)
	}
	if ref.Path != "artifacts/runs/event.stdout" {
		t.Fatalf("artifact path mismatch: %+v", ref)
	}
	data, err := os.ReadFile(filepath.Join(dir, "event.stdout"))
	if err != nil {
		t.Fatal(err)
	}
	if ref.SHA256 != sha256Hex(data) || ref.ByteSize != int64(len(data)) {
		t.Fatalf("artifact ref does not verify against file: ref=%+v data=%q", ref, data)
	}
}
