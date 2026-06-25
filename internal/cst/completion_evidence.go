package cst

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	EvidenceBoundary  = "boundary"
	EvidenceRationale = "rationale"
	EvidenceContest   = "contest"
)

type boundaryEvidenceData struct {
	Includes []string `json:"includes,omitempty"`
	Excludes []string `json:"excludes,omitempty"`
}

type rationaleEvidenceData struct {
	Invariant     string `json:"invariant"`
	Failure       string `json:"failure"`
	MinimalFix    string `json:"minimal_fix"`
	RemainingRisk string `json:"remaining_risk"`
	NotDoing      string `json:"not_doing,omitempty"`
}

type contestEvidenceData struct {
	TargetEvidenceID string `json:"target_evidence_id"`
	Reason           string `json:"reason"`
}

type ClosureProjection struct {
	Boundary  []ClosureEvidenceProjection `json:"boundary,omitempty"`
	Rationale []ClosureEvidenceProjection `json:"rationale,omitempty"`
	Contests  []ClosureContestProjection  `json:"contests,omitempty"`
}

type ClosureEvidenceProjection struct {
	EventID   string                    `json:"event_id"`
	Kind      string                    `json:"kind"`
	Summary   string                    `json:"summary"`
	Contested *ClosureContestProjection `json:"contested,omitempty"`
}

type ClosureContestProjection struct {
	EventID          string `json:"event_id"`
	TargetEvidenceID string `json:"target_evidence_id"`
	Summary          string `json:"summary"`
	Reason           string `json:"reason"`
}

func completionEvidenceIDs(e *Event) []string {
	return normalizeEvidenceIDs(append(append([]string(nil), e.EvidenceIDs...), e.EvidenceID))
}

func normalizeEvidenceIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func validateBoundaryEvidence(raw json.RawMessage) error {
	data, err := parseBoundaryEvidence(raw)
	if err != nil {
		return err
	}
	if len(data.Includes) == 0 && len(data.Excludes) == 0 {
		return fmt.Errorf("boundary evidence requires includes or excludes")
	}
	return nil
}

func parseBoundaryEvidence(raw json.RawMessage) (boundaryEvidenceData, error) {
	if len(raw) == 0 {
		return boundaryEvidenceData{}, fmt.Errorf("boundary evidence requires --data")
	}
	var data boundaryEvidenceData
	if err := json.Unmarshal(raw, &data); err != nil {
		return boundaryEvidenceData{}, fmt.Errorf("boundary evidence data must be JSON object: %w", err)
	}
	var err error
	data.Includes, err = normalizeBoundaryPaths(data.Includes)
	if err != nil {
		return boundaryEvidenceData{}, fmt.Errorf("boundary evidence includes: %w", err)
	}
	data.Excludes, err = normalizeBoundaryPaths(data.Excludes)
	if err != nil {
		return boundaryEvidenceData{}, fmt.Errorf("boundary evidence excludes: %w", err)
	}
	for _, field := range []struct {
		name   string
		values []string
	}{
		{name: "includes", values: data.Includes},
		{name: "excludes", values: data.Excludes},
	} {
		for i, value := range field.values {
			if strings.TrimSpace(value) == "" {
				return boundaryEvidenceData{}, fmt.Errorf("boundary evidence %s[%d] is empty", field.name, i)
			}
		}
	}
	return data, nil
}

func validateRationaleEvidence(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("rationale evidence requires --data")
	}
	var data rationaleEvidenceData
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("rationale evidence data must be JSON object: %w", err)
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "invariant", value: data.Invariant},
		{name: "failure", value: data.Failure},
		{name: "minimal_fix", value: data.MinimalFix},
		{name: "remaining_risk", value: data.RemainingRisk},
	}
	for _, field := range fields {
		if vacuousEvidenceText(field.value) {
			return fmt.Errorf("rationale evidence %s is empty or vacuous", field.name)
		}
	}
	if strings.TrimSpace(data.NotDoing) != "" && vacuousEvidenceText(data.NotDoing) {
		return fmt.Errorf("rationale evidence not_doing is vacuous")
	}
	return nil
}

func vacuousEvidenceText(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "-", "--", "n/a", "na", "none", "nothing", "no", "null", "nil", "tbd", "todo":
		return true
	default:
		return false
	}
}

func validateContestEvidence(s *State, e *Event) error {
	if len(e.EvidenceData) == 0 {
		return fmt.Errorf("contest evidence requires --data")
	}
	var data contestEvidenceData
	if err := json.Unmarshal(e.EvidenceData, &data); err != nil {
		return fmt.Errorf("contest evidence data must be JSON object: %w", err)
	}
	data.TargetEvidenceID = strings.TrimSpace(data.TargetEvidenceID)
	if data.TargetEvidenceID == "" {
		return fmt.Errorf("contest evidence target_evidence_id is required")
	}
	if vacuousEvidenceText(data.Reason) {
		return fmt.Errorf("contest evidence reason is empty or vacuous")
	}
	target, ok := s.EvidenceIDs[data.TargetEvidenceID]
	if !ok {
		return fmt.Errorf("contest evidence target %s not found", data.TargetEvidenceID)
	}
	if target.NodeID != e.NodeID {
		return fmt.Errorf("contest evidence target %s belongs to #%d", data.TargetEvidenceID, target.NodeID)
	}
	if target.Kind != EvidenceBoundary && target.Kind != EvidenceRationale {
		return fmt.Errorf("contest evidence target %s has non-contestable kind %s", data.TargetEvidenceID, target.Kind)
	}
	return nil
}

