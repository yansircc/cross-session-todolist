package cst

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type ClaimsArgs struct {
	Within int64
}

type ClaimsView struct {
	GeneratedAt time.Time            `json:"generated_at"`
	ScopeID     int64                `json:"scope_id,omitempty"`
	Claims      []ClaimEntry         `json:"claims"`
	Overlaps    []PathOverlapWarning `json:"path_overlaps,omitempty"`
}

type ClaimEntry struct {
	NodeID           int64                `json:"node_id"`
	Intent           string               `json:"intent,omitempty"`
	Actor            string               `json:"actor"`
	AttemptID        string               `json:"attempt_id,omitempty"`
	LeaseID          string               `json:"lease_id"`
	LeaseExpiresAt   time.Time            `json:"lease_expires_at"`
	Stale            bool                 `json:"stale"`
	LatestExecCWD    string               `json:"latest_exec_cwd,omitempty"`
	LatestGitHead    string               `json:"latest_git_head,omitempty"`
	LatestGitBranch  string               `json:"latest_git_branch,omitempty"`
	LatestGitStatus  string               `json:"latest_git_status_short,omitempty"`
	LatestDiffDigest string               `json:"latest_git_identity_digest,omitempty"`
	LatestRun        *ScriptRunRecord     `json:"latest_run,omitempty"`
	ExecCWD          string               `json:"exec_cwd,omitempty"`
	ExecSurface      string               `json:"exec_surface,omitempty"`
	OwnedPaths       []string             `json:"owned_paths,omitempty"`
	PathOverlaps     []PathOverlapWarning `json:"path_overlaps,omitempty"`
	Recommendation   string               `json:"recommendation,omitempty"`
}

type PathOverlapWarning struct {
	NodeID          int64    `json:"node_id"`
	OtherNodeID     int64    `json:"other_node_id"`
	ExecCWD         string   `json:"exec_cwd,omitempty"`
	OwnedPaths      []string `json:"owned_paths,omitempty"`
	OtherOwnedPaths []string `json:"other_owned_paths,omitempty"`
}

func BuildClaimsView(s *State, scopeID int64, now time.Time, actor string) (ClaimsView, error) {
	if scopeID != 0 {
		if _, ok := s.Nodes[scopeID]; !ok {
			return ClaimsView{}, fmt.Errorf("node #%d not found", scopeID)
		}
	}
	nodes, _ := s.CurrentClaimsWithin(scopeID, 0)
	view := ClaimsView{GeneratedAt: now, ScopeID: scopeID}
	for _, n := range nodes {
		entry := ClaimEntry{
			NodeID:         n.ID,
			Intent:         n.Intent,
			Actor:          n.Claim.Actor,
			AttemptID:      n.Claim.AttemptID,
			LeaseID:        n.Claim.LeaseID,
			LeaseExpiresAt: n.Claim.LeaseExpiresAt,
			Stale:          now.After(n.Claim.LeaseExpiresAt),
		}
		env := effectiveExecutionEnvelope(n)
		entry.ExecCWD = env.ExecCWD
		entry.ExecSurface = env.ExecSurface
		entry.OwnedPaths = append([]string(nil), env.OwnedPaths...)
		if run := latestRunForAttempt(n, n.Claim.AttemptID); run != nil {
			entry.LatestRun = run
			entry.LatestExecCWD = run.ExecCWD
			entry.LatestGitHead = run.GitHead
			entry.LatestGitBranch = run.GitBranch
			entry.LatestGitStatus = run.GitStatusShort
			entry.LatestDiffDigest = run.GitIdentityDigest
		}
		switch {
		case entry.Stale:
			entry.Recommendation = fmt.Sprintf("inspect with cst show %d; original actor may still renew, other actors must take over explicitly", n.ID)
		case n.Claim.Actor == actor:
			entry.Recommendation = fmt.Sprintf("continue task %d or release it explicitly", n.ID)
		default:
			entry.Recommendation = fmt.Sprintf("inspect with cst show %d; do not release another actor's active claim", n.ID)
		}
		view.Claims = append(view.Claims, entry)
	}
	for i := range view.Claims {
		for j := i + 1; j < len(view.Claims); j++ {
			a := &view.Claims[i]
			b := &view.Claims[j]
			if a.ExecCWD != "" && b.ExecCWD != "" && a.ExecCWD != b.ExecCWD {
				continue
			}
			if !pathsOverlap(a.OwnedPaths, b.OwnedPaths) {
				continue
			}
			warn := PathOverlapWarning{
				NodeID:          a.NodeID,
				OtherNodeID:     b.NodeID,
				ExecCWD:         firstNonEmpty(a.ExecCWD, b.ExecCWD),
				OwnedPaths:      append([]string(nil), a.OwnedPaths...),
				OtherOwnedPaths: append([]string(nil), b.OwnedPaths...),
			}
			view.Overlaps = append(view.Overlaps, warn)
			a.PathOverlaps = append(a.PathOverlaps, warn)
			b.PathOverlaps = append(b.PathOverlaps, PathOverlapWarning{
				NodeID:          b.NodeID,
				OtherNodeID:     a.NodeID,
				ExecCWD:         warn.ExecCWD,
				OwnedPaths:      append([]string(nil), b.OwnedPaths...),
				OtherOwnedPaths: append([]string(nil), a.OwnedPaths...),
			})
		}
	}
	return view, nil
}

