package cst

import (
	"encoding/json"
	"fmt"
	"time"
)

// Apply runs all events through the state machine in append order. It is a
// checked reducer: any event that violates the state machine (duplicate id,
// double terminal, claim/lease drift, rule under rule, etc.) is reported as
// an error so corrupt or buggy histories fail loudly instead of producing
// quietly wrong projections. Lazy-abandon repair runs separately.
func Apply(events []*Event) (*State, error) {
	s := NewState()
	for i, e := range events {
		if err := s.applyOne(e); err != nil {
			return nil, fmt.Errorf("event[%d] %s: %w", i, e.Type, err)
		}
	}
	return s, nil
}

// LazyAbandonExpired returns synthetic claim_abandoned events for any active
// claim whose lease has expired by `now`. Caller is expected to append them
// before performing any mutating operation.
func (s *State) LazyAbandonExpired(now time.Time) []*Event {
	var out []*Event
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Claim == nil {
			continue
		}
		if !n.Claim.LeaseExpiresAt.IsZero() && now.After(n.Claim.LeaseExpiresAt) {
			exp := now
			out = append(out, &Event{
				EventID:        NewEventID(),
				Timestamp:      now,
				Actor:          "system",
				Type:           EvClaimAbandoned,
				AttemptID:      n.Claim.AttemptID,
				NodeID:         id,
				LeaseID:        n.Claim.LeaseID,
				LeaseExpiresAt: &exp,
				Reason:         "lease_expired",
			})
		}
	}
	return out
}

