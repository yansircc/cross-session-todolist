package cst

import (
	"encoding/json"
	"fmt"
)

type AcceptanceRunSetData struct {
	AcceptanceDigest  string                      `json:"acceptance_digest"`
	ExecutionContext  *AcceptanceExecutionContext `json:"execution_context,omitempty"`
	ContextDigest     string                      `json:"context_digest,omitempty"`
	ExecContextDigest string                      `json:"exec_context_digest,omitempty"`
	StoreID           string                      `json:"store_id,omitempty"`
	ExecCWD           string                      `json:"exec_cwd,omitempty"`
	GitIdentityDigest string                      `json:"git_identity_digest,omitempty"`
	Checks            []AcceptanceRunSetCheck     `json:"checks"`
}

type AcceptanceExecutionContext struct {
	StoreID              string   `json:"store_id,omitempty"`
	ExecCWD              string   `json:"exec_cwd,omitempty"`
	ExecSurface          string   `json:"exec_surface,omitempty"`
	OwnedPaths           []string `json:"owned_paths,omitempty"`
	ScopedDigest         string   `json:"scoped_digest,omitempty"`
	OutOfScopeDigest     string   `json:"out_of_scope_digest,omitempty"`
	OutOfScopeDeltaCount int      `json:"out_of_scope_delta_count"`
	WholeRepoDigest      string   `json:"whole_repo_digest,omitempty"`
	GitAvailable         bool     `json:"git_available"`
	GitIdentityDigest    string   `json:"git_identity_digest,omitempty"`
}

type AcceptanceRunSetCheck struct {
	Name             string `json:"name"`
	Cmd              string `json:"cmd"`
	ScriptRunEventID string `json:"script_run_event_id"`
}

func acceptanceDigest(checks []VerifyCheck) string {
	b, _ := json.Marshal(struct {
		Checks []VerifyCheck `json:"checks"`
	}{Checks: cloneVerifyChecks(checks)})
	return sha256Hex(b)
}

func buildAcceptanceRunSetData(checks []VerifyCheck, runs []*Event) (AcceptanceRunSetData, error) {
	if len(checks) == 0 {
		return AcceptanceRunSetData{}, fmt.Errorf("acceptance_run_set requires checks")
	}
	if len(checks) != len(runs) {
		return AcceptanceRunSetData{}, fmt.Errorf("acceptance_run_set needs %d runs, got %d", len(checks), len(runs))
	}
	data := AcceptanceRunSetData{
		AcceptanceDigest: acceptanceDigest(checks),
		Checks:           make([]AcceptanceRunSetCheck, 0, len(checks)),
	}
	for i, check := range checks {
		run := runs[i]
		if run.Type != EvScriptRun {
			return AcceptanceRunSetData{}, fmt.Errorf("acceptance_run_set references non-run event %s", run.EventID)
		}
		if run.Trigger != TriggerAcceptance {
			return AcceptanceRunSetData{}, fmt.Errorf("run %s trigger is %s, not acceptance", run.EventID, run.Trigger)
		}
		if run.ExitCode != 0 {
			return AcceptanceRunSetData{}, fmt.Errorf("run %s exit=%d", run.EventID, run.ExitCode)
		}
		if normalizedCheckName(run.CheckName) != check.Name || run.Cmd != check.Cmd {
			return AcceptanceRunSetData{}, fmt.Errorf("run %s does not match check %s", run.EventID, check.Name)
		}
		digest := run.ExecContextDigest
		if digest == "" {
			gitAvailable := false
			if run.GitAvailable != nil {
				gitAvailable = *run.GitAvailable
			}
			digest = execContextDigest(run.StoreID, ExecIdentity{
				ExecCWD:           run.ExecCWD,
				GitAvailable:      gitAvailable,
				GitIdentityDigest: run.GitIdentityDigest,
				ParallelWorktree:  run.ParallelWorktree,
			})
		}
		if i == 0 {
			data.ExecContextDigest = digest
			data.ContextDigest = digest
			data.StoreID = run.StoreID
			data.ExecCWD = run.ExecCWD
			data.GitIdentityDigest = run.GitIdentityDigest
			ctx := executionContextFromRun(run)
			data.ExecutionContext = &ctx
		} else if data.ExecContextDigest != digest {
			return AcceptanceRunSetData{}, fmt.Errorf("run %s has mixed exec context", run.EventID)
		}
		data.Checks = append(data.Checks, AcceptanceRunSetCheck{
			Name:             check.Name,
			Cmd:              check.Cmd,
			ScriptRunEventID: run.EventID,
		})
	}
	return data, nil
}

func marshalAcceptanceRunSetData(data AcceptanceRunSetData) json.RawMessage {
	b, _ := json.Marshal(data)
	return append(json.RawMessage(nil), b...)
}

func parseAcceptanceRunSetData(raw json.RawMessage) (AcceptanceRunSetData, error) {
	var data AcceptanceRunSetData
	if len(raw) == 0 {
		return data, fmt.Errorf("acceptance_run_set missing data")
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, fmt.Errorf("acceptance_run_set data must be JSON object: %w", err)
	}
	if data.AcceptanceDigest == "" {
		return data, fmt.Errorf("acceptance_run_set missing acceptance_digest")
	}
	if data.ExecContextDigest == "" && data.ContextDigest == "" {
		return data, fmt.Errorf("acceptance_run_set missing context digest")
	}
	if data.ExecContextDigest == "" {
		data.ExecContextDigest = data.ContextDigest
	}
	if data.ContextDigest == "" {
		data.ContextDigest = data.ExecContextDigest
	}
	if data.ExecutionContext != nil {
		if data.ExecutionContext.ExecSurface == "" {
			data.ExecutionContext.ExecSurface = ExecSurfaceShared
		}
		if data.ExecutionContext.ExecSurface != ExecSurfaceShared && data.ExecutionContext.ExecSurface != ExecSurfacePrivate {
			return data, fmt.Errorf("acceptance_run_set execution_context has invalid exec_surface")
		}
		data.ExecutionContext.OwnedPaths = normalizeOwnedPaths(data.ExecutionContext.OwnedPaths)
	}
	if len(data.Checks) == 0 {
		return data, fmt.Errorf("acceptance_run_set missing checks")
	}
	seen := map[string]bool{}
	for _, check := range data.Checks {
		if check.Name == "" || check.Cmd == "" || check.ScriptRunEventID == "" {
			return data, fmt.Errorf("acceptance_run_set has incomplete check entry")
		}
		if seen[check.Name] {
			return data, fmt.Errorf("acceptance_run_set repeats check %q", check.Name)
		}
		seen[check.Name] = true
	}
	return data, nil
}

func executionContextFromRun(run *Event) AcceptanceExecutionContext {
	gitAvailable := false
	if run.GitAvailable != nil {
		gitAvailable = *run.GitAvailable
	}
	return AcceptanceExecutionContext{
		StoreID:              run.StoreID,
		ExecCWD:              run.ExecCWD,
		ExecSurface:          firstNonEmpty(run.ExecSurface, ExecSurfaceShared),
		OwnedPaths:           normalizeOwnedPaths(run.OwnedPaths),
		ScopedDigest:         run.ScopedDigest,
		OutOfScopeDigest:     run.OutOfScopeDigest,
		OutOfScopeDeltaCount: run.OutOfScopeDeltaCount,
		WholeRepoDigest:      run.WholeRepoDigest,
		GitAvailable:         gitAvailable,
		GitIdentityDigest:    run.GitIdentityDigest,
	}
}

func firstNonEmpty(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
