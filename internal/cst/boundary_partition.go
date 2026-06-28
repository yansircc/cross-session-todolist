package cst

import "fmt"

func (s *State) validateBoundaryPartition() error {
	for _, id := range s.Order {
		n := s.Nodes[id]
		if !s.participatesInBoundaryPartition(n) {
			continue
		}
		if n.ParentID != 0 {
			parent := s.Nodes[n.ParentID]
			if parent != nil && parent.Boundary != nil && len(parent.Boundary.Owned) > 0 {
				for _, owned := range n.Boundary.Owned {
					if !pathInOwnedPaths(parent.Boundary.Owned, owned) {
						return fmt.Errorf("boundary partition violation: node #%d owned path %s is outside parent #%d owned boundary", n.ID, owned, parent.ID)
					}
				}
			}
		}
	}
	for _, parentID := range s.Order {
		parent := s.Nodes[parentID]
		if parent == nil {
			continue
		}
		for i, leftID := range parent.Children {
			left := s.Nodes[leftID]
			if !s.participatesInBoundaryPartition(left) {
				continue
			}
			for _, rightID := range parent.Children[i+1:] {
				right := s.Nodes[rightID]
				if !s.participatesInBoundaryPartition(right) {
					continue
				}
				if pathsOverlap(left.Boundary.Owned, right.Boundary.Owned) {
					return fmt.Errorf("boundary partition violation: sibling nodes #%d and #%d have overlapping owned boundaries", left.ID, right.ID)
				}
			}
		}
	}
	return nil
}

func (s *State) participatesInBoundaryPartition(n *Node) bool {
	if n == nil || n.Boundary == nil || len(n.Boundary.Owned) == 0 {
		return false
	}
	if n.Kind == KindTask {
		return !n.Terminal()
	}
	if n.Kind != KindGoal {
		return false
	}
	if s.NodeStatus(n) != StatusCompleted {
		return true
	}
	// An empty goal is still an active modeling surface; only a goal with
	// completed descendant work has become historical boundary evidence.
	return s.SubtreeProgress(n.ID).TotalTasks == 0
}

func validateNodeBoundaryCompletion(n *Node, runSet EvidenceRecord) error {
	if n == nil || n.Boundary == nil {
		return nil
	}
	changed, ok, err := changedPathsForAcceptanceRunSet(n, runSet)
	if err != nil {
		return err
	}
	if !ok && (len(n.Boundary.Owned) > 0 || len(n.Boundary.Excluded) > 0) {
		return fmt.Errorf("node boundary requires git status from acceptance runs")
	}
	for _, path := range changed {
		if len(n.Boundary.Owned) > 0 && !pathInOwnedPaths(n.Boundary.Owned, path) {
			return fmt.Errorf("node boundary owned paths do not cover changed path %s", path)
		}
		if pathInOwnedPaths(n.Boundary.Excluded, path) {
			return fmt.Errorf("node boundary excludes changed path %s", path)
		}
	}
	return nil
}
