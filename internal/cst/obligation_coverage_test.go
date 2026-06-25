package cst

import (
	"strings"
	"testing"
	"time"
)

func TestObligationCoverageKeepsGoalOpenUntilLeafClaimsCoverRequiredSet(t *testing.T) {
	now := time.Now()
	exp := now.Add(time.Hour)
	events := []*Event{
		{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 1, Kind: KindGoal, Intent: "root", Context: &NodeContext{SuccessObligations: []string{"api", "ui"}}},
		{EventID: "task-a", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "a", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}, ObligationClaims: []string{"api"}},
		{EventID: "claim-a", Timestamp: now, Actor: "a", Type: EvClaimTaken,
			NodeID: 2, LeaseID: "lease-a", LeaseExpiresAt: &exp},
		{EventID: "ev-a", Timestamp: now, Actor: "a", Type: EvEvidence,
			NodeID: 2, EvidenceKind: EvidenceNote, EvidenceSummary: "reviewed"},
		{EventID: "done-a", Timestamp: now, Actor: "a", Type: EvTaskCompleted,
			NodeID: 2, EvidenceID: "ev-a"},
	}
	state, err := Apply(events)
	if err != nil {
		t.Fatal(err)
	}
	coverage := state.ObligationCoverage(1)
	if len(coverage.Missing) != 1 || coverage.Missing[0] != "ui" {
		t.Fatalf("coverage gap mismatch: %+v", coverage)
	}
	if state.NodeStatus(state.Root()) != StatusOpen {
		t.Fatalf("root should stay open with missing obligation: %+v", coverage)
	}
	events = append(events,
		&Event{EventID: "task-b", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 3, ParentID: 1, Kind: KindTask, Intent: "b", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}, ObligationClaims: []string{"ui"}},
		&Event{EventID: "claim-b", Timestamp: now, Actor: "a", Type: EvClaimTaken,
			NodeID: 3, LeaseID: "lease-b", LeaseExpiresAt: &exp},
		&Event{EventID: "ev-b", Timestamp: now, Actor: "a", Type: EvEvidence,
			NodeID: 3, EvidenceKind: EvidenceNote, EvidenceSummary: "reviewed"},
		&Event{EventID: "done-b", Timestamp: now, Actor: "a", Type: EvTaskCompleted,
			NodeID: 3, EvidenceID: "ev-b"},
	)
	state, err = Apply(events)
	if err != nil {
		t.Fatal(err)
	}
	coverage = state.ObligationCoverage(1)
	if len(coverage.Missing) != 0 || state.NodeStatus(state.Root()) != StatusCompleted {
		t.Fatalf("root should complete after coverage closes: status=%s coverage=%+v", state.NodeStatus(state.Root()), coverage)
	}
}

func TestShowAndBriefProjectObligationCoverageGaps(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(nilWriter{}, AddArgs{
		Intent:  "root",
		Context: &NodeContext{SuccessObligations: []string{"api"}},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(nilWriter{}, AddArgs{
		Parent:           1,
		Intent:           "leaf",
		AcceptanceReview: "self",
	}, false); err != nil {
		t.Fatal(err)
	}
	state := replayState(t)
	show, err := BuildShow(state, 1, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if show.ObligationCoverage == nil || len(show.ObligationCoverage.Missing) != 1 {
		t.Fatalf("show coverage gap missing: %+v", show.ObligationCoverage)
	}
	brief, err := BuildBrief(state, DefaultConfig(), "agent", 0)
	if err != nil {
		t.Fatal(err)
	}
	if brief.ObligationCoverage == nil || len(brief.ObligationCoverage.Missing) != 1 {
		t.Fatalf("brief coverage gap missing: %+v", brief.ObligationCoverage)
	}
}

func TestTaskCompletionRejectsOwnUncoveredObligation(t *testing.T) {
	now := time.Now()
	exp := now.Add(time.Hour)
	events := []*Event{
		{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 1, Kind: KindGoal, Intent: "root"},
		{EventID: "task", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}, Context: &NodeContext{SuccessObligations: []string{"api"}}},
		{EventID: "claim", Timestamp: now, Actor: "a", Type: EvClaimTaken,
			NodeID: 2, LeaseID: "lease", LeaseExpiresAt: &exp},
		{EventID: "evidence", Timestamp: now, Actor: "a", Type: EvEvidence,
			NodeID: 2, EvidenceKind: EvidenceNote, EvidenceSummary: "reviewed"},
		{EventID: "done", Timestamp: now, Actor: "a", Type: EvTaskCompleted,
			NodeID: 2, EvidenceID: "evidence"},
	}
	_, err := Apply(events)
	if err == nil || !strings.Contains(err.Error(), "uncovered success obligation") {
		t.Fatalf("expected uncovered obligation rejection, got %v", err)
	}
}

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) { return len(p), nil }
