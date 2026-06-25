package cst

import (
	"encoding/json"
	"strings"
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

func TestLegacyVerifyCompletionEvidenceIDCanReferenceFinalScriptRun(t *testing.T) {
	now := time.Now()
	exp := now.Add(time.Hour)
	events := []*Event{
		{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 1, Kind: KindGoal, Intent: "root"},
		{EventID: "task", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: &Acceptance{
				Kind: AcceptanceVerify,
				Checks: []VerifyCheck{
					{Name: "unit", Cmd: "go test ./..."},
					{Name: "lint", Cmd: "git diff --check"},
				},
			}},
		{EventID: "claim", Timestamp: now, Actor: "a", Type: EvClaimTaken,
			AttemptID: "attempt", NodeID: 2, LeaseID: "lease", LeaseExpiresAt: &exp},
		{EventID: "run-unit", Timestamp: now, Actor: "a", Type: EvScriptRun,
			AttemptID: "attempt", NodeID: 2, Trigger: TriggerAcceptance, CheckName: "unit", Cmd: "go test ./...", ExitCode: 0},
		{EventID: "run-lint", Timestamp: now, Actor: "a", Type: EvScriptRun,
			AttemptID: "attempt", NodeID: 2, Trigger: TriggerAcceptance, CheckName: "lint", Cmd: "git diff --check", ExitCode: 0},
		{EventID: "done", Timestamp: now, Actor: "a", Type: EvTaskCompleted,
			AttemptID: "attempt", NodeID: 2, EvidenceID: "run-lint"},
	}

	state, err := Apply(events)
	if err != nil {
		t.Fatalf("legacy completion rejected: %v", err)
	}
	task := state.Nodes[2]
	if !task.Completed || task.CompletedEvidence != "run-lint" {
		t.Fatalf("legacy completion projection mismatch: %+v", task)
	}
}

func TestModernVerifyCompletionRejectsScriptEvidenceIDs(t *testing.T) {
	now := time.Now()
	exp := now.Add(time.Hour)
	events := []*Event{
		{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 1, Kind: KindGoal, Intent: "root"},
		{EventID: "task", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: NewVerifyAcceptance("true")},
		{EventID: "claim", Timestamp: now, Actor: "a", Type: EvClaimTaken,
			AttemptID: "attempt", NodeID: 2, LeaseID: "lease", LeaseExpiresAt: &exp},
		{EventID: "run", Timestamp: now, Actor: "a", Type: EvScriptRun,
			AttemptID: "attempt", NodeID: 2, Trigger: TriggerAcceptance, CheckName: DefaultVerifyCheckName, Cmd: "true", ExitCode: 0},
		{EventID: "done", Timestamp: now, Actor: "a", Type: EvTaskCompleted,
			AttemptID: "attempt", NodeID: 2, EvidenceIDs: []string{"run"}},
	}

	_, err := Apply(events)
	if err == nil || !strings.Contains(err.Error(), "requires acceptance_run_set") {
		t.Fatalf("expected modern script evidence_ids rejection, got %v", err)
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

func TestVerifierContractEvidenceShape(t *testing.T) {
	now := time.Now()
	root := &Event{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
		NodeID: 1, Kind: KindGoal, Intent: "root"}
	task := &Event{EventID: "task", Timestamp: now, Actor: "a", Type: EvNodeCreated,
		NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "contract", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}}
	valid := &Event{EventID: "evidence", Timestamp: now, Actor: "a", Type: EvEvidence,
		NodeID: 2, EvidenceKind: EvidenceVerifierContract, EvidenceSummary: "contract", EvidenceData: validVerifierContractData()}
	if _, err := Apply([]*Event{root, task, valid}); err != nil {
		t.Fatalf("valid verifier_contract evidence rejected: %v", err)
	}

	invalids := []struct {
		name string
		data json.RawMessage
	}{
		{"missing data", nil},
		{"mutable source", verifierContractDataWith(t, `"canonical_source":{"ref":"README.md"}`)},
		{"missing artifacts", verifierContractDataWith(t, `"contract_artifacts":[]`)},
		{"bad script hash", verifierContractDataWith(t, `"verifier_scripts":[{"path":"scripts/verify-contract-lock","sha256":"bad"}]`)},
		{"prose red case", verifierContractDataWith(t, `"red_case_runs":[]`)},
		{"passing red case", verifierContractDataWith(t, `"red_case_runs":[{"name":"lazy","diff_path":"testdata/lazy.diff","diff_sha256":"`+testSHA256("4")+`","command":"make red","expected_exit":0,"observed_exit":0,"stderr_path":"testdata/lazy.stderr","stderr_sha256":"`+testSHA256("5")+`"}]`)},
	}
	for _, tc := range invalids {
		t.Run(tc.name, func(t *testing.T) {
			ev := *valid
			ev.EventID = "bad-" + tc.name
			ev.EvidenceData = tc.data
			if _, err := Apply([]*Event{root, task, &ev}); err == nil {
				t.Fatal("expected verifier_contract shape rejection")
			}
		})
	}
}

func validVerifierContractData() json.RawMessage {
	return json.RawMessage(`{
		"canonical_source":{"ref":"git:1234567890abcdef:README.md","description":"fixture"},
		"contract_artifacts":[{"path":".artifacts/verifier-contract.json","sha256":"` + testSHA256("1") + `"}],
		"verifier_scripts":[{"path":"scripts/verify-contract-lock","sha256":"` + testSHA256("2") + `"},{"path":"cmd/verify-contract-lock/main.go","sha256":"` + testSHA256("6") + `"}],
		"manifest":{"path":".artifacts/manifest.json","sha256":"` + testSHA256("3") + `","count":1},
		"cheapest_plausible_lie":"partial output passes",
		"red_case_runs":[{"name":"lazy","diff_path":"testdata/lazy.diff","diff_sha256":"` + testSHA256("4") + `","command":"make red","expected_exit":1,"observed_exit":1,"stderr_path":"testdata/lazy.stderr","stderr_sha256":"` + testSHA256("5") + `"}],
		"blind_spots":[]
	}`)
}

func verifierContractDataWith(t *testing.T, replacement string) json.RawMessage {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(validVerifierContractData(), &obj); err != nil {
		t.Fatal(err)
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{`+replacement+`}`), &patch); err != nil {
		t.Fatal(err)
	}
	for k, v := range patch {
		obj[k] = v
	}
	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func testSHA256(seed string) string {
	return strings.Repeat(seed, 64)
}
