package cst

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestBriefWithinIsBounded(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "stream a"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "stream b"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Intent: "a1", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Intent: "a2", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 3, Intent: "b1", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.BriefMaxTasks = 1
	bv, err := BuildBrief(replayState(t), cfg, "agent", 2)
	if err != nil {
		t.Fatal(err)
	}
	if bv.Root.ID != 1 || bv.Scope.ID != 2 {
		t.Fatalf("wrong root/scope: root=%+v scope=%+v", bv.Root, bv.Scope)
	}
	if bv.Summary.TotalTasks != 2 || bv.Summary.ReadyTasks != 2 {
		t.Fatalf("scoped summary wrong: %+v", bv.Summary)
	}
	if bv.ReadyMeta.Total != 2 || bv.ReadyMeta.Shown != 1 || !bv.ReadyMeta.Truncated {
		t.Fatalf("ready meta wrong: %+v", bv.ReadyMeta)
	}
	if len(bv.Ready) != 1 || bv.Ready[0].ID != 4 {
		t.Fatalf("ready list wrong: %+v", bv.Ready)
	}
	if bv.SubtreesMeta.Total != 2 || bv.SubtreesMeta.Shown != 1 || !bv.SubtreesMeta.Truncated {
		t.Fatalf("subtree meta wrong: %+v", bv.SubtreesMeta)
	}
}

func TestBriefDefaultsToActiveFrontier(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "completed stream"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Intent: "old task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 3, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRun(io.Discard, 3, "false", "", false); err == nil {
		t.Fatal("expected failed historical probe")
	}
	if err := DoDone(io.Discard, 3, DoneArgs{Note: "reviewed"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "active stream"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 4, Intent: "current task", AcceptanceVerify: "true"}, false); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	bv, err := BuildBrief(replayState(t), cfg, "agent", 0)
	if err != nil {
		t.Fatal(err)
	}
	if bv.Mode != "frontier" {
		t.Fatalf("expected frontier mode, got %q", bv.Mode)
	}
	if len(bv.Subtrees) != 1 || bv.Subtrees[0].ID != 4 {
		t.Fatalf("default brief should show active child only, got %+v", bv.Subtrees)
	}
	if bv.CompletedSubtrees == nil || bv.CompletedSubtrees.Total != 1 {
		t.Fatalf("completed subtree summary wrong: %+v", bv.CompletedSubtrees)
	}
	if len(bv.RecentFailures) != 0 {
		t.Fatalf("default brief should hide terminal-task failures, got %+v", bv.RecentFailures)
	}
	if len(bv.RecentDone) != 0 {
		t.Fatalf("default brief should hide recent done while work remains, got %+v", bv.RecentDone)
	}

	history, err := BuildBriefWithOptions(replayState(t), cfg, "agent", BriefOptions{History: true})
	if err != nil {
		t.Fatal(err)
	}
	if history.Mode != "history" {
		t.Fatalf("expected history mode, got %q", history.Mode)
	}
	if len(history.Subtrees) != 2 || history.Subtrees[0].ID != 2 || history.Subtrees[1].ID != 4 {
		t.Fatalf("history brief should preserve child order, got %+v", history.Subtrees)
	}
	if len(history.RecentFailures) != 1 || history.RecentFailures[0].NodeID != 3 {
		t.Fatalf("history brief should include terminal-task failure, got %+v", history.RecentFailures)
	}
	if len(history.RecentDone) == 0 || history.RecentDone[0] != 3 {
		t.Fatalf("history brief should include recent done, got %+v", history.RecentDone)
	}
}

func TestShowIsSingleNodeBounded(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "stream a"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "stream b"}, false); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.BriefMaxTasks = 1
	v, err := BuildShow(replayState(t), 1, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if v.Node.ID != 1 || v.Node.Kind != KindGoal {
		t.Fatalf("wrong node detail: %+v", v.Node)
	}
	if v.Progress == nil || v.Progress.TotalTasks != 0 {
		t.Fatalf("wrong progress: %+v", v.Progress)
	}
	if v.ChildrenMeta.Total != 2 || v.ChildrenMeta.Shown != 1 || !v.ChildrenMeta.Truncated {
		t.Fatalf("children meta wrong: %+v", v.ChildrenMeta)
	}
	if len(v.Children) != 1 || v.Children[0].ID != 2 {
		t.Fatalf("children preview wrong: %+v", v.Children)
	}
}

