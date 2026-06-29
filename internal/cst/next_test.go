package cst

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNextProjectsInitForEmptyStore(t *testing.T) {
	withTempStore(t)

	view := currentNextView(t)
	if view.Phase != NextPhaseInit || view.Required != "input" {
		t.Fatalf("wrong init projection: %+v", view)
	}
	if view.Repair == nil || view.Repair.Phase != NextPhaseInit || len(view.Repair.Commands) != 1 {
		t.Fatalf("missing init repair: %+v", view.Repair)
	}
	if !strings.Contains(view.Repair.Commands[0], "cst add --intent") {
		t.Fatalf("init repair command is not constructive: %+v", view.Repair.Commands)
	}
}

func TestNextRootOnlyStoreRequiresModeledWork(t *testing.T) {
	withTempStore(t)
	mustDoAdd(t, AddArgs{Intent: "root"})

	view := currentNextView(t)
	if view.Phase == NextPhaseNoOp {
		t.Fatalf("root-only store must not no-op: %+v", view)
	}
	if view.Phase != NextPhaseWork || view.Required != "input" || view.Repair == nil {
		t.Fatalf("root-only store should request first modeled work: %+v", view)
	}
	if !strings.Contains(view.Repair.Commands[0], "cst add --parent 1") {
		t.Fatalf("root-only repair should add work under root: %+v", view.Repair.Commands)
	}
}

func TestNextProjectsReadyTaskTakeActionWithBriefing(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		AcceptanceVerify: "true",
		Boundary:         &NodeBoundary{Owned: []string{"src"}},
	})

	view := currentNextView(t)
	if view.Phase != NextPhaseWork || view.Action == nil || view.Action.Kind != ActionTakeReadyTask {
		t.Fatalf("wrong ready projection: %+v", view)
	}
	if view.Briefing == nil || view.Briefing.Boundary == nil || view.Briefing.Boundary.Owned[0] != "src" {
		t.Fatalf("ready projection missing briefing: %+v", view.Briefing)
	}
}

func TestNextProjectsCompleteBeforeRerunForAcceptedClaim(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		AcceptanceVerify: "true",
		Boundary:         &NodeBoundary{Owned: []string{"src"}},
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
		t.Fatal(err)
	}

	view := currentNextView(t)
	if view.Phase != NextPhaseComplete || view.Action == nil || view.Action.Kind != ActionCompleteFromAcceptance {
		t.Fatalf("next should recommend completion from fresh run-set: %+v", view)
	}
	if view.WorkerStatus == nil || len(view.WorkerStatus.Actions) < 2 {
		t.Fatalf("next should reuse worker-status actions: %+v", view.WorkerStatus)
	}
}

func TestNextAllowsNoDiffReviewTaskWithoutBoundary(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "review", AcceptanceReview: "self"})

	view := currentNextView(t)
	if view.Phase != NextPhaseWork || view.Action == nil || view.Action.Kind != ActionTakeReadyTask {
		t.Fatalf("no-diff review task should project work action without boundary repair: %+v", view)
	}
}

func TestNextReconcileUsesNodeBoundaryNotExecutionScope(t *testing.T) {
	dir := withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, dir)
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		AcceptanceVerify: "true",
		Envelope:         &ExecutionEnvelope{OwnedPaths: []string{"scope"}},
		Boundary:         &NodeBoundary{Owned: []string{"owned"}},
	})
	writeFile(t, dir, "scope/drift.txt", "dirty\n")

	view := currentNextView(t)
	if view.Phase != NextPhaseReconcile {
		t.Fatalf("execution scope must not cover unreconciled diff: %+v", view)
	}
	if len(view.UnreconciledDiffs) != 1 || view.UnreconciledDiffs[0].Path != "scope/drift.txt" {
		t.Fatalf("wrong unreconciled diff projection: %+v", view.UnreconciledDiffs)
	}
}

func TestNextDoesNotReconcileDiffCoveredByActiveNodeBoundary(t *testing.T) {
	dir := withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, dir)
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		AcceptanceVerify: "true",
		Boundary:         &NodeBoundary{Owned: []string{"owned"}},
	})
	writeFile(t, dir, "owned/work.txt", "dirty\n")

	view := currentNextView(t)
	if view.Phase != NextPhaseWork || view.Action == nil || view.Action.Kind != ActionTakeReadyTask {
		t.Fatalf("diff covered by active node boundary should allow work action: %+v", view)
	}
}

func TestNextSelectsDirtyOwnerTaskInsteadOfFirstReadyTask(t *testing.T) {
	dir := withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, dir)
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "first task",
		AcceptanceVerify: "true",
		Boundary:         &NodeBoundary{Owned: []string{"a"}},
	})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "dirty owner",
		AcceptanceVerify: "true",
		Boundary:         &NodeBoundary{Owned: []string{"b"}},
	})
	writeFile(t, dir, "b/work.txt", "dirty\n")

	view := currentNextView(t)
	if view.Phase != NextPhaseWork || view.Action == nil || view.Action.TaskID != 3 {
		t.Fatalf("next should select dirty owner task, got %+v", view)
	}
}

func TestNextSelectsCurrentOwnerWhenFutureOrderedTaskSharesBoundary(t *testing.T) {
	dir := withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, dir)
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "current owner",
		AcceptanceReview: "self",
		Boundary:         &NodeBoundary{Owned: []string{"src"}},
	})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "future owner",
		AcceptanceReview: "self",
		After:            []int64{2},
		Boundary:         &NodeBoundary{Owned: []string{"src/file"}},
	})
	writeFile(t, dir, "src/file/work.txt", "dirty\n")

	view := currentNextView(t)
	if view.Phase != NextPhaseWork || view.Action == nil || view.Action.TaskID != 2 {
		t.Fatalf("next should select current owner while future owner waits, got %+v", view)
	}
}

