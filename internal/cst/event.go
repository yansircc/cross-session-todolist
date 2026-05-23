package cst

import (
	"encoding/json"
	"time"
)

const (
	EvNodeCreated    = "node_created"
	EvNodeRevised    = "node_revised"
	EvNodeHeld       = "node_held"
	EvNodeUnheld     = "node_unheld"
	EvEvidence       = "evidence_recorded"
	EvClaimTaken     = "claim_taken"
	EvClaimRenewed   = "claim_renewed"
	EvClaimReleased  = "claim_released"
	EvClaimAbandoned = "claim_abandoned"
	EvScriptRun      = "script_run"
	EvTaskCompleted  = "task_completed"
	EvNodeCanceled   = "node_canceled"
)

const (
	KindGoal = "goal"
	KindTask = "task"
	KindRule = "rule"
)

const (
	AcceptanceVerify = "verify"
	AcceptanceReview = "review"
)

const DefaultVerifyCheckName = "default"

const (
	TriggerProbe      = "probe"
	TriggerAcceptance = "acceptance"
)

const (
	HoldBlocked  = "blocked"
	HoldWaiting  = "waiting"
	HoldDeferred = "deferred"
)

const (
	EvidenceNote             = "note"
	EvidenceCommit           = "commit"
	EvidenceFile             = "file"
	EvidenceURL              = "url"
	EvidenceRunID            = "run_id"
	EvidenceTest             = "test"
	EvidenceScript           = "script_run"
	EvidenceVerifierContract = "verifier_contract"
)

type Acceptance struct {
	Kind   string        `json:"kind"`
	Checks []VerifyCheck `json:"checks,omitempty"`
	// COMPAT: failure_model=stores created before named verify checks stored
	// verify acceptance as a single cmd string; redesign_blocker=append-only
	// histories cannot be rewritten by readers; removal_condition=all installed
	// stores have passed an explicit cmd-to-checks migration.
	Cmd string `json:"cmd,omitempty"`
	Who string `json:"who,omitempty"`
}

type VerifyCheck struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
}

type LegacyGate struct {
	Kind   string `json:"kind"`
	Cmd    string `json:"cmd,omitempty"`
	Who    string `json:"who,omitempty"`
	GateID int64  `json:"gate_id,omitempty"`
}

type Event struct {
	EventID   string    `json:"event_id"`
	Timestamp time.Time `json:"ts"`
	Actor     string    `json:"actor"`
	Type      string    `json:"type"`
	AttemptID string    `json:"attempt_id,omitempty"`

	NodeID     int64       `json:"node_id,omitempty"`
	ParentID   int64       `json:"parent_id,omitempty"`
	Kind       string      `json:"kind,omitempty"`
	Intent     string      `json:"intent,omitempty"`
	RuleText   string      `json:"rule_text,omitempty"`
	Acceptance *Acceptance `json:"acceptance,omitempty"`
	// COMPAT: failure_model=stores created before the acceptance/prerequisite split
	// persisted task acceptance under "gate"; redesign_blocker=old append-only stores
	// cannot be rewritten safely by readers; removal_condition=all installed stores
	// have passed an explicit gate-to-acceptance migration.
	LegacyGate *LegacyGate `json:"gate,omitempty"`
	After      []int64     `json:"after,omitempty"`
	AfterSet   bool        `json:"after_set,omitempty"`

	LeaseID        string     `json:"lease_id,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`

	Trigger    string `json:"trigger,omitempty"`
	CheckName  string `json:"check_name,omitempty"`
	Cmd        string `json:"cmd,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	StdoutHead string `json:"stdout_head,omitempty"`
	StderrHead string `json:"stderr_head,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`

	HoldKind string `json:"hold_kind,omitempty"`
	Reason   string `json:"reason,omitempty"`

	EvidenceID      string          `json:"evidence_id,omitempty"`
	EvidenceKind    string          `json:"evidence_kind,omitempty"`
	EvidenceSummary string          `json:"evidence_summary,omitempty"`
	EvidenceData    json.RawMessage `json:"evidence_data,omitempty"`
}

func (e *Event) MarshalLine() ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func UnmarshalEvent(line []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(line, &e); err != nil {
		return nil, err
	}
	hydrateLegacyGate(&e)
	hydrateLegacyVerifyCmd(&e)
	hydrateLegacyTrigger(&e)
	e.LegacyGate = nil
	return &e, nil
}

func hydrateLegacyGate(e *Event) {
	if e.LegacyGate == nil {
		return
	}
	switch e.LegacyGate.Kind {
	case AcceptanceVerify:
		if e.Acceptance == nil {
			e.Acceptance = &Acceptance{Kind: AcceptanceVerify, Cmd: e.LegacyGate.Cmd}
		}
	case AcceptanceReview:
		if e.Acceptance == nil {
			e.Acceptance = &Acceptance{Kind: AcceptanceReview, Who: e.LegacyGate.Who}
		}
	case "gate":
		if e.Acceptance == nil {
			e.Acceptance = &Acceptance{Kind: AcceptanceReview, Who: "self"}
		}
		if e.LegacyGate.GateID != 0 && !int64ListContains(e.After, e.LegacyGate.GateID) {
			e.After = append(e.After, e.LegacyGate.GateID)
		}
	}
}

func hydrateLegacyVerifyCmd(e *Event) {
	if e.Acceptance == nil || e.Acceptance.Kind != AcceptanceVerify {
		return
	}
	if len(e.Acceptance.Checks) == 0 && e.Acceptance.Cmd != "" {
		e.Acceptance.Checks = []VerifyCheck{{Name: DefaultVerifyCheckName, Cmd: e.Acceptance.Cmd}}
		e.Acceptance.Cmd = ""
	}
}

func int64ListContains(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hydrateLegacyTrigger(e *Event) {
	if e.Trigger == "gate" {
		e.Trigger = TriggerAcceptance
	}
}
