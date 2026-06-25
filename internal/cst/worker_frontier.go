package cst

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	ActionTakeReadyTask                 = "take_ready_task"
	ActionRunAcceptance                 = "run_acceptance"
	ActionCompleteFromAcceptance        = "complete_from_acceptance"
	ActionCompleteReviewWithEvidence    = "complete_review_with_existing_evidence"
	ActionInvalidCompleteVerifyWithNote = "invalid_complete_verify_with_note"
)

type BoundAction struct {
	ActionID     string   `json:"action_id"`
	Kind         string   `json:"kind"`
	StoreRoot    string   `json:"store_root"`
	StoreID      string   `json:"store_id"`
	Revision     Revision `json:"revision"`
	Actor        string   `json:"actor"`
	TaskID       int64    `json:"task_id"`
	ClaimLeaseID string   `json:"claim_lease_id,omitempty"`
	AttemptID    string   `json:"attempt_id,omitempty"`
	ExecCWD      string   `json:"exec_cwd,omitempty"`
	ExecSurface  string   `json:"exec_surface,omitempty"`
	OwnedPaths   []string `json:"owned_paths,omitempty"`
	EvidenceID   string   `json:"evidence_id,omitempty"`
	Preview      string   `json:"preview,omitempty"`
}

type ActionDecision struct {
	Accept bool   `json:"accept"`
	Reason string `json:"reason,omitempty"`
}

type FrontierInput struct {
	State     *State
	StoreRoot string
	StoreID   string
	Revision  Revision
	Actor     string
	Now       time.Time
	TaskID    int64
}

type WorkerStatusView struct {
	TaskID               int64                     `json:"task_id"`
	StoreRoot            string                    `json:"store_root"`
	StoreID              string                    `json:"store_id"`
	Revision             Revision                  `json:"revision"`
	Actor                string                    `json:"actor"`
	Status               string                    `json:"status"`
	Claim                *WorkerClaimView          `json:"claim,omitempty"`
	ExecutionEnvelope    ExecutionEnvelope         `json:"execution_envelope"`
	Briefing             *DeveloperBriefing        `json:"briefing,omitempty"`
	Actions              []BoundAction             `json:"actions"`
	ExternalObservations WorkerExternalObservation `json:"external_observations"`
}

