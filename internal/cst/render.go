package cst

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// BriefView is the bounded Agent-facing projection of state.
type BriefView struct {
	Revision             Revision         `json:"revision"`
	Mode                 string           `json:"mode,omitempty"`
	Root                 *NodeBrief       `json:"root,omitempty"`
	Scope                *NodeBrief       `json:"scope,omitempty"`
	Summary              *Progress        `json:"summary,omitempty"`
	Subtrees             []*SubtreeBrief  `json:"subtrees,omitempty"`
	SubtreesMeta         *CollectionMeta  `json:"subtrees_meta,omitempty"`
	CompletedSubtrees    *CollectionMeta  `json:"completed_subtrees_meta,omitempty"`
	InheritedRules       []*RuleBrief     `json:"inherited_rules,omitempty"`
	Ready                []*TaskBrief     `json:"ready,omitempty"`
	ReadyMeta            *CollectionMeta  `json:"ready_meta,omitempty"`
	ReviewReady          []*TaskBrief     `json:"review_ready,omitempty"`
	ReviewReadyMeta      *CollectionMeta  `json:"review_ready_meta,omitempty"`
	Waiting              []*TaskBrief     `json:"waiting_on,omitempty"`
	WaitingMeta          *CollectionMeta  `json:"waiting_on_meta,omitempty"`
	DependencyFailed     []*TaskBrief     `json:"dependency_failed,omitempty"`
	DependencyFailedMeta *CollectionMeta  `json:"dependency_failed_meta,omitempty"`
	Held                 []*HeldBrief     `json:"held,omitempty"`
	HeldMeta             *CollectionMeta  `json:"held_meta,omitempty"`
	Claims               []*ClaimBrief    `json:"claims,omitempty"`
	ClaimsMeta           *CollectionMeta  `json:"claims_meta,omitempty"`
	RecentFailures       []ScriptRunBrief `json:"recent_failures,omitempty"`
	RecentRuns           []ScriptRunBrief `json:"recent_runs,omitempty"`
	RecentDone           []int64          `json:"recent_done,omitempty"`
	RecentCanceled       []int64          `json:"recent_canceled,omitempty"`
	Actor                string           `json:"actor"`
	GeneratedAt          time.Time        `json:"generated_at"`
}

type CollectionMeta struct {
	Total     int  `json:"total"`
	Shown     int  `json:"shown"`
	Truncated bool `json:"truncated"`
}

type NodeBrief struct {
	ID     int64  `json:"id"`
	Intent string `json:"intent"`
	Status string `json:"status"`
}

type RuleBrief struct {
	ID       int64  `json:"id"`
	ParentID int64  `json:"parent_id"`
	Text     string `json:"text"`
}

type TaskBrief struct {
	ID             int64   `json:"id"`
	ParentID       int64   `json:"parent_id"`
	Intent         string  `json:"intent"`
	AcceptanceKind string  `json:"acceptance_kind,omitempty"`
	After          []int64 `json:"after,omitempty"`
	WaitingOn      []int64 `json:"waiting_on,omitempty"`
	BlockedBy      []int64 `json:"blocked_by,omitempty"`
	Inherited      []int64 `json:"inherited_rule_ids,omitempty"`
}

type HeldBrief struct {
	ID       int64  `json:"id"`
	ParentID int64  `json:"parent_id"`
	Intent   string `json:"intent"`
	Kind     string `json:"kind"`
	Reason   string `json:"reason"`
}

