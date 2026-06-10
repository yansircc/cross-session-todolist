package cst

import (
	"encoding/json"
	"time"
)

type NodeStatus string

const (
	StatusOpen      NodeStatus = "open"
	StatusClaimed   NodeStatus = "claimed"
	StatusHeld      NodeStatus = "held"
	StatusCompleted NodeStatus = "completed"
	StatusCanceled  NodeStatus = "canceled"
)

type Revision struct {
	EventCount  int       `json:"event_count"`
	LastEventID string    `json:"last_event_id,omitempty"`
	LastEventAt time.Time `json:"last_event_at,omitempty"`
}

type Claim struct {
	Actor          string    `json:"actor"`
	AttemptID      string    `json:"attempt_id,omitempty"`
	LeaseID        string    `json:"lease_id"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
	TakenAt        time.Time `json:"taken_at"`
}

type ScriptRunRecord struct {
	EventID                       string       `json:"event_id"`
	NodeID                        int64        `json:"node_id"`
	AttemptID                     string       `json:"attempt_id,omitempty"`
	Trigger                       string       `json:"trigger"`
	CheckName                     string       `json:"check_name,omitempty"`
	Cmd                           string       `json:"cmd"`
	ExitCode                      int          `json:"exit_code"`
	DurationMs                    int64        `json:"duration_ms"`
	StdoutHead                    string       `json:"stdout_head,omitempty"`
	StderrHead                    string       `json:"stderr_head,omitempty"`
	Truncated                     bool         `json:"truncated,omitempty"`
	StoreID                       string       `json:"store_id,omitempty"`
	ExecCWD                       string       `json:"exec_cwd,omitempty"`
	GitAvailable                  bool         `json:"git_available,omitempty"`
	GitRoot                       string       `json:"git_root,omitempty"`
	GitHead                       string       `json:"git_head,omitempty"`
	GitBranch                     string       `json:"git_branch,omitempty"`
	GitStatusShort                string       `json:"git_status_short,omitempty"`
	StagedDiffSHA256              string       `json:"staged_diff_sha256,omitempty"`
	UnstagedDiffSHA256            string       `json:"unstaged_diff_sha256,omitempty"`
	UntrackedManifestSHA256       string       `json:"untracked_manifest_sha256,omitempty"`
	GitIdentityDigest             string       `json:"git_identity_digest,omitempty"`
	ExecSurface                   string       `json:"exec_surface,omitempty"`
	OwnedPaths                    []string     `json:"owned_paths,omitempty"`
	ScopedGitStatusShort          string       `json:"scoped_git_status_short,omitempty"`
	ScopedStagedDiffSHA256        string       `json:"scoped_staged_diff_sha256,omitempty"`
	ScopedUnstagedDiffSHA256      string       `json:"scoped_unstaged_diff_sha256,omitempty"`
	ScopedUntrackedManifestSHA256 string       `json:"scoped_untracked_manifest_sha256,omitempty"`
	ScopedDigest                  string       `json:"scoped_digest,omitempty"`
	OutOfScopeGitStatusShort      string       `json:"out_of_scope_git_status_short,omitempty"`
	OutOfScopeDeltaCount          int          `json:"out_of_scope_delta_count,omitempty"`
	OutOfScopeDigest              string       `json:"out_of_scope_digest,omitempty"`
	WholeRepoDigest               string       `json:"whole_repo_digest,omitempty"`
	ParallelWorktree              string       `json:"parallel_worktree,omitempty"`
	ExecContextDigest             string       `json:"exec_context_digest,omitempty"`
	StdoutArtifact                *ArtifactRef `json:"stdout_artifact,omitempty"`
	StderrArtifact                *ArtifactRef `json:"stderr_artifact,omitempty"`
	Actor                         string       `json:"actor"`
	At                            time.Time    `json:"at"`
}

type Hold struct {
	Kind   string    `json:"kind"`
	Reason string    `json:"reason"`
	Actor  string    `json:"actor"`
	At     time.Time `json:"at"`
}

type EvidenceRecord struct {
	EventID   string          `json:"event_id"`
	NodeID    int64           `json:"node_id"`
	AttemptID string          `json:"attempt_id,omitempty"`
	Kind      string          `json:"kind"`
	Summary   string          `json:"summary"`
	Data      json.RawMessage `json:"data,omitempty"`
	Actor     string          `json:"actor"`
	At        time.Time       `json:"at"`
}

type Attempt struct {
	ID          string    `json:"id"`
	NodeID      int64     `json:"node_id"`
	Actor       string    `json:"actor"`
	LeaseID     string    `json:"lease_id"`
	StartedAt   time.Time `json:"started_at"`
	LastEventAt time.Time `json:"last_event_at"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
	CloseReason string    `json:"close_reason,omitempty"`
}

type Node struct {
	ID                int64              `json:"id"`
	ParentID          int64              `json:"parent_id,omitempty"`
	Kind              string             `json:"kind"`
	Intent            string             `json:"intent,omitempty"`
	RuleText          string             `json:"rule_text,omitempty"`
	Acceptance        *Acceptance        `json:"acceptance,omitempty"`
	Envelope          *ExecutionEnvelope `json:"execution_envelope,omitempty"`
	After             []int64            `json:"after,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	CreatedBy         string             `json:"created_by"`
	CreatedEventID    string             `json:"created_event_id,omitempty"`
	AcceptanceEventAt time.Time          `json:"acceptance_event_at,omitempty"`
	EnvelopeEventAt   time.Time          `json:"envelope_event_at,omitempty"`

	Children []int64 `json:"children,omitempty"`

	// terminal state
	Completed         bool      `json:"completed,omitempty"`
	CompletedAt       time.Time `json:"completed_at,omitempty"`
	CompletedEvidence string    `json:"completed_evidence_id,omitempty"`
	Canceled          bool      `json:"canceled,omitempty"`
	CanceledAt        time.Time `json:"canceled_at,omitempty"`
	CanceledReason    string    `json:"canceled_reason,omitempty"`

	// runtime
	Claim     *Claim            `json:"claim,omitempty"`
	Hold      *Hold             `json:"hold,omitempty"`
	Runs      []ScriptRunRecord `json:"runs,omitempty"`
	Evidences []EvidenceRecord  `json:"evidences,omitempty"`
	LastEvent time.Time         `json:"last_event,omitempty"`
}

type State struct {
	Nodes       map[int64]*Node
	Order       []int64 // creation order, useful for stable lists
	NextID      int64
	EvidenceIDs map[string]EvidenceRecord
	Attempts    map[string]*Attempt
	Revision    Revision

	completedOrder []int64
	canceledOrder  []int64
}

func NewState() *State {
	return &State{
		Nodes:       map[int64]*Node{},
		Order:       nil,
		NextID:      1,
		EvidenceIDs: map[string]EvidenceRecord{},
		Attempts:    map[string]*Attempt{},
	}
}

func (s *State) StoreID() string {
	root := s.Root()
	if root == nil {
		return ""
	}
	return root.CreatedEventID
}
