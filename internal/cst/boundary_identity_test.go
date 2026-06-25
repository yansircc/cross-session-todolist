package cst

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withExplicitStore(t *testing.T, root string) {
	t.Helper()
	if err := SetStoreRoot(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := SetStoreRoot(""); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStoreExecSeparation(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "printf worker > side-effect.txt"})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoDone(io.Discard, 2, DoneArgs{ExecCWD: worker}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(central, ".cst", "events.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(worker, ".cst", "events.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("worker store should not be written, stat err=%v", err)
	}
	got, err := os.ReadFile(filepath.Join(worker, "side-effect.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "worker" {
		t.Fatalf("side effect happened outside worker cwd: %q", got)
	}

	state := replayState(t)
	task := state.Nodes[2]
	if !task.Completed {
		t.Fatal("task not completed")
	}
	if len(task.Runs) != 1 {
		t.Fatalf("expected one acceptance run, got %+v", task.Runs)
	}
	run := task.Runs[0]
	if run.StoreID != state.StoreID() {
		t.Fatalf("run store_id=%q want %q", run.StoreID, state.StoreID())
	}
	if run.ExecCWD != worker {
		t.Fatalf("run exec_cwd=%q want %q", run.ExecCWD, worker)
	}
	ev := state.EvidenceIDs[task.CompletedEvidence]
	if ev.Kind != EvidenceAcceptanceRunSet {
		t.Fatalf("completion evidence kind=%q", ev.Kind)
	}
}

func TestWorkerStoreBindingUsesGitMetadata(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, worker)

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		Envelope:         &ExecutionEnvelope{ExecCWD: worker, ExecSurface: ExecSurfacePrivate},
		AcceptanceVerify: "true",
	})

	if _, err := os.Stat(filepath.Join(worker, StoreDirName, workerBindingFallbackFile)); !os.IsNotExist(err) {
		t.Fatalf("git worker binding should not live in worktree .cst, stat err=%v", err)
	}
	binding, ok, err := DetectWorkerStoreBinding(worker)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected worker store binding")
	}
	if binding.StoreRoot != central || binding.ExecCWD != worker || binding.ExecSurface != ExecSurfacePrivate {
		t.Fatalf("bad binding: %+v", binding)
	}
	recovery := WorkerRecoveryCommand("take", []string{"2"}, binding)
	if !strings.Contains(recovery, "--store "+central) ||
		!strings.Contains(recovery, "--exec-cwd "+worker) ||
		!strings.Contains(recovery, "--private-exec-cwd") {
		t.Fatalf("recovery command lost binding: %s", recovery)
	}
	cmd := exec.Command("git", "-C", worker, "status", "--short")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("worker binding must not pollute git status:\n%s", out)
	}
}

func TestWorkerCheckoutRejectsImplicitMutatingStore(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, worker)

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		Envelope:         &ExecutionEnvelope{ExecCWD: worker, ExecSurface: ExecSurfacePrivate},
		AcceptanceVerify: "true",
	})

	if err := SetStoreRoot(""); err != nil {
		t.Fatal(err)
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(worker); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	err = DoTake(io.Discard, 2, false)
	if err == nil || !strings.Contains(err.Error(), "mutating commands require explicit --store") {
		t.Fatalf("expected worker store guard, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(worker, StoreDirName, eventsFile)); !os.IsNotExist(err) {
		t.Fatalf("implicit worker mutation wrote local events, stat err=%v", err)
	}

	if err := SetStoreRoot(central); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	task := replayState(t).Nodes[2]
	if task.Claim == nil || task.Claim.Actor != "alice" {
		t.Fatalf("explicit central store did not receive claim: %+v", task.Claim)
	}
	if _, err := os.Stat(filepath.Join(worker, StoreDirName, eventsFile)); !os.IsNotExist(err) {
		t.Fatalf("explicit central mutation wrote worker events, stat err=%v", err)
	}
}

func TestExecCWDInsideStoreRootDoesNotCreateWorkerBinding(t *testing.T) {
	central := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, central)
	subdir := filepath.Join(central, "pkg")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		Envelope:         &ExecutionEnvelope{ExecCWD: subdir, ExecSurface: ExecSurfaceShared},
		AcceptanceVerify: "true",
	})

	if binding, ok, err := DetectWorkerStoreBinding(central); err != nil || ok {
		t.Fatalf("same-checkout exec_cwd must not create worker binding, ok=%v binding=%+v err=%v", ok, binding, err)
	}
}