func TestNextReconcilesDirtyDiffOutsideCurrentClaimBoundary(t *testing.T) {
	dir := withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, dir)
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "claimed task",
		AcceptanceVerify: "true",
		Boundary:         &NodeBoundary{Owned: []string{"a"}},
	})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "other owner",
		AcceptanceVerify: "true",
		Boundary:         &NodeBoundary{Owned: []string{"b"}},
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "b/work.txt", "dirty\n")

	view := currentNextView(t)
	if view.Phase != NextPhaseReconcile {
		t.Fatalf("dirty diff outside current claim should reconcile, got %+v", view)
	}
	if len(view.UnreconciledDiffs) != 1 || view.UnreconciledDiffs[0].Path != "b/work.txt" {
		t.Fatalf("wrong unreconciled diff projection: %+v", view.UnreconciledDiffs)
	}
}

func TestNextReconcilesDiffCoveredOnlyByCompletedNodeBoundary(t *testing.T) {
	dir := withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, dir)
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "completed historical task",
		AcceptanceReview: "self",
		Boundary:         &NodeBoundary{Owned: []string{"src"}},
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoDone(io.Discard, 2, DoneArgs{Note: "reviewed"}, false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "src/new.txt", "dirty\n")

	view := currentNextView(t)
	if view.Phase != NextPhaseReconcile {
		t.Fatalf("completed task boundary must not cover new dirty diff: %+v", view)
	}
	if len(view.UnreconciledDiffs) != 1 || view.UnreconciledDiffs[0].Path != "src/new.txt" {
		t.Fatalf("wrong unreconciled diff projection: %+v", view.UnreconciledDiffs)
	}
}

func TestNextReconcileRepairCanAddTaskAfterCompletedGoalBoundary(t *testing.T) {
	dir := withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, dir)
	mustDoAdd(t, AddArgs{
		Intent:   "root",
		Boundary: &NodeBoundary{Owned: []string{"."}},
	})
	mustDoAdd(t, AddArgs{
		Parent:   1,
		Goal:     true,
		Intent:   "historical workstream",
		Boundary: &NodeBoundary{Owned: []string{"product-loop"}},
	})
	mustDoAdd(t, AddArgs{
		Parent:           2,
		Intent:           "historical task",
		AcceptanceReview: "self",
		Boundary:         &NodeBoundary{Owned: []string{"product-loop"}},
	})
	if err := DoTake(io.Discard, 3, false); err != nil {
		t.Fatal(err)
	}
	if err := DoDone(io.Discard, 3, DoneArgs{Note: "reviewed"}, false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "product-loop/bun.lock", "dirty\n")

	view := currentNextView(t)
	if view.Phase != NextPhaseReconcile || view.Repair == nil {
		t.Fatalf("dirty diff under completed goal should require reconcile: %+v", view)
	}
	if len(view.UnreconciledDiffs) != 1 || view.UnreconciledDiffs[0].Path != "product-loop/bun.lock" {
		t.Fatalf("wrong unreconciled diff projection: %+v", view.UnreconciledDiffs)
	}
	if !strings.Contains(view.Repair.Commands[0], "--parent 1") || !strings.Contains(view.Repair.Commands[0], "--owned product-loop/bun.lock") {
		t.Fatalf("repair command should point at root with dirty path ownership: %+v", view.Repair.Commands)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent:           1,
		Intent:           "reconcile dirty lockfile",
		AcceptanceReview: "self",
		Boundary:         &NodeBoundary{Owned: []string{"product-loop/bun.lock"}},
	}, false); err != nil {
		t.Fatalf("next reconcile repair shape must be legal for reducer, got %v", err)
	}
}

func TestNextReconcileRepairQuotesDirtyPath(t *testing.T) {
	dir := withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, dir)
	mustDoAdd(t, AddArgs{Intent: "root"})
	writeFile(t, dir, "space dir/work.txt", "dirty\n")

	view := currentNextView(t)
	if view.Phase != NextPhaseReconcile || view.Repair == nil || len(view.Repair.Commands) != 1 {
		t.Fatalf("dirty path should produce reconcile repair: %+v", view)
	}
	if !strings.Contains(view.Repair.Commands[0], `--owned "space dir/work.txt"`) {
		t.Fatalf("repair command should quote dirty path: %+v", view.Repair.Commands)
	}
}

func TestNextReconcilesDirtyDiffWhenTaskHasNoBoundary(t *testing.T) {
	dir := withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, dir)
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"})
	writeFile(t, dir, "src/work.txt", "dirty\n")

	view := currentNextView(t)
	if view.Phase != NextPhaseReconcile || view.Repair == nil || view.Repair.Phase != NextPhaseReconcile {
		t.Fatalf("dirty diff without node boundary should require reconcile: %+v", view)
	}
	if len(view.UnreconciledDiffs) != 1 || view.UnreconciledDiffs[0].Path != "src/work.txt" {
		t.Fatalf("wrong dirty diff projection: %+v", view.UnreconciledDiffs)
	}
}

func currentNextView(t *testing.T) NextView {
	t.Helper()
	var view NextView
	if err := WithStore(TxOpts{Mutating: false, RepairLease: false}, func(tx *Tx) error {
		var err error
		view, err = BuildNextView(NextInputFromTx(tx))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return view
}

func writeFile(t *testing.T, dir string, path string, data string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
