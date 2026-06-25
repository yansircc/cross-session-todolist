package cst

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// ExitCode mirrors the spec's CLI semantics so callers can distinguish kinds
// of failure without parsing stderr.
type ExitCode int

const (
	ExitOK              ExitCode = 0
	ExitGenericError    ExitCode = 1
	ExitAcceptanceFail  ExitCode = 2
	ExitClaimConflict   ExitCode = 3
	ExitInvariantBroken ExitCode = 4
	ExitNotFound        ExitCode = 5
	ExitUsage           ExitCode = 64
)

// HandlerError carries an exit code together with a message.
type HandlerError struct {
	Code ExitCode
	Msg  string
}

func (e *HandlerError) Error() string { return e.Msg }
func herr(code ExitCode, format string, args ...any) error {
	return &HandlerError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// AddArgs covers the union of `cst add` flag combinations.
type AddArgs struct {
	Parent           int64
	Intent           string
	Rule             string
	Goal             bool
	AcceptanceVerify string
	VerifyChecks     []VerifyCheck
	AcceptanceReview string
	After            []int64
	Envelope         *ExecutionEnvelope
	Context          *NodeContext
	Boundary         *NodeBoundary
	ObligationClaims []string
}

type TakeArgs struct {
	Envelope *ExecutionEnvelope
}

type DoneArgs struct {
	EvidenceID       string
	EvidenceIDs      []string
	Note             string
	FromAcceptanceID string
	ExecCWD          string
	CommitSHA        string
}

type RunArgs struct {
	Override   string
	CheckName  string
	Acceptance bool
	ExecCWD    string
}

type WorkerRunArgs struct {
	ActionID  string
	CommitSHA string
}

type EvidenceArgs struct {
	Kind    string
	Summary string
	Data    string
}

type TakeView struct {
	Claim    *Event             `json:"claim"`
	Briefing *DeveloperBriefing `json:"briefing,omitempty"`
}

type EventsArgs struct {
	NodeID       int64
	AttemptID    string
	SinceEventID string
	All          bool
	Raw          bool
}

type ReviseArgs struct {
	ParentSet           bool
	Parent              int64
	Intent              string
	Rule                string
	AcceptanceVerify    string
	VerifyChecks        []VerifyCheck
	AcceptanceReview    string
	EnvelopeSet         bool
	EnvelopePatch       ExecutionEnvelopePatch
	ContextSet          bool
	ContextPatch        NodeContextPatch
	BoundarySet         bool
	BoundaryPatch       NodeBoundaryPatch
	ObligationClaimsSet bool
	ObligationClaims    []string
	AfterSet            bool
	After               []int64
	Reason              string
}

func DoAdd(out io.Writer, args AddArgs, asJSON bool) error {
	var emitted *Event
	err := WithStore(TxOpts{Mutating: true, RepairLease: false}, func(tx *Tx) error {
		if args.Rule != "" {
			if args.Context != nil || args.Boundary != nil || len(args.ObligationClaims) > 0 {
				return herr(ExitUsage, "rule nodes use --rule; context, boundary, and obligation claims apply to goals/tasks")
			}
			ev, err := tx.CreateRule(args.Parent, args.Rule)
			if err != nil {
				return err
			}
			emitted = ev
			return nil
		}
		if args.Parent == 0 && args.AcceptanceVerify == "" && len(args.VerifyChecks) == 0 && args.AcceptanceReview == "" && len(args.After) == 0 {
			if len(args.ObligationClaims) > 0 {
				return herr(ExitUsage, "obligation claims belong to task acceptance")
			}
			ev, err := tx.CreateGoal(0, args.Intent, args.Context, args.Boundary)
			if err != nil {
				return err
			}
			emitted = ev
			return nil
		}
		if args.Goal {
			if len(args.ObligationClaims) > 0 {
				return herr(ExitUsage, "obligation claims belong to task acceptance")
			}
			ev, err := tx.CreateGoal(args.Parent, args.Intent, args.Context, args.Boundary)
			if err != nil {
				return err
			}
			emitted = ev
			return nil
		}
		acceptance, err := buildAcceptance(args)
		if err != nil {
			return err
		}
		ev, err := tx.CreateTask(args.Parent, args.Intent, acceptance, args.After, args.Envelope, args.Context, args.Boundary, args.ObligationClaims)
		if err != nil {
			return err
		}
		emitted = ev
		return nil
	})
	if err != nil {
		return err
	}
	emitNodeCreated(out, asJSON, emitted)
	return nil
}

func buildAcceptance(args AddArgs) (*Acceptance, error) {
	count := 0
	if args.AcceptanceVerify != "" {
		count++
	}
	if len(args.VerifyChecks) > 0 {
		count++
	}
	if args.AcceptanceReview != "" {
		count++
	}
	if count > 1 {
		return nil, herr(ExitUsage, "use only one of --verify / --check / --review")
	}
	switch {
	case args.AcceptanceVerify != "":
		return NewVerifyAcceptance(args.AcceptanceVerify), nil
	case len(args.VerifyChecks) > 0:
		return NewVerifyChecksAcceptance(args.VerifyChecks), nil
	case args.AcceptanceReview != "":
		return &Acceptance{Kind: AcceptanceReview, Who: args.AcceptanceReview}, nil
	default:
		return nil, herr(ExitUsage,
			"task requires acceptance (--verify <cmd> | --check <name=cmd> | --review <who>)")
	}
}

func buildOptionalAcceptance(verify string, checks []VerifyCheck, review string) (*Acceptance, error) {
	count := 0
	if verify != "" {
		count++
	}
	if len(checks) > 0 {
		count++
	}
	if review != "" {
		count++
	}
	if count > 1 {
		return nil, herr(ExitUsage, "use only one of --verify / --check / --review")
	}
	switch {
	case verify != "":
		return NewVerifyAcceptance(verify), nil
	case len(checks) > 0:
		return NewVerifyChecksAcceptance(checks), nil
	case review != "":
		return &Acceptance{Kind: AcceptanceReview, Who: review}, nil
	default:
		return nil, nil
	}
}

func DoRevise(out io.Writer, id int64, args ReviseArgs, asJSON bool) error {
	acceptance, err := buildOptionalAcceptance(args.AcceptanceVerify, args.VerifyChecks, args.AcceptanceReview)
	if err != nil {
		return err
	}
	var emitted *Event
	err = WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
		ev, err := tx.ReviseNode(id, ReviseSpec{
			ParentSet:           args.ParentSet,
			Parent:              args.Parent,
			Intent:              args.Intent,
			RuleText:            args.Rule,
			Acceptance:          acceptance,
			EnvelopeSet:         args.EnvelopeSet,
			EnvelopePatch:       args.EnvelopePatch,
			ContextSet:          args.ContextSet,
			ContextPatch:        args.ContextPatch,
			BoundarySet:         args.BoundarySet,
			BoundaryPatch:       args.BoundaryPatch,
			ObligationClaimsSet: args.ObligationClaimsSet,
			ObligationClaims:    args.ObligationClaims,
			AfterSet:            args.AfterSet,
			After:               args.After,
			Reason:              args.Reason,
		})
		if err != nil {
			return err
		}
		emitted = ev
		return nil
	})
	if err != nil {
		return err
	}
	if asJSON {
		WriteJSON(out, emitted)
	} else {
		fmt.Fprintf(out, "revised #%d\n", id)
	}
	return nil
}

