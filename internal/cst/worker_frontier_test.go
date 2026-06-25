package cst

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerFrontierCanonicalEquality(t *testing.T) {
	t.Run("ready unclaimed task", func(t *testing.T) {
		withTempStore(t)
		t.Setenv("CST_ACTOR", "alice")
		mustDoAdd(t, AddArgs{Intent: "root"})
		mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"})

		input := currentFrontierInput(t, 2)
		assertCanonicalEquality(t, input)
		assertFrontierKinds(t, input, ActionTakeReadyTask)
	})

	t.Run("claimed verify task without run set", func(t *testing.T) {
		withTempStore(t)
		t.Setenv("CST_ACTOR", "alice")
		mustDoAdd(t, AddArgs{Intent: "root"})
		mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"})
		if err := DoTake(io.Discard, 2, false); err != nil {
			t.Fatal(err)
		}

		input := currentFrontierInput(t, 2)
		assertCanonicalEquality(t, input)
		assertFrontierKinds(t, input, ActionRunAcceptance)
		if containsActionKind(LegalFrontier(input), ActionCompleteFromAcceptance) {
			t.Fatal("completion should not be available before acceptance_run_set")
		}
		invalid := BoundAction{
			Kind:      ActionInvalidCompleteVerifyWithNote,
			StoreRoot: input.StoreRoot,
			StoreID:   input.StoreID,
			Revision:  input.Revision,
			Actor:     input.Actor,
			TaskID:    input.TaskID,
		}
		if Admissible(input, invalid).Accept {
			t.Fatal("verify done --note action must be rejected")
		}
	})

	t.Run("fresh acceptance run set", func(t *testing.T) {
		withTempStore(t)
		t.Setenv("CST_ACTOR", "alice")
		mustDoAdd(t, AddArgs{Intent: "root"})
		mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"})
		if err := DoTake(io.Discard, 2, false); err != nil {
			t.Fatal(err)
		}
		if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
			t.Fatal(err)
		}

		input := currentFrontierInput(t, 2)
		assertCanonicalEquality(t, input)
		assertFrontierKinds(t, input, ActionRunAcceptance, ActionCompleteFromAcceptance)
	})

	t.Run("review task with existing evidence", func(t *testing.T) {
		withTempStore(t)
		t.Setenv("CST_ACTOR", "alice")
		mustDoAdd(t, AddArgs{Intent: "root"})
		mustDoAdd(t, AddArgs{Parent: 1, Intent: "review", AcceptanceReview: "self"})
		if err := DoTake(io.Discard, 2, false); err != nil {
			t.Fatal(err)
		}
		if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceNote, Summary: "reviewed"}, false); err != nil {
			t.Fatal(err)
		}

		input := currentFrontierInput(t, 2)
		assertCanonicalEquality(t, input)
		assertFrontierKinds(t, input, ActionCompleteReviewWithEvidence)
	})
}

func TestReviewFrontierRejectsProbeRunEvidence(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "review", AcceptanceReview: "self"})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRun(io.Discard, 2, "true", "", false); err != nil {
		t.Fatal(err)
	}

	input := currentFrontierInput(t, 2)
	if containsActionKind(LegalFrontier(input), ActionCompleteReviewWithEvidence) {
		t.Fatalf("probe script_run must not complete review: %+v", LegalFrontier(input))
	}
	scriptID := latestEvidenceID(t, 2, EvidenceScript)
	action := BoundAction{
		Kind:       ActionCompleteReviewWithEvidence,
		StoreRoot:  input.StoreRoot,
		StoreID:    input.StoreID,
		Revision:   input.Revision,
		Actor:      input.Actor,
		TaskID:     input.TaskID,
		AttemptID:  input.State.Nodes[2].Claim.AttemptID,
		EvidenceID: scriptID,
	}
	if Admissible(input, action).Accept {
		t.Fatal("probe script_run evidence should be inadmissible for review completion")
	}
}

func TestReviewFrontierRejectsPreClaimEvidence(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "review", AcceptanceReview: "self"})
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceNote, Summary: "pre-claim note"}, false); err != nil {
		t.Fatal(err)
	}
	preClaimID := latestEvidenceID(t, 2, EvidenceNote)
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}

	input := currentFrontierInput(t, 2)
	if containsActionKind(LegalFrontier(input), ActionCompleteReviewWithEvidence) {
		t.Fatalf("pre-claim evidence must not produce review completion action: %+v", LegalFrontier(input))
	}
	action := BoundAction{
		Kind:       ActionCompleteReviewWithEvidence,
		StoreRoot:  input.StoreRoot,
		StoreID:    input.StoreID,
		Revision:   input.Revision,
		Actor:      input.Actor,
		TaskID:     input.TaskID,
		AttemptID:  input.State.Nodes[2].Claim.AttemptID,
		EvidenceID: preClaimID,
	}
	if Admissible(input, action).Accept {
		t.Fatal("pre-claim evidence should be inadmissible for current review attempt")
	}
	err := DoDone(io.Discard, 2, DoneArgs{EvidenceID: preClaimID}, false)
	if err == nil || !strings.Contains(err.Error(), "belongs to attempt") {
		t.Fatalf("expected reducer attempt rejection, got %v", err)
	}
}