type ClaimBrief struct {
	NodeID         int64     `json:"node_id"`
	Actor          string    `json:"actor"`
	AttemptID      string    `json:"attempt_id,omitempty"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}

type SubtreeBrief struct {
	ID       int64    `json:"id"`
	Kind     string   `json:"kind"`
	Intent   string   `json:"intent,omitempty"`
	Progress Progress `json:"progress"`
}

type ScriptRunBrief struct {
	NodeID    int64     `json:"node_id"`
	AttemptID string    `json:"attempt_id,omitempty"`
	Trigger   string    `json:"trigger"`
	CheckName string    `json:"check_name,omitempty"`
	ExitCode  int       `json:"exit_code"`
	Cmd       string    `json:"cmd"`
	At        time.Time `json:"at"`
}

type BriefOptions struct {
	ScopeID int64
	History bool
}

func BuildBrief(s *State, cfg Config, actor string, scopeID int64) (BriefView, error) {
	return BuildBriefWithOptions(s, cfg, actor, BriefOptions{ScopeID: scopeID})
}

func BuildBriefWithOptions(s *State, cfg Config, actor string, opts BriefOptions) (BriefView, error) {
	mode := "frontier"
	if opts.History {
		mode = "history"
	}
	bv := BriefView{Actor: actor, GeneratedAt: time.Now(), Revision: s.Revision, Mode: mode}
	root := s.Root()
	if root != nil {
		bv.Root = &NodeBrief{ID: root.ID, Intent: root.Intent, Status: string(s.NodeStatus(root))}
		scope := root
		if opts.ScopeID != 0 {
			n, ok := s.Nodes[opts.ScopeID]
			if !ok {
				return bv, fmt.Errorf("scope node #%d not found", opts.ScopeID)
			}
			if n.Kind != KindGoal && n.Kind != KindTask {
				return bv, fmt.Errorf("scope node #%d is %s, not a goal/task", opts.ScopeID, n.Kind)
			}
			scope = n
		}
		p := s.SubtreeProgress(scope.ID)
		bv.Summary = &p
		bv.Scope = &NodeBrief{ID: scope.ID, Intent: scope.Intent, Status: string(s.NodeStatus(scope))}
		for _, r := range s.InheritedRules(scope.ID) {
			bv.InheritedRules = append(bv.InheritedRules, &RuleBrief{ID: r.ID, ParentID: r.ParentID, Text: r.RuleText})
			if cfg.BriefMaxRules > 0 && len(bv.InheritedRules) >= cfg.BriefMaxRules {
				break
			}
		}
		children, total := s.ChildWorkNodes(scope.ID, cfg.BriefMaxTasks)
		completedTotal := 0
		if !opts.History {
			children, total, completedTotal = s.FrontierChildWorkNodes(scope.ID, cfg.BriefMaxTasks)
		}
		bv.SubtreesMeta = collectionMeta(total, len(children))
		if completedTotal > 0 {
			bv.CompletedSubtrees = collectionMeta(completedTotal, 0)
		}
		for _, c := range children {
			bv.Subtrees = append(bv.Subtrees, &SubtreeBrief{
				ID:       c.ID,
				Kind:     c.Kind,
				Intent:   c.Intent,
				Progress: s.SubtreeProgress(c.ID),
			})
		}
		ready, readyTotal := s.HeadOpenTasksWithin(scope.ID, cfg.BriefMaxTasks)
		bv.ReadyMeta = collectionMeta(readyTotal, len(ready))
		for _, t := range ready {
			bv.Ready = append(bv.Ready, buildTaskBrief(s, t))
		}
		reviewReady, reviewReadyTotal := s.ReviewReadyTasksWithin(scope.ID, cfg.BriefMaxTasks)
		bv.ReviewReadyMeta = collectionMeta(reviewReadyTotal, len(reviewReady))
		for _, t := range reviewReady {
			bv.ReviewReady = append(bv.ReviewReady, buildTaskBrief(s, t))
		}
		waiting, waitingTotal := s.WaitingTasksWithin(scope.ID, cfg.BriefMaxTasks)
		bv.WaitingMeta = collectionMeta(waitingTotal, len(waiting))
		for _, t := range waiting {
			bv.Waiting = append(bv.Waiting, buildTaskBrief(s, t))
		}
		failed, failedTotal := s.DependencyFailedTasksWithin(scope.ID, cfg.BriefMaxTasks)
		bv.DependencyFailedMeta = collectionMeta(failedTotal, len(failed))
		for _, t := range failed {
			bv.DependencyFailed = append(bv.DependencyFailed, buildTaskBrief(s, t))
		}
		held, heldTotal := s.HeldTasksWithin(scope.ID, cfg.BriefMaxTasks)
		bv.HeldMeta = collectionMeta(heldTotal, len(held))
		for _, h := range held {
			bv.Held = append(bv.Held, &HeldBrief{
				ID:       h.ID,
				ParentID: h.ParentID,
				Intent:   h.Intent,
				Kind:     h.Hold.Kind,
				Reason:   h.Hold.Reason,
			})
		}
		claims, claimsTotal := s.CurrentClaimsWithin(scope.ID, cfg.BriefMaxTasks)
		bv.ClaimsMeta = collectionMeta(claimsTotal, len(claims))
		for _, c := range claims {
			bv.Claims = append(bv.Claims, &ClaimBrief{
				NodeID:         c.ID,
				Actor:          c.Claim.Actor,
				AttemptID:      c.Claim.AttemptID,
				LeaseExpiresAt: c.Claim.LeaseExpiresAt,
			})
		}
	}
	activeRecentOnly := bv.Summary != nil && bv.Summary.OpenTasks > 0 && !opts.History
	scopeID := opts.ScopeID
	if scopeID == 0 && root != nil {
		scopeID = root.ID
	}
	for _, r := range s.RecentFailuresWithin(scopeID, cfg.BriefMaxRecent, activeRecentOnly) {
		bv.RecentFailures = append(bv.RecentFailures, ScriptRunBrief{
			NodeID:    r.NodeID,
			AttemptID: r.AttemptID,
			Trigger:   r.Trigger,
			CheckName: r.CheckName,
			ExitCode:  r.ExitCode,
			Cmd:       truncate(r.Cmd, 120),
			At:        r.At,
		})
	}
	for _, r := range s.RecentRunsWithin(scopeID, cfg.BriefMaxRecent, activeRecentOnly) {
		bv.RecentRuns = append(bv.RecentRuns, ScriptRunBrief{
			NodeID:    r.NodeID,
			AttemptID: r.AttemptID,
			Trigger:   r.Trigger,
			CheckName: r.CheckName,
			ExitCode:  r.ExitCode,
			Cmd:       truncate(r.Cmd, 120),
			At:        r.At,
		})
	}
	if opts.History || (bv.Summary != nil && bv.Summary.OpenTasks == 0) {
		bv.RecentDone = s.RecentCompleted(cfg.BriefMaxRecent)
		bv.RecentCanceled = s.RecentCanceled(cfg.BriefMaxRecent)
	}
	return bv, nil
}

func collectionMeta(total, shown int) *CollectionMeta {
	return &CollectionMeta{Total: total, Shown: shown, Truncated: shown < total}
}

func buildTaskBrief(s *State, t *Node) *TaskBrief {
	tb := &TaskBrief{ID: t.ID, ParentID: t.ParentID, Intent: t.Intent}
	if t.Acceptance != nil {
		tb.AcceptanceKind = t.Acceptance.Kind
	}
	tb.After = append([]int64(nil), t.After...)
	tb.WaitingOn = s.WaitingOnIDs(t)
	tb.BlockedBy = s.DependencyFailedIDs(t)
	for _, r := range s.InheritedRules(t.ID) {
		tb.Inherited = append(tb.Inherited, r.ID)
	}
	return tb
}

func RenderBriefText(w io.Writer, bv BriefView) {
	if bv.Root == nil {
		fmt.Fprintln(w, "(empty store — `cst add --intent ...` to create root goal)")
		return
	}
	fmt.Fprintf(w, "actor: %s\n", bv.Actor)
	fmt.Fprintf(w, "revision: events=%d last=%s\n", bv.Revision.EventCount, bv.Revision.LastEventID)
	fmt.Fprintf(w, "root #%d [%s] %s\n", bv.Root.ID, bv.Root.Status, bv.Root.Intent)
	if bv.Scope != nil && bv.Scope.ID != bv.Root.ID {
		fmt.Fprintf(w, "scope #%d [%s] %s\n", bv.Scope.ID, bv.Scope.Status, bv.Scope.Intent)
	}
	if bv.Summary != nil {
		fmt.Fprintf(w, "summary: total=%d open=%d ready=%d claimed=%d held=%d completed=%d canceled=%d done=%d%%\n",
			bv.Summary.TotalTasks, bv.Summary.OpenTasks, bv.Summary.ReadyTasks,
			bv.Summary.ClaimedTasks, bv.Summary.HeldTasks, bv.Summary.CompletedTasks,
			bv.Summary.CanceledTasks, bv.Summary.PercentDone)
	}
	if len(bv.Subtrees) > 0 {
		label := "active subtrees"
		if bv.Mode == "history" {
			label = "subtrees"
		}
		fmt.Fprintf(w, "%s:%s\n", label, metaSuffix(bv.SubtreesMeta))
		for _, st := range bv.Subtrees {
			fmt.Fprintf(w, "  #%d %s done=%d%% open=%d ready=%d held=%d  %s\n",
				st.ID, st.Kind, st.Progress.PercentDone, st.Progress.OpenTasks,
				st.Progress.ReadyTasks, st.Progress.HeldTasks, st.Intent)
		}
	}
	if bv.CompletedSubtrees != nil && bv.CompletedSubtrees.Total > 0 {
		fmt.Fprintf(w, "completed subtrees: %d hidden (use cst brief --history to inspect)\n",
			bv.CompletedSubtrees.Total)
	}
	if len(bv.InheritedRules) > 0 {
		fmt.Fprintln(w, "rules:")
		for _, r := range bv.InheritedRules {
			fmt.Fprintf(w, "  #%d (under #%d) %s\n", r.ID, r.ParentID, r.Text)
		}
	}
	if len(bv.Claims) > 0 {
		fmt.Fprintf(w, "claims:%s\n", metaSuffix(bv.ClaimsMeta))
		for _, c := range bv.Claims {
			attempt := ""
			if c.AttemptID != "" {
				attempt = " attempt=" + c.AttemptID
			}
			fmt.Fprintf(w, "  #%d by %s%s (lease until %s)\n", c.NodeID, c.Actor, attempt, c.LeaseExpiresAt.Format(time.RFC3339))
		}
	}
	if len(bv.Held) > 0 {
		fmt.Fprintf(w, "held:%s\n", metaSuffix(bv.HeldMeta))
		for _, h := range bv.Held {
			fmt.Fprintf(w, "  #%d under #%d (%s) %s — %s\n", h.ID, h.ParentID, h.Kind, h.Intent, h.Reason)
		}
	}
	if len(bv.Waiting) > 0 {
		fmt.Fprintf(w, "waiting on:%s\n", metaSuffix(bv.WaitingMeta))
		for _, t := range bv.Waiting {
			renderTaskBriefLine(w, t)
		}
	}
	if len(bv.DependencyFailed) > 0 {
		fmt.Fprintf(w, "dependency failed:%s\n", metaSuffix(bv.DependencyFailedMeta))
		for _, t := range bv.DependencyFailed {
			renderTaskBriefLine(w, t)
		}
	}
	if len(bv.Ready) > 0 {
		fmt.Fprintf(w, "ready:%s\n", metaSuffix(bv.ReadyMeta))
		for _, t := range bv.Ready {
			renderTaskBriefLine(w, t)
		}
	} else {
		fmt.Fprintln(w, "ready: (none)")
	}
	if len(bv.ReviewReady) > 0 {
		fmt.Fprintf(w, "review ready:%s\n", metaSuffix(bv.ReviewReadyMeta))
		for _, t := range bv.ReviewReady {
			renderTaskBriefLine(w, t)
		}
	}
	if len(bv.RecentFailures) > 0 {
		label := "active failures"
		if bv.Mode == "history" || (bv.Summary != nil && bv.Summary.OpenTasks == 0) {
			label = "recent failures"
		}
		fmt.Fprintf(w, "%s:\n", label)
		for _, r := range bv.RecentFailures {
			fmt.Fprintf(w, "  #%d %s exit=%d%s  %s\n", r.NodeID, r.Trigger, r.ExitCode, checkSuffix(r.CheckName), r.Cmd)
		}
	}
	if len(bv.RecentRuns) > 0 {
		label := "active runs"
		if bv.Mode == "history" || (bv.Summary != nil && bv.Summary.OpenTasks == 0) {
			label = "recent runs"
		}
		fmt.Fprintf(w, "%s:\n", label)
		for _, r := range bv.RecentRuns {
			fmt.Fprintf(w, "  %s exit=%d%s  %s\n", r.Trigger, r.ExitCode, checkSuffix(r.CheckName), r.Cmd)
		}
	}
	if len(bv.RecentDone) > 0 {
		fmt.Fprintf(w, "recent done: %s\n", joinIDs(bv.RecentDone))
	}
	if len(bv.RecentCanceled) > 0 {
		fmt.Fprintf(w, "recent canceled: %s\n", joinIDs(bv.RecentCanceled))
	}
}

func metaSuffix(meta *CollectionMeta) string {
	if meta == nil {
		return ""
	}
	if meta.Truncated {
		return fmt.Sprintf(" showing %d/%d", meta.Shown, meta.Total)
	}
	return fmt.Sprintf(" %d/%d", meta.Shown, meta.Total)
}

func checkSuffix(name string) string {
	if name == "" {
		return ""
	}
	return " check=" + name
}

func attemptSuffix(id string) string {
	if id == "" {
		return ""
	}
	return " attempt=" + id
}

func renderTaskBriefLine(w io.Writer, t *TaskBrief) {
	parts := []string{}
	if t.AcceptanceKind != "" {
		parts = append(parts, t.AcceptanceKind)
	}
	if len(t.WaitingOn) > 0 {
		parts = append(parts, "after="+joinIDsBare(t.WaitingOn))
	}
	if len(t.BlockedBy) > 0 {
		parts = append(parts, "blocked_by="+joinIDsBare(t.BlockedBy))
	}
	label := ""
	if len(parts) > 0 {
		label = " (" + strings.Join(parts, " ") + ")"
	}
	fmt.Fprintf(w, "  #%d under #%d%s %s\n", t.ID, t.ParentID, label, t.Intent)
}

// ShowView is the bounded record for a single node. Full event history is read
// through `cst events --for <id>`.
type ShowView struct {
	Node               *NodeDetail       `json:"node"`
	Status             string            `json:"status"`
	Progress           *Progress         `json:"progress,omitempty"`
	Lineage            []int64           `json:"lineage"`
	InheritedRules     []*RuleBrief      `json:"inherited_rules,omitempty"`
	Children           []*ChildBrief     `json:"children,omitempty"`
	ChildrenMeta       *CollectionMeta   `json:"children_meta,omitempty"`
	RecentRuns         []ScriptRunRecord `json:"recent_runs,omitempty"`
	RecentRunsMeta     *CollectionMeta   `json:"recent_runs_meta,omitempty"`
	RecentEvidence     []EvidenceRecord  `json:"recent_evidence,omitempty"`
	RecentEvidenceMeta *CollectionMeta   `json:"recent_evidence_meta,omitempty"`
}

type NodeDetail struct {
	ID                int64       `json:"id"`
	ParentID          int64       `json:"parent_id,omitempty"`
	Kind              string      `json:"kind"`
	Intent            string      `json:"intent,omitempty"`
	RuleText          string      `json:"rule_text,omitempty"`
	Acceptance        *Acceptance `json:"acceptance,omitempty"`
	After             []int64     `json:"after,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	CreatedBy         string      `json:"created_by"`
	CompletedAt       time.Time   `json:"completed_at,omitempty"`
	CompletedEvidence string      `json:"completed_evidence_id,omitempty"`
	CanceledAt        time.Time   `json:"canceled_at,omitempty"`
	CanceledReason    string      `json:"canceled_reason,omitempty"`
	Claim             *Claim      `json:"claim,omitempty"`
	Hold              *Hold       `json:"hold,omitempty"`
	LastEvent         time.Time   `json:"last_event,omitempty"`
}