func (s *State) applyOne(e *Event) error {
	switch e.Type {
	case EvNodeCreated:
		if _, dup := s.Nodes[e.NodeID]; dup {
			return fmt.Errorf("duplicate node_created for #%d", e.NodeID)
		}
		switch e.Kind {
		case KindGoal:
			if e.Intent == "" {
				return fmt.Errorf("goal #%d created without intent", e.NodeID)
			}
			if e.Acceptance != nil {
				return fmt.Errorf("goal #%d cannot have acceptance", e.NodeID)
			}
			if len(e.After) > 0 || e.AfterSet {
				return fmt.Errorf("goal #%d cannot have prerequisites", e.NodeID)
			}
			if e.ParentID == 0 && s.AnyRoot() != nil {
				return fmt.Errorf("multiple root goals are not allowed")
			}
		case KindTask:
			if e.Intent == "" {
				return fmt.Errorf("task #%d created without intent", e.NodeID)
			}
			if e.ParentID == 0 {
				return fmt.Errorf("task #%d requires a goal/task parent", e.NodeID)
			}
			if err := s.validateAcceptance(e.NodeID, e.Acceptance); err != nil {
				return err
			}
			if err := s.validateAfter(e.NodeID, e.After); err != nil {
				return err
			}
		case KindRule:
			if e.RuleText == "" {
				return fmt.Errorf("rule #%d created without text", e.NodeID)
			}
			if e.ParentID == 0 {
				return fmt.Errorf("rule #%d requires a goal/task parent", e.NodeID)
			}
			if len(e.After) > 0 || e.AfterSet {
				return fmt.Errorf("rule #%d cannot have prerequisites", e.NodeID)
			}
		default:
			return fmt.Errorf("node #%d has unknown kind %q", e.NodeID, e.Kind)
		}
		if e.ParentID != 0 {
			p, ok := s.Nodes[e.ParentID]
			if !ok {
				return fmt.Errorf("node #%d parent #%d missing", e.NodeID, e.ParentID)
			}
			if p.Terminal() {
				return fmt.Errorf("node #%d created under terminal parent #%d", e.NodeID, e.ParentID)
			}
			if !p.CanParentWork() {
				return fmt.Errorf("node #%d parent #%d must be a goal or task", e.NodeID, e.ParentID)
			}
			if e.Kind == KindGoal && p.Kind != KindGoal {
				return fmt.Errorf("goal #%d parent #%d must be a goal", e.NodeID, e.ParentID)
			}
		}
		n := &Node{
			ID:         e.NodeID,
			ParentID:   e.ParentID,
			Kind:       e.Kind,
			Intent:     e.Intent,
			RuleText:   e.RuleText,
			Acceptance: e.Acceptance,
			After:      append([]int64(nil), e.After...),
			CreatedAt:  e.Timestamp,
			CreatedBy:  e.Actor,
			LastEvent:  e.Timestamp,
		}
		s.Nodes[n.ID] = n
		s.Order = append(s.Order, n.ID)
		if e.NodeID >= s.NextID {
			s.NextID = e.NodeID + 1
		}
		if n.ParentID != 0 {
			p := s.Nodes[n.ParentID]
			p.Children = append(p.Children, n.ID)
		}
	case EvNodeRevised:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("node_revised targets missing node #%d", e.NodeID)
		}
		if e.ParentID == 0 && e.Intent == "" && e.RuleText == "" && e.Acceptance == nil && !e.AfterSet {
			return fmt.Errorf("node_revised #%d has no changes", e.NodeID)
		}
		if n.Terminal() {
			return fmt.Errorf("node_revised targets terminal node #%d", e.NodeID)
		}
		if n.Claim != nil {
			return fmt.Errorf("node_revised targets claimed node #%d", e.NodeID)
		}
		if e.ParentID != 0 && e.ParentID != n.ParentID {
			if n.ParentID == 0 {
				return fmt.Errorf("node_revised cannot move root goal #%d", e.NodeID)
			}
			if err := s.moveNode(n, e.ParentID); err != nil {
				return err
			}
		}
		if e.Intent != "" {
			if n.Kind == KindRule {
				return fmt.Errorf("node_revised cannot set intent on rule #%d", e.NodeID)
			}
			n.Intent = e.Intent
		}
		if e.RuleText != "" {
			if n.Kind != KindRule {
				return fmt.Errorf("node_revised cannot set rule text on %s #%d", n.Kind, e.NodeID)
			}
			n.RuleText = e.RuleText
		}
		if e.Acceptance != nil {
			if n.Kind != KindTask {
				return fmt.Errorf("node_revised cannot set acceptance on %s #%d", n.Kind, e.NodeID)
			}
			if err := s.validateAcceptance(e.NodeID, e.Acceptance); err != nil {
				return err
			}
			n.Acceptance = e.Acceptance
		}
		if e.AfterSet {
			if n.Kind != KindTask {
				return fmt.Errorf("node_revised cannot set prerequisites on %s #%d", n.Kind, e.NodeID)
			}
			if err := s.validateAfter(e.NodeID, e.After); err != nil {
				return err
			}
			n.After = append([]int64(nil), e.After...)
		}
		n.LastEvent = e.Timestamp
	case EvClaimTaken:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("claim_taken targets missing node #%d", e.NodeID)
		}
		if n.Kind != KindTask {
			return fmt.Errorf("claim_taken targets non-task #%d", e.NodeID)
		}
		if e.LeaseID == "" {
			return fmt.Errorf("claim_taken on #%d missing lease id", e.NodeID)
		}
		if e.LeaseExpiresAt == nil {
			return fmt.Errorf("claim_taken on #%d missing lease expiry", e.NodeID)
		}
		if n.Terminal() {
			return fmt.Errorf("claim_taken targets terminal #%d", e.NodeID)
		}
		if n.Hold != nil {
			return fmt.Errorf("claim_taken targets held task #%d", e.NodeID)
		}
		if n.Claim != nil {
			return fmt.Errorf("claim_taken on already-claimed #%d", e.NodeID)
		}
		if !s.IsReadyTask(e.NodeID) {
			return fmt.Errorf("claim_taken targets non-ready task #%d: %s", e.NodeID, s.ReadyBlockReason(e.NodeID))
		}
		if e.AttemptID != "" {
			if _, dup := s.Attempts[e.AttemptID]; dup {
				return fmt.Errorf("claim_taken repeats attempt_id %s", e.AttemptID)
			}
			s.Attempts[e.AttemptID] = &Attempt{
				ID:          e.AttemptID,
				NodeID:      e.NodeID,
				Actor:       e.Actor,
				LeaseID:     e.LeaseID,
				StartedAt:   e.Timestamp,
				LastEventAt: e.Timestamp,
			}
		}
		exp := time.Time{}
		if e.LeaseExpiresAt != nil {
			exp = *e.LeaseExpiresAt
		}
		n.Claim = &Claim{
			Actor:          e.Actor,
			AttemptID:      e.AttemptID,
			LeaseID:        e.LeaseID,
			LeaseExpiresAt: exp,
			TakenAt:        e.Timestamp,
		}
		n.LastEvent = e.Timestamp
	case EvNodeHeld:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("node_held targets missing node #%d", e.NodeID)
		}
		if n.Kind != KindTask {
			return fmt.Errorf("node_held targets non-task #%d", e.NodeID)
		}
		if n.Terminal() {
			return fmt.Errorf("node_held targets terminal #%d", e.NodeID)
		}
		if n.Claim != nil {
			return fmt.Errorf("node_held targets claimed task #%d", e.NodeID)
		}
		if e.HoldKind != HoldBlocked && e.HoldKind != HoldWaiting && e.HoldKind != HoldDeferred {
			return fmt.Errorf("node_held #%d has unknown hold kind %q", e.NodeID, e.HoldKind)
		}
		if e.Reason == "" {
			return fmt.Errorf("node_held #%d requires reason", e.NodeID)
		}
		n.Hold = &Hold{Kind: e.HoldKind, Reason: e.Reason, Actor: e.Actor, At: e.Timestamp}
		n.LastEvent = e.Timestamp
	case EvNodeUnheld:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("node_unheld targets missing node #%d", e.NodeID)
		}
		if n.Kind != KindTask {
			return fmt.Errorf("node_unheld targets non-task #%d", e.NodeID)
		}
		if n.Terminal() {
			return fmt.Errorf("node_unheld targets terminal #%d", e.NodeID)
		}
		if n.Hold == nil {
			return fmt.Errorf("node_unheld targets unheld task #%d", e.NodeID)
		}
		n.Hold = nil
		n.LastEvent = e.Timestamp
	case EvClaimRenewed:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("claim_renewed targets missing node #%d", e.NodeID)
		}
		if n.Claim == nil {
			return fmt.Errorf("claim_renewed on unclaimed #%d", e.NodeID)
		}
		if e.LeaseExpiresAt == nil {
			return fmt.Errorf("claim_renewed on #%d missing lease expiry", e.NodeID)
		}
		if n.Claim.LeaseID != e.LeaseID {
			return fmt.Errorf("claim_renewed lease id mismatch on #%d", e.NodeID)
		}
		if n.Claim.AttemptID != "" && e.AttemptID == "" {
			return fmt.Errorf("claim_renewed on #%d missing attempt_id", e.NodeID)
		}
		if err := s.applyAttemptEvent(e, false); err != nil {
			return err
		}
		n.Claim.LeaseExpiresAt = *e.LeaseExpiresAt
		n.LastEvent = e.Timestamp
	case EvClaimReleased, EvClaimAbandoned:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("%s targets missing node #%d", e.Type, e.NodeID)
		}
		if n.Claim == nil {
			return fmt.Errorf("%s on unclaimed #%d", e.Type, e.NodeID)
		}
		if e.LeaseID == "" {
			return fmt.Errorf("%s on #%d missing lease id", e.Type, e.NodeID)
		}
		if n.Claim.LeaseID != e.LeaseID {
			return fmt.Errorf("%s lease id mismatch on #%d", e.Type, e.NodeID)
		}
		if n.Claim.AttemptID != "" && e.AttemptID == "" {
			return fmt.Errorf("%s on #%d missing attempt_id", e.Type, e.NodeID)
		}
		if e.AttemptID != "" {
			if err := s.applyAttemptEvent(e, e.Type == EvClaimAbandoned && e.Actor == "system"); err != nil {
				return err
			}
			s.closeAttempt(e.AttemptID, e.Type, e.Timestamp)
		}
		n.Claim = nil
		n.LastEvent = e.Timestamp
	case EvScriptRun:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("script_run targets missing node #%d", e.NodeID)
		}
		if n.Kind != KindTask {
			return fmt.Errorf("script_run targets non-task #%d", e.NodeID)
		}
		if e.Trigger != TriggerProbe && e.Trigger != TriggerAcceptance {
			return fmt.Errorf("script_run #%d has unknown trigger %q", e.NodeID, e.Trigger)
		}
		if n.Claim != nil && n.Claim.Actor == e.Actor && n.Claim.AttemptID != "" && e.AttemptID == "" {
			return fmt.Errorf("script_run on #%d missing attempt_id", e.NodeID)
		}
		if err := s.applyAttemptEvent(e, false); err != nil {
			return err
		}
		n.Runs = append(n.Runs, ScriptRunRecord{
			EventID:    e.EventID,
			NodeID:     e.NodeID,
			AttemptID:  e.AttemptID,
			Trigger:    e.Trigger,
			CheckName:  e.CheckName,
			Cmd:        e.Cmd,
			ExitCode:   e.ExitCode,
			DurationMs: e.DurationMs,
			StdoutHead: e.StdoutHead,
			StderrHead: e.StderrHead,
			Truncated:  e.Truncated,
			Actor:      e.Actor,
			At:         e.Timestamp,
		})
		evidence := EvidenceRecord{
			EventID:   e.EventID,
			NodeID:    e.NodeID,
			AttemptID: e.AttemptID,
			Kind:      EvidenceScript,
			Summary:   scriptRunSummary(e),
			Actor:     e.Actor,
			At:        e.Timestamp,
		}
		n.Evidences = append(n.Evidences, evidence)
		s.EvidenceIDs[e.EventID] = evidence
		n.LastEvent = e.Timestamp
	case EvEvidence:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("evidence_recorded targets missing node #%d", e.NodeID)
		}
		if !n.CanHaveEvidence() {
			return fmt.Errorf("evidence_recorded targets invalid node kind %q", n.Kind)
		}
		if e.EvidenceKind == "" {
			return fmt.Errorf("evidence_recorded #%d missing kind", e.NodeID)
		}
		if e.EvidenceSummary == "" {
			return fmt.Errorf("evidence_recorded #%d missing summary", e.NodeID)
		}
		if n.Claim != nil && n.Claim.Actor == e.Actor && n.Claim.AttemptID != "" && e.AttemptID == "" {
			return fmt.Errorf("evidence_recorded on #%d missing attempt_id", e.NodeID)
		}
		if err := s.applyAttemptEvent(e, false); err != nil {
			return err
		}
		rec := EvidenceRecord{
			EventID:   e.EventID,
			NodeID:    e.NodeID,
			AttemptID: e.AttemptID,
			Kind:      e.EvidenceKind,
			Summary:   e.EvidenceSummary,
			Data:      append(json.RawMessage(nil), e.EvidenceData...),
			Actor:     e.Actor,
			At:        e.Timestamp,
		}
		n.Evidences = append(n.Evidences, rec)
		s.EvidenceIDs[e.EventID] = rec
		n.LastEvent = e.Timestamp
	case EvTaskCompleted:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("task_completed targets missing node #%d", e.NodeID)
		}
		if n.Kind != KindTask {
			return fmt.Errorf("task_completed targets non-task #%d", e.NodeID)
		}
		if n.Terminal() {
			return fmt.Errorf("task_completed on terminal #%d (already %s)", e.NodeID, n.Status())
		}
		if n.Claim == nil {
			return fmt.Errorf("task_completed on unclaimed #%d", e.NodeID)
		}
		if n.Hold != nil {
			return fmt.Errorf("task_completed on held task #%d", e.NodeID)
		}
		if n.Claim.Actor != e.Actor {
			return fmt.Errorf("task_completed actor %s does not own #%d", e.Actor, e.NodeID)
		}
		if n.Claim.AttemptID != "" && e.AttemptID == "" {
			return fmt.Errorf("task_completed on #%d missing attempt_id", e.NodeID)
		}
		if child := s.OpenTaskChild(n); child != nil {
			return fmt.Errorf("task_completed #%d while child #%d still open", e.NodeID, child.ID)
		}
		if failed := s.DependencyFailedIDs(n); len(failed) > 0 {
			return fmt.Errorf("task_completed #%d with canceled prerequisite(s): %v", e.NodeID, failed)
		}
		if waiting := s.WaitingOnIDs(n); len(waiting) > 0 {
			return fmt.Errorf("task_completed #%d with incomplete prerequisite(s): %v", e.NodeID, waiting)
		}
		if err := s.validateCompletionAcceptance(n, e); err != nil {
			return err
		}
		if err := s.applyAttemptEvent(e, false); err != nil {
			return err
		}
		n.Completed = true
		n.CompletedAt = e.Timestamp
		n.CompletedEvidence = e.EvidenceID
		n.Claim = nil
		n.Hold = nil
		n.LastEvent = e.Timestamp
		if e.AttemptID != "" {
			s.closeAttempt(e.AttemptID, e.Type, e.Timestamp)
		}
		s.completedOrder = append(s.completedOrder, n.ID)
	case EvNodeCanceled:
		n, ok := s.Nodes[e.NodeID]
		if !ok {
			return fmt.Errorf("node_canceled targets missing node #%d", e.NodeID)
		}
		if n.Kind == KindGoal {
			return fmt.Errorf("node_canceled targets goal #%d; goals complete by subtree progress", e.NodeID)
		}
		if n.Terminal() {
			return fmt.Errorf("node_canceled on terminal #%d (already %s)", e.NodeID, n.Status())
		}
		if e.Reason == "" {
			return fmt.Errorf("node_canceled #%d requires reason", e.NodeID)
		}
		if n.Kind == KindTask && n.Claim != nil && n.Claim.Actor != e.Actor {
			return fmt.Errorf("node_canceled actor %s does not own #%d", e.Actor, e.NodeID)
		}
		if child := s.OpenTaskChild(n); child != nil {
			return fmt.Errorf("node_canceled #%d while child task #%d still open", e.NodeID, child.ID)
		}
		if n.Kind == KindTask && n.Claim != nil && n.Claim.AttemptID != "" && e.AttemptID == "" {
			return fmt.Errorf("node_canceled on #%d missing attempt_id", e.NodeID)
		}
		if err := s.applyAttemptEvent(e, false); err != nil {
			return err
		}
		n.Canceled = true
		n.CanceledAt = e.Timestamp
		n.CanceledReason = e.Reason
		n.Claim = nil
		n.Hold = nil
		n.LastEvent = e.Timestamp
		if e.AttemptID != "" {
			s.closeAttempt(e.AttemptID, e.Type, e.Timestamp)
		}
		s.canceledOrder = append(s.canceledOrder, n.ID)
	default:
		return fmt.Errorf("unknown event type %q", e.Type)
	}
	s.Revision.EventCount++
	s.Revision.LastEventID = e.EventID
	s.Revision.LastEventAt = e.Timestamp
	return nil
}

