package cst

import (
	"fmt"
	"sort"
)

type Progress struct {
	TotalTasks     int `json:"total_tasks"`
	CompletedTasks int `json:"completed_tasks"`
	CanceledTasks  int `json:"canceled_tasks"`
	OpenTasks      int `json:"open_tasks"`
	ClaimedTasks   int `json:"claimed_tasks"`
	HeldTasks      int `json:"held_tasks"`
	ReadyTasks     int `json:"ready_tasks"`
	PercentDone    int `json:"percent_done"`
}

// Status reports the high-level state of a node.
func (n *Node) Status() NodeStatus {
	if n.Canceled {
		return StatusCanceled
	}
	if n.Completed {
		return StatusCompleted
	}
	if n.Hold != nil {
		return StatusHeld
	}
	if n.Claim != nil {
		return StatusClaimed
	}
	return StatusOpen
}

// Terminal returns true for completed/canceled nodes.
func (n *Node) Terminal() bool { return n.Completed || n.Canceled }

// IsTask returns true only for executable work.
func (n *Node) IsTask() bool { return n.Kind == KindTask }
func (n *Node) IsGoal() bool { return n.Kind == KindGoal }
func (n *Node) CanParentWork() bool {
	return n.Kind == KindGoal || n.Kind == KindTask
}
func (n *Node) CanHaveEvidence() bool {
	return n.Kind == KindGoal || n.Kind == KindTask
}

// Root returns the store's single root goal, if any.
func (s *State) Root() *Node {
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.ParentID == 0 && n.Kind == KindGoal {
			return n
		}
	}
	return nil
}

// AnyRoot returns the first root goal encountered, terminal or not. Used by Tx
// to enforce the one-root invariant.
func (s *State) AnyRoot() *Node {
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.ParentID == 0 && n.Kind == KindGoal {
			return n
		}
	}
	return nil
}

// InheritedRules walks ancestor chain (including self) collecting active
// rule nodes. Ordered from root to self; each child's rules appear after its
// ancestors.
func (s *State) InheritedRules(nodeID int64) []*Node {
	chain := s.ancestorChain(nodeID)
	var rules []*Node
	for _, ancID := range chain {
		anc := s.Nodes[ancID]
		for _, cid := range anc.Children {
			c := s.Nodes[cid]
			if c.Kind == KindRule && !c.Canceled {
				rules = append(rules, c)
			}
		}
	}
	return rules
}

func (s *State) ancestorChain(nodeID int64) []int64 {
	var chain []int64
	cur := nodeID
	for cur != 0 {
		n, ok := s.Nodes[cur]
		if !ok {
			break
		}
		chain = append([]int64{cur}, chain...)
		cur = n.ParentID
	}
	return chain
}

// CanComplete returns whether a task may transition to completed: all child
// tasks must be terminal. (Rule children don't acceptance completion.)
func (s *State) CanComplete(nodeID int64) (bool, string) {
	n, ok := s.Nodes[nodeID]
	if !ok {
		return false, "node not found"
	}
	if !n.IsTask() {
		return false, "rules cannot be completed"
	}
	if n.Terminal() {
		return false, "node already terminal"
	}
	for _, cid := range n.Children {
		c := s.Nodes[cid]
		if c.Kind == KindTask && !c.Terminal() {
			return false, fmt.Sprintf("child task %d not terminal", c.ID)
		}
	}
	return true, ""
}

func (s *State) OpenTaskChild(n *Node) *Node {
	for _, cid := range n.Children {
		c := s.Nodes[cid]
		if c.Kind == KindTask && !c.Terminal() {
			return c
		}
	}
	return nil
}

func (s *State) NodeStatus(n *Node) NodeStatus {
	if n.Kind != KindGoal {
		return n.Status()
	}
	if n.Canceled {
		return StatusCanceled
	}
	p := s.SubtreeProgress(n.ID)
	if p.OpenTasks == 0 {
		return StatusCompleted
	}
	return StatusOpen
}

func (s *State) SubtreeProgress(id int64) Progress {
	var p Progress
	for _, n := range s.SubtreeNodes(id) {
		if n.Kind != KindTask {
			continue
		}
		p.TotalTasks++
		switch {
		case n.Completed:
			p.CompletedTasks++
		case n.Canceled:
			p.CanceledTasks++
		default:
			p.OpenTasks++
			if n.Claim != nil {
				p.ClaimedTasks++
			}
			if n.Hold != nil {
				p.HeldTasks++
			}
			if s.IsReadyTask(n.ID) {
				p.ReadyTasks++
			}
		}
	}
	terminal := p.CompletedTasks + p.CanceledTasks
	if p.TotalTasks > 0 {
		p.PercentDone = (terminal * 100) / p.TotalTasks
	}
	return p
}

