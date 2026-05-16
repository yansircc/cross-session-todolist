package cst

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// TestAddRejectsTaskWithoutAcceptance enforces the explicit-kind invariant: a task
// must declare acceptance. Anything weaker pushes the case-analysis back into
// done() and breaks the kind algebra.
func TestAddRejectsTaskWithoutAcceptance(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root goal"}, false); err != nil {
		t.Fatal(err)
	}
	err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "no acceptance"}, false)
	if err == nil {
		t.Fatal("expected error for nil-acceptance task")
	}
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitUsage {
		t.Fatalf("expected ExitUsage, got %T %v", err, err)
	}
}

func TestAddCheckWithoutParentIsNotRootShorthand(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root goal"}, false); err != nil {
		t.Fatal(err)
	}
	err := DoAdd(io.Discard, AddArgs{
		Intent:       "missing parent",
		VerifyChecks: []VerifyCheck{{Name: "unit", Cmd: "true"}},
	}, false)
	if err == nil {
		t.Fatal("expected --check task without parent to be rejected")
	}
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitUsage {
		t.Fatalf("expected ExitUsage, got %T %v", err, err)
	}
}

// TestDoneRequiresClaim — claim must be ownership, not a UI marker. Doing a
// task without taking it first fails with InvariantBroken.
func TestDoneRequiresClaim(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	err := DoDone(io.Discard, 2, DoneArgs{Note: "ev"}, false)
	if err == nil {
		t.Fatal("expected error: done before take")
	}
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitInvariantBroken {
		t.Fatalf("expected ExitInvariantBroken, got %T %v", err, err)
	}
}

// TestDoneRejectsForeignClaim — caller B cannot complete A's claimed task.
func TestDoneRejectsForeignClaim(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CST_ACTOR", "bob")
	err := DoDone(io.Discard, 2, DoneArgs{Note: "ev"}, false)
	if err == nil {
		t.Fatal("expected claim conflict")
	}
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitClaimConflict {
		t.Fatalf("expected ExitClaimConflict, got %T %v", err, err)
	}
}

func TestConcurrentTakeSingleWinner(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}

	const contenders = 8
	var wg sync.WaitGroup
	errs := make(chan error, contenders)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- DoTake(io.Discard, 2, false)
		}()
	}
	wg.Wait()
	close(errs)

	wins := 0
	conflicts := 0
	for err := range errs {
		if err == nil {
			wins++
			continue
		}
		var hErr *HandlerError
		if !errors.As(err, &hErr) || hErr.Code != ExitClaimConflict {
			t.Fatalf("expected claim conflict, got %T %v", err, err)
		}
		conflicts++
	}
	if wins != 1 || conflicts != contenders-1 {
		t.Fatalf("take race mismatch: wins=%d conflicts=%d", wins, conflicts)
	}
}

// TestCompleteTaskRejectsCanceledRace closes the original TOCTOU window.
// Phase 1 prepares a guard; before phase 3 commits the completion, a cancel
// is committed. The CompleteTask call must reject the stale guard rather
// than producing a (completed, canceled) double terminal.
func TestCompleteTaskRejectsCanceledRace(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}

	// Phase 1: capture guard under read tx.
	var guard CompletionGuard
	if err := WithStore(TxOpts{Mutating: false, RepairLease: false}, func(tx *Tx) error {
		g, err := tx.PrepareCompletionGuard(2)
		guard = g
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Race window: cancel the task.
	if err := DoCancel(io.Discard, 2, "raced", false); err != nil {
		t.Fatal(err)
	}

	// Phase 3: stale completion must be rejected.
	err := WithStore(TxOpts{Mutating: true, RepairLease: false}, func(tx *Tx) error {
		ev, e := tx.RecordEvidence(2, EvidenceNote, "ev", nil)
		if e != nil {
			return e
		}
		_, err := tx.CompleteTask(guard, ev.EventID)
		return err
	})
	if err == nil {
		t.Fatal("CompleteTask accepted a stale guard against a canceled task")
	}

	// Final state must be canceled, not completed.
	events, err := Replay()
	if err != nil {
		t.Fatal(err)
	}
	state, err := Apply(events)
	if err != nil {
		t.Fatal(err)
	}
	n := state.Nodes[2]
	if n.Completed {
		t.Fatal("task incorrectly marked completed despite cancel race")
	}
	if !n.Canceled {
		t.Fatalf("expected canceled state, got %+v", n)
	}
}

func TestSecondRootRejectedEvenAfterTerminal(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	err := DoAdd(io.Discard, AddArgs{Intent: "root2"}, false)
	if err == nil {
		t.Fatal("expected second root to be rejected")
	}
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitInvariantBroken {
		t.Fatalf("expected ExitInvariantBroken, got %T %v", err, err)
	}
}

func TestRuleUnderRuleRejected(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Rule: "no fallback"}, false); err != nil {
		t.Fatal(err)
	}
	err := DoAdd(io.Discard, AddArgs{Parent: 2, Rule: "dead nested rule"}, false)
	if err == nil {
		t.Fatal("expected rule under rule to be rejected")
	}
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitInvariantBroken {
		t.Fatalf("expected ExitInvariantBroken, got %T %v", err, err)
	}
}

