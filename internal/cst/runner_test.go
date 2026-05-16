package cst

import (
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