func TestScriptRunIdentityAndArtifacts(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, worker)
	if err := os.WriteFile(filepath.Join(worker, "tracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worker, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"})
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{
		Override: "printf 'hello stdout'; printf 'hello stderr' >&2",
		ExecCWD:  worker,
	}, false); err != nil {
		t.Fatal(err)
	}

	task := replayState(t).Nodes[2]
	if len(task.Runs) != 1 {
		t.Fatalf("expected one run, got %+v", task.Runs)
	}
	run := task.Runs[0]
	if !run.GitAvailable {
		t.Fatalf("git identity missing: %+v", run)
	}
	if run.GitHead == "" || run.GitBranch == "" || run.UnstagedDiffSHA256 == "" || run.UntrackedManifestSHA256 == "" {
		t.Fatalf("git fields incomplete: %+v", run)
	}
	if run.StdoutArtifact == nil || run.StderrArtifact == nil {
		t.Fatalf("artifacts missing: stdout=%+v stderr=%+v", run.StdoutArtifact, run.StderrArtifact)
	}
	assertArtifactHash(t, central, run.StdoutArtifact)
	assertArtifactHash(t, central, run.StderrArtifact)
}

func TestNonGitExecCWDRecordsGitUnavailable(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"})
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Override: "true", ExecCWD: worker}, false); err != nil {
		t.Fatal(err)
	}
	run := replayState(t).Nodes[2].Runs[0]
	if run.GitAvailable {
		t.Fatalf("non-git worker reported git identity: %+v", run)
	}
	if run.GitHead != "" || run.GitRoot != "" {
		t.Fatalf("non-git worker should not forge git fields: %+v", run)
	}
}

func TestRunAcceptanceThenDoneFromAcceptance(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:   1,
		Intent:   "task",
		Envelope: &ExecutionEnvelope{ExecCWD: worker, ExecSurface: ExecSurfaceShared},
		VerifyChecks: []VerifyCheck{
			{Name: "unit", Cmd: "true"},
			{Name: "lint", Cmd: "true"},
		},
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
		t.Fatal(err)
	}
	state := replayState(t)
	task := state.Nodes[2]
	if task.Completed {
		t.Fatal("run --acceptance must not complete the task")
	}
	var runSet EvidenceRecord
	for _, ev := range task.Evidences {
		if ev.Kind == EvidenceAcceptanceRunSet {
			runSet = ev
		}
	}
	if runSet.EventID == "" {
		t.Fatalf("acceptance_run_set not recorded: %+v", task.Evidences)
	}
	if err := DoDone(io.Discard, 2, DoneArgs{FromAcceptanceID: runSet.EventID}, false); err != nil {
		t.Fatal(err)
	}
	state = replayState(t)
	task = state.Nodes[2]
	if !task.Completed || task.CompletedEvidence != runSet.EventID {
		t.Fatalf("from-acceptance completion mismatch: %+v", task)
	}
}

func TestExplicitActorRenewsExpiredClaimForAcceptance(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	now := time.Now().Add(-time.Hour)
	expired := now.Add(time.Minute)
	events := []*Event{
		{EventID: "root", Timestamp: now, Actor: "alice", Type: EvNodeCreated, NodeID: 1, Kind: KindGoal, Intent: "root"},
		{EventID: "task", Timestamp: now, Actor: "alice", Type: EvNodeCreated, NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: NewVerifyAcceptance("true")},
		{EventID: "claim", Timestamp: now, Actor: "alice", Type: EvClaimTaken, AttemptID: "attempt", NodeID: 2, LeaseID: "lease", LeaseExpiresAt: &expired},
	}
	if err := Append(events...); err != nil {
		t.Fatal(err)
	}
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
		t.Fatal(err)
	}
	renewed := false
	for _, ev := range replayEvents(t) {
		if ev.Type == EvClaimRenewed && ev.Actor == "alice" && ev.LeaseID == "lease" {
			renewed = true
		}
		if ev.Type == EvClaimAbandoned {
			t.Fatalf("same actor acceptance should renew, not abandon: %+v", ev)
		}
	}
	if !renewed {
		t.Fatal("expected claim_renewed before acceptance run")
	}
}

func TestOtherActorExpiredClaimIsNotSilentlyRetaken(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "bob")
	now := time.Now().Add(-time.Hour)
	expired := now.Add(time.Minute)
	events := []*Event{
		{EventID: "root", Timestamp: now, Actor: "alice", Type: EvNodeCreated, NodeID: 1, Kind: KindGoal, Intent: "root"},
		{EventID: "task", Timestamp: now, Actor: "alice", Type: EvNodeCreated, NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: NewVerifyAcceptance("true")},
		{EventID: "claim", Timestamp: now, Actor: "alice", Type: EvClaimTaken, AttemptID: "attempt", NodeID: 2, LeaseID: "lease", LeaseExpiresAt: &expired},
	}
	if err := Append(events...); err != nil {
		t.Fatal(err)
	}
	err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false)
	if err == nil || !strings.Contains(err.Error(), "expired claim by alice") {
		t.Fatalf("expected explicit conflict, got %v", err)
	}
	for _, ev := range replayEvents(t) {
		if ev.Type == EvClaimAbandoned || ev.Type == EvClaimTaken && ev.Actor == "bob" {
			t.Fatalf("other actor must not auto-retake expired claim: %+v", ev)
		}
	}
}