func emitNodeCreated(out io.Writer, asJSON bool, ev *Event) {
	if asJSON {
		WriteJSON(out, ev)
		return
	}
	if ev.Kind == KindRule {
		fmt.Fprintf(out, "created rule #%d under #%d\n", ev.NodeID, ev.ParentID)
		return
	}
	if ev.Kind == KindGoal {
		if ev.ParentID == 0 {
			fmt.Fprintf(out, "created root goal #%d\n", ev.NodeID)
		} else {
			fmt.Fprintf(out, "created goal #%d under #%d\n", ev.NodeID, ev.ParentID)
		}
		return
	}
	acceptance := ""
	if ev.Acceptance != nil {
		acceptance = " acceptance=" + ev.Acceptance.Kind
	}
	fmt.Fprintf(out, "created task #%d (parent=#%d%s)\n", ev.NodeID, ev.ParentID, acceptance)
}

func DoHold(out io.Writer, id int64, kind string, reason string, clear bool, asJSON bool) error {
	var emitted *Event
	err := WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
		var ev *Event
		var err error
		if clear {
			ev, err = tx.ClearHold(id)
		} else {
			ev, err = tx.HoldNode(id, kind, reason)
		}
		if err != nil {
			return err
		}
		emitted = ev
		return nil
	})
	if err != nil {
		return err
	}
	if asJSON {
		WriteJSON(out, emitted)
	} else if clear {
		fmt.Fprintf(out, "#%d hold cleared\n", id)
	} else {
		fmt.Fprintf(out, "#%d held kind=%s\n", id, kind)
	}
	return nil
}

