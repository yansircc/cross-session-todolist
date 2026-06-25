package cst

import (
	"fmt"
	"sort"
	"time"
)

const (
	uiTaskRowLimit = 8
	uiRecentLimit  = 6
)

// uiView is the bounded human-facing projection consumed by the HTML emitter.
// It derives every task state from State queries; the renderer does not infer
// frontier legality from raw node status.
type uiView struct {
	Project     string
	GeneratedAt time.Time
	EventsPath  string
	TotalEvents int
	LastEvent   time.Time

	Root    *Node
	Scope   *Node
	Summary Progress

	GoalCount             int
	RuleCount             int
	TotalTasks            int
	CompletedSubtreeTotal int

	RecentFailures []ScriptRunRecord
	RecentRuns     []ScriptRunRecord
	RecentDone     []int64
	RecentCanceled []int64
	CurrentClaims  []*Node
	ActivePhases   []phaseView
}

type phaseView struct {
	Node         *Node
	Briefing     *DeveloperBriefing
	Ancestors    []*Node
	Rules        []*Node
	Progress     Progress
	LastActivity time.Time

	ReadyTotal            int
	ReviewReadyTotal      int
	WaitingTotal          int
	DependencyFailedTotal int
	HeldTotal             int
	ClaimTotal            int

	TaskRows      []taskRowView
	TaskRowsTotal int
}

type taskRowView struct {
	Node        *Node
	Briefing    *DeveloperBriefing
	StateClass  string
	StateLabel  string
	StateDetail string
	Acceptance  string
	WaitingOn   []int64
	BlockedBy   []int64
	Commands    []string
	LatestRun   *ScriptRunRecord
	Evidence    *EvidenceRecord
	Closure     *ClosureProjection
}

// uiViewFrom builds a uiView from State, scoped to a subtree rooted at scopeID.
// scopeID == 0 means the whole project.
func uiViewFrom(s *State, scopeID int64, eventsPath, project string, totalEvents int, lastEvent time.Time) uiView {
	now := time.Now()
	v := uiView{
		Project:     project,
		GeneratedAt: now,
		EventsPath:  eventsPath,
		TotalEvents: totalEvents,
		LastEvent:   lastEvent,
		Root:        s.Root(),
	}
	v.Scope = v.Root
	if scopeID != 0 {
		v.Scope = s.Nodes[scopeID]
	}

	if v.Scope != nil {
		v.Summary = s.SubtreeProgress(v.Scope.ID)
		v.CompletedSubtreeTotal = completedChildWorkTotal(s, v.Scope.ID)
	}

	for _, id := range s.Order {
		n := s.Nodes[id]
		if scopeID != 0 && !s.IsWithin(scopeID, id) {
			continue
		}
		switch n.Kind {
		case KindTask:
			v.TotalTasks++
		case KindGoal:
			v.GoalCount++
		case KindRule:
			v.RuleCount++
		}
	}

	activeOnly := v.Summary.OpenTasks > 0
	v.RecentRuns = recentUIRuns(s, scopeID, false, activeOnly, uiRecentLimit)
	v.RecentFailures = recentUIRuns(s, scopeID, true, activeOnly, uiRecentLimit)
	v.RecentDone = recentCompletedWithin(s, scopeID, uiRecentLimit)
	v.RecentCanceled = recentCanceledWithin(s, scopeID, uiRecentLimit)
	v.CurrentClaims, _ = s.CurrentClaimsWithin(scopeID, uiRecentLimit)
	v.ActivePhases = buildActivePhases(s, scopeID)

	return v
}

func buildActivePhases(s *State, scopeID int64) []phaseView {
	phaseIDs := map[int64]struct{}{}
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Kind != KindTask || n.Terminal() {
			continue
		}
		if scopeID != 0 && !s.IsWithin(scopeID, id) {
			continue
		}
		g := innermostGoal(s, n.ParentID)
		if g == nil {
			continue
		}
		if scopeID != 0 && !s.IsWithin(scopeID, g.ID) {
			continue
		}
		phaseIDs[g.ID] = struct{}{}
	}

	phases := make([]phaseView, 0, len(phaseIDs))
	for _, id := range s.Order {
		if _, ok := phaseIDs[id]; !ok {
			continue
		}
		if p := buildPhaseView(s, s.Nodes[id]); p.Node != nil {
			phases = append(phases, p)
		}
	}
	return phases
}

func buildPhaseView(s *State, n *Node) phaseView {
	if n == nil {
		return phaseView{}
	}
	p := phaseView{
		Node:         n,
		Briefing:     BuildDeveloperBriefing(s, n.ID),
		Progress:     s.SubtreeProgress(n.ID),
		Ancestors:    ancestorGoals(s, n.ID),
		Rules:        s.InheritedRules(n.ID),
		LastActivity: subtreeLastActivity(s, n.ID),
	}

	p.ReadyTotal = takeReadyTotalWithin(s, n.ID)
	_, p.ReviewReadyTotal = s.ReviewReadyTasksWithin(n.ID, 0)
	_, p.WaitingTotal = s.WaitingTasksWithin(n.ID, 0)
	_, p.DependencyFailedTotal = s.DependencyFailedTasksWithin(n.ID, 0)
	_, p.HeldTotal = s.HeldTasksWithin(n.ID, 0)
	_, p.ClaimTotal = s.CurrentClaimsWithin(n.ID, 0)

	rows := buildPhaseTaskRows(s, n.ID)
	p.TaskRowsTotal = len(rows)
	if len(rows) > uiTaskRowLimit {
		rows = rows[:uiTaskRowLimit]
	}
	p.TaskRows = rows
	return p
}