func TestPrivateExecutionContextDriftRejectsFromAcceptance(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, worker)
	writeAndCommit(t, worker, "owned.txt", "clean\n")

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		Envelope:         &ExecutionEnvelope{ExecCWD: worker, ExecSurface: ExecSurfacePrivate, OwnedPaths: []string{"owned.txt"}},
		AcceptanceVerify: "true",
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
		t.Fatal(err)
	}
	runSetID := latestEvidenceID(t, 2, EvidenceAcceptanceRunSet)
	if err := os.WriteFile(filepath.Join(worker, "owned.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := DoDone(io.Discard, 2, DoneArgs{FromAcceptanceID: runSetID}, false)
	if err == nil || !strings.Contains(err.Error(), "private execution context drifted") {
		t.Fatalf("expected private drift rejection, got %v", err)
	}
}

func TestSharedOutOfScopeDriftRecordsEvidenceAndCompletes(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, worker)
	writeAndCommit(t, worker, "owned.txt", "clean\n")
	writeAndCommit(t, worker, "other.txt", "clean\n")

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		Envelope:         &ExecutionEnvelope{ExecCWD: worker, ExecSurface: ExecSurfaceShared, OwnedPaths: []string{"owned.txt"}},
		AcceptanceVerify: "true",
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
		t.Fatal(err)
	}
	runSetID := latestEvidenceID(t, 2, EvidenceAcceptanceRunSet)
	if err := os.WriteFile(filepath.Join(worker, "other.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DoDone(io.Discard, 2, DoneArgs{FromAcceptanceID: runSetID}, false); err != nil {
		t.Fatal(err)
	}
	state := replayState(t)
	task := state.Nodes[2]
	if !task.Completed {
		t.Fatal("task should complete on shared out-of-scope drift")
	}
	if latestEvidenceID(t, 2, EvidenceContextDrift) == "" {
		t.Fatalf("expected context_drift evidence: %+v", task.Evidences)
	}
}

func TestSharedScopedDriftRejectsFromAcceptance(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, worker)
	writeAndCommit(t, worker, "owned.txt", "clean\n")

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		Envelope:         &ExecutionEnvelope{ExecCWD: worker, ExecSurface: ExecSurfaceShared, OwnedPaths: []string{"owned.txt"}},
		AcceptanceVerify: "true",
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
		t.Fatal(err)
	}
	runSetID := latestEvidenceID(t, 2, EvidenceAcceptanceRunSet)
	if err := os.WriteFile(filepath.Join(worker, "owned.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := DoDone(io.Discard, 2, DoneArgs{FromAcceptanceID: runSetID}, false)
	if err == nil || !strings.Contains(err.Error(), "scoped execution context drifted") {
		t.Fatalf("expected scoped drift rejection, got %v", err)
	}
}

func TestReviewChecklistEvidenceShape(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "review", AcceptanceReview: "self"})
	valid := `{"items":[{"id":"api","criterion":"review API boundary","status":"pass","evidence":"checked handlers"}],"blind_spots":[]}`
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceReviewChecklist, Summary: "review checklist", Data: valid}, false); err != nil {
		t.Fatal(err)
	}
	invalid := `{"items":[{"id":"api","criterion":"review API boundary","status":"maybe","evidence":"checked"}]}`
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceReviewChecklist, Summary: "bad checklist", Data: invalid}, false); err == nil {
		t.Fatal("expected invalid review checklist status to fail")
	}
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceReviewChecklist, Summary: "empty checklist", Data: `{}`}, false); err == nil {
		t.Fatal("expected empty review checklist to fail")
	}
}

func TestClaimsShowPathOverlaps(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "a", AcceptanceReview: "self", Envelope: &ExecutionEnvelope{ExecCWD: "/tmp/work", ExecSurface: ExecSurfaceShared, OwnedPaths: []string{"src"}}})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "b", AcceptanceReview: "self", Envelope: &ExecutionEnvelope{ExecCWD: "/tmp/work", ExecSurface: ExecSurfaceShared, OwnedPaths: []string{"src/lib"}}})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CST_ACTOR", "bob")
	if err := DoTake(io.Discard, 3, false); err != nil {
		t.Fatal(err)
	}
	view, err := BuildClaimsView(replayState(t), 0, time.Now(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Overlaps) != 1 {
		t.Fatalf("expected one path overlap, got %+v", view.Overlaps)
	}
}