func DoEvidence(out io.Writer, id int64, args EvidenceArgs, asJSON bool) error {
	data, err := parseEvidenceData(args.Data)
	if err != nil {
		return err
	}
	var emitted *Event
	err = WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
		ev, err := tx.RecordEvidence(id, args.Kind, args.Summary, data)
		if err != nil {
			return err
		}
		emitted = ev
		return nil
	})
	if err != nil {
		return err
	}
	if asJSON {
		WriteJSON(out, emitted)
	} else {
		fmt.Fprintf(out, "recorded evidence %s on #%d\n", emitted.EventID, id)
	}
	return nil
}

func parseEvidenceData(raw string) (json.RawMessage, error) {
	if raw == "" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, herr(ExitUsage, "--data must be valid JSON: %v", err)
	}
	return append(json.RawMessage(nil), []byte(raw)...), nil
}

func DoTake(out io.Writer, id int64, asJSON bool) error {
	return DoTakeWithArgs(out, id, TakeArgs{}, asJSON)
}

func DoTakeWithArgs(out io.Writer, id int64, args TakeArgs, asJSON bool) error {
	var emitted *Event
	var briefing *DeveloperBriefing
	err := WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
		target := id
		if target == 0 {
			head := tx.Snapshot().HeadOpenTasks(1)
			if len(head) == 0 {
				return herr(ExitNotFound, "no open task available to take")
			}
			target = head[0].ID
		}
		if args.Envelope != nil {
			if _, err := tx.ReviseNode(target, ReviseSpec{
				EnvelopeSet: true,
				EnvelopePatch: ExecutionEnvelopePatch{
					ExecCWDSet:     true,
					ExecCWD:        args.Envelope.ExecCWD,
					ExecSurfaceSet: true,
					ExecSurface:    args.Envelope.ExecSurface,
					OwnedPathsSet:  true,
					OwnedPaths:     args.Envelope.OwnedPaths,
				},
				Reason: "bind execution envelope for claim",
			}); err != nil {
				return err
			}
		}
		ev, err := tx.TakeClaim(target)
		if err != nil {
			return err
		}
		emitted = ev
		briefing = BuildDeveloperBriefing(tx.Snapshot(), target)
		return nil
	})
	if err != nil {
		return err
	}
	if asJSON {
		WriteJSON(out, TakeView{Claim: emitted, Briefing: briefing})
	} else {
		fmt.Fprintf(out, "took #%d (actor=%s, lease until %s)\n",
			emitted.NodeID, emitted.Actor, emitted.LeaseExpiresAt.Format(time.RFC3339))
		RenderDeveloperBriefingText(out, briefing)
	}
	return nil
}

func DoRelease(out io.Writer, id int64, asJSON bool) error {
	var emitted *Event
	err := WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
		ev, err := tx.ReleaseClaim(id)
		if err != nil {
			return err
		}
		emitted = ev
		return nil
	})
	if err != nil {
		return err
	}
	if asJSON {
		WriteJSON(out, emitted)
	} else {
		fmt.Fprintf(out, "released #%d\n", id)
	}
	return nil
}

// DoRun runs the task acceptance command (or override) without changing status.
// If the caller holds the claim, the lease is renewed periodically while the
// command runs.
func DoRun(out io.Writer, id int64, override string, checkName string, asJSON bool) error {
	return DoRunWithArgs(out, id, RunArgs{Override: override, CheckName: checkName}, asJSON)
}

func DoRunWithArgs(out io.Writer, id int64, args RunArgs, asJSON bool) error {
	if args.Acceptance {
		if args.Override != "" || args.CheckName != "" {
			return herr(ExitUsage, "run --acceptance cannot be combined with --cmd or --check")
		}
		return runAcceptanceWithoutCompletion(out, id, args.ExecCWD, asJSON)
	}
	var (
		cmdToRun         string
		label            string
		leaseID          string
		cfg              Config
		paths            StorePaths
		storeID          string
		env              ExecutionEnvelope
		effectiveExecCWD string
	)
	err := WithStore(TxOpts{Mutating: false, RepairLease: true}, func(tx *Tx) error {
		n, err := tx.requireOpenTask(id)
		if err != nil {
			return err
		}
		cfg = tx.cfg
		paths = tx.StorePaths()
		storeID = tx.StoreID()
		env = effectiveExecutionEnvelope(n)
		effectiveExecCWD = resolveExecCWD(args.ExecCWD, env)
		if n.Claim != nil && n.Claim.Actor == tx.actor {
			leaseID = n.Claim.LeaseID
		}
		if args.Override != "" {
			cmdToRun = args.Override
			label = args.CheckName
			return nil
		}
		if n.Acceptance == nil || n.Acceptance.Kind != AcceptanceVerify {
			return herr(ExitUsage, "task #%d has no verify acceptance; pass --cmd", id)
		}
		check, err := selectVerifyCheck(n.Acceptance.VerifyChecks(), args.CheckName)
		if err != nil {
			return err
		}
		cmdToRun = check.Cmd
		label = check.Name
		return nil
	})
	if err != nil {
		return err
	}
	actor := ResolveActor(cfg.ActorDefault)
	var renew LeaseRenewer
	if leaseID != "" {
		renew = func(t time.Time) error {
			return renewClaimUnderLock(paths, actor, id, leaseID, cfg.ClaimLeaseTTL)
		}
	}
	res := runScript(cfg, RunOpts{
		EventID:     NewEventID(),
		Cmd:         cmdToRun,
		Trigger:     TriggerProbe,
		CheckName:   label,
		ExecCWD:     effectiveExecCWD,
		StoreID:     storeID,
		ArtifactDir: paths.RunArtifactsDir(),
		Envelope:    env,
		Renew:       renew,
		RenewEvery:  cfg.ClaimRenewEvery,
	})

	if err := WithStore(TxOpts{Mutating: true, RepairLease: false}, func(tx *Tx) error {
		_, e := tx.RecordScriptRun(id, res)
		return e
	}); err != nil {
		return err
	}
	emitRun(out, asJSON, id, res)
	if res.ExitCode != 0 {
		return &HandlerError{Code: ExitAcceptanceFail, Msg: fmt.Sprintf("script exit=%d", res.ExitCode)}
	}
	if res.ArtifactError != nil {
		return res.ArtifactError
	}
	return nil
}

