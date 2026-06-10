package cst

import (
	"encoding/json"
	"fmt"
)

type contextDriftEvidence struct {
	AcceptanceEvidenceID string                     `json:"acceptance_evidence_id"`
	Mode                 string                     `json:"mode"`
	Reason               string                     `json:"reason"`
	Expected             AcceptanceExecutionContext `json:"expected_context"`
	Current              AcceptanceExecutionContext `json:"current_context"`
}

func currentAcceptanceContext(storeID string, execCWD string, env ExecutionEnvelope) (AcceptanceExecutionContext, string) {
	identity := CaptureExecIdentity(execCWD, env)
	identity.ExecContextDigest = execContextDigest(storeID, identity)
	return executionContextFromIdentity(storeID, identity), identity.ExecContextDigest
}

func executionContextFromIdentity(storeID string, id ExecIdentity) AcceptanceExecutionContext {
	return AcceptanceExecutionContext{
		StoreID:              storeID,
		ExecCWD:              id.ExecCWD,
		ExecSurface:          firstNonEmpty(id.ExecSurface, ExecSurfaceShared),
		OwnedPaths:           normalizeOwnedPaths(id.OwnedPaths),
		ScopedDigest:         id.ScopedDigest,
		OutOfScopeDigest:     id.OutOfScopeDigest,
		OutOfScopeDeltaCount: id.OutOfScopeDeltaCount,
		WholeRepoDigest:      id.WholeRepoDigest,
		GitAvailable:         id.GitAvailable,
		GitIdentityDigest:    id.GitIdentityDigest,
	}
}

func validateAcceptanceContextForCompletion(tx *Tx, id int64, evidenceID string, execCWDOverride string) (*Event, error) {
	rec, ok := tx.state.EvidenceIDs[evidenceID]
	if !ok || rec.Kind != EvidenceAcceptanceRunSet {
		return nil, nil
	}
	data, err := parseAcceptanceRunSetData(rec.Data)
	if err != nil {
		return nil, err
	}
	if data.ExecutionContext == nil {
		return nil, nil
	}
	n := tx.state.Nodes[id]
	if n == nil {
		return nil, herr(ExitNotFound, "task #%d not found", id)
	}
	if !n.EnvelopeEventAt.IsZero() && n.EnvelopeEventAt.After(rec.At) {
		return nil, herr(ExitInvariantBroken, "task #%d execution envelope changed after acceptance run-set", id)
	}
	env := effectiveExecutionEnvelope(n)
	execCWD := resolveExecCWD(execCWDOverride, env)
	current, currentDigest := currentAcceptanceContext(tx.StoreID(), execCWD, env)
	expected := *data.ExecutionContext
	expected.ExecSurface = firstNonEmpty(expected.ExecSurface, ExecSurfaceShared)
	expected.OwnedPaths = normalizeOwnedPaths(expected.OwnedPaths)
	if current.ExecCWD != expected.ExecCWD {
		return nil, herr(ExitInvariantBroken, "task #%d acceptance exec_cwd drifted; rerun acceptance", id)
	}
	if current.ExecSurface != expected.ExecSurface || !sameStringList(current.OwnedPaths, expected.OwnedPaths) {
		return nil, herr(ExitInvariantBroken, "task #%d acceptance execution envelope drifted; rerun acceptance", id)
	}
	if expected.ExecSurface == ExecSurfacePrivate {
		if currentDigest != data.ContextDigest {
			return nil, herr(ExitInvariantBroken, "task #%d private execution context drifted; rerun acceptance", id)
		}
		return nil, nil
	}
	if len(expected.OwnedPaths) > 0 && current.ScopedDigest != expected.ScopedDigest {
		return nil, herr(ExitInvariantBroken, "task #%d scoped execution context drifted; rerun acceptance", id)
	}
	reason := ""
	if len(expected.OwnedPaths) > 0 && current.OutOfScopeDigest != expected.OutOfScopeDigest {
		reason = "out_of_scope_drift"
	}
	if len(expected.OwnedPaths) == 0 && current.WholeRepoDigest != expected.WholeRepoDigest {
		reason = "whole_repo_drift"
	}
	if reason == "" {
		return nil, nil
	}
	raw, err := json.Marshal(contextDriftEvidence{
		AcceptanceEvidenceID: evidenceID,
		Mode:                 ExecSurfaceShared,
		Reason:               reason,
		Expected:             expected,
		Current:              current,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal context drift evidence: %w", err)
	}
	return tx.RecordEvidence(id, EvidenceContextDrift, reason, raw)
}

func sameStringList(a []string, b []string) bool {
	a = normalizeOwnedPaths(a)
	b = normalizeOwnedPaths(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
