package cst

type ObligationCoverage struct {
	Required        []string `json:"required,omitempty"`
	Claimed         []string `json:"claimed,omitempty"`
	Missing         []string `json:"missing,omitempty"`
	UnmatchedClaims []string `json:"unmatched_claims,omitempty"`
}

func (s *State) ObligationCoverage(scopeID int64) ObligationCoverage {
	requiredSet := map[string]bool{}
	claimedSet := map[string]bool{}
	for _, n := range s.SubtreeNodes(scopeID) {
		if n == nil || n.Canceled {
			continue
		}
		if n.Context != nil {
			for _, obligation := range n.Context.SuccessObligations {
				requiredSet[obligation] = true
			}
		}
		if n.Kind == KindTask && s.IsStaticLeafTask(n.ID) {
			for _, claim := range n.ObligationClaims {
				claimedSet[claim] = true
			}
		}
	}
	required := sortedStringSet(requiredSet)
	claimed := sortedStringSet(claimedSet)
	var missing []string
	for _, obligation := range required {
		if !claimedSet[obligation] {
			missing = append(missing, obligation)
		}
	}
	var unmatched []string
	for _, claim := range claimed {
		if !requiredSet[claim] {
			unmatched = append(unmatched, claim)
		}
	}
	return ObligationCoverage{
		Required:        required,
		Claimed:         claimed,
		Missing:         missing,
		UnmatchedClaims: unmatched,
	}
}

func (s *State) IsStaticLeafTask(id int64) bool {
	n := s.Nodes[id]
	if n == nil || n.Kind != KindTask {
		return false
	}
	for _, cid := range n.Children {
		c := s.Nodes[cid]
		if c.Kind == KindTask && !c.Canceled {
			return false
		}
	}
	return true
}

func sortedStringSet(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	return normalizeStringSet(out)
}