func TestNodeDeclarationsProjectFromAddAndRevise(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{
		Intent: "root",
		Context: &NodeContext{
			Invariant:          "global invariant",
			SuccessObligations: []string{"coverage"},
		},
		Boundary: &NodeBoundary{Owned: []string{"."}},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent:           1,
		Intent:           "leaf",
		AcceptanceReview: "self",
		Context:          &NodeContext{NonGoals: []string{"do not redesign runtime"}},
		Boundary:         &NodeBoundary{Owned: []string{"internal/cst"}, Excluded: []string{"internal/cst/runtime"}},
		ObligationClaims: []string{"coverage"},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRevise(io.Discard, 2, ReviseArgs{
		ContextSet: true,
		ContextPatch: NodeContextPatch{
			InvariantSet: true,
			Invariant:    "leaf invariant",
		},
		BoundarySet: true,
		BoundaryPatch: NodeBoundaryPatch{
			ExcludedSet: true,
			Excluded:    []string{"internal/cst/generated"},
		},
	}, false); err != nil {
		t.Fatal(err)
	}

	state := replayState(t)
	task := state.Nodes[2]
	if task.Context == nil || task.Context.Invariant != "leaf invariant" || len(task.Context.NonGoals) != 1 {
		t.Fatalf("context revise should merge with existing context: %+v", task.Context)
	}
	if task.Boundary == nil || len(task.Boundary.Owned) != 1 || task.Boundary.Excluded[0] != "internal/cst/generated" {
		t.Fatalf("boundary revise should merge with existing boundary: %+v", task.Boundary)
	}
	if len(task.ObligationClaims) != 1 || task.ObligationClaims[0] != "coverage" {
		t.Fatalf("obligation claims missing: %+v", task.ObligationClaims)
	}

	show, err := BuildShow(state, 2, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if show.Node.Context == nil || show.Node.Boundary == nil || len(show.Node.ObligationClaims) != 1 {
		t.Fatalf("show projection missing declarations: %+v", show.Node)
	}
	brief, err := BuildBrief(state, DefaultConfig(), "agent", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(brief.Ready) != 1 || brief.Ready[0].Boundary == nil || len(brief.Ready[0].ObligationClaims) != 1 {
		t.Fatalf("brief projection missing declarations: %+v", brief.Ready)
	}
}

func TestNodeDeclarationClear(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent:           1,
		Intent:           "leaf",
		AcceptanceReview: "self",
		Context:          &NodeContext{Invariant: "x"},
		Boundary:         &NodeBoundary{Owned: []string{"internal/cst"}},
		ObligationClaims: []string{"coverage"},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRevise(io.Discard, 2, ReviseArgs{
		ContextSet:          true,
		ContextPatch:        NodeContextPatch{Clear: true},
		BoundarySet:         true,
		BoundaryPatch:       NodeBoundaryPatch{Clear: true},
		ObligationClaimsSet: true,
	}, false); err != nil {
		t.Fatal(err)
	}
	task := replayState(t).Nodes[2]
	if task.Context != nil || task.Boundary != nil || len(task.ObligationClaims) != 0 {
		t.Fatalf("declarations should be cleared: %+v", task)
	}
}

func TestDeveloperBriefingProjectsThroughShowTakeWorkerStatusAndUI(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{
		Intent:  "root",
		Context: &NodeContext{Invariant: "root invariant", SuccessObligations: []string{"api"}},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent:  1,
		Goal:    true,
		Intent:  "phase",
		Context: &NodeContext{NonGoals: []string{"do not touch runtime"}},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent:           2,
		Intent:           "leaf",
		AcceptanceReview: "self",
		Boundary:         &NodeBoundary{Owned: []string{"internal/cst"}},
		ObligationClaims: []string{"api"},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent:           2,
		Intent:           "downstream",
		AcceptanceReview: "self",
		After:            []int64{3},
	}, false); err != nil {
		t.Fatal(err)
	}

	state := replayState(t)
	show, err := BuildShow(state, 3, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if show.Briefing == nil || show.Briefing.ContextFold == nil {
		t.Fatalf("show briefing missing: %+v", show.Briefing)
	}
	if show.Briefing.ContextFold.Invariant != "root invariant" ||
		len(show.Briefing.ContextFold.NonGoals) != 1 ||
		len(show.Briefing.Downstream) != 1 ||
		show.Briefing.Downstream[0] != 4 ||
		show.Briefing.Boundary == nil ||
		len(show.Briefing.ObligationClaims) != 1 {
		t.Fatalf("show briefing incomplete: %+v", show.Briefing)
	}

	var takeOut bytes.Buffer
	if err := DoTake(&takeOut, 3, true); err != nil {
		t.Fatal(err)
	}
	var takeView TakeView
	if err := json.Unmarshal(takeOut.Bytes(), &takeView); err != nil {
		t.Fatalf("take output is not TakeView JSON: %v\n%s", err, takeOut.String())
	}
	if takeView.Briefing == nil || takeView.Briefing.ContextFold.Invariant != "root invariant" {
		t.Fatalf("take briefing missing: %+v", takeView.Briefing)
	}

	var workerOut bytes.Buffer
	if err := DoWorkerStatus(&workerOut, 3, true); err != nil {
		t.Fatal(err)
	}
	var worker WorkerStatusView
	if err := json.Unmarshal(workerOut.Bytes(), &worker); err != nil {
		t.Fatalf("worker-status output is not WorkerStatusView JSON: %v\n%s", err, workerOut.String())
	}
	if worker.Briefing == nil || len(worker.Briefing.Downstream) != 1 {
		t.Fatalf("worker briefing missing: %+v", worker.Briefing)
	}

	ui := renderHTML(uiViewFrom(replayState(t), 0, EventsPath(), "sample", 0, time.Now()))
	for _, want := range []string{"Developer Briefing", "Success obligations: api", "boundary: owned=internal/cst"} {
		if !strings.Contains(ui, want) {
			t.Fatalf("ui missing %q\n%s", want, head(ui, 4000))
		}
	}
}

func TestEventsRequireScopeAndDoNotRepairLeases(t *testing.T) {
	withTempStore(t)
	now := time.Now().Add(-2 * time.Hour)
	expired := time.Now().Add(-time.Hour)
	lock, err := AcquireLock()
	if err != nil {
		t.Fatal(err)
	}
	err = Append(
		&Event{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 1, Kind: KindGoal, Intent: "root"},
		&Event{EventID: "task", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}},
		&Event{EventID: "claim", Timestamp: now, Actor: "a", Type: EvClaimTaken,
			NodeID: 2, LeaseID: "old", LeaseExpiresAt: &expired},
	)
	if releaseErr := lock.Release(); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if err != nil {
		t.Fatal(err)
	}

	if err := DoEvents(io.Discard, EventsArgs{}, false); err == nil {
		t.Fatal("events without explicit scope should fail")
	}
	if err := DoEvents(io.Discard, EventsArgs{All: true}, false); err == nil {
		t.Fatal("events --all without --raw should fail")
	}
	if err := DoEvents(io.Discard, EventsArgs{All: true, Raw: true}, false); err != nil {
		t.Fatal(err)
	}
	if got := len(replayEvents(t)); got != 3 {
		t.Fatalf("events --all --raw should not repair leases; got %d events", got)
	}

	if err := DoEvents(io.Discard, EventsArgs{NodeID: 2}, false); err != nil {
		t.Fatal(err)
	}
	if got := len(replayEvents(t)); got != 3 {
		t.Fatalf("scoped events should not repair leases; got %d events", got)
	}
	filtered, err := filterEvents(replayEvents(t), EventsArgs{NodeID: 2, SinceEventID: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].EventID != "claim" {
		t.Fatalf("filtered events wrong: %+v", filtered)
	}
}

func TestReadProjectionsDoNotRepairExpiredClaims(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "reader")
	seedExpiredClaim(t)

	projections := []struct {
		name string
		run  func() error
	}{
		{name: "next", run: func() error { return DoNext(io.Discard, true) }},
		{name: "brief", run: func() error { return DoBrief(io.Discard, 0, true) }},
		{name: "show", run: func() error { return DoShow(io.Discard, 2, true) }},
		{name: "worker-status", run: func() error { return DoWorkerStatus(io.Discard, 2, true) }},
		{name: "claims", run: func() error { return DoClaims(io.Discard, ClaimsArgs{}, true) }},
		{name: "ui", run: func() error { return DoUI(io.Discard, UIArgs{Stdout: true}, true) }},
	}
	for _, projection := range projections {
		t.Run(projection.name, func(t *testing.T) {
			before := len(replayEvents(t))
			if err := projection.run(); err != nil {
				t.Fatal(err)
			}
			if after := len(replayEvents(t)); after != before {
				t.Fatalf("%s appended events: before=%d after=%d", projection.name, before, after)
			}
		})
	}
	state := replayState(t)
	if state.Nodes[2].Claim == nil {
		t.Fatal("read projections should leave stale claim in ledger state")
	}
}

func seedExpiredClaim(t *testing.T) {
	t.Helper()
	now := time.Now().Add(-2 * time.Hour)
	expired := time.Now().Add(-time.Hour)
	lock, err := AcquireLock()
	if err != nil {
		t.Fatal(err)
	}
	err = Append(
		&Event{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 1, Kind: KindGoal, Intent: "root"},
		&Event{EventID: "task", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}},
		&Event{EventID: "claim", Timestamp: now, Actor: "a", Type: EvClaimTaken,
			NodeID: 2, LeaseID: "old", LeaseExpiresAt: &expired},
	)
	if releaseErr := lock.Release(); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if err != nil {
		t.Fatal(err)
	}
}