type ChildBrief struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Intent   string `json:"intent,omitempty"`
	RuleText string `json:"rule_text,omitempty"`
	Status   string `json:"status"`
}

func BuildShow(s *State, id int64, cfg Config) (ShowView, error) {
	n, ok := s.Nodes[id]
	if !ok {
		return ShowView{}, fmt.Errorf("node #%d not found", id)
	}
	v := ShowView{
		Node:               buildNodeDetail(n),
		Status:             string(s.NodeStatus(n)),
		Lineage:            s.ancestorChain(id),
		ChildrenMeta:       collectionMeta(len(n.Children), 0),
		RecentRunsMeta:     collectionMeta(len(n.Runs), 0),
		RecentEvidenceMeta: collectionMeta(len(n.Evidences), 0),
	}
	if n.Kind == KindGoal || n.Kind == KindTask {
		p := s.SubtreeProgress(id)
		v.Progress = &p
	}
	children := n.Children
	if cfg.BriefMaxTasks > 0 && len(children) > cfg.BriefMaxTasks {
		children = children[:cfg.BriefMaxTasks]
	}
	v.ChildrenMeta = collectionMeta(len(n.Children), len(children))
	for _, cid := range children {
		c := s.Nodes[cid]
		v.Children = append(v.Children, &ChildBrief{
			ID:       c.ID,
			Kind:     c.Kind,
			Intent:   c.Intent,
			RuleText: c.RuleText,
			Status:   string(s.NodeStatus(c)),
		})
	}
	v.RecentRuns = recentRunsForNode(n, cfg.BriefMaxRecent)
	v.RecentRunsMeta = collectionMeta(len(n.Runs), len(v.RecentRuns))
	v.RecentEvidence = recentEvidenceForNode(n, cfg.BriefMaxRecent)
	v.RecentEvidenceMeta = collectionMeta(len(n.Evidences), len(v.RecentEvidence))
	for _, r := range s.InheritedRules(id) {
		v.InheritedRules = append(v.InheritedRules, &RuleBrief{ID: r.ID, ParentID: r.ParentID, Text: r.RuleText})
	}
	return v, nil
}