func (s *State) SubtreeNodes(id int64) []*Node {
	var out []*Node
	var walk func(int64)
	walk = func(cur int64) {
		n, ok := s.Nodes[cur]
		if !ok {
			return
		}
		out = append(out, n)
		for _, cid := range n.Children {
			walk(cid)
		}
	}
	walk(id)
	return out
}

// HeadOpenTasks returns open tasks in creation order, optionally filtered to
// leaves (no open task descendants). Used by `take` and `brief`.
func (s *State) HeadOpenTasks(limit int) []*Node {
	var out []*Node
	for _, id := range s.Order {
		n := s.Nodes[id]
		if !s.IsReadyTask(n.ID) {
			continue
		}
		out = append(out, n)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (s *State) IsReadyTask(id int64) bool {
	n, ok := s.Nodes[id]
	if !ok || n.Kind != KindTask {
		return false
	}
	if n.Terminal() || n.Claim != nil || n.Hold != nil {
		return false
	}
	for _, refID := range n.After {
		ref, ok := s.Nodes[refID]
		if !ok || s.NodeStatus(ref) != StatusCompleted {
			return false
		}
	}
	return s.IsLeafTask(id)
}

func (s *State) ReadyBlockReason(id int64) string {
	n, ok := s.Nodes[id]
	if !ok {
		return fmt.Sprintf("task #%d not found", id)
	}
	if n.Kind != KindTask {
		return fmt.Sprintf("#%d is %s, not a task", id, n.Kind)
	}
	if n.Terminal() {
		return fmt.Sprintf("task #%d already terminal", id)
	}
	if n.Claim != nil {
		return fmt.Sprintf("task #%d already claimed by %s", id, n.Claim.Actor)
	}
	if n.Hold != nil {
		return fmt.Sprintf("task #%d is held (%s): %s", id, n.Hold.Kind, n.Hold.Reason)
	}
	if child := s.OpenTaskChild(n); child != nil {
		return fmt.Sprintf("task #%d has open child task #%d", id, child.ID)
	}
	if failed := s.DependencyFailedIDs(n); len(failed) > 0 {
		return fmt.Sprintf("task #%d has canceled prerequisite(s): %v", id, failed)
	}
	if waiting := s.WaitingOnIDs(n); len(waiting) > 0 {
		return fmt.Sprintf("task #%d is waiting on prerequisite(s): %v", id, waiting)
	}
	return fmt.Sprintf("task #%d is not ready", id)
}

// IsLeafTask returns true when the task has no non-terminal task children.
func (s *State) IsLeafTask(id int64) bool {
	n := s.Nodes[id]
	for _, cid := range n.Children {
		c := s.Nodes[cid]
		if c.Kind != KindTask {
			continue
		}
		if !c.Terminal() {
			return false
		}
	}
	return true
}

func (s *State) IsWithin(scopeID, nodeID int64) bool {
	if scopeID == 0 {
		return true
	}
	for cur := nodeID; cur != 0; {
		if cur == scopeID {
			return true
		}
		n, ok := s.Nodes[cur]
		if !ok {
			return false
		}
		cur = n.ParentID
	}
	return false
}

func (s *State) ChildWorkNodes(parentID int64, limit int) ([]*Node, int) {
	var out []*Node
	parent, ok := s.Nodes[parentID]
	if !ok {
		return nil, 0
	}
	total := 0
	for _, cid := range parent.Children {
		c := s.Nodes[cid]
		if c.Kind != KindTask && c.Kind != KindGoal {
			continue
		}
		total++
		if limit <= 0 || len(out) < limit {
			out = append(out, c)
		}
	}
	return out, total
}

// FrontierChildWorkNodes returns direct child work nodes that still contain
// open tasks. Completed/canceled child subtrees are counted separately so the
// default brief can show the current frontier without losing the closed-history
// count.
func (s *State) FrontierChildWorkNodes(parentID int64, limit int) ([]*Node, int, int) {
	var out []*Node
	parent, ok := s.Nodes[parentID]
	if !ok {
		return nil, 0, 0
	}
	activeTotal := 0
	completedTotal := 0
	for _, cid := range parent.Children {
		c := s.Nodes[cid]
		if c.Kind != KindTask && c.Kind != KindGoal {
			continue
		}
		p := s.SubtreeProgress(c.ID)
		if p.OpenTasks == 0 {
			completedTotal++
			continue
		}
		activeTotal++
		if limit <= 0 || len(out) < limit {
			out = append(out, c)
		}
	}
	return out, activeTotal, completedTotal
}

func (s *State) HeldTasks() []*Node {
	var out []*Node
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Kind == KindTask && n.Hold != nil && !n.Terminal() {
			out = append(out, n)
		}
	}
	return out
}

func (s *State) HeldTasksWithin(scopeID int64, limit int) ([]*Node, int) {
	var out []*Node
	total := 0
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Kind != KindTask || n.Hold == nil || n.Terminal() || !s.IsWithin(scopeID, id) {
			continue
		}
		total++
		if limit <= 0 || len(out) < limit {
			out = append(out, n)
		}
	}
	return out, total
}