func latestRunForAttempt(n *Node, attemptID string) *ScriptRunRecord {
	for i := len(n.Runs) - 1; i >= 0; i-- {
		if attemptID == "" || n.Runs[i].AttemptID == attemptID {
			run := n.Runs[i]
			return &run
		}
	}
	return nil
}

func DoClaims(out io.Writer, args ClaimsArgs, asJSON bool) error {
	return WithStore(TxOpts{Mutating: false, RepairLease: false}, func(tx *Tx) error {
		view, err := BuildClaimsView(tx.state, args.Within, tx.now, tx.actor)
		if err != nil {
			return herr(ExitNotFound, "%s", err.Error())
		}
		if asJSON {
			WriteJSON(out, view)
		} else {
			RenderClaimsText(out, view)
		}
		return nil
	})
}

func DoRecover(out io.Writer, args ClaimsArgs, asJSON bool) error {
	return DoClaims(out, args, asJSON)
}

func RenderClaimsText(w io.Writer, view ClaimsView) {
	if len(view.Claims) == 0 {
		fmt.Fprintln(w, "claims: none")
		return
	}
	fmt.Fprintf(w, "claims: %d\n", len(view.Claims))
	for _, claim := range view.Claims {
		stale := "active"
		if claim.Stale {
			stale = "stale"
		}
		fmt.Fprintf(w, "#%d %s actor=%s attempt=%s lease_until=%s\n",
			claim.NodeID, stale, claim.Actor, claim.AttemptID, claim.LeaseExpiresAt.Format(time.RFC3339))
		if claim.ExecCWD != "" || len(claim.OwnedPaths) > 0 {
			fmt.Fprintf(w, "  envelope: exec_cwd=%s surface=%s scope=%s\n",
				claim.ExecCWD, claim.ExecSurface, strings.Join(claim.OwnedPaths, ","))
		}
		if claim.LatestExecCWD != "" {
			fmt.Fprintf(w, "  latest_exec_cwd=%s git_head=%s git_identity=%s\n",
				claim.LatestExecCWD, claim.LatestGitHead, claim.LatestDiffDigest)
		}
		if claim.Recommendation != "" {
			fmt.Fprintf(w, "  recover: %s\n", claim.Recommendation)
		}
		for _, overlap := range claim.PathOverlaps {
			fmt.Fprintf(w, "  overlap: #%d scope=%s\n",
				overlap.OtherNodeID, strings.Join(overlap.OtherOwnedPaths, ","))
		}
	}
}