type WorkerClaimView struct {
	Actor          string    `json:"actor"`
	AttemptID      string    `json:"attempt_id,omitempty"`
	LeaseID        string    `json:"lease_id"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
	Stale          bool      `json:"stale"`
}

type WorkerExternalObservation struct {
	Subagents  string    `json:"subagents"`
	ObservedAt time.Time `json:"observed_at"`
}

func FrontierInputFromTx(tx *Tx, taskID int64) FrontierInput {
	return FrontierInput{
		State:     tx.state,
		StoreRoot: tx.paths.Root,
		StoreID:   tx.StoreID(),
		Revision:  tx.state.Revision,
		Actor:     tx.actor,
		Now:       tx.now,
		TaskID:    taskID,
	}
}

func BuildWorkerStatus(input FrontierInput) (WorkerStatusView, error) {
	n, err := requireFrontierTask(input)
	if err != nil {
		return WorkerStatusView{}, err
	}
	env := effectiveExecutionEnvelope(n)
	view := WorkerStatusView{
		TaskID:            input.TaskID,
		StoreRoot:         input.StoreRoot,
		StoreID:           input.StoreID,
		Revision:          input.Revision,
		Actor:             input.Actor,
		Status:            string(input.State.NodeStatus(n)),
		ExecutionEnvelope: env,
		Briefing:          BuildDeveloperBriefing(input.State, input.TaskID),
		Actions:           LegalFrontier(input),
		ExternalObservations: WorkerExternalObservation{
			Subagents:  "unknown",
			ObservedAt: input.Now,
		},
	}
	if n.Claim != nil {
		view.Claim = &WorkerClaimView{
			Actor:          n.Claim.Actor,
			AttemptID:      n.Claim.AttemptID,
			LeaseID:        n.Claim.LeaseID,
			LeaseExpiresAt: n.Claim.LeaseExpiresAt,
			Stale:          input.Now.After(n.Claim.LeaseExpiresAt),
		}
	}
	return view, nil
}

func LegalFrontier(input FrontierInput) []BoundAction {
	candidates := CanonicalActions(input)
	out := make([]BoundAction, 0, len(candidates))
	for _, action := range candidates {
		if Admissible(input, action).Accept {
			out = append(out, action)
		}
	}
	return out
}

func CanonicalActions(input FrontierInput) []BoundAction {
	n := input.State.Nodes[input.TaskID]
	if n == nil || n.Kind != KindTask || n.Terminal() {
		return nil
	}
	env := effectiveExecutionEnvelope(n)
	base := BoundAction{
		StoreRoot:   input.StoreRoot,
		StoreID:     input.StoreID,
		Revision:    input.Revision,
		Actor:       input.Actor,
		TaskID:      input.TaskID,
		ExecCWD:     env.ExecCWD,
		ExecSurface: firstNonEmpty(env.ExecSurface, ExecSurfaceShared),
		OwnedPaths:  normalizeOwnedPaths(env.OwnedPaths),
	}
	if n.Claim != nil {
		base.ClaimLeaseID = n.Claim.LeaseID
		base.AttemptID = n.Claim.AttemptID
	}

	actions := []BoundAction{}
	if n.Claim == nil {
		actions = append(actions, bindAction(base, ActionTakeReadyTask, ""))
	}
	if n.Acceptance != nil {
		switch n.Acceptance.Kind {
		case AcceptanceVerify:
			if n.Claim != nil {
				actions = append(actions, bindAction(base, ActionRunAcceptance, ""))
				for _, ev := range acceptanceRunSetEvidence(n) {
					a := base
					a.EvidenceID = ev.EventID
					actions = append(actions, bindAction(a, ActionCompleteFromAcceptance, ""))
				}
			}
		case AcceptanceReview:
			if n.Claim != nil {
				for _, ev := range reviewCompletionEvidence(n) {
					a := base
					a.EvidenceID = ev.EventID
					actions = append(actions, bindAction(a, ActionCompleteReviewWithEvidence, ""))
				}
			}
		}
	}
	return actions
}

func Admissible(input FrontierInput, action BoundAction) ActionDecision {
	n, err := requireFrontierTask(input)
	if err != nil {
		return rejectDecision(err)
	}
	if action.StoreRoot != input.StoreRoot {
		return rejectf("store root changed")
	}
	if action.StoreID != input.StoreID {
		return rejectf("store id changed")
	}
	if action.Actor != input.Actor {
		return rejectf("actor changed")
	}
	if action.TaskID != input.TaskID {
		return rejectf("task mismatch")
	}
	if action.Revision.EventCount != input.Revision.EventCount || action.Revision.LastEventID != input.Revision.LastEventID {
		return rejectf("store revision changed")
	}
	env := effectiveExecutionEnvelope(n)
	if action.ExecCWD != env.ExecCWD ||
		firstNonEmpty(action.ExecSurface, ExecSurfaceShared) != firstNonEmpty(env.ExecSurface, ExecSurfaceShared) ||
		!sameStringList(normalizeOwnedPaths(action.OwnedPaths), normalizeOwnedPaths(env.OwnedPaths)) {
		return rejectf("execution envelope changed")
	}

	switch action.Kind {
	case ActionTakeReadyTask:
		if n.Claim != nil {
			return rejectf("task is already claimed")
		}
		if !input.State.IsReadyTask(input.TaskID) {
			return rejectf("%s", input.State.ReadyBlockReason(input.TaskID))
		}
		return acceptDecision()
	case ActionRunAcceptance:
		guard, decision := completionGuardFromState(input)
		if !decision.Accept {
			return decision
		}
		if guard.AcceptanceKind != AcceptanceVerify {
			return rejectf("run acceptance is only valid for verify acceptance")
		}
		return acceptDecision()
	case ActionCompleteFromAcceptance:
		guard, decision := completionGuardFromState(input)
		if !decision.Accept {
			return decision
		}
		if guard.AcceptanceKind != AcceptanceVerify {
			return rejectf("from-acceptance is only valid for verify acceptance")
		}
		if action.EvidenceID == "" {
			return rejectf("acceptance_run_set evidence_id is required")
		}
		if decision := completeEvidenceAdmissible(input, guard, action.EvidenceID); !decision.Accept {
			return decision
		}
		if decision := acceptanceContextAdmissible(input, action.EvidenceID, ""); !decision.Accept {
			return decision
		}
		return acceptDecision()
	case ActionCompleteReviewWithEvidence:
		guard, decision := completionGuardFromState(input)
		if !decision.Accept {
			return decision
		}
		if guard.AcceptanceKind != AcceptanceReview {
			return rejectf("review evidence completion is only valid for review acceptance")
		}
		if action.EvidenceID == "" {
			return rejectf("review completion requires evidence_id")
		}
		return completeEvidenceAdmissible(input, guard, action.EvidenceID)
	case ActionInvalidCompleteVerifyWithNote:
		return rejectf("verify acceptance records acceptance_run_set evidence; --note is invalid")
	default:
		return rejectf("unknown action kind %q", action.Kind)
	}
}

func CanonicalActionEqualSets(input FrontierInput) bool {
	frontier := map[string]bool{}
	for _, action := range LegalFrontier(input) {
		frontier[action.ActionID] = true
	}
	admissible := map[string]bool{}
	for _, action := range CanonicalActions(input) {
		if Admissible(input, action).Accept {
			admissible[action.ActionID] = true
		}
	}
	if len(frontier) != len(admissible) {
		return false
	}
	for id := range admissible {
		if !frontier[id] {
			return false
		}
	}
	return true
}

func RenderWorkerStatusText(w io.Writer, view WorkerStatusView) {
	fmt.Fprintf(w, "worker-status #%d status=%s\n", view.TaskID, view.Status)
	fmt.Fprintf(w, "store: %s\n", view.StoreRoot)
	fmt.Fprintf(w, "store_id: %s revision=%d:%s\n", view.StoreID, view.Revision.EventCount, view.Revision.LastEventID)
	if view.ExecutionEnvelope.ExecCWD != "" {
		fmt.Fprintf(w, "exec: cwd=%s surface=%s", view.ExecutionEnvelope.ExecCWD, firstNonEmpty(view.ExecutionEnvelope.ExecSurface, ExecSurfaceShared))
		if len(view.ExecutionEnvelope.OwnedPaths) > 0 {
			fmt.Fprintf(w, " scope=%s", strings.Join(view.ExecutionEnvelope.OwnedPaths, ","))
		}
		fmt.Fprintln(w)
	}
	if view.Claim != nil {
		stale := ""
		if view.Claim.Stale {
			stale = " stale"
		}
		fmt.Fprintf(w, "claim: actor=%s attempt=%s lease=%s%s\n", view.Claim.Actor, view.Claim.AttemptID, view.Claim.LeaseID, stale)
	}
	RenderDeveloperBriefingText(w, view.Briefing)
	fmt.Fprintf(w, "subagents: %s observed_at=%s\n", view.ExternalObservations.Subagents, view.ExternalObservations.ObservedAt.Format(time.RFC3339))
	if len(view.Actions) == 0 {
		fmt.Fprintln(w, "actions: none")
		return
	}
	fmt.Fprintln(w, "actions:")
	for _, action := range view.Actions {
		fmt.Fprintf(w, "  %s  %s\n", action.ActionID, action.Kind)
		fmt.Fprintf(w, "    run: cst --store %s worker-run %d --action %s\n", action.StoreRoot, action.TaskID, action.ActionID)
		if action.Preview != "" {
			fmt.Fprintf(w, "    preview: %s\n", action.Preview)
		}
	}
}

func bindAction(action BoundAction, kind string, preview string) BoundAction {
	action.Kind = kind
	action.Preview = previewForAction(action, preview)
	action.ActionID = actionDigest(action)
	return action
}

func actionDigest(action BoundAction) string {
	copy := action
	copy.ActionID = ""
	copy.Preview = ""
	b, _ := json.Marshal(copy)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}

func previewForAction(action BoundAction, explicit string) string {
	if explicit != "" {
		return explicit
	}
	prefix := fmt.Sprintf("cst --store %s ", action.StoreRoot)
	switch action.Kind {
	case ActionTakeReadyTask:
		return prefix + fmt.Sprintf("take %d", action.TaskID)
	case ActionRunAcceptance:
		return prefix + fmt.Sprintf("run %d --acceptance", action.TaskID)
	case ActionCompleteFromAcceptance:
		return prefix + fmt.Sprintf("done %d --from-acceptance %s", action.TaskID, action.EvidenceID)
	case ActionCompleteReviewWithEvidence:
		return prefix + fmt.Sprintf("done %d --evidence %s", action.TaskID, action.EvidenceID)
	default:
		return ""
	}
}

func requireFrontierTask(input FrontierInput) (*Node, error) {
	if input.State == nil {
		return nil, herr(ExitInvariantBroken, "frontier requires state")
	}
	n := input.State.Nodes[input.TaskID]
	if n == nil {
		return nil, herr(ExitNotFound, "node #%d not found", input.TaskID)
	}
	if n.Kind != KindTask {
		return nil, herr(ExitInvariantBroken, "node #%d is %s, not task", input.TaskID, n.Kind)
	}
	if n.Terminal() {
		return nil, herr(ExitInvariantBroken, "task #%d already terminal", input.TaskID)
	}
	return n, nil
}

func completionGuardFromState(input FrontierInput) (CompletionGuard, ActionDecision) {
	guard, err := completionGuardFromSnapshot(input.State, input.TaskID, input.Actor)
	if err != nil {
		return CompletionGuard{}, rejectDecision(err)
	}
	return guard, acceptDecision()
}

func completeEvidenceAdmissible(input FrontierInput, guard CompletionGuard, evidenceID string) ActionDecision {
	rec, ok := input.State.EvidenceIDs[evidenceID]
	if !ok {
		return rejectf("evidence %s not found", evidenceID)
	}
	if rec.NodeID != guard.NodeID {
		return rejectf("evidence %s belongs to #%d", evidenceID, rec.NodeID)
	}
	if guard.ClaimAttemptID != "" && rec.AttemptID != guard.ClaimAttemptID {
		return rejectf("evidence %s attempt mismatch", evidenceID)
	}
	if guard.AcceptanceKind == AcceptanceVerify && rec.Kind != EvidenceAcceptanceRunSet {
		return rejectf("verify completion requires acceptance_run_set evidence, got %s", rec.Kind)
	}
	if guard.AcceptanceKind == AcceptanceReview && !reviewCompletionEvidenceKind(rec.Kind) {
		if reviewAuxiliaryEvidenceKind(rec.Kind) {
			return rejectf("%s evidence is auxiliary and cannot complete review acceptance", rec.Kind)
		}
		return rejectf("review completion cannot use evidence kind %s", rec.Kind)
	}
	if guard.AcceptanceKind == AcceptanceVerify {
		n := input.State.Nodes[guard.NodeID]
		if n == nil {
			return rejectf("task #%d not found", guard.NodeID)
		}
		fake := &Event{NodeID: guard.NodeID, AttemptID: guard.ClaimAttemptID}
		if err := input.State.validateAcceptanceRunSetCompletion(n, fake, rec); err != nil {
			return rejectf("%s", err.Error())
		}
	}
	return acceptDecision()
}

func acceptanceContextAdmissible(input FrontierInput, evidenceID string, execCWDOverride string) ActionDecision {
	rec, ok := input.State.EvidenceIDs[evidenceID]
	if !ok || rec.Kind != EvidenceAcceptanceRunSet {
		return acceptDecision()
	}
	decision, _ := acceptanceContextDecision(input.State, input.StoreID, input.TaskID, rec, execCWDOverride, input.Now)
	return decision
}

func acceptanceContextDecision(state *State, storeID string, id int64, rec EvidenceRecord, execCWDOverride string, now time.Time) (ActionDecision, *Event) {
	if rec.Kind != EvidenceAcceptanceRunSet {
		return acceptDecision(), nil
	}
	data, err := parseAcceptanceRunSetData(rec.Data)
	if err != nil {
		return rejectf("%s", err.Error()), nil
	}
	if data.ExecutionContext == nil {
		return acceptDecision(), nil
	}
	n := state.Nodes[id]
	if n == nil {
		return rejectf("task #%d not found", id), nil
	}
	if !n.EnvelopeEventAt.IsZero() && n.EnvelopeEventAt.After(rec.At) {
		return rejectf("task #%d execution envelope changed after acceptance run-set", id), nil
	}
	env := effectiveExecutionEnvelope(n)
	execCWD := resolveExecCWD(execCWDOverride, env)
	current, currentDigest := currentAcceptanceContext(storeID, execCWD, env)
	expected := *data.ExecutionContext
	expected.ExecSurface = firstNonEmpty(expected.ExecSurface, ExecSurfaceShared)
	expected.OwnedPaths = normalizeOwnedPaths(expected.OwnedPaths)
	if current.ExecCWD != expected.ExecCWD {
		return rejectf("task #%d acceptance exec_cwd drifted; rerun acceptance", id), nil
	}
	if current.ExecSurface != expected.ExecSurface || !sameStringList(current.OwnedPaths, expected.OwnedPaths) {
		return rejectf("task #%d acceptance execution envelope drifted; rerun acceptance", id), nil
	}
	if expected.ExecSurface == ExecSurfacePrivate {
		if currentDigest != data.ContextDigest {
			return rejectf("task #%d private execution context drifted; rerun acceptance", id), nil
		}
		return acceptDecision(), nil
	}
	if len(expected.OwnedPaths) > 0 && current.ScopedDigest != expected.ScopedDigest {
		return rejectf("task #%d scoped execution context drifted; rerun acceptance", id), nil
	}
	reason := ""
	if len(expected.OwnedPaths) > 0 && current.OutOfScopeDigest != expected.OutOfScopeDigest {
		reason = "out_of_scope_drift"
	}
	if len(expected.OwnedPaths) == 0 && current.WholeRepoDigest != expected.WholeRepoDigest {
		reason = "whole_repo_drift"
	}
	if reason == "" {
		return acceptDecision(), nil
	}
	raw, err := json.Marshal(contextDriftEvidence{
		AcceptanceEvidenceID: rec.EventID,
		Mode:                 ExecSurfaceShared,
		Reason:               reason,
		Expected:             expected,
		Current:              current,
	})
	if err != nil {
		return rejectf("%s", err.Error()), nil
	}
	ev := &Event{
		EventID:         NewEventID(),
		Timestamp:       now,
		Actor:           "system",
		Type:            EvEvidence,
		NodeID:          id,
		AttemptID:       rec.AttemptID,
		EvidenceKind:    EvidenceContextDrift,
		EvidenceSummary: reason,
		EvidenceData:    raw,
	}
	return acceptDecision(), ev
}

func acceptanceRunSetEvidence(n *Node) []EvidenceRecord {
	var out []EvidenceRecord
	for _, ev := range n.Evidences {
		if ev.Kind == EvidenceAcceptanceRunSet {
			out = append(out, ev)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].At.After(out[j].At)
	})
	return out
}

func reviewCompletionEvidence(n *Node) []EvidenceRecord {
	var out []EvidenceRecord
	for _, ev := range n.Evidences {
		if reviewCompletionEvidenceKind(ev.Kind) {
			out = append(out, ev)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].At.After(out[j].At)
	})
	return out
}

func acceptDecision() ActionDecision {
	return ActionDecision{Accept: true}
}

func rejectDecision(err error) ActionDecision {
	return rejectf("%s", err.Error())
}

func rejectf(format string, args ...any) ActionDecision {
	return ActionDecision{Accept: false, Reason: fmt.Sprintf(format, args...)}
}