func (s *State) moveNode(n *Node, parentID int64) error {
	p, ok := s.Nodes[parentID]
	if !ok {
		return fmt.Errorf("node_revised parent #%d missing", parentID)
	}
	if p.Terminal() {
		return fmt.Errorf("node_revised parent #%d is terminal", parentID)
	}
	if !p.CanParentWork() {
		return fmt.Errorf("node_revised parent #%d must be a goal or task", parentID)
	}
	if n.Kind == KindGoal && p.Kind != KindGoal {
		return fmt.Errorf("goal #%d parent #%d must be a goal", n.ID, parentID)
	}
	if s.isAncestor(n.ID, parentID) {
		return fmt.Errorf("node_revised would create cycle moving #%d under #%d", n.ID, parentID)
	}
	oldParent := s.Nodes[n.ParentID]
	oldParent.Children = removeChild(oldParent.Children, n.ID)
	p.Children = append(p.Children, n.ID)
	n.ParentID = parentID
	return nil
}

func (s *State) isAncestor(ancestorID, nodeID int64) bool {
	for cur := nodeID; cur != 0; {
		n, ok := s.Nodes[cur]
		if !ok {
			return false
		}
		if n.ID == ancestorID {
			return true
		}
		cur = n.ParentID
	}
	return false
}

