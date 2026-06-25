package cst

import (
	"fmt"
	"io"
	"strings"
)

type DeveloperBriefing struct {
	NodeID             int64               `json:"node_id"`
	Lineage            []int64             `json:"lineage,omitempty"`
	ContextPath        []ContextFrame      `json:"context_path,omitempty"`
	ContextFold        *NodeContext        `json:"context_fold,omitempty"`
	Boundary           *NodeBoundary       `json:"boundary,omitempty"`
	Acceptance         *Acceptance         `json:"acceptance,omitempty"`
	Upstream           []int64             `json:"upstream,omitempty"`
	Downstream         []int64             `json:"downstream,omitempty"`
	ObligationClaims   []string            `json:"obligation_claims,omitempty"`
	ObligationCoverage *ObligationCoverage `json:"obligation_coverage,omitempty"`
	PartitionWarnings  []string            `json:"partition_warnings,omitempty"`
	Warnings           []string            `json:"warnings,omitempty"`
}

type ContextFrame struct {
	NodeID  int64        `json:"node_id"`
	Intent  string       `json:"intent,omitempty"`
	Context *NodeContext `json:"context"`
}

func BuildDeveloperBriefing(s *State, nodeID int64) *DeveloperBriefing {
	n := s.Nodes[nodeID]
	if n == nil || !n.CanParentWork() {
		return nil
	}
	out := &DeveloperBriefing{
		NodeID:           nodeID,
		Lineage:          s.ancestorChain(nodeID),
		Boundary:         cloneNodeBoundary(n.Boundary),
		Acceptance:       cloneAcceptance(n.Acceptance),
		Upstream:         append([]int64(nil), n.After...),
		Downstream:       s.DirectDownstreamIDs(nodeID),
		ObligationClaims: append([]string(nil), n.ObligationClaims...),
	}
	var contexts []*NodeContext
	for _, id := range out.Lineage {
		anc := s.Nodes[id]
		if anc == nil || anc.Context == nil {
			continue
		}
		ctx := cloneNodeContext(anc.Context)
		contexts = append(contexts, ctx)
		out.ContextPath = append(out.ContextPath, ContextFrame{
			NodeID:  id,
			Intent:  anc.Intent,
			Context: ctx,
		})
	}
	out.ContextFold = foldNodeContexts(contexts)
	coverage := s.ObligationCoverage(nodeID)
	if len(coverage.Required) > 0 || len(coverage.Claimed) > 0 {
		out.ObligationCoverage = &coverage
		if len(coverage.Missing) > 0 {
			out.Warnings = append(out.Warnings, "missing success obligations: "+strings.Join(coverage.Missing, ","))
		}
	}
	out.PartitionWarnings = s.BoundaryPartitionWarnings(nodeID)
	out.Warnings = append(out.Warnings, out.PartitionWarnings...)
	return out
}

func (s *State) DirectDownstreamIDs(nodeID int64) []int64 {
	var out []int64
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n == nil || n.Canceled || !n.CanParentWork() {
			continue
		}
		if int64ListContains(n.After, nodeID) {
			out = append(out, n.ID)
		}
	}
	return out
}

func (s *State) BoundaryPartitionWarnings(nodeID int64) []string {
	n := s.Nodes[nodeID]
	if n == nil || n.Boundary == nil || len(n.Boundary.Owned) == 0 || n.ParentID == 0 {
		return nil
	}
	parent := s.Nodes[n.ParentID]
	if parent == nil || parent.Boundary == nil || len(parent.Boundary.Owned) == 0 {
		return []string{fmt.Sprintf("parent #%d has no owned boundary; node boundary is checked only against siblings and completion diff", n.ParentID)}
	}
	return nil
}

func foldNodeContexts(contexts []*NodeContext) *NodeContext {
	if len(contexts) == 0 {
		return nil
	}
	var invariants []string
	var nonGoals []string
	var obligations []string
	for _, ctx := range contexts {
		if ctx == nil {
			continue
		}
		if ctx.Invariant != "" {
			invariants = append(invariants, ctx.Invariant)
		}
		nonGoals = append(nonGoals, ctx.NonGoals...)
		obligations = append(obligations, ctx.SuccessObligations...)
	}
	fold, _ := normalizeNodeContext(&NodeContext{
		Invariant:          strings.Join(invariants, "\n"),
		NonGoals:           nonGoals,
		SuccessObligations: obligations,
	})
	return fold
}

func cloneAcceptance(a *Acceptance) *Acceptance {
	if a == nil {
		return nil
	}
	return &Acceptance{
		Kind:   a.Kind,
		Checks: cloneVerifyChecks(a.Checks),
		Cmd:    a.Cmd,
		Who:    a.Who,
	}
}

func RenderDeveloperBriefingText(w io.Writer, b *DeveloperBriefing) {
	if b == nil {
		return
	}
	fmt.Fprintf(w, "briefing: node=#%d lineage=%s\n", b.NodeID, joinIDs(b.Lineage))
	if b.ContextFold != nil {
		fmt.Fprintln(w, "context fold:")
		if b.ContextFold.Invariant != "" {
			fmt.Fprintf(w, "  invariant: %s\n", strings.ReplaceAll(b.ContextFold.Invariant, "\n", " | "))
		}
		if len(b.ContextFold.NonGoals) > 0 {
			fmt.Fprintf(w, "  non_goals: %s\n", strings.Join(b.ContextFold.NonGoals, "; "))
		}
		if len(b.ContextFold.SuccessObligations) > 0 {
			fmt.Fprintf(w, "  success_obligations: %s\n", strings.Join(b.ContextFold.SuccessObligations, ","))
		}
	}
	if b.Boundary != nil {
		fmt.Fprintf(w, "node boundary: %s\n", boundarySummary(b.Boundary))
	}
	if len(b.Upstream) > 0 || len(b.Downstream) > 0 {
		fmt.Fprintf(w, "edges: upstream=%s downstream=%s\n", joinIDsOrNone(b.Upstream), joinIDsOrNone(b.Downstream))
	}
	if b.Acceptance != nil {
		fmt.Fprintf(w, "local contract: acceptance=%s", b.Acceptance.Kind)
		if len(b.ObligationClaims) > 0 {
			fmt.Fprintf(w, " obligation_claims=%s", strings.Join(b.ObligationClaims, ","))
		}
		fmt.Fprintln(w)
	}
	if b.ObligationCoverage != nil {
		fmt.Fprintf(w, "success coverage: required=%s claimed=%s missing=%s unmatched=%s\n",
			joinStringsOrNone(b.ObligationCoverage.Required),
			joinStringsOrNone(b.ObligationCoverage.Claimed),
			joinStringsOrNone(b.ObligationCoverage.Missing),
			joinStringsOrNone(b.ObligationCoverage.UnmatchedClaims))
	}
	if len(b.Warnings) > 0 {
		fmt.Fprintf(w, "warnings: %s\n", strings.Join(b.Warnings, "; "))
	}
}

func joinIDsOrNone(ids []int64) string {
	if len(ids) == 0 {
		return "-"
	}
	return joinIDsBare(ids)
}
