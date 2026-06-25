package cst

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type NodeContext struct {
	Invariant          string   `json:"invariant,omitempty"`
	NonGoals           []string `json:"non_goals,omitempty"`
	SuccessObligations []string `json:"success_obligations,omitempty"`
}

type NodeBoundary struct {
	Owned    []string `json:"owned,omitempty"`
	Excluded []string `json:"excluded,omitempty"`
}

type NodeContextPatch struct {
	InvariantSet          bool
	Invariant             string
	NonGoalsSet           bool
	NonGoals              []string
	SuccessObligationsSet bool
	SuccessObligations    []string
	Clear                 bool
}

type NodeBoundaryPatch struct {
	OwnedSet    bool
	Owned       []string
	ExcludedSet bool
	Excluded    []string
	Clear       bool
}

func normalizeNodeContext(ctx *NodeContext) (*NodeContext, error) {
	if ctx == nil {
		return nil, nil
	}
	out := &NodeContext{
		Invariant:          strings.TrimSpace(ctx.Invariant),
		NonGoals:           normalizeTextList(ctx.NonGoals),
		SuccessObligations: normalizeObligationNames(ctx.SuccessObligations),
	}
	if out.Invariant == "" && len(out.NonGoals) == 0 && len(out.SuccessObligations) == 0 {
		return nil, nil
	}
	return out, nil
}

func mergeNodeContextPatch(base *NodeContext, patch NodeContextPatch) (*NodeContext, error) {
	if patch.Clear {
		return nil, nil
	}
	ctx := &NodeContext{}
	if base != nil {
		ctx = cloneNodeContext(base)
	}
	if patch.InvariantSet {
		ctx.Invariant = patch.Invariant
	}
	if patch.NonGoalsSet {
		ctx.NonGoals = patch.NonGoals
	}
	if patch.SuccessObligationsSet {
		ctx.SuccessObligations = patch.SuccessObligations
	}
	return normalizeNodeContext(ctx)
}

func cloneNodeContext(ctx *NodeContext) *NodeContext {
	if ctx == nil {
		return nil
	}
	return &NodeContext{
		Invariant:          ctx.Invariant,
		NonGoals:           append([]string(nil), ctx.NonGoals...),
		SuccessObligations: append([]string(nil), ctx.SuccessObligations...),
	}
}

func normalizeNodeBoundary(boundary *NodeBoundary) (*NodeBoundary, error) {
	if boundary == nil {
		return nil, nil
	}
	owned, err := normalizeBoundaryPaths(boundary.Owned)
	if err != nil {
		return nil, fmt.Errorf("owned: %w", err)
	}
	excluded, err := normalizeBoundaryPaths(boundary.Excluded)
	if err != nil {
		return nil, fmt.Errorf("excluded: %w", err)
	}
	if len(owned) == 0 && len(excluded) == 0 {
		return nil, nil
	}
	return &NodeBoundary{Owned: owned, Excluded: excluded}, nil
}

func mergeNodeBoundaryPatch(base *NodeBoundary, patch NodeBoundaryPatch) (*NodeBoundary, error) {
	if patch.Clear {
		return nil, nil
	}
	boundary := &NodeBoundary{}
	if base != nil {
		boundary = cloneNodeBoundary(base)
	}
	if patch.OwnedSet {
		boundary.Owned = patch.Owned
	}
	if patch.ExcludedSet {
		boundary.Excluded = patch.Excluded
	}
	return normalizeNodeBoundary(boundary)
}

func cloneNodeBoundary(boundary *NodeBoundary) *NodeBoundary {
	if boundary == nil {
		return nil
	}
	return &NodeBoundary{
		Owned:    append([]string(nil), boundary.Owned...),
		Excluded: append([]string(nil), boundary.Excluded...),
	}
}

func normalizeObligationNames(values []string) []string {
	return normalizeStringSet(values)
}

func normalizeTextList(values []string) []string {
	return normalizeStringSet(values)
}

func normalizeStringSet(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeBoundaryPaths(paths []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if filepath.IsAbs(raw) {
			return nil, fmt.Errorf("path %q must be repository-relative", raw)
		}
		p := filepath.ToSlash(filepath.Clean(raw))
		p = strings.TrimPrefix(p, "./")
		if p == "" {
			p = "."
		}
		if p == ".." || strings.HasPrefix(p, "../") {
			return nil, fmt.Errorf("path %q escapes the repository", raw)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

func sameStringSet(a, b []string) bool {
	a = normalizeStringSet(a)
	b = normalizeStringSet(b)
	return sameStringList(a, b)
}

func normalizeNodeDeclarations(nodeID int64, kind string, ctx *NodeContext, boundary *NodeBoundary, claims []string) (*NodeContext, *NodeBoundary, []string, error) {
	if kind != KindGoal && kind != KindTask {
		if ctx != nil {
			return nil, nil, nil, fmt.Errorf("%s #%d cannot have context", kind, nodeID)
		}
		if boundary != nil {
			return nil, nil, nil, fmt.Errorf("%s #%d cannot have boundary", kind, nodeID)
		}
		if len(claims) > 0 {
			return nil, nil, nil, fmt.Errorf("%s #%d cannot have obligation claims", kind, nodeID)
		}
		return nil, nil, nil, nil
	}
	normalizedContext, err := normalizeNodeContext(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("context: %w", err)
	}
	normalizedBoundary, err := normalizeNodeBoundary(boundary)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("boundary: %w", err)
	}
	normalizedClaims := normalizeObligationNames(claims)
	if kind != KindTask && len(normalizedClaims) > 0 {
		return nil, nil, nil, fmt.Errorf("%s #%d cannot have obligation claims; claims belong to task acceptance", kind, nodeID)
	}
	return normalizedContext, normalizedBoundary, normalizedClaims, nil
}