func removeChild(children []int64, id int64) []int64 {
	for i, child := range children {
		if child == id {
			return append(children[:i], children[i+1:]...)
		}
	}
	return children
}

func (s *State) applyAttemptEvent(e *Event, allowSystemActor bool) error {
	if e.AttemptID == "" {
		return nil
	}
	attempt, ok := s.Attempts[e.AttemptID]
	if !ok {
		return fmt.Errorf("%s references unknown attempt_id %s", e.Type, e.AttemptID)
	}
	if attempt.NodeID != e.NodeID {
		return fmt.Errorf("%s attempt_id %s belongs to #%d", e.Type, e.AttemptID, attempt.NodeID)
	}
	if attempt.Actor != e.Actor && !(allowSystemActor && e.Actor == "system") {
		return fmt.Errorf("%s attempt_id %s belongs to actor %s", e.Type, e.AttemptID, attempt.Actor)
	}
	if !attempt.ClosedAt.IsZero() {
		return fmt.Errorf("%s references closed attempt_id %s", e.Type, e.AttemptID)
	}
	attempt.LastEventAt = e.Timestamp
	return nil
}

func (s *State) closeAttempt(attemptID string, reason string, at time.Time) {
	attempt := s.Attempts[attemptID]
	if attempt == nil {
		return
	}
	attempt.ClosedAt = at
	attempt.CloseReason = reason
	attempt.LastEventAt = at
}