func TestDoneRejectsStaleAcceptanceRunSet(t *testing.T) {
	now := time.Now()
	ttl := time.Hour
	exp := now.Add(ttl)
	events := []*Event{
		{EventID: "root", Timestamp: now, Actor: "alice", Type: EvNodeCreated, NodeID: 1, Kind: KindGoal, Intent: "root"},
		{EventID: "task", Timestamp: now, Actor: "alice", Type: EvNodeCreated, NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: NewVerifyAcceptance("true")},
		{EventID: "claim", Timestamp: now, Actor: "alice", Type: EvClaimTaken, AttemptID: "attempt", NodeID: 2, LeaseID: "lease", LeaseExpiresAt: &exp},
		{EventID: "run", Timestamp: now, Actor: "alice", Type: EvScriptRun, AttemptID: "attempt", NodeID: 2, Trigger: TriggerAcceptance, CheckName: DefaultVerifyCheckName, Cmd: "true", ExitCode: 0, StoreID: "root", ExecCWD: "/tmp/worker", ExecContextDigest: "ctx"},
	}
	data := marshalAcceptanceRunSetData(AcceptanceRunSetData{
		AcceptanceDigest:  acceptanceDigest(NewVerifyAcceptance("true").VerifyChecks()),
		ExecContextDigest: "ctx",
		Checks:            []AcceptanceRunSetCheck{{Name: DefaultVerifyCheckName, Cmd: "true", ScriptRunEventID: "run"}},
	})
	events = append(events,
		&Event{EventID: "runset", Timestamp: now, Actor: "alice", Type: EvEvidence, AttemptID: "attempt", NodeID: 2, EvidenceKind: EvidenceAcceptanceRunSet, EvidenceSummary: "acceptance run set", EvidenceData: data},
		&Event{EventID: "abandon", Timestamp: now.Add(ttl), Actor: "system", Type: EvClaimAbandoned, AttemptID: "attempt", NodeID: 2, LeaseID: "lease", LeaseExpiresAt: &exp, Reason: "test"},
		&Event{EventID: "revise", Timestamp: now.Add(ttl * 2), Actor: "alice", Type: EvNodeRevised, NodeID: 2, Acceptance: NewVerifyAcceptance("true")},
	)
	exp2 := now.Add(ttl * 3)
	events = append(events,
		&Event{EventID: "claim2", Timestamp: now.Add(ttl * 3), Actor: "alice", Type: EvClaimTaken, AttemptID: "attempt2", NodeID: 2, LeaseID: "lease2", LeaseExpiresAt: &exp2},
		&Event{EventID: "done", Timestamp: now.Add(ttl * 3), Actor: "alice", Type: EvTaskCompleted, AttemptID: "attempt2", NodeID: 2, EvidenceID: "runset"},
	)
	if _, err := Apply(events); err == nil || !strings.Contains(err.Error(), "attempt") {
		t.Fatalf("expected stale run-set rejection, got %v", err)
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "tracked.txt")
	runGit(t, dir, "commit", "-m", "initial")
}

func writeAndCommit(t *testing.T, dir string, path string, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", path)
	runGit(t, dir, "commit", "-m", "add "+path)
}

func latestEvidenceID(t *testing.T, nodeID int64, kind string) string {
	t.Helper()
	task := replayState(t).Nodes[nodeID]
	for i := len(task.Evidences) - 1; i >= 0; i-- {
		if task.Evidences[i].Kind == kind {
			return task.Evidences[i].EventID
		}
	}
	return ""
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func assertArtifactHash(t *testing.T, storeRoot string, ref *ArtifactRef) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(storeRoot, ".cst", filepath.FromSlash(ref.Path)))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != ref.SHA256 {
		t.Fatalf("artifact sha mismatch for %s", ref.Path)
	}
	if int64(len(data)) != ref.ByteSize {
		t.Fatalf("artifact size mismatch for %s", ref.Path)
	}
}

func TestAcceptanceRunSetJSONShape(t *testing.T) {
	data := AcceptanceRunSetData{
		AcceptanceDigest:  "digest",
		ExecContextDigest: "ctx",
		Checks: []AcceptanceRunSetCheck{
			{Name: "unit", Cmd: "true", ScriptRunEventID: "run"},
		},
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseAcceptanceRunSetData(raw); err != nil {
		t.Fatal(err)
	}
}