func TestRevisePreservesIDWhileChangingProjection(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "phase a"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "phase b"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Intent: "old intent", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Rule: "old rule"}, false); err != nil {
		t.Fatal(err)
	}

	if err := DoRevise(io.Discard, 4, ReviseArgs{
		ParentSet:        true,
		Parent:           3,
		Intent:           "new intent",
		AcceptanceVerify: "true",
		Reason:           "tree correction",
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRevise(io.Discard, 5, ReviseArgs{Rule: "new rule"}, false); err != nil {
		t.Fatal(err)
	}

	state := replayState(t)
	task := state.Nodes[4]
	checks := task.Acceptance.VerifyChecks()
	if task.ParentID != 3 || task.Intent != "new intent" || task.Acceptance.Kind != AcceptanceVerify || len(checks) != 1 || checks[0].Cmd != "true" {
		t.Fatalf("task revision not projected: %+v", task)
	}
	if containsID(state.Nodes[2].Children, 4) || !containsID(state.Nodes[3].Children, 4) {
		t.Fatalf("parent child indexes not moved: p2=%v p3=%v", state.Nodes[2].Children, state.Nodes[3].Children)
	}
	if state.Nodes[5].RuleText != "new rule" {
		t.Fatalf("rule revision not projected: %+v", state.Nodes[5])
	}
}

func TestReviseRejectsClaimedNoopAndCycles(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "phase"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Goal: true, Intent: "child phase"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Intent: "task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}

	err := DoRevise(io.Discard, 2, ReviseArgs{ParentSet: true, Parent: 3}, false)
	if err == nil {
		t.Fatal("expected cycle move rejection")
	}
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitInvariantBroken {
		t.Fatalf("expected ExitInvariantBroken, got %T %v", err, err)
	}

	err = DoRevise(io.Discard, 4, ReviseArgs{ParentSet: true, Parent: 2}, false)
	if err == nil {
		t.Fatal("expected no-op revise rejection")
	}
	if !errors.As(err, &hErr) || hErr.Code != ExitUsage {
		t.Fatalf("expected ExitUsage for no-op, got %T %v", err, err)
	}

	if err := DoTake(io.Discard, 4, false); err != nil {
		t.Fatal(err)
	}
	err = DoRevise(io.Discard, 4, ReviseArgs{Intent: "new"}, false)
	if err == nil {
		t.Fatal("expected claimed revise rejection")
	}
	if !errors.As(err, &hErr) || hErr.Code != ExitClaimConflict {
		t.Fatalf("expected ExitClaimConflict, got %T %v", err, err)
	}
}

func TestCancelRejectsGoalAndOpenChildTask(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "parent", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Intent: "child", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}

	err := DoCancel(io.Discard, 1, "stop", false)
	if err == nil {
		t.Fatal("expected goal cancel rejection")
	}
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitInvariantBroken {
		t.Fatalf("expected ExitInvariantBroken, got %T %v", err, err)
	}

	err = DoCancel(io.Discard, 2, "stop", false)
	if err == nil {
		t.Fatal("expected open-child cancel rejection")
	}
	if !errors.As(err, &hErr) || hErr.Code != ExitInvariantBroken {
		t.Fatalf("expected ExitInvariantBroken, got %T %v", err, err)
	}

	if err := DoCancel(io.Discard, 3, "not needed", false); err != nil {
		t.Fatal(err)
	}
	if err := DoCancel(io.Discard, 2, "not needed", false); err != nil {
		t.Fatal(err)
	}
}

func TestAfterTaskIsNotReadyUntilPrerequisiteCompletes(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "approval", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "dependent", AcceptanceReview: "self", After: []int64{2}}, false); err != nil {
		t.Fatal(err)
	}

	state := replayState(t)
	if state.IsReadyTask(3) {
		t.Fatal("after task should not be ready before prerequisite completes")
	}
	if head := state.HeadOpenTasks(10); len(head) != 1 || head[0].ID != 2 {
		t.Fatalf("expected only approval ready, got %+v", head)
	}
	if waiting, _ := state.WaitingTasksWithin(1, 10); len(waiting) != 1 || waiting[0].ID != 3 {
		t.Fatalf("expected dependent waiting on prerequisite, got %+v", waiting)
	}

	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoDone(io.Discard, 2, DoneArgs{Note: "approved"}, false); err != nil {
		t.Fatal(err)
	}
	state = replayState(t)
	if !state.IsReadyTask(3) {
		t.Fatal("after task should become ready after prerequisite completes")
	}
}