func TestWorkerFrontierRejectsDriftedRunSet(t *testing.T) {
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
	input := currentFrontierInput(t, 2)
	staleAction := firstActionOfKind(t, input, ActionCompleteFromAcceptance)

	if err := os.WriteFile(filepath.Join(worker, "owned.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input = currentFrontierInput(t, 2)
	assertCanonicalEquality(t, input)
	if containsActionKind(LegalFrontier(input), ActionCompleteFromAcceptance) {
		t.Fatal("drifted acceptance_run_set must not produce completion action")
	}
	if Admissible(input, staleAction).Accept {
		t.Fatal("drifted acceptance_run_set action must be rejected")
	}
}

func TestWorkerRunRejectsStaleActionID(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"})

	action := firstActionOfKind(t, currentFrontierInput(t, 2), ActionTakeReadyTask)
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceNote, Summary: "revision bump"}, false); err != nil {
		t.Fatal(err)
	}
	err := DoWorkerRun(io.Discard, 2, WorkerRunArgs{ActionID: action.ActionID}, false)
	if err == nil || !strings.Contains(err.Error(), "not in the current frontier") {
		t.Fatalf("expected stale action rejection, got %v", err)
	}
}

func TestWorkerRunAcceptanceUsesBoundExecSurface(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		Envelope:         &ExecutionEnvelope{ExecCWD: worker, ExecSurface: ExecSurfaceShared},
		AcceptanceVerify: "printf worker > side-effect.txt",
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	action := firstActionOfKind(t, currentFrontierInput(t, 2), ActionRunAcceptance)
	if err := DoWorkerRun(io.Discard, 2, WorkerRunArgs{ActionID: action.ActionID}, false); err != nil {
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
		t.Fatalf("worker command ran in wrong cwd: %q", got)
	}
	state := replayState(t)
	task := state.Nodes[2]
	if task.Completed {
		t.Fatal("worker-run run_acceptance must not complete the task")
	}
	if latestEvidenceID(t, 2, EvidenceAcceptanceRunSet) == "" {
		t.Fatalf("acceptance_run_set missing: %+v", task.Evidences)
	}
}

func TestWorkerRunCompleteFromAcceptance(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	run := firstActionOfKind(t, currentFrontierInput(t, 2), ActionRunAcceptance)
	if err := DoWorkerRun(io.Discard, 2, WorkerRunArgs{ActionID: run.ActionID}, false); err != nil {
		t.Fatal(err)
	}
	complete := firstActionOfKind(t, currentFrontierInput(t, 2), ActionCompleteFromAcceptance)
	var out bytes.Buffer
	if err := DoWorkerRun(&out, 2, WorkerRunArgs{ActionID: complete.ActionID}, false); err != nil {
		t.Fatal(err)
	}
	task := replayState(t).Nodes[2]
	if !task.Completed || task.CompletedEvidence == "" {
		t.Fatalf("task not completed from acceptance: %+v", task)
	}
}

func currentFrontierInput(t *testing.T, id int64) FrontierInput {
	t.Helper()
	var input FrontierInput
	if err := WithStore(TxOpts{Mutating: false, RepairLease: true}, func(tx *Tx) error {
		input = FrontierInputFromTx(tx, id)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return input
}

func assertCanonicalEquality(t *testing.T, input FrontierInput) {
	t.Helper()
	if !CanonicalActionEqualSets(input) {
		t.Fatalf("frontier/admissible canonical sets differ: frontier=%+v canonical=%+v", LegalFrontier(input), CanonicalActions(input))
	}
}

func assertFrontierKinds(t *testing.T, input FrontierInput, want ...string) {
	t.Helper()
	frontier := LegalFrontier(input)
	if len(frontier) != len(want) {
		t.Fatalf("frontier len=%d want=%d: %+v", len(frontier), len(want), frontier)
	}
	for _, kind := range want {
		if !containsActionKind(frontier, kind) {
			t.Fatalf("frontier missing kind %s: %+v", kind, frontier)
		}
	}
}

func containsActionKind(actions []BoundAction, kind string) bool {
	for _, action := range actions {
		if action.Kind == kind {
			return true
		}
	}
	return false
}

func firstActionOfKind(t *testing.T, input FrontierInput, kind string) BoundAction {
	t.Helper()
	for _, action := range LegalFrontier(input) {
		if action.Kind == kind {
			return action
		}
	}
	t.Fatalf("frontier missing kind %s: %+v", kind, LegalFrontier(input))
	return BoundAction{}
}