func (s *State) WaitingOnIDs(n *Node) []int64 {
	var out []int64
	for _, refID := range n.After {
		ref := s.Nodes[refID]
		if ref == nil || s.NodeStatus(ref) == StatusCanceled {
			continue
		}
		if s.NodeStatus(ref) != StatusCompleted {
			out = append(out, refID)
		}
	}
	return out
}

func (s *State) DependencyFailedIDs(n *Node) []int64 {
	var out []int64
	for _, refID := range n.After {
		ref := s.Nodes[refID]
		if ref == nil || s.NodeStatus(ref) == StatusCanceled {
			out = append(out, refID)
		}
	}
	return out
}

func (s *State) WaitingTasksWithin(scopeID int64, limit int) ([]*Node, int) {
	var out []*Node
	total := 0
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Kind != KindTask || n.Terminal() || n.Claim != nil || n.Hold != nil || !s.IsWithin(scopeID, id) {
			continue
		}
		if len(s.WaitingOnIDs(n)) == 0 || len(s.DependencyFailedIDs(n)) > 0 {
			continue
		}
		total++
		if limit <= 0 || len(out) < limit {
			out = append(out, n)
		}
	}
	return out, total
}

func (s *State) DependencyFailedTasksWithin(scopeID int64, limit int) ([]*Node, int) {
	var out []*Node
	total := 0
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Kind != KindTask || n.Terminal() || n.Claim != nil || n.Hold != nil || !s.IsWithin(scopeID, id) {
			continue
		}
		if len(s.DependencyFailedIDs(n)) == 0 {
			continue
		}
		total++
		if limit <= 0 || len(out) < limit {
			out = append(out, n)
		}
	}
	return out, total
}

func (s *State) ReviewReadyTasks(limit int) []*Node {
	out, _ := s.ReviewReadyTasksWithin(0, limit)
	return out
}

func (s *State) ReviewReadyTasksWithin(scopeID int64, limit int) ([]*Node, int) {
	var out []*Node
	total := 0
	for _, id := range s.Order {
		n := s.Nodes[id]
		if !s.IsWithin(scopeID, id) || !s.IsReadyTask(id) || n.Acceptance == nil || n.Acceptance.Kind != AcceptanceReview {
			continue
		}
		total++
		if limit <= 0 || len(out) < limit {
			out = append(out, n)
		}
	}
	return out, total
}

func (s *State) HeadOpenTasksWithin(scopeID int64, limit int) ([]*Node, int) {
	var out []*Node
	total := 0
	for _, id := range s.Order {
		n := s.Nodes[id]
		if !s.IsWithin(scopeID, id) || !s.IsReadyTask(n.ID) {
			continue
		}
		total++
		if limit <= 0 || len(out) < limit {
			out = append(out, n)
		}
	}
	return out, total
}

// CurrentClaims returns nodes that currently hold a claim, sorted by claim time.
func (s *State) CurrentClaims() []*Node {
	out, _ := s.CurrentClaimsWithin(0, 0)
	return out
}

func (s *State) CurrentClaimsWithin(scopeID int64, limit int) ([]*Node, int) {
	var out []*Node
	total := 0
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Claim != nil && s.IsWithin(scopeID, id) {
			total++
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Claim.TakenAt.Before(out[j].Claim.TakenAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, total
}

// RecentRuns returns the latest script runs across the whole tree.
func (s *State) RecentRuns(limit int) []ScriptRunRecord {
	return s.RecentRunsWithin(0, limit, false)
}

func (s *State) RecentRunsWithin(scopeID int64, limit int, activeOnly bool) []ScriptRunRecord {
	var all []ScriptRunRecord
	for _, id := range s.Order {
		n := s.Nodes[id]
		if !s.IsWithin(scopeID, n.ID) {
			continue
		}
		if activeOnly && n.Terminal() {
			continue
		}
		all = append(all, n.Runs...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].At.After(all[j].At) })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

func (s *State) RecentFailures(limit int) []ScriptRunRecord {
	return s.RecentFailuresWithin(0, limit, false)
}

func (s *State) RecentFailuresWithin(scopeID int64, limit int, activeOnly bool) []ScriptRunRecord {
	var out []ScriptRunRecord
	for _, r := range s.RecentRunsWithin(scopeID, 0, activeOnly) {
		if r.ExitCode != 0 {
			out = append(out, r)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

// RecentCompleted returns the latest completed task ids, newest first.
func (s *State) RecentCompleted(limit int) []int64 {
	out := append([]int64(nil), s.completedOrder...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// RecentCanceled returns the latest canceled node ids, newest first.
func (s *State) RecentCanceled(limit int) []int64 {
	out := append([]int64(nil), s.canceledOrder...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
