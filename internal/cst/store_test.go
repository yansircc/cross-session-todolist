package cst

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReplayRebuildsState(t *testing.T) {
	dir := withTempStore(t)
	now := time.Now()
	exp := now.Add(time.Hour)
	ev1 := &Event{EventID: "e1", Timestamp: now, Actor: "a", Type: EvNodeCreated,
		NodeID: 1, Kind: KindGoal, Intent: "root"}
	ev2 := &Event{EventID: "e2", Timestamp: now, Actor: "a", Type: EvNodeCreated,
		NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "child", Acceptance: &Acceptance{Kind: AcceptanceVerify, Cmd: "true"}}
	ev3 := &Event{EventID: "e3", Timestamp: now, Actor: "a", Type: EvNodeCreated,
		NodeID: 3, ParentID: 1, Kind: KindRule, RuleText: "no fallback"}
	ev4 := &Event{EventID: "e4", Timestamp: now, Actor: "a", Type: EvClaimTaken,
		NodeID: 2, LeaseID: "lease-x", LeaseExpiresAt: &exp}

	lock, err := AcquireLock()
	if err != nil {
		t.Fatal(err)
	}
	if err := Append(ev1, ev2, ev3, ev4); err != nil {
		t.Fatal(err)
	}
	lock.Release()

	events, err := Replay()
	if err != nil {
		t.Fatal(err)
	}
	state, err := Apply(events)
	if err != nil {
		t.Fatal(err)
	}

	if got := state.Root(); got == nil || got.ID != 1 {
		t.Fatalf("root mismatch: %+v", got)
	}
	if rules := state.InheritedRules(2); len(rules) != 1 || rules[0].ID != 3 {
		t.Fatalf("inherited rules wrong: %+v", rules)
	}
	if got := state.Nodes[2].Claim; got == nil || got.LeaseID != "lease-x" {
		t.Fatalf("claim lost in replay: %+v", got)
	}
	if state.NextID != 4 {
		t.Fatalf("nextID wrong: %d", state.NextID)
	}

	// Sanity: events.jsonl is the only state truth.
	if _, err := os.Stat(filepath.Join(dir, ".cst", "events.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestAppendRejectsWholeBatchBeforeWriting(t *testing.T) {
	withTempStore(t)
	now := time.Now()
	root := &Event{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
		NodeID: 1, Kind: KindGoal, Intent: "root"}
	if err := Append(root); err != nil {
		t.Fatal(err)
	}

	good := &Event{EventID: "good", Timestamp: now, Actor: "a", Type: EvNodeCreated,
		NodeID: 2, ParentID: 1, Kind: KindRule, RuleText: "must not partially commit"}
	bad := &Event{EventID: "bad", Timestamp: now, Actor: "a", Type: EvEvidence,
		NodeID: 1, EvidenceKind: EvidenceNote, EvidenceSummary: "bad", EvidenceData: json.RawMessage(`{`)}
	if err := Append(good, bad); err == nil {
		t.Fatal("expected invalid batch to fail")
	}

	events := replayEvents(t)
	if len(events) != 1 || events[0].EventID != "root" {
		t.Fatalf("append partially committed batch: %+v", events)
	}
}