func buildNodeDetail(n *Node) *NodeDetail {
	d := &NodeDetail{
		ID:         n.ID,
		ParentID:   n.ParentID,
		Kind:       n.Kind,
		Intent:     n.Intent,
		RuleText:   n.RuleText,
		Acceptance: n.Acceptance,
		After:      append([]int64(nil), n.After...),
		CreatedAt:  n.CreatedAt,
		CreatedBy:  n.CreatedBy,
		LastEvent:  n.LastEvent,
	}
	if !n.Terminal() {
		d.Claim = n.Claim
		d.Hold = n.Hold
	}
	if n.Completed {
		d.CompletedAt = n.CompletedAt
		d.CompletedEvidence = n.CompletedEvidence
	}
	if n.Canceled {
		d.CanceledAt = n.CanceledAt
		d.CanceledReason = n.CanceledReason
	}
	return d
}

func recentRunsForNode(n *Node, limit int) []ScriptRunRecord {
	out := append([]ScriptRunRecord(nil), n.Runs...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func recentEvidenceForNode(n *Node, limit int) []EvidenceRecord {
	out := append([]EvidenceRecord(nil), n.Evidences...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func RenderShowText(w io.Writer, v ShowView) {
	n := v.Node
	fmt.Fprintf(w, "#%d [%s] kind=%s parent=%d created=%s by %s\n",
		n.ID, v.Status, n.Kind, n.ParentID, n.CreatedAt.Format(time.RFC3339), n.CreatedBy)
	if n.Intent != "" {
		fmt.Fprintf(w, "intent: %s\n", n.Intent)
	}
	if n.RuleText != "" {
		fmt.Fprintf(w, "rule: %s\n", n.RuleText)
	}
	if n.Acceptance != nil {
		switch n.Acceptance.Kind {
		case AcceptanceVerify:
			fmt.Fprintln(w, "acceptance: verify")
			for _, check := range n.Acceptance.VerifyChecks() {
				fmt.Fprintf(w, "  check %s cmd=%q\n", check.Name, check.Cmd)
			}
		case AcceptanceReview:
			fmt.Fprintf(w, "acceptance: review who=%s\n", n.Acceptance.Who)
		}
	}
	if len(n.After) > 0 {
		fmt.Fprintf(w, "after: %s\n", joinIDs(n.After))
	}
	if n.Claim != nil {
		attempt := ""
		if n.Claim.AttemptID != "" {
			attempt = " attempt=" + n.Claim.AttemptID
		}
		fmt.Fprintf(w, "claim: actor=%s%s lease_until=%s\n", n.Claim.Actor, attempt, n.Claim.LeaseExpiresAt.Format(time.RFC3339))
	}
	if !n.CompletedAt.IsZero() {
		fmt.Fprintf(w, "completed: at=%s evidence_id=%q\n", n.CompletedAt.Format(time.RFC3339), n.CompletedEvidence)
	}
	if !n.CanceledAt.IsZero() {
		fmt.Fprintf(w, "canceled: at=%s reason=%q\n", n.CanceledAt.Format(time.RFC3339), n.CanceledReason)
	}
	if v.Progress != nil {
		fmt.Fprintf(w, "progress: total=%d open=%d ready=%d claimed=%d held=%d completed=%d canceled=%d done=%d%%\n",
			v.Progress.TotalTasks, v.Progress.OpenTasks, v.Progress.ReadyTasks,
			v.Progress.ClaimedTasks, v.Progress.HeldTasks, v.Progress.CompletedTasks,
			v.Progress.CanceledTasks, v.Progress.PercentDone)
	}
	if len(v.Lineage) > 1 {
		fmt.Fprintf(w, "lineage: %s\n", joinIDs(v.Lineage))
	}
	if len(v.InheritedRules) > 0 {
		fmt.Fprintln(w, "inherited rules:")
		for _, r := range v.InheritedRules {
			fmt.Fprintf(w, "  #%d (under #%d) %s\n", r.ID, r.ParentID, r.Text)
		}
	}
	if len(v.Children) > 0 {
		fmt.Fprintf(w, "children:%s\n", metaSuffix(v.ChildrenMeta))
		for _, c := range v.Children {
			label := c.Intent
			if label == "" {
				label = c.RuleText
			}
			fmt.Fprintf(w, "  #%d [%s] %s %s\n", c.ID, c.Status, c.Kind, label)
		}
	}
	if len(v.RecentRuns) > 0 {
		fmt.Fprintf(w, "recent runs:%s\n", metaSuffix(v.RecentRunsMeta))
		for _, r := range v.RecentRuns {
			fmt.Fprintf(w, "  [%s] exit=%d%s dur=%dms cmd=%s\n",
				r.Trigger, r.ExitCode, checkSuffix(r.CheckName), r.DurationMs, truncate(r.Cmd, 120))
		}
	}
	if len(v.RecentEvidence) > 0 {
		fmt.Fprintf(w, "recent evidence:%s\n", metaSuffix(v.RecentEvidenceMeta))
		for _, e := range v.RecentEvidence {
			fmt.Fprintf(w, "  %s kind=%s %s\n", e.EventID, e.Kind, e.Summary)
		}
	}
}

func RenderEventsText(w io.Writer, events []*Event) {
	for _, e := range events {
		switch e.Type {
		case EvNodeCreated:
			label := e.Kind
			if label == KindRule {
				fmt.Fprintf(w, "%s  #%d  rule under #%d  %q\n",
					e.Timestamp.Format(time.RFC3339), e.NodeID, e.ParentID, e.RuleText)
			} else {
				acceptance := ""
				if e.Acceptance != nil {
					acceptance = " acceptance=" + e.Acceptance.Kind
				}
				after := ""
				if len(e.After) > 0 {
					after = " after=" + joinIDsBare(e.After)
				}
				fmt.Fprintf(w, "%s  #%d  task under #%d%s%s  %q\n",
					e.Timestamp.Format(time.RFC3339), e.NodeID, e.ParentID, acceptance, after, e.Intent)
			}
		case EvNodeRevised:
			parts := []string{}
			if e.ParentID != 0 {
				parts = append(parts, fmt.Sprintf("parent=#%d", e.ParentID))
			}
			if e.Intent != "" {
				parts = append(parts, fmt.Sprintf("intent=%q", e.Intent))
			}
			if e.RuleText != "" {
				parts = append(parts, fmt.Sprintf("rule=%q", e.RuleText))
			}
			if e.Acceptance != nil {
				parts = append(parts, fmt.Sprintf("acceptance=%s", e.Acceptance.Kind))
			}
			if e.AfterSet {
				parts = append(parts, fmt.Sprintf("after=%s", joinIDsBare(e.After)))
			}
			if e.Reason != "" {
				parts = append(parts, fmt.Sprintf("reason=%q", e.Reason))
			}
			fmt.Fprintf(w, "%s  #%d  revised %s\n",
				e.Timestamp.Format(time.RFC3339), e.NodeID, strings.Join(parts, " "))
		case EvClaimTaken:
			fmt.Fprintf(w, "%s  #%d  claim taken by %s%s\n", e.Timestamp.Format(time.RFC3339), e.NodeID, e.Actor, attemptSuffix(e.AttemptID))
		case EvClaimRenewed:
			fmt.Fprintf(w, "%s  #%d  claim renewed by %s%s\n", e.Timestamp.Format(time.RFC3339), e.NodeID, e.Actor, attemptSuffix(e.AttemptID))
		case EvClaimReleased:
			fmt.Fprintf(w, "%s  #%d  claim released by %s%s\n", e.Timestamp.Format(time.RFC3339), e.NodeID, e.Actor, attemptSuffix(e.AttemptID))
		case EvClaimAbandoned:
			fmt.Fprintf(w, "%s  #%d  claim abandoned%s (%s)\n", e.Timestamp.Format(time.RFC3339), e.NodeID, attemptSuffix(e.AttemptID), e.Reason)
		case EvScriptRun:
			fmt.Fprintf(w, "%s  #%d  script_run trigger=%s exit=%d%s%s %s\n",
				e.Timestamp.Format(time.RFC3339), e.NodeID, e.Trigger, e.ExitCode, checkSuffix(e.CheckName), attemptSuffix(e.AttemptID), truncate(e.Cmd, 80))
		case EvEvidence:
			fmt.Fprintf(w, "%s  #%d  evidence kind=%s%s %q\n",
				e.Timestamp.Format(time.RFC3339), e.NodeID, e.EvidenceKind, attemptSuffix(e.AttemptID), e.EvidenceSummary)
		case EvTaskCompleted:
			fmt.Fprintf(w, "%s  #%d  completed%s evidence_id=%q\n", e.Timestamp.Format(time.RFC3339), e.NodeID, attemptSuffix(e.AttemptID), e.EvidenceID)
		case EvNodeHeld:
			fmt.Fprintf(w, "%s  #%d  held kind=%s reason=%q\n", e.Timestamp.Format(time.RFC3339), e.NodeID, e.HoldKind, e.Reason)
		case EvNodeUnheld:
			fmt.Fprintf(w, "%s  #%d  hold cleared\n", e.Timestamp.Format(time.RFC3339), e.NodeID)
		case EvNodeCanceled:
			fmt.Fprintf(w, "%s  #%d  canceled reason=%q\n", e.Timestamp.Format(time.RFC3339), e.NodeID, e.Reason)
		default:
			fmt.Fprintf(w, "%s  #%d  %s\n", e.Timestamp.Format(time.RFC3339), e.NodeID, e.Type)
		}
	}
}

func WriteJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("#%d", id)
	}
	return strings.Join(parts, " ")
}

func joinIDsBare(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",")
}
