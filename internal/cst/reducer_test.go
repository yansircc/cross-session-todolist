package cst

import (
	"testing"
	"time"
)

func TestParentCannotCompleteWithOpenChild(t *testing.T) {
	withTempStore(t)
	state := NewState()
	applyEvents(t, state,
		&Event{Type: EvNodeCreated, NodeID: 1, Kind: KindGoal, Intent: "root"},
		&Event{Type: EvNodeCreated, NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "child", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}},
	)
	if state.NodeStatus(state.Root()) != StatusOpen {
		t.Fatal("root goal should stay open while child is open")
	}
	applyEvents(t, state, &Event{Type: EvNodeCanceled, NodeID: 2, Actor: "a", Reason: "not needed"})
	if state.NodeStatus(state.Root()) != StatusCompleted {
		t.Fatal("root goal should derive completed after child terminal")
	}
}

func TestLazyAbandonExpired(t *testing.T) {
	withTempStore(t)
	state := NewState()
	pastExp := time.Now().Add(-time.Hour)
	applyEvents(t, state,
		&Event{Type: EvNodeCreated, NodeID: 1, Kind: KindGoal, Intent: "root"},
		&Event{Type: EvNodeCreated, NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}},
		&Event{Type: EvClaimTaken, NodeID: 2, LeaseID: "old", LeaseExpiresAt: &pastExp, Actor: "a"},
	)
	abandoned := state.LazyAbandonExpired(time.Now())
	if len(abandoned) != 1 {
		t.Fatalf("expected one abandoned event, got %d", len(abandoned))
	}
	if abandoned[0].NodeID != 2 || abandoned[0].LeaseID != "old" {
		t.Fatalf("abandoned mismatch: %+v", abandoned[0])
	}
}

func TestTerminalEventsClearActiveHold(t *testing.T) {
	state := NewState()
	applyEvents(t, state,
		&Event{Type: EvNodeCreated, NodeID: 1, Kind: KindGoal, Intent: "root"},
		&Event{Type: EvNodeCreated, NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}},
		&Event{Type: EvNodeHeld, NodeID: 2, HoldKind: HoldBlocked, Reason: "waiting", Actor: "a"},
		&Event{Type: EvNodeCanceled, NodeID: 2, Reason: "stop", Actor: "a"},
	)
	if state.Nodes[2].Hold != nil || state.Nodes[2].Claim != nil {
		t.Fatalf("terminal task should not project active hold/claim: %+v", state.Nodes[2])
	}
}

func TestLegacyGateEventsHydrateAcceptance(t *testing.T) {
	event, err := UnmarshalEvent([]byte(`{"type":"node_created","node_id":2,"parent_id":1,"kind":"task","intent":"legacy","gate":{"kind":"verify","cmd":"true"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.Acceptance == nil || event.Acceptance.Kind != AcceptanceVerify {
		t.Fatalf("legacy gate did not hydrate acceptance: %+v", event.Acceptance)
	}
	checks := event.Acceptance.VerifyChecks()
	if len(checks) != 1 || checks[0].Name != DefaultVerifyCheckName || checks[0].Cmd != "true" {
		t.Fatalf("legacy gate did not hydrate acceptance: %+v", event.Acceptance)
	}
	if event.LegacyGate != nil {
		t.Fatal("legacy gate should not re-emit as a second fact")
	}

	event, err = UnmarshalEvent([]byte(`{"type":"node_created","node_id":3,"parent_id":1,"kind":"task","intent":"legacy dependent","gate":{"kind":"gate","gate_id":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.Acceptance == nil || event.Acceptance.Kind != AcceptanceReview || event.Acceptance.Who != "self" {
		t.Fatalf("legacy dependency gate did not hydrate review acceptance: %+v", event.Acceptance)
	}
	if len(event.After) != 1 || event.After[0] != 2 {
		t.Fatalf("legacy dependency gate did not hydrate after edge: %+v", event.After)
	}

	event, err = UnmarshalEvent([]byte(`{"type":"script_run","node_id":2,"trigger":"gate","cmd":"true","exit_code":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.Trigger != TriggerAcceptance {
		t.Fatalf("legacy gate trigger did not hydrate acceptance trigger: %q", event.Trigger)
	}
}

func TestApplyRejectsCorruptHistories(t *testing.T) {
	now := time.Now()
	exp := now.Add(time.Hour)
	validRoot := &Event{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
		NodeID: 1, Kind: KindGoal, Intent: "root"}
	validTask := &Event{EventID: "task", Timestamp: now, Actor: "a", Type: EvNodeCreated,
		NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}}
	validClaim := &Event{EventID: "claim", Timestamp: now, Actor: "a", Type: EvClaimTaken,
		NodeID: 2, LeaseID: "lease", LeaseExpiresAt: &exp}
	validEvidence := &Event{EventID: "evidence", Timestamp: now, Actor: "a", Type: EvEvidence,
		NodeID: 2, EvidenceKind: EvidenceNote, EvidenceSummary: "ok"}

	cases := []struct {
		name   string
		events []*Event
	}{
		{"nil acceptance task", []*Event{{EventID: "bad", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 1, ParentID: 1, Kind: KindTask, Intent: "root"}}},
		{"second root", []*Event{validRoot, {EventID: "root2", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 2, Kind: KindGoal, Intent: "root2"}}},
		{"rule under rule", []*Event{validRoot,
			{EventID: "r1", Timestamp: now, Actor: "a", Type: EvNodeCreated,
				NodeID: 2, ParentID: 1, Kind: KindRule, RuleText: "a"},
			{EventID: "r2", Timestamp: now, Actor: "a", Type: EvNodeCreated,
				NodeID: 3, ParentID: 2, Kind: KindRule, RuleText: "b"}}},
		{"double terminal", []*Event{validRoot, validTask, validClaim, validEvidence,
			{EventID: "done", Timestamp: now, Actor: "a", Type: EvTaskCompleted, NodeID: 2, EvidenceID: "evidence"},
			{EventID: "cancel", Timestamp: now, Actor: "a", Type: EvNodeCanceled, NodeID: 2, Reason: "bad"}}},
		{"claim renew wrong lease", []*Event{validRoot, validTask, validClaim,
			{EventID: "renew", Timestamp: now, Actor: "a", Type: EvClaimRenewed,
				NodeID: 2, LeaseID: "other", LeaseExpiresAt: &exp}}},
		{"review completion without evidence", []*Event{validRoot, validTask, validClaim,
			{EventID: "done", Timestamp: now, Actor: "a", Type: EvTaskCompleted, NodeID: 2}}},
		{"verify completion without acceptance run", []*Event{
			{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
				NodeID: 1, Kind: KindGoal, Intent: "root"},
			{EventID: "task", Timestamp: now, Actor: "a", Type: EvNodeCreated,
				NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: &Acceptance{Kind: AcceptanceVerify, Cmd: "true"}},
			validClaim,
			{EventID: "done", Timestamp: now, Actor: "a", Type: EvTaskCompleted, NodeID: 2},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Apply(tc.events); err == nil {
				t.Fatal("expected Apply to reject corrupt history")
			}
		})
	}
}
