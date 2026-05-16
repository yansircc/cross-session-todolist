package cst

import (
	"sort"
	"time"
)

// uiView is the data the HTML emitter consumes. It contains only what's
// rendered — no further reducer logic happens during emission.
type uiView struct {
	Project     string
	GeneratedAt time.Time
	EventsPath  string
	TotalEvents int
	LastEvent   time.Time

	TotalTasks   int
	OpenTasks    int
	ClaimedTasks int
	HeldTasks    int
	GoalCount    int
	RuleCount    int

	RecentFailures []ScriptRunRecord
	RecentRuns     []ScriptRunRecord
	ActiveScopes   []scopeView
}

type scopeView struct {
	Goal         *Node
	Ancestors    []*Node // path from root toward self, excluding self
	SubGoals     []*Node // direct goal children, for sub-scope nav
	Rules        []*Node // inherited active rules, ordered root-to-scope
	Total        int     // direct task count (innermost goal == this)
	Done         int
	PctDone      int
	OpenByStatus map[NodeStatus]int
	LastActivity time.Time
	AllTasks     []*Node // direct tasks (done + open), sorted: open first, then done newest-first
}

// uiViewFrom builds a uiView from State, scoped to a subtree rooted at
// scopeID. scopeID == 0 means the whole project.
func uiViewFrom(s *State, scopeID int64, eventsPath, project string, totalEvents int, lastEvent time.Time) uiView {
	v := uiView{
		Project:     project,
		GeneratedAt: time.Now(),
		EventsPath:  eventsPath,
		TotalEvents: totalEvents,
		LastEvent:   lastEvent,
	}

	for _, id := range s.Order {
		n := s.Nodes[id]
		if scopeID != 0 && !s.IsWithin(scopeID, id) {
			continue
		}
		switch n.Kind {
		case KindTask:
			v.TotalTasks++
			if !n.Terminal() {
				v.OpenTasks++
				if n.Claim != nil {
					v.ClaimedTasks++
				}
				if n.Hold != nil {
					v.HeldTasks++
				}
			}
		case KindGoal:
			v.GoalCount++
		case KindRule:
			v.RuleCount++
		}
	}
	activeRecentOnly := v.OpenTasks > 0
	v.RecentRuns = recentUIRuns(s, scopeID, false, activeRecentOnly, 6)
	v.RecentFailures = recentUIRuns(s, scopeID, true, activeRecentOnly, 6)

	// Bucket every non-terminal task to its innermost ancestor goal.
	openByGoal := map[int64]struct{}{}
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
		openByGoal[g.ID] = struct{}{}
	}

	// Drop scopes whose direct task count is 0 (their open work lives in
	// nested goals; those nested goals show up as their own cards).
	for gid := range openByGoal {
		sv := buildScope(s, s.Nodes[gid])
		if sv.Total == 0 {
			continue
		}
		v.ActiveScopes = append(v.ActiveScopes, sv)
	}

	sort.Slice(v.ActiveScopes, func(i, j int) bool {
		return v.ActiveScopes[i].LastActivity.After(v.ActiveScopes[j].LastActivity)
	})

	return v
}

func buildScope(s *State, g *Node) scopeView {
	sv := scopeView{Goal: g, OpenByStatus: map[NodeStatus]int{}}

	// Ancestor goals from root to (but not including) self.
	chain := s.ancestorChain(g.ID)
	for _, id := range chain {
		if id == g.ID {
			continue
		}
		anc := s.Nodes[id]
		if anc != nil && anc.Kind == KindGoal {
			sv.Ancestors = append(sv.Ancestors, anc)
		}
	}

	sv.Rules = s.InheritedRules(g.ID)

	// Direct child goals, for sub-scope nav.
	for _, cid := range g.Children {
		c := s.Nodes[cid]
		if c == nil {
			continue
		}
		switch c.Kind {
		case KindGoal:
			sv.SubGoals = append(sv.SubGoals, c)
		}
	}

	// Direct tasks: any task whose innermost ancestor goal is g.
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n.Kind != KindTask {
			continue
		}
		ig := innermostGoal(s, n.ParentID)
		if ig == nil || ig.ID != g.ID {
			continue
		}
		sv.AllTasks = append(sv.AllTasks, n)
		sv.Total++
		if n.Completed {
			sv.Done++
		} else if !n.Canceled {
			sv.OpenByStatus[n.Status()]++
		} else {
			sv.OpenByStatus[StatusCanceled]++
		}
		if n.LastEvent.After(sv.LastActivity) {
			sv.LastActivity = n.LastEvent
		}
	}
	if sv.Total > 0 {
		sv.PctDone = sv.Done * 100 / sv.Total
	}

	sort.SliceStable(sv.AllTasks, func(i, j int) bool {
		a, b := sv.AllTasks[i], sv.AllTasks[j]
		ar, br := taskRank(a), taskRank(b)
		if ar != br {
			return ar < br
		}
		if a.Status() == StatusCompleted && b.Status() == StatusCompleted {
			return a.CompletedAt.After(b.CompletedAt)
		}
		return a.ID < b.ID
	})

	return sv
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

// taskRank groups tasks: open first (sub-ordered by status), then completed,
// then canceled.
func taskRank(n *Node) int {
	switch n.Status() {
	case StatusClaimed:
		return 0
	case StatusHeld:
		return 1
	case StatusOpen:
		return 2
	case StatusCompleted:
		return 3
	case StatusCanceled:
		return 4
	}
	return 9
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