func DoWorkerStatus(out io.Writer, id int64, asJSON bool) error {
	return WithStore(TxOpts{Mutating: false, RepairLease: true}, func(tx *Tx) error {
		view, err := BuildWorkerStatus(FrontierInputFromTx(tx, id))
		if err != nil {
			return err
		}
		if asJSON {
			WriteJSON(out, view)
		} else {
			RenderWorkerStatusText(out, view)
		}
		return nil
	})
}

func DoWorkerRun(out io.Writer, id int64, args WorkerRunArgs, asJSON bool) error {
	if args.ActionID == "" {
		return herr(ExitUsage, "worker-run requires --action <action-id>")
	}
	if args.CommitSHA != "" && !validCommitSHA(args.CommitSHA) {
		return herr(ExitUsage, "--commit requires a git commit sha")
	}
	var selected BoundAction
	err := WithStore(TxOpts{Mutating: false, RepairLease: true}, func(tx *Tx) error {
		input := FrontierInputFromTx(tx, id)
		for _, action := range LegalFrontier(input) {
			if action.ActionID == args.ActionID {
				selected = action
				return nil
			}
		}
		return herr(ExitInvariantBroken, "worker action %s is not in the current frontier; rerun `cst worker-status %d`", args.ActionID, id)
	})
	if err != nil {
		return err
	}
	switch selected.Kind {
	case ActionTakeReadyTask:
		return DoTake(out, id, asJSON)
	case ActionRunAcceptance:
		return DoRunWithArgs(out, id, RunArgs{Acceptance: true, ExecCWD: selected.ExecCWD}, asJSON)
	case ActionCompleteFromAcceptance:
		return DoDone(out, id, DoneArgs{FromAcceptanceID: selected.EvidenceID, CommitSHA: args.CommitSHA}, asJSON)
	case ActionCompleteReviewWithEvidence:
		return DoDone(out, id, DoneArgs{EvidenceID: selected.EvidenceID, CommitSHA: args.CommitSHA}, asJSON)
	default:
		return herr(ExitInvariantBroken, "worker action %s has unsupported kind %q", selected.ActionID, selected.Kind)
	}
}

func emitRun(out io.Writer, asJSON bool, id int64, res RunResult) {
	if asJSON {
		WriteJSON(out, res)
		return
	}
	flag := ""
	if res.TimedOut {
		flag = " (timed out)"
	}
	check := ""
	if res.CheckName != "" {
		check = " check=" + res.CheckName
	}
	fmt.Fprintf(out, "#%d run trigger=%s%s exit=%d dur=%dms%s\n",
		id, res.Trigger, check, res.ExitCode, res.DurationMs, flag)
	if res.StderrHead != "" {
		fmt.Fprintln(out, "stderr:")
		fmt.Fprintln(out, indent(res.StderrHead, "  "))
	}
}

func emitRuns(out io.Writer, asJSON bool, id int64, results []RunResult) {
	if asJSON {
		WriteJSON(out, results)
		return
	}
	for _, result := range results {
		emitRun(out, false, id, result)
	}
}