func scriptRunSummary(e *Event) string {
	if e.CheckName == "" {
		return fmt.Sprintf("%s exit=%d %s", e.Trigger, e.ExitCode, e.Cmd)
	}
	return fmt.Sprintf("%s check=%s exit=%d %s", e.Trigger, e.CheckName, e.ExitCode, e.Cmd)
}

func (s *State) validateCompletionAcceptance(n *Node, e *Event) error {
	if n.Acceptance == nil {
		return fmt.Errorf("task_completed #%d has no acceptance", n.ID)
	}
	if e.EvidenceID != "" {
		rec, ok := s.EvidenceIDs[e.EvidenceID]
		if !ok {
			return fmt.Errorf("task_completed #%d references missing evidence %s", n.ID, e.EvidenceID)
		}
		if rec.NodeID != n.ID {
			return fmt.Errorf("task_completed #%d evidence %s belongs to #%d", n.ID, e.EvidenceID, rec.NodeID)
		}
		if e.AttemptID != "" && rec.AttemptID != "" && rec.AttemptID != e.AttemptID {
			return fmt.Errorf("task_completed #%d evidence %s belongs to attempt %s", n.ID, e.EvidenceID, rec.AttemptID)
		}
	}
	switch n.Acceptance.Kind {
	case AcceptanceVerify:
		for _, check := range n.Acceptance.VerifyChecks() {
			matched := false
			for i := len(n.Runs) - 1; i >= 0; i-- {
				run := n.Runs[i]
				if run.Trigger != TriggerAcceptance || run.Cmd != check.Cmd || run.ExitCode != 0 {
					continue
				}
				if normalizedCheckName(run.CheckName) != check.Name {
					continue
				}
				if e.AttemptID != "" && run.AttemptID != e.AttemptID {
					continue
				}
				matched = true
				break
			}
			if !matched {
				return fmt.Errorf("task_completed #%d without successful acceptance check %q", n.ID, check.Name)
			}
		}
	case AcceptanceReview:
		if e.EvidenceID == "" {
			return fmt.Errorf("task_completed #%d review acceptance requires evidence_id", n.ID)
		}
	default:
		return fmt.Errorf("task_completed #%d has unknown acceptance kind %q", n.ID, n.Acceptance.Kind)
	}
	return nil
}

