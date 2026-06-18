package cst

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
	decision, ev := acceptanceContextDecision(tx.state, tx.StoreID(), id, rec, execCWDOverride, tx.now)
	if !decision.Accept {
		return nil, herr(ExitInvariantBroken, "%s", decision.Reason)
	}
	if ev == nil {
		return nil, nil
	}
	return tx.RecordEvidence(id, EvidenceContextDrift, ev.EvidenceSummary, ev.EvidenceData)
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