func emitAcceptanceRunSet(out io.Writer, asJSON bool, id int64, evidence *Event, results []RunResult) {
	if asJSON {
		WriteJSON(out, struct {
			Evidence *Event      `json:"evidence"`
			Runs     []RunResult `json:"runs,omitempty"`
		}{evidence, results})
		return
	}
	fmt.Fprintf(out, "#%d acceptance_run_set evidence=%s\n", id, evidence.EventID)
}

func selectVerifyCheck(checks []VerifyCheck, name string) (VerifyCheck, error) {
	if len(checks) == 0 {
		return VerifyCheck{}, herr(ExitInvariantBroken, "verify acceptance has no checks")
	}
	if name == "" {
		if len(checks) == 1 {
			return checks[0], nil
		}
		return VerifyCheck{}, herr(ExitUsage, "task has multiple verify checks; pass --check <name>")
	}
	for _, check := range checks {
		if check.Name == name {
			return check, nil
		}
	}
	return VerifyCheck{}, herr(ExitNotFound, "verify check %q not found", name)
}

type verifyRunContext struct {
	ExecCWD     string
	StoreID     string
	ArtifactDir string
	Envelope    ExecutionEnvelope
}

func runVerifyChecks(cfg Config, checks []VerifyCheck, renew LeaseRenewer, runCtx verifyRunContext) []RunResult {
	results := make([]RunResult, 0, len(checks))
	for _, check := range checks {
		res := runScript(cfg, RunOpts{
			EventID:     NewEventID(),
			Cmd:         check.Cmd,
			Trigger:     TriggerAcceptance,
			CheckName:   check.Name,
			ExecCWD:     runCtx.ExecCWD,
			StoreID:     runCtx.StoreID,
			ArtifactDir: runCtx.ArtifactDir,
			Envelope:    runCtx.Envelope,
			Renew:       renew,
			RenewEvery:  cfg.ClaimRenewEvery,
		})
		results = append(results, res)
	}
	return results
}

func firstArtifactError(results []RunResult) error {
	for _, result := range results {
		if result.ArtifactError != nil {
			return result.ArtifactError
		}
	}
	return nil
}

func firstFailedRun(results []RunResult) *RunResult {
	for i := range results {
		if results[i].ExitCode != 0 {
			return &results[i]
		}
	}
	return nil
}

func recordScriptRunsOnly(id int64, results []RunResult) error {
	return WithStore(TxOpts{Mutating: true, RepairLease: false}, func(tx *Tx) error {
		for _, result := range results {
			if _, err := tx.RecordScriptRun(id, result); err != nil {
				return err
			}
		}
		return nil
	})
}

func runAcceptanceWithoutCompletion(out io.Writer, id int64, execCWD string, asJSON bool) error {
	var (
		guard            CompletionGuard
		cfg              Config
		paths            StorePaths
		storeID          string
		env              ExecutionEnvelope
		effectiveExecCWD string
		actor            string
	)
	if err := WithStore(TxOpts{Mutating: true, RepairLease: false}, func(tx *Tx) error {
		if _, err := tx.RenewExecutionClaim(id); err != nil {
			return err
		}
		g, err := tx.PrepareCompletionGuard(id)
		if err != nil {
			return err
		}
		if g.AcceptanceKind != AcceptanceVerify {
			return herr(ExitUsage, "run --acceptance is only valid for verify acceptance")
		}
		guard = g
		cfg = tx.cfg
		paths = tx.StorePaths()
		storeID = tx.StoreID()
		actor = tx.actor
		env = effectiveExecutionEnvelope(tx.Snapshot().Nodes[id])
		effectiveExecCWD = resolveExecCWD(execCWD, env)
		return nil
	}); err != nil {
		return err
	}
	var renew LeaseRenewer
	if guard.ClaimLeaseID != "" {
		renew = func(t time.Time) error {
			return renewClaimUnderLock(paths, actor, id, guard.ClaimLeaseID, cfg.ClaimLeaseTTL)
		}
	}
	results := runVerifyChecks(cfg, guard.VerifyChecks, renew, verifyRunContext{
		ExecCWD:     effectiveExecCWD,
		StoreID:     storeID,
		ArtifactDir: paths.RunArtifactsDir(),
		Envelope:    env,
	})
	failed := firstFailedRun(results)
	artifactErr := firstArtifactError(results)
	if failed != nil || artifactErr != nil {
		_ = recordScriptRunsOnly(id, results)
		emitRuns(out, asJSON, id, results)
		if artifactErr != nil {
			return artifactErr
		}
		return &HandlerError{Code: ExitAcceptanceFail,
			Msg: fmt.Sprintf("acceptance failed (check=%s exit=%d)", normalizedCheckName(failed.CheckName), failed.ExitCode)}
	}
	var evidence *Event
	err := WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
		current, err := tx.PrepareCompletionGuard(id)
		if err != nil {
			return err
		}
		if !sameCompletionGuard(guard, current) {
			return herr(ExitClaimConflict, "task #%d claim or acceptance changed under acceptance run", id)
		}
		runEvents := make([]*Event, 0, len(results))
		for _, result := range results {
			ev, err := tx.RecordScriptRun(id, result)
			if err != nil {
				return err
			}
			runEvents = append(runEvents, ev)
		}
		ev, err := tx.RecordAcceptanceRunSet(id, guard.VerifyChecks, runEvents)
		if err != nil {
			return err
		}
		evidence = ev
		return nil
	})
	if err != nil {
		if isHandlerErr(err) {
			_ = recordScriptRunsOnly(id, results)
		}
		emitRuns(out, asJSON, id, results)
		return err
	}
	emitAcceptanceRunSet(out, asJSON, id, evidence, results)
	return nil
}