func latestContestForEvidence(n *Node, evidenceID string) *EvidenceRecord {
	for i := len(n.Evidences) - 1; i >= 0; i-- {
		ev := n.Evidences[i]
		if ev.Kind != EvidenceContest {
			continue
		}
		var data contestEvidenceData
		if err := json.Unmarshal(ev.Data, &data); err != nil {
			continue
		}
		if data.TargetEvidenceID == evidenceID {
			rec := ev
			return &rec
		}
	}
	return nil
}

func closureProjection(n *Node) *ClosureProjection {
	if n == nil {
		return nil
	}
	out := &ClosureProjection{}
	for _, ev := range n.Evidences {
		switch ev.Kind {
		case EvidenceBoundary, EvidenceRationale:
			item := ClosureEvidenceProjection{
				EventID: ev.EventID,
				Kind:    ev.Kind,
				Summary: ev.Summary,
			}
			if contest := latestContestForEvidence(n, ev.EventID); contest != nil {
				item.Contested = contestProjection(*contest)
			}
			if ev.Kind == EvidenceBoundary {
				out.Boundary = append(out.Boundary, item)
			} else {
				out.Rationale = append(out.Rationale, item)
			}
		case EvidenceContest:
			if contest := contestProjection(ev); contest != nil {
				out.Contests = append(out.Contests, *contest)
			}
		}
	}
	if len(out.Boundary) == 0 && len(out.Rationale) == 0 && len(out.Contests) == 0 {
		return nil
	}
	return out
}

func contestProjection(ev EvidenceRecord) *ClosureContestProjection {
	var data contestEvidenceData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		return nil
	}
	return &ClosureContestProjection{
		EventID:          ev.EventID,
		TargetEvidenceID: data.TargetEvidenceID,
		Summary:          ev.Summary,
		Reason:           data.Reason,
	}
}

func closureSummary(closure *ClosureProjection) string {
	if closure == nil {
		return ""
	}
	var parts []string
	if len(closure.Boundary) > 0 {
		parts = append(parts, fmt.Sprintf("boundary=%d", len(closure.Boundary)))
	}
	if len(closure.Rationale) > 0 {
		parts = append(parts, fmt.Sprintf("rationale=%d", len(closure.Rationale)))
	}
	if len(closure.Contests) > 0 {
		parts = append(parts, fmt.Sprintf("contested=%d", len(closure.Contests)))
	}
	return strings.Join(parts, " ")
}

func (s *State) validateCompletionBoundaryEvidence(n *Node, runSet EvidenceRecord, ids []string) error {
	boundaries := make([]EvidenceRecord, 0, len(ids))
	for _, evidenceID := range ids {
		rec := s.EvidenceIDs[evidenceID]
		if rec.Kind == EvidenceBoundary {
			boundaries = append(boundaries, rec)
		}
	}
	if len(boundaries) == 0 {
		return nil
	}
	changed, ok, err := changedPathsForAcceptanceRunSet(n, runSet)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("boundary evidence requires git status from acceptance runs")
	}
	for _, rec := range boundaries {
		data, err := parseBoundaryEvidence(rec.Data)
		if err != nil {
			return err
		}
		for _, path := range changed {
			if len(data.Includes) > 0 && !pathInOwnedPaths(data.Includes, path) {
				return fmt.Errorf("boundary evidence %s includes do not cover changed path %s", rec.EventID, path)
			}
			if pathInOwnedPaths(data.Excludes, path) {
				return fmt.Errorf("boundary evidence %s excludes changed path %s", rec.EventID, path)
			}
		}
	}
	return nil
}

func changedPathsForAcceptanceRunSet(n *Node, runSet EvidenceRecord) ([]string, bool, error) {
	data, err := parseAcceptanceRunSetData(runSet.Data)
	if err != nil {
		return nil, false, err
	}
	runs := map[string]ScriptRunRecord{}
	for _, run := range n.Runs {
		runs[run.EventID] = run
	}
	seen := map[string]bool{}
	gitKnown := false
	for _, check := range data.Checks {
		run, ok := runs[check.ScriptRunEventID]
		if !ok {
			return nil, false, fmt.Errorf("boundary evidence references missing run %s", check.ScriptRunEventID)
		}
		if !run.GitAvailable {
			continue
		}
		gitKnown = true
		for _, path := range changedPathsFromStatus(run.GitStatusShort) {
			if isCSTStorePath(path) {
				continue
			}
			seen[path] = true
		}
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	sort.Strings(out)
	return out, gitKnown, nil
}

func changedPathsFromStatus(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 3 {
			continue
		}
		path := ""
		switch {
		case len(line) >= 4 && line[2] == ' ':
			path = strings.TrimSpace(line[3:])
		case len(line) >= 3 && line[1] == ' ':
			path = strings.TrimSpace(line[2:])
		default:
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				path = strings.Join(fields[1:], " ")
			}
		}
		if path == "" {
			continue
		}
		if before, after, ok := strings.Cut(path, " -> "); ok {
			if before = strings.TrimSpace(before); before != "" {
				out = append(out, before)
			}
			if after = strings.TrimSpace(after); after != "" {
				out = append(out, after)
			}
			continue
		}
		out = append(out, strings.Trim(path, `"`))
	}
	return out
}

func isCSTStorePath(path string) bool {
	path = strings.Trim(strings.TrimPrefix(path, "./"), "/")
	return path == StoreDirName || strings.HasPrefix(path, StoreDirName+"/")
}
