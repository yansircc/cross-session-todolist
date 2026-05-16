package cst

import (
	"io"
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