func sameCompletionGuard(a, b CompletionGuard) bool {
	return a.NodeID == b.NodeID &&
		a.AcceptanceKind == b.AcceptanceKind &&
		a.ClaimLeaseID == b.ClaimLeaseID &&
		a.ClaimAttemptID == b.ClaimAttemptID &&
		sameVerifyChecks(a.VerifyChecks, b.VerifyChecks)
}

// DoDone completes a task. It is the only path that writes task_completed.
// For verify acceptances, it executes outside the lock and re-validates under a
// fresh transaction before committing — closing the TOCTOU window where
// cancel/release could race the shell run.
func DoDone(out io.Writer, id int64, args DoneArgs, asJSON bool) error {
	args.EvidenceIDs = normalizeEvidenceIDs(append(args.EvidenceIDs, args.EvidenceID))
	if len(args.EvidenceIDs) > 0 && args.Note != "" {
		return herr(ExitUsage, "use only one of --evidence / --note")
	}
	if args.FromAcceptanceID != "" && (args.Note != "" || args.ExecCWD != "") {
		return herr(ExitUsage, "--from-acceptance cannot be combined with --note or --exec-cwd")
	}
	if args.CommitSHA != "" && !validCommitSHA(args.CommitSHA) {
		return herr(ExitUsage, "--commit requires a git commit sha")
	}
	var (
		guard            CompletionGuard
		cfg              Config
		paths            StorePaths
		storeID          string
		env              ExecutionEnvelope
		effectiveExecCWD string
		actor            string
	)
	// Phase 1: validate, capture guard, release lock.
	err := WithStore(TxOpts{Mutating: true, RepairLease: false}, func(tx *Tx) error {
		if _, err := tx.RenewExecutionClaim(id); err != nil {
			return err
		}
		g, err := tx.PrepareCompletionGuard(id)
		if err != nil {
			return err
		}
		guard = g
		cfg = tx.cfg
		paths = tx.StorePaths()
		storeID = tx.StoreID()
		env = effectiveExecutionEnvelope(tx.Snapshot().Nodes[id])
		effectiveExecCWD = resolveExecCWD(args.ExecCWD, env)
		actor = tx.actor
		return nil
	})
	if err != nil {
		return err
	}
	if guard.AcceptanceKind == AcceptanceVerify && args.FromAcceptanceID == "" && (len(args.EvidenceIDs) > 0 || args.Note != "") {
		return herr(ExitUsage,
			"verify acceptance records the successful acceptance run as evidence; do not pass --evidence or --note")
	}
	if args.FromAcceptanceID != "" {
		if guard.AcceptanceKind != AcceptanceVerify {
			return herr(ExitUsage, "--from-acceptance is only valid for verify acceptance")
		}
		var emitted *Event
		err := WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
			evidenceIDs := append([]string{args.FromAcceptanceID}, args.EvidenceIDs...)
			if ev, err := validateAcceptanceContextForCompletion(tx, id, args.FromAcceptanceID, ""); err != nil {
				return err
			} else if ev != nil {
				evidenceIDs = append(evidenceIDs, ev.EventID)
			}
			if evID, err := recordCommitEvidenceIfRequested(tx, id, args.CommitSHA); err != nil {
				return err
			} else if evID != "" {
				evidenceIDs = append(evidenceIDs, evID)
			}
			ev, err := tx.CompleteTask(guard, evidenceIDs)
			if err != nil {
				return err
			}
			emitted = ev
			return nil
		})
		if err != nil {
			return err
		}
		emitDone(out, asJSON, emitted, nil)
		return nil
	}

	// Non-shell acceptances complete inside one fresh transaction.
	if guard.AcceptanceKind != AcceptanceVerify {
		var emitted *Event
		err := WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
			evidenceIDs := append([]string(nil), args.EvidenceIDs...)
			if args.Note != "" {
				ev, err := tx.RecordEvidence(id, EvidenceNote, args.Note, nil)
				if err != nil {
					return err
				}
				evidenceIDs = append(evidenceIDs, ev.EventID)
			}
			if evID, err := recordCommitEvidenceIfRequested(tx, id, args.CommitSHA); err != nil {
				return err
			} else if evID != "" {
				evidenceIDs = append(evidenceIDs, evID)
			}
			ev, err := tx.CompleteTask(guard, evidenceIDs)
			if err != nil {
				return err
			}
			emitted = ev
			return nil
		})
		if err != nil {
			return err
		}
		emitDone(out, asJSON, emitted, nil)
		return nil
	}

	// Phase 2: run the verify shell with no lock held.
	var renew LeaseRenewer
	if guard.ClaimLeaseID != "" {
		renew = func(t time.Time) error {
			return renewClaimUnderLock(paths, actor, id, guard.ClaimLeaseID, cfg.ClaimLeaseTTL)
		}
	}
	results := runVerifyChecks(cfg, guard.VerifyChecks, renew, verifyRunContext{
		ExecCWD:     effectiveExecCWD,
		StoreID:     storeID,
		ArtifactDir: paths.RunArtifactsDir(),
		Envelope:    env,
	})
	failed := firstFailedRun(results)
	artifactErr := firstArtifactError(results)

	// Phase 3: under a fresh tx, always record the runs, complete only if
	// acceptance passed AND guard preconditions still hold.
	if failed != nil || artifactErr != nil {
		_ = recordScriptRunsOnly(id, results)
		emitRuns(out, asJSON, id, results)
		if artifactErr != nil {
			return artifactErr
		}
		return &HandlerError{Code: ExitAcceptanceFail,
			Msg: fmt.Sprintf("acceptance failed (check=%s exit=%d)", normalizedCheckName(failed.CheckName), failed.ExitCode)}
	}

	var emitted *Event
	var runSetEvent *Event
	err = WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
		current, err := tx.PrepareCompletionGuard(id)
		if err != nil {
			return err
		}
		if !sameCompletionGuard(guard, current) {
			return herr(ExitClaimConflict, "task #%d claim or acceptance changed under acceptance run", id)
		}
		runEvents := make([]*Event, 0, len(results))
		for _, result := range results {
			ev, err := tx.RecordScriptRun(id, result)
			if err != nil {
				return err
			}
			runEvents = append(runEvents, ev)
		}
		ev, err := tx.RecordAcceptanceRunSet(id, guard.VerifyChecks, runEvents)
		if err != nil {
			return err
		}
		runSetEvent = ev
		evidenceIDs := []string{runSetEvent.EventID}
		if ev, err := validateAcceptanceContextForCompletion(tx, id, runSetEvent.EventID, effectiveExecCWD); err != nil {
			return err
		} else if ev != nil {
			evidenceIDs = append(evidenceIDs, ev.EventID)
		}
		if evID, err := recordCommitEvidenceIfRequested(tx, id, args.CommitSHA); err != nil {
			return err
		} else if evID != "" {
			evidenceIDs = append(evidenceIDs, evID)
		}
		ev, err = tx.CompleteTask(guard, evidenceIDs)
		if err != nil {
			return err
		}
		emitted = ev
		return nil
	})
	if err != nil {
		// RecordScriptRun ran before CompleteTask in the same Tx, but Tx commits
		// all-or-nothing on success. Preserve the command facts if completion
		// races with cancel/release/lease-expiry after the shell work finished.
		if isHandlerErr(err) {
			_ = recordScriptRunsOnly(id, results)
		}
		emitRuns(out, asJSON, id, results)
		return err
	}
	emitDone(out, asJSON, emitted, results)
	return nil
}