func TestCanceledPrerequisiteDoesNotBecomeReady(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "approval", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "dependent", AcceptanceReview: "self", After: []int64{2}}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoCancel(io.Discard, 2, "not approved", false); err != nil {
		t.Fatal(err)
	}
	state := replayState(t)
	if state.IsReadyTask(3) {
		t.Fatal("dependent should not become ready after prerequisite is canceled")
	}
	if failed, _ := state.DependencyFailedTasksWithin(1, 10); len(failed) != 1 || failed[0].ID != 3 {
		t.Fatalf("expected dependency failure for dependent, got %+v", failed)
	}
}

func TestRenewRequiresOriginalLeaseID(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	oldLease := replayState(t).Nodes[2].Claim.LeaseID
	if err := DoRelease(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := renewClaimUnderLock("alice", 2, oldLease, time.Hour); err == nil {
		t.Fatal("old lease renewed a newer claim")
	}
	for _, ev := range replayEvents(t) {
		if ev.Type == EvClaimRenewed {
			t.Fatalf("unexpected claim renewal from old lease: %+v", ev)
		}
	}
}

func TestVerifyDoneRejectsManualEvidence(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	err := DoDone(io.Discard, 2, DoneArgs{Note: "manual note"}, false)
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitUsage {
		t.Fatalf("expected usage error for verify note, got %#v", err)
	}
	if got := replayState(t).Nodes[2].Status(); got != StatusClaimed {
		t.Fatalf("verify note should not change status, got %s", got)
	}
}

func TestAttemptIDCorrelatesClaimRunEvidenceAndCompletion(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent: 1,
		Intent: "task",
		VerifyChecks: []VerifyCheck{
			{Name: "unit", Cmd: "true"},
			{Name: "lint", Cmd: "true"},
		},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	attemptID := replayState(t).Nodes[2].Claim.AttemptID
	if attemptID == "" {
		t.Fatal("take did not mint attempt_id")
	}
	if err := DoRun(io.Discard, 2, "", "unit", false); err != nil {
		t.Fatal(err)
	}
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceNote, Summary: "attempt note"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoDone(io.Discard, 2, DoneArgs{}, false); err != nil {
		t.Fatal(err)
	}

	state := replayState(t)
	task := state.Nodes[2]
	if task.Claim != nil || !task.Completed {
		t.Fatalf("task should be completed with claim closed: %+v", task)
	}
	attempt := state.Attempts[attemptID]
	if attempt == nil || attempt.NodeID != 2 || attempt.Actor != "alice" || attempt.CloseReason != EvTaskCompleted {
		t.Fatalf("attempt projection wrong: %+v", attempt)
	}
	if len(task.Runs) != 3 {
		t.Fatalf("expected probe plus two acceptance runs, got %+v", task.Runs)
	}
	for _, run := range task.Runs {
		if run.AttemptID != attemptID {
			t.Fatalf("run missing attempt correlation: %+v", run)
		}
		if run.CheckName == "" {
			t.Fatalf("run missing check name: %+v", run)
		}
	}
	for _, ev := range replayEvents(t) {
		switch ev.Type {
		case EvClaimTaken, EvScriptRun, EvEvidence, EvTaskCompleted:
			if ev.NodeID == 2 && ev.AttemptID != attemptID {
				t.Fatalf("%s missing attempt_id: %+v", ev.Type, ev)
			}
		}
	}
}

func TestNamedVerifyCheckFailureRecordsFailedCheck(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent: 1,
		Intent: "task",
		VerifyChecks: []VerifyCheck{
			{Name: "unit", Cmd: "true"},
			{Name: "lint", Cmd: "false"},
		},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	attemptID := replayState(t).Nodes[2].Claim.AttemptID
	err := DoDone(io.Discard, 2, DoneArgs{}, false)
	if err == nil {
		t.Fatal("expected acceptance failure")
	}
	var hErr *HandlerError
	if !errors.As(err, &hErr) || hErr.Code != ExitAcceptanceFail {
		t.Fatalf("expected acceptance failure, got %T %v", err, err)
	}

	task := replayState(t).Nodes[2]
	if task.Completed {
		t.Fatal("failed check should not complete task")
	}
	if len(task.Runs) != 2 {
		t.Fatalf("expected pass check plus failed check, got %+v", task.Runs)
	}
	last := task.Runs[len(task.Runs)-1]
	if last.CheckName != "lint" || last.ExitCode == 0 || last.AttemptID != attemptID {
		t.Fatalf("failed check not recorded structurally: %+v", last)
	}
}