func buildPhaseTaskRows(s *State, phaseID int64) []taskRowView {
	var rows []taskRowView
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Kind != KindTask || !s.IsWithin(phaseID, id) {
			continue
		}
		g := innermostGoal(s, n.ParentID)
		if g == nil || g.ID != phaseID {
			continue
		}
		rows = append(rows, buildTaskRow(s, n))
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := taskRowRank(rows[i]), taskRowRank(rows[j])
		if ri != rj {
			return ri < rj
		}
		if rows[i].Node.Completed && rows[j].Node.Completed {
			return rows[i].Node.CompletedAt.After(rows[j].Node.CompletedAt)
		}
		return rows[i].Node.ID < rows[j].Node.ID
	})
	return rows
}

func buildTaskRow(s *State, n *Node) taskRowView {
	row := taskRowView{
		Node:       n,
		Briefing:   BuildDeveloperBriefing(s, n.ID),
		Acceptance: taskAcceptanceSummary(n),
		WaitingOn:  s.WaitingOnIDs(n),
		BlockedBy:  s.DependencyFailedIDs(n),
		Commands:   taskCommands(n),
		Evidence:   latestHumanEvidence(n),
		Closure:    closureProjection(n),
	}
	row.LatestRun = latestRun(n)
	row.StateClass, row.StateLabel, row.StateDetail = taskFrontierState(s, n)
	return row
}

func taskFrontierState(s *State, n *Node) (string, string, string) {
	switch {
	case n.Completed:
		return "done", "completed", "terminal"
	case n.Canceled:
		return "failed", "canceled", firstNonEmpty(n.CanceledReason, "terminal")
	case n.Claim != nil:
		return "claimed", "claimed", fmt.Sprintf("by %s", n.Claim.Actor)
	case n.Hold != nil:
		return "held", "held", fmt.Sprintf("%s: %s", n.Hold.Kind, n.Hold.Reason)
	}
	if blocked := s.DependencyFailedIDs(n); len(blocked) > 0 {
		return "failed", "dependency failed", "blocked_by=" + joinIDsBare(blocked)
	}
	if waiting := s.WaitingOnIDs(n); len(waiting) > 0 {
		return "waiting", "waiting", "after=" + joinIDsBare(waiting)
	}
	if s.IsReadyTask(n.ID) {
		if n.Acceptance != nil && n.Acceptance.Kind == AcceptanceReview {
			return "review", "review ready", "legal review action"
		}
		return "ready", "ready", "legal take action"
	}
	if child := s.OpenTaskChild(n); child != nil {
		return "waiting", "child open", fmt.Sprintf("child #%d", child.ID)
	}
	return "waiting", "not ready", s.ReadyBlockReason(n.ID)
}

func taskRowRank(row taskRowView) int {
	switch row.StateClass {
	case "claimed":
		return 0
	case "failed":
		return 1
	case "held":
		return 2
	case "ready", "review":
		return 3
	case "waiting":
		return 4
	case "done":
		return 5
	}
	return 9
}

func taskAcceptanceSummary(n *Node) string {
	if n.Acceptance == nil {
		return ""
	}
	switch n.Acceptance.Kind {
	case AcceptanceVerify:
		checks := n.Acceptance.VerifyChecks()
		if len(checks) == 0 {
			return "verify"
		}
		parts := make([]string, 0, len(checks))
		for _, check := range checks {
			parts = append(parts, "verify."+normalizedCheckName(check.Name))
		}
		return stringsJoin(parts, ", ")
	case AcceptanceReview:
		who := n.Acceptance.Who
		if who == "" {
			who = "?"
		}
		return "review." + who
	default:
		return n.Acceptance.Kind
	}
}

func taskCommands(n *Node) []string {
	cmds := []string{fmt.Sprintf("cst show %d", n.ID)}
	if n.Claim != nil {
		cmds = append(cmds, fmt.Sprintf("cst worker-status %d --human", n.ID))
	}
	if attemptID := taskAttemptID(n); attemptID != "" {
		cmds = append(cmds, fmt.Sprintf("cst events --attempt %s", attemptID))
	}
	return cmds
}

func latestRun(n *Node) *ScriptRunRecord {
	if len(n.Runs) == 0 {
		return nil
	}
	return &n.Runs[len(n.Runs)-1]
}