func isHandlerErr(err error) bool {
	_, ok := err.(*HandlerError)
	return ok
}

var commitSHARE = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

func validCommitSHA(value string) bool {
	return commitSHARE.MatchString(strings.TrimSpace(value))
}

func recordCommitEvidenceIfRequested(tx *Tx, id int64, sha string) (string, error) {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return "", nil
	}
	raw, err := json.Marshal(struct {
		SHA string `json:"sha"`
	}{SHA: sha})
	if err != nil {
		return "", err
	}
	ev, err := tx.RecordEvidence(id, EvidenceCommit, "commit "+sha, raw)
	if err != nil {
		return "", err
	}
	return ev.EventID, nil
}

func emitDone(out io.Writer, asJSON bool, ev *Event, runs []RunResult) {
	if asJSON {
		var run *RunResult
		if len(runs) == 1 {
			run = &runs[0]
		}
		WriteJSON(out, struct {
			Completed *Event      `json:"completed"`
			Run       *RunResult  `json:"run,omitempty"`
			Runs      []RunResult `json:"runs,omitempty"`
		}{ev, run, runs})
		return
	}
	fmt.Fprintf(out, "#%d completed\n", ev.NodeID)
}

func DoCancel(out io.Writer, id int64, reason string, asJSON bool) error {
	var emitted *Event
	err := WithStore(TxOpts{Mutating: true, RepairLease: true}, func(tx *Tx) error {
		ev, err := tx.CancelNode(id, reason)
		if err != nil {
			return err
		}
		emitted = ev
		return nil
	})
	if err != nil {
		return err
	}
	if asJSON {
		WriteJSON(out, emitted)
	} else {
		fmt.Fprintf(out, "#%d canceled\n", id)
	}
	return nil
}