func (s *State) validateAcceptance(nodeID int64, g *Acceptance) error {
	if g == nil {
		return fmt.Errorf("task #%d created without acceptance", nodeID)
	}
	switch g.Kind {
	case AcceptanceVerify:
		return validateVerifyChecks(nodeID, g.VerifyChecks())
	case AcceptanceReview:
		if g.Who == "" {
			return fmt.Errorf("task #%d review acceptance missing reviewer", nodeID)
		}
	default:
		return fmt.Errorf("task #%d has unknown acceptance kind %q", nodeID, g.Kind)
	}
	return nil
}

func normalizedCheckName(name string) string {
	if name == "" {
		return DefaultVerifyCheckName
	}
	return name
}

func (s *State) validateAfter(nodeID int64, after []int64) error {
	seen := map[int64]bool{}
	for _, refID := range after {
		if refID == 0 {
			return fmt.Errorf("task #%d after contains zero id", nodeID)
		}
		if refID == nodeID {
			return fmt.Errorf("task #%d cannot depend on itself", nodeID)
		}
		if seen[refID] {
			return fmt.Errorf("task #%d repeats prerequisite #%d", nodeID, refID)
		}
		seen[refID] = true
		ref, ok := s.Nodes[refID]
		if !ok {
			return fmt.Errorf("task #%d after references missing node #%d", nodeID, refID)
		}
		if !ref.CanParentWork() {
			return fmt.Errorf("task #%d after references %s #%d", nodeID, ref.Kind, refID)
		}
		if s.isAncestor(refID, nodeID) {
			return fmt.Errorf("task #%d cannot depend on ancestor #%d", nodeID, refID)
		}
		if s.hasPrereqPath(refID, nodeID) {
			return fmt.Errorf("task #%d after #%d would create a prerequisite cycle", nodeID, refID)
		}
	}
	return nil
}

func (s *State) hasPrereqPath(fromID, targetID int64) bool {
	seen := map[int64]bool{}
	var walk func(int64) bool
	walk = func(cur int64) bool {
		if cur == targetID {
			return true
		}
		if seen[cur] {
			return false
		}
		seen[cur] = true
		n, ok := s.Nodes[cur]
		if !ok {
			return false
		}
		for _, next := range n.After {
			if walk(next) {
				return true
			}
		}
		return false
	}
	return walk(fromID)
}