func latestHumanEvidence(n *Node) *EvidenceRecord {
	for i := len(n.Evidences) - 1; i >= 0; i-- {
		if isHumanEvidenceKind(n.Evidences[i].Kind) {
			return &n.Evidences[i]
		}
	}
	return nil
}

func ancestorGoals(s *State, nodeID int64) []*Node {
	chain := s.ancestorChain(nodeID)
	out := make([]*Node, 0, len(chain))
	for _, id := range chain {
		if id == nodeID {
			continue
		}
		n := s.Nodes[id]
		if n != nil && n.Kind == KindGoal {
			out = append(out, n)
		}
	}
	return out
}

func subtreeLastActivity(s *State, id int64) time.Time {
	var last time.Time
	for _, n := range s.SubtreeNodes(id) {
		if n.LastEvent.After(last) {
			last = n.LastEvent
		}
	}
	return last
}

func completedChildWorkTotal(s *State, parentID int64) int {
	parent := s.Nodes[parentID]
	if parent == nil {
		return 0
	}
	total := 0
	for _, cid := range parent.Children {
		c := s.Nodes[cid]
		if c == nil || (c.Kind != KindTask && c.Kind != KindGoal) {
			continue
		}
		if s.SubtreeProgress(c.ID).OpenTasks == 0 {
			total++
		}
	}
	return total
}

func recentUIRuns(s *State, scopeID int64, failuresOnly bool, activeOnly bool, limit int) []ScriptRunRecord {
	var runs []ScriptRunRecord
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Kind != KindTask {
			continue
		}
		if scopeID != 0 && !s.IsWithin(scopeID, id) {
			continue
		}
		if activeOnly && n.Terminal() {
			continue
		}
		for _, run := range n.Runs {
			if failuresOnly && run.ExitCode == 0 {
				continue
			}
			runs = append(runs, run)
		}
	}
	sort.SliceStable(runs, func(i, j int) bool {
		return runs[i].At.After(runs[j].At)
	})
	if limit > 0 && len(runs) > limit {
		return runs[:limit]
	}
	return runs
}

func recentCompletedWithin(s *State, scopeID int64, limit int) []int64 {
	return recentTerminalWithin(s, s.completedOrder, scopeID, limit)
}

func recentCanceledWithin(s *State, scopeID int64, limit int) []int64 {
	return recentTerminalWithin(s, s.canceledOrder, scopeID, limit)
}

func recentTerminalWithin(s *State, ids []int64, scopeID int64, limit int) []int64 {
	var out []int64
	for i := len(ids) - 1; i >= 0; i-- {
		id := ids[i]
		if !s.IsWithin(scopeID, id) {
			continue
		}
		out = append(out, id)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func takeReadyTotalWithin(s *State, scopeID int64) int {
	ready, _ := s.HeadOpenTasksWithin(scopeID, 0)
	total := 0
	for _, n := range ready {
		if n.Acceptance != nil && n.Acceptance.Kind == AcceptanceReview {
			continue
		}
		total++
	}
	return total
}

func phaseStatusClass(p phaseView) string {
	switch {
	case p.DependencyFailedTotal > 0:
		return "failed"
	case p.ClaimTotal > 0:
		return "claimed"
	case p.ReadyTotal > 0:
		return "ready"
	case p.ReviewReadyTotal > 0:
		return "review"
	case p.HeldTotal > 0:
		return "held"
	case p.WaitingTotal > 0:
		return "waiting"
	case p.Progress.OpenTasks == 0:
		return "done"
	default:
		return "waiting"
	}
}

func phaseStatusLabel(p phaseView) string {
	switch phaseStatusClass(p) {
	case "failed":
		return "blocked"
	case "claimed":
		return "active"
	case "ready":
		return "ready"
	case "review":
		return "review"
	case "held":
		return "held"
	case "done":
		return "done"
	default:
		return "waiting"
	}
}

func phaseBlocker(p phaseView) (string, string) {
	for _, row := range p.TaskRows {
		switch row.StateClass {
		case "claimed":
			return fmt.Sprintf("Now executing #%d", row.Node.ID), "Completion should unlock dependent frontier work."
		case "failed":
			return fmt.Sprintf("Dependency failed at #%d", row.Node.ID), row.StateDetail
		case "ready", "review":
			return fmt.Sprintf("Ready now #%d", row.Node.ID), "A legal action is available for this phase."
		case "held":
			return fmt.Sprintf("Held at #%d", row.Node.ID), row.StateDetail
		case "waiting":
			return fmt.Sprintf("Waiting at #%d", row.Node.ID), row.StateDetail
		}
	}
	return "No active task rows", "This phase has no visible active task row."
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += sep + p
	}
	return out
}

// innermostGoal walks parents until it finds a goal ancestor. Returns nil if
// no goal is found (e.g. the from node is already a root goal).
func innermostGoal(s *State, fromID int64) *Node {
	for cur := fromID; cur != 0; {
		n, ok := s.Nodes[cur]
		if !ok {
			return nil
		}
		if n.Kind == KindGoal {
			return n
		}
		cur = n.ParentID
	}
	return nil
}