func DoBrief(out io.Writer, scopeID int64, asJSON bool) error {
	return DoBriefWithOptions(out, BriefOptions{ScopeID: scopeID}, asJSON)
}

func DoBriefWithOptions(out io.Writer, opts BriefOptions, asJSON bool) error {
	return WithStore(TxOpts{Mutating: false, RepairLease: true}, func(tx *Tx) error {
		bv, err := BuildBriefWithOptions(tx.state, tx.cfg, tx.actor, opts)
		if err != nil {
			return herr(ExitNotFound, "%s", err.Error())
		}
		if asJSON {
			WriteJSON(out, bv)
		} else {
			RenderBriefText(out, bv)
		}
		return nil
	})
}

func DoShow(out io.Writer, id int64, asJSON bool) error {
	return WithStore(TxOpts{Mutating: false, RepairLease: true}, func(tx *Tx) error {
		v, err := BuildShow(tx.state, id, tx.cfg)
		if err != nil {
			return herr(ExitNotFound, "%s", err.Error())
		}
		if asJSON {
			WriteJSON(out, v)
		} else {
			RenderShowText(out, v)
		}
		return nil
	})
}

func DoEvents(out io.Writer, args EventsArgs, asJSON bool) error {
	if args.All {
		if args.NodeID != 0 || args.AttemptID != "" || args.SinceEventID != "" {
			return herr(ExitUsage, "events --all cannot be combined with --for, --attempt, or --since")
		}
		if !args.Raw {
			return herr(ExitUsage, "events --all requires --raw")
		}
	} else if args.NodeID == 0 && args.AttemptID == "" && args.SinceEventID == "" {
		return herr(ExitUsage, "events requires --for <id>, --attempt <id>, --since <event-id>, or --all --raw")
	}

	events, err := Replay()
	if err != nil {
		return err
	}

	if args.NodeID != 0 || args.AttemptID != "" {
		state, err := Apply(events)
		if err != nil {
			return err
		}
		if args.NodeID != 0 {
			if _, ok := state.Nodes[args.NodeID]; !ok {
				return herr(ExitNotFound, "node #%d not found", args.NodeID)
			}
		}
		if args.AttemptID != "" {
			if _, ok := state.Attempts[args.AttemptID]; !ok {
				return herr(ExitNotFound, "attempt %s not found", args.AttemptID)
			}
		}
	}

	if args.NodeID != 0 && args.AttemptID != "" {
		state, err := Apply(events)
		if err != nil {
			return err
		}
		attempt := state.Attempts[args.AttemptID]
		if attempt.NodeID != args.NodeID {
			return herr(ExitNotFound, "attempt %s belongs to #%d, not #%d", args.AttemptID, attempt.NodeID, args.NodeID)
		}
	}

	filtered, err := filterEvents(events, args)
	if err != nil {
		return err
	}
	if args.Raw {
		for _, e := range filtered {
			line, err := e.MarshalLine()
			if err != nil {
				return err
			}
			if _, err := out.Write(line); err != nil {
				return err
			}
		}
		return nil
	}
	if asJSON {
		WriteJSON(out, filtered)
	} else {
		RenderEventsText(out, filtered)
	}
	return nil
}

func filterEvents(events []*Event, args EventsArgs) ([]*Event, error) {
	start := 0
	if args.SinceEventID != "" {
		found := false
		for i, e := range events {
			if e.EventID == args.SinceEventID {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil, herr(ExitNotFound, "event %s not found", args.SinceEventID)
		}
	}
	var out []*Event
	for _, e := range events[start:] {
		if args.NodeID != 0 && e.NodeID != args.NodeID {
			continue
		}
		if args.AttemptID != "" && e.AttemptID != args.AttemptID {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, ln := range lines {
		lines[i] = prefix + ln
	}
	return strings.Join(lines, "\n")
}
