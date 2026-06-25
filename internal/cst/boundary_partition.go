package cst

import "fmt"

func (s *State) validateBoundaryPartition() error {
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n == nil || n.Canceled || n.Boundary == nil || len(n.Boundary.Owned) == 0 {
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
			if left == nil || left.Canceled || left.Boundary == nil || len(left.Boundary.Owned) == 0 {
				continue
			}
			for _, rightID := range parent.Children[i+1:] {
				right := s.Nodes[rightID]
				if right == nil || right.Canceled || right.Boundary == nil || len(right.Boundary.Owned) == 0 {
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
