package cst

import (
	"encoding/json"
	"errors"
	"time"
)

// Tx is a single store transaction. It carries a coherent snapshot built
// from one replay under one held lock, plus a queue of events to commit
// when the transaction closes successfully. Mutation primitives append into
// the queue; nothing reaches disk until WithStore commits.
type Tx struct {
	cfg           Config
	state         *State
	actor         string
	actorIdentity ActorIdentity
	now           time.Time
	paths         StorePaths
	pending       []*Event
}

type TxOpts struct {
	// Mutating: true if the transaction is allowed to append events. Read
	// commands open with mutating=false; lazy-abandon repair is the only
	// exception and runs in its own scope.
	Mutating bool
	// RepairLease: if true, lazy-abandon expired claims and reload state
	// before fn runs. Explicit event stream reads run outside Tx.
	RepairLease bool
}

// WithStore wraps the load-cfg → lock → replay → fn → append loop so handlers
// don't open-code it (and can't accidentally drift on, say, lease repair).
// Errors from fn propaacceptance; pending events commit only on success.
func WithStore(opts TxOpts, fn func(*Tx) error) error {
	if opts.Mutating {
		if err := GuardImplicitWorkerStoreMutation(""); err != nil {
			return err
		}
	}
	paths, err := CurrentStorePaths()
	if err != nil {
		return err
	}
	cfg, err := loadCfg(paths)
	if err != nil {
		return err
	}
	lock, err := AcquireLockAt(paths)
	if err != nil {
		return err
	}
	defer lock.Release()

	events, err := ReplayAt(paths)
	if err != nil {
		return err
	}
	state, err := Apply(events)
	if err != nil {
		return err
	}
	actorIdentity := ResolveActorIdentity(cfg.ActorDefault)
	actor := actorIdentity.Name
	now := time.Now()

	if opts.RepairLease {
		if abandoned := state.LazyAbandonExpired(now); len(abandoned) > 0 {
			if err := AppendAt(paths, abandoned...); err != nil {
				return err
			}
			events = append(events, abandoned...)
			state, err = Apply(events)
			if err != nil {
				return err
			}
		}
	}

	tx := &Tx{cfg: cfg, state: state, actor: actor, actorIdentity: actorIdentity, now: now, paths: paths}
	if err := fn(tx); err != nil {
		return err
	}
	if len(tx.pending) > 0 {
		if !opts.Mutating {
			return errors.New("internal: read-only tx attempted to write events")
		}
		if err := recordWorkerStoreBindings(paths, state.StoreID(), tx.pending); err != nil {
			return err
		}
		return AppendAt(paths, tx.pending...)
	}
	return nil
}

// Snapshot exposes the tx's read view to handlers without granting write
// access. Callers may inspect but must mutate only via Tx primitives.
func (tx *Tx) Snapshot() *State       { return tx.state }
func (tx *Tx) Cfg() Config            { return tx.cfg }
func (tx *Tx) Actor() string          { return tx.actor }
func (tx *Tx) ActorExplicit() bool    { return tx.actorIdentity.Explicit }
func (tx *Tx) ActorSource() string    { return tx.actorIdentity.Source }
func (tx *Tx) StorePaths() StorePaths { return tx.paths }
func (tx *Tx) StoreID() string        { return tx.state.StoreID() }

// CreateGoal appends an aggregate node. The root goal is the only parent==0
// node in a store; child goals must live under another goal.
func (tx *Tx) CreateGoal(parent int64, intent string, context *NodeContext, boundary *NodeBoundary) (*Event, error) {
	if intent == "" {
		return nil, herr(ExitUsage, "goal requires --intent")
	}
	context, boundary, _, err := normalizeNodeDeclarations(tx.state.NextID, KindGoal, context, boundary, nil)
	if err != nil {
		return nil, herr(ExitUsage, "%s", err.Error())
	}
	if parent == 0 {
		if existing := tx.state.AnyRoot(); existing != nil {
			return nil, herr(ExitInvariantBroken,
				"store already has root goal #%d; a cst store has one root for life",
				existing.ID)
		}
	} else {
		p, ok := tx.state.Nodes[parent]
		if !ok {
			return nil, herr(ExitNotFound, "parent #%d not found", parent)
		}
		if p.Terminal() {
			return nil, herr(ExitInvariantBroken, "parent #%d is terminal", parent)
		}
		if p.Kind != KindGoal {
			return nil, herr(ExitInvariantBroken, "goal parent must be a goal; #%d is %s", parent, p.Kind)
		}
	}
	ev := &Event{
		EventID:   NewEventID(),
		Timestamp: tx.now,
		Actor:     tx.actor,
		Type:      EvNodeCreated,
		NodeID:    tx.state.NextID,
		ParentID:  parent,
		Kind:      KindGoal,
		Intent:    intent,
		Context:   context,
		Boundary:  boundary,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// CreateTask appends a node_created event for an executable task. Tasks must
// live under a goal or another task and must declare acceptance. Readiness
// prerequisites are independent `after` edges.
func (tx *Tx) CreateTask(parent int64, intent string, acceptance *Acceptance, after []int64, envelope *ExecutionEnvelope, context *NodeContext, boundary *NodeBoundary, obligationClaims []string) (*Event, error) {
	if intent == "" {
		return nil, herr(ExitUsage, "task requires --intent")
	}
	if acceptance == nil {
		return nil, herr(ExitUsage, "task requires acceptance (--verify or --review)")
	}
	if parent == 0 {
		return nil, herr(ExitUsage, "task requires --parent; root is a goal")
	}
	p, ok := tx.state.Nodes[parent]
	if !ok {
		return nil, herr(ExitNotFound, "parent #%d not found", parent)
	}
	if p.Terminal() {
		return nil, herr(ExitInvariantBroken, "parent #%d is terminal", parent)
	}
	if !p.CanParentWork() {
		return nil, herr(ExitInvariantBroken, "parent #%d is %s, not a goal/task", parent, p.Kind)
	}
	if err := tx.validateAfter(tx.state.NextID, after); err != nil {
		return nil, err
	}
	env, err := normalizeExecutionEnvelope(envelope)
	if err != nil {
		return nil, herr(ExitUsage, "%s", err.Error())
	}
	context, boundary, obligationClaims, err = normalizeNodeDeclarations(tx.state.NextID, KindTask, context, boundary, obligationClaims)
	if err != nil {
		return nil, herr(ExitUsage, "%s", err.Error())
	}
	ev := &Event{
		EventID:          NewEventID(),
		Timestamp:        tx.now,
		Actor:            tx.actor,
		Type:             EvNodeCreated,
		NodeID:           tx.state.NextID,
		ParentID:         parent,
		Kind:             KindTask,
		Intent:           intent,
		Acceptance:       acceptance,
		Envelope:         env,
		Context:          context,
		Boundary:         boundary,
		ObligationClaims: obligationClaims,
		After:            append([]int64(nil), after...),
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// CreateRule appends a rule node under a goal or task parent. Rules under
// rules are rejected because no descendant task would inherit them.
func (tx *Tx) CreateRule(parent int64, text string) (*Event, error) {
	if parent == 0 {
		return nil, herr(ExitUsage, "rule requires --parent")
	}
	if text == "" {
		return nil, herr(ExitUsage, "rule requires text")
	}
	p, ok := tx.state.Nodes[parent]
	if !ok {
		return nil, herr(ExitNotFound, "parent #%d not found", parent)
	}
	if p.Terminal() {
		return nil, herr(ExitInvariantBroken, "parent #%d is terminal", parent)
	}
	if !p.CanParentWork() {
		return nil, herr(ExitInvariantBroken,
			"rule parent must be a goal/task; #%d is %s", parent, p.Kind)
	}
	ev := &Event{
		EventID:   NewEventID(),
		Timestamp: tx.now,
		Actor:     tx.actor,
		Type:      EvNodeCreated,
		NodeID:    tx.state.NextID,
		ParentID:  parent,
		Kind:      KindRule,
		RuleText:  text,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

type ReviseSpec struct {
	ParentSet           bool
	Parent              int64
	Intent              string
	RuleText            string
	Acceptance          *Acceptance
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

func (tx *Tx) ReviseNode(id int64, spec ReviseSpec) (*Event, error) {
	n, ok := tx.state.Nodes[id]
	if !ok {
		return nil, herr(ExitNotFound, "node #%d not found", id)
	}
	if n.Terminal() {
		return nil, herr(ExitInvariantBroken, "node #%d already terminal", id)
	}
	if n.Claim != nil {
		return nil, herr(ExitClaimConflict, "node #%d is claimed by %s; release before revise", id, n.Claim.Actor)
	}
	if spec.ParentSet {
		if spec.Parent == 0 {
			return nil, herr(ExitUsage, "revise --parent requires a non-zero parent id")
		}
		if spec.Parent == n.ParentID {
			spec.ParentSet = false
		} else {
			if n.ParentID == 0 {
				return nil, herr(ExitInvariantBroken, "root goal #%d cannot be moved", id)
			}
			p, ok := tx.state.Nodes[spec.Parent]
			if !ok {
				return nil, herr(ExitNotFound, "parent #%d not found", spec.Parent)
			}
			if p.Terminal() {
				return nil, herr(ExitInvariantBroken, "parent #%d is terminal", spec.Parent)
			}
			if !p.CanParentWork() {
				return nil, herr(ExitInvariantBroken, "parent #%d is %s, not a goal/task", spec.Parent, p.Kind)
			}
			if n.Kind == KindGoal && p.Kind != KindGoal {
				return nil, herr(ExitInvariantBroken, "goal #%d parent must be a goal; #%d is %s", id, spec.Parent, p.Kind)
			}
			if tx.state.isAncestor(id, spec.Parent) {
				return nil, herr(ExitInvariantBroken, "moving #%d under #%d would create a cycle", id, spec.Parent)
			}
		}
	}
	if !spec.ParentSet && spec.Intent == "" && spec.RuleText == "" && spec.Acceptance == nil && !spec.EnvelopeSet && !spec.ContextSet && !spec.BoundarySet && !spec.ObligationClaimsSet && !spec.AfterSet {
		return nil, herr(ExitUsage, "revise requires at least one changed field")
	}
	if spec.Intent != "" && n.Kind == KindRule {
		return nil, herr(ExitInvariantBroken, "rule #%d uses --rule, not --intent", id)
	}
	if spec.RuleText != "" && n.Kind != KindRule {
		return nil, herr(ExitInvariantBroken, "%s #%d uses --intent, not --rule", n.Kind, id)
	}
	if spec.Acceptance != nil && n.Kind != KindTask {
		return nil, herr(ExitInvariantBroken, "%s #%d cannot have acceptance", n.Kind, id)
	}
	if spec.AfterSet && n.Kind != KindTask {
		return nil, herr(ExitInvariantBroken, "%s #%d cannot have prerequisites", n.Kind, id)
	}
	if spec.EnvelopeSet && n.Kind != KindTask {
		return nil, herr(ExitInvariantBroken, "%s #%d cannot have execution envelope", n.Kind, id)
	}
	if (spec.ContextSet || spec.BoundarySet) && n.Kind != KindGoal && n.Kind != KindTask {
		return nil, herr(ExitInvariantBroken, "%s #%d cannot have context or boundary", n.Kind, id)
	}
	if spec.ObligationClaimsSet && n.Kind != KindTask {
		return nil, herr(ExitInvariantBroken, "%s #%d cannot have obligation claims", n.Kind, id)
	}
	if spec.AfterSet {
		if err := tx.validateAfter(id, spec.After); err != nil {
			return nil, err
		}
	}
	var env *ExecutionEnvelope
	if spec.EnvelopeSet {
		var err error
		env, err = mergeExecutionEnvelopePatch(n.Envelope, spec.EnvelopePatch)
		if err != nil {
			return nil, herr(ExitUsage, "%s", err.Error())
		}
	}
	var context *NodeContext
	if spec.ContextSet {
		var err error
		context, err = mergeNodeContextPatch(n.Context, spec.ContextPatch)
		if err != nil {
			return nil, herr(ExitUsage, "%s", err.Error())
		}
	}
	var boundary *NodeBoundary
	if spec.BoundarySet {
		var err error
		boundary, err = mergeNodeBoundaryPatch(n.Boundary, spec.BoundaryPatch)
		if err != nil {
			return nil, herr(ExitUsage, "%s", err.Error())
		}
	}
	var obligationClaims []string
	if spec.ObligationClaimsSet {
		obligationClaims = normalizeObligationNames(spec.ObligationClaims)
	}
	parentID := int64(0)
	if spec.ParentSet {
		parentID = spec.Parent
	}
	ev := &Event{
		EventID:             NewEventID(),
		Timestamp:           tx.now,
		Actor:               tx.actor,
		Type:                EvNodeRevised,
		NodeID:              id,
		ParentID:            parentID,
		Intent:              spec.Intent,
		RuleText:            spec.RuleText,
		Acceptance:          spec.Acceptance,
		Envelope:            env,
		Context:             context,
		Boundary:            boundary,
		ObligationClaims:    obligationClaims,
		ContextSet:          spec.ContextSet,
		BoundarySet:         spec.BoundarySet,
		ObligationClaimsSet: spec.ObligationClaimsSet,
		Reason:              spec.Reason,
	}
	if spec.AfterSet {
		ev.After = append([]int64(nil), spec.After...)
		ev.AfterSet = true
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// TakeClaim atomically acquires a claim on the target task or fails.
func (tx *Tx) TakeClaim(id int64) (*Event, error) {
	n, err := tx.requireOpenTask(id)
	if err != nil {
		return nil, err
	}
	if n.Claim != nil {
		return nil, herr(ExitClaimConflict, "task #%d already claimed by %s", id, n.Claim.Actor)
	}
	if !tx.state.IsReadyTask(id) {
		return nil, herr(ExitInvariantBroken, "%s", tx.state.ReadyBlockReason(id))
	}
	exp := tx.now.Add(tx.cfg.ClaimLeaseTTL)
	ev := &Event{
		EventID:        NewEventID(),
		Timestamp:      tx.now,
		Actor:          tx.actor,
		Type:           EvClaimTaken,
		AttemptID:      NewAttemptID(),
		NodeID:         id,
		LeaseID:        NewLeaseID(),
		LeaseExpiresAt: &exp,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

func (tx *Tx) HoldNode(id int64, kind string, reason string) (*Event, error) {
	if reason == "" {
		return nil, herr(ExitUsage, "hold requires --reason")
	}
	if kind != HoldBlocked && kind != HoldWaiting && kind != HoldDeferred {
		return nil, herr(ExitUsage, "hold --kind must be blocked, waiting, or deferred")
	}
	n, err := tx.requireOpenTask(id)
	if err != nil {
		return nil, err
	}
	if n.Claim != nil {
		return nil, herr(ExitInvariantBroken, "task #%d is claimed; release before hold", id)
	}
	ev := &Event{
		EventID:   NewEventID(),
		Timestamp: tx.now,
		Actor:     tx.actor,
		Type:      EvNodeHeld,
		NodeID:    id,
		HoldKind:  kind,
		Reason:    reason,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

func (tx *Tx) ClearHold(id int64) (*Event, error) {
	n, err := tx.requireOpenTask(id)
	if err != nil {
		return nil, err
	}
	if n.Hold == nil {
		return nil, herr(ExitInvariantBroken, "task #%d is not held", id)
	}
	ev := &Event{
		EventID:   NewEventID(),
		Timestamp: tx.now,
		Actor:     tx.actor,
		Type:      EvNodeUnheld,
		NodeID:    id,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// ReleaseClaim drops the caller's own claim on a task. Releasing someone
// else's claim is a conflict.
func (tx *Tx) ReleaseClaim(id int64) (*Event, error) {
	n, ok := tx.state.Nodes[id]
	if !ok {
		return nil, herr(ExitNotFound, "task #%d not found", id)
	}
	if n.Claim == nil {
		return nil, herr(ExitInvariantBroken, "task #%d is not claimed", id)
	}
	if n.Claim.Actor != tx.actor {
		return nil, herr(ExitClaimConflict, "task #%d held by %s, not %s", id, n.Claim.Actor, tx.actor)
	}
	ev := &Event{
		EventID:   NewEventID(),
		Timestamp: tx.now,
		Actor:     tx.actor,
		Type:      EvClaimReleased,
		AttemptID: n.Claim.AttemptID,
		NodeID:    id,
		LeaseID:   n.Claim.LeaseID,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// RecordScriptRun appends a script_run event regardless of trigger. The
// caller should have already executed the shell command.
func (tx *Tx) RecordScriptRun(id int64, res RunResult) (*Event, error) {
	if _, ok := tx.state.Nodes[id]; !ok {
		return nil, herr(ExitNotFound, "task #%d not found", id)
	}
	eventID := res.EventID
	if eventID == "" {
		eventID = NewEventID()
	}
	gitAvailable := res.GitAvailable
	ev := &Event{
		EventID:                       eventID,
		Timestamp:                     tx.now,
		Actor:                         tx.actor,
		Type:                          EvScriptRun,
		AttemptID:                     tx.attemptIDForActorClaim(id),
		NodeID:                        id,
		Trigger:                       res.Trigger,
		CheckName:                     res.CheckName,
		Cmd:                           res.Cmd,
		ExitCode:                      res.ExitCode,
		DurationMs:                    res.DurationMs,
		StdoutHead:                    res.StdoutHead,
		StderrHead:                    res.StderrHead,
		Truncated:                     res.Truncated,
		StoreID:                       res.StoreID,
		ExecCWD:                       res.ExecCWD,
		GitAvailable:                  &gitAvailable,
		GitRoot:                       res.GitRoot,
		GitHead:                       res.GitHead,
		GitBranch:                     res.GitBranch,
		GitStatusShort:                res.GitStatusShort,
		StagedDiffSHA256:              res.StagedDiffSHA256,
		UnstagedDiffSHA256:            res.UnstagedDiffSHA256,
		UntrackedManifestSHA256:       res.UntrackedManifestSHA256,
		GitIdentityDigest:             res.GitIdentityDigest,
		ExecSurface:                   res.ExecSurface,
		OwnedPaths:                    append([]string(nil), res.OwnedPaths...),
		ScopedGitStatusShort:          res.ScopedGitStatusShort,
		ScopedStagedDiffSHA256:        res.ScopedStagedDiffSHA256,
		ScopedUnstagedDiffSHA256:      res.ScopedUnstagedDiffSHA256,
		ScopedUntrackedManifestSHA256: res.ScopedUntrackedManifestSHA256,
		ScopedDigest:                  res.ScopedDigest,
		OutOfScopeGitStatusShort:      res.OutOfScopeGitStatusShort,
		OutOfScopeDeltaCount:          res.OutOfScopeDeltaCount,
		OutOfScopeDigest:              res.OutOfScopeDigest,
		WholeRepoDigest:               res.WholeRepoDigest,
		ParallelWorktree:              res.ParallelWorktree,
		ExecContextDigest:             res.ExecContextDigest,
		StdoutArtifact:                res.StdoutArtifact,
		StderrArtifact:                res.StderrArtifact,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

func (tx *Tx) RecordEvidence(id int64, kind string, summary string, data json.RawMessage) (*Event, error) {
	n, ok := tx.state.Nodes[id]
	if !ok {
		return nil, herr(ExitNotFound, "node #%d not found", id)
	}
	if !n.CanHaveEvidence() {
		return nil, herr(ExitInvariantBroken, "node #%d cannot carry evidence", id)
	}
	if kind == "" {
		return nil, herr(ExitUsage, "evidence requires --kind")
	}
	if summary == "" {
		return nil, herr(ExitUsage, "evidence requires --summary")
	}
	ev := &Event{
		EventID:         NewEventID(),
		Timestamp:       tx.now,
		Actor:           tx.actor,
		Type:            EvEvidence,
		AttemptID:       tx.attemptIDForActorClaim(id),
		NodeID:          id,
		EvidenceKind:    kind,
		EvidenceSummary: summary,
		EvidenceData:    append(json.RawMessage(nil), data...),
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

func (tx *Tx) RecordAcceptanceRunSet(id int64, checks []VerifyCheck, runEvents []*Event) (*Event, error) {
	data, err := buildAcceptanceRunSetData(checks, runEvents)
	if err != nil {
		return nil, herr(ExitInvariantBroken, "%s", err.Error())
	}
	return tx.RecordEvidence(id, EvidenceAcceptanceRunSet, "acceptance run set", marshalAcceptanceRunSetData(data))
}

// CompletionGuard captures the preconditions an in-flight verify acceptance locked
// in before releasing the lock. The post-shell tx must verify these still
// hold before completing.
type CompletionGuard struct {
	NodeID         int64
	AcceptanceKind string
	VerifyChecks   []VerifyCheck
	ClaimLeaseID   string // must still match this lease for completion
	ClaimAttemptID string
}

// PrepareCompletionGuard validates that a task is currently completable by
// the caller and returns a guard the caller passes to CompleteTask after any
// out-of-lock work (such as running a verify shell command).
func (tx *Tx) PrepareCompletionGuard(id int64) (CompletionGuard, error) {
	return completionGuardFromSnapshot(tx.state, id, tx.actor)
}

func completionGuardFromSnapshot(s *State, id int64, actor string) (CompletionGuard, error) {
	n, ok := s.Nodes[id]
	if !ok {
		return CompletionGuard{}, herr(ExitNotFound, "task #%d not found", id)
	}
	if n.Kind != KindTask {
		return CompletionGuard{}, herr(ExitInvariantBroken, "node #%d is %s, not task", id, n.Kind)
	}
	if n.Terminal() {
		return CompletionGuard{}, herr(ExitInvariantBroken, "task #%d already terminal", id)
	}
	if n.Claim == nil {
		return CompletionGuard{}, herr(ExitInvariantBroken, "task #%d is not claimed; run `cst take %d` first", n.ID, n.ID)
	}
	if n.Claim.Actor != actor {
		return CompletionGuard{}, herr(ExitClaimConflict, "task #%d held by %s, not %s", n.ID, n.Claim.Actor, actor)
	}
	if ok, why := s.CanComplete(id); !ok {
		return CompletionGuard{}, herr(ExitInvariantBroken, "%s", why)
	}
	if failed := s.DependencyFailedIDs(n); len(failed) > 0 {
		return CompletionGuard{}, herr(ExitInvariantBroken, "task #%d has canceled prerequisite(s): %v", id, failed)
	}
	if waiting := s.WaitingOnIDs(n); len(waiting) > 0 {
		return CompletionGuard{}, herr(ExitInvariantBroken, "task #%d is waiting on prerequisite(s): %v", id, waiting)
	}
	if n.Acceptance == nil {
		return CompletionGuard{}, herr(ExitInvariantBroken, "task #%d has no acceptance", id)
	}
	return CompletionGuard{
		NodeID:         id,
		AcceptanceKind: n.Acceptance.Kind,
		VerifyChecks:   n.Acceptance.VerifyChecks(),
		ClaimLeaseID:   n.Claim.LeaseID,
		ClaimAttemptID: n.Claim.AttemptID,
	}, nil
}

func (tx *Tx) RenewExecutionClaim(id int64) (*Event, error) {
	n, err := tx.requireOpenTask(id)
	if err != nil {
		return nil, err
	}
	if n.Claim == nil {
		return nil, herr(ExitInvariantBroken, "task #%d is not claimed; run `cst take %d` first", id, id)
	}
	stale := tx.now.After(n.Claim.LeaseExpiresAt)
	if n.Claim.Actor != tx.actor {
		state := "active"
		if stale {
			state = "expired"
		}
		return nil, herr(ExitClaimConflict,
			"task #%d has %s claim by %s; inspect with `cst claims` and use `cst take %d` only when takeover is intentional",
			id, state, n.Claim.Actor, id)
	}
	if !tx.actorIdentity.Explicit {
		if stale {
			return nil, herr(ExitClaimConflict,
				"task #%d claim for fallback actor %s is expired; rerun with --actor, CST_ACTOR, or actor.default, then take explicitly",
				id, tx.actor)
		}
		return nil, nil
	}
	exp := tx.now.Add(tx.cfg.ClaimLeaseTTL)
	ev := &Event{
		EventID:        NewEventID(),
		Timestamp:      tx.now,
		Actor:          tx.actor,
		Type:           EvClaimRenewed,
		AttemptID:      n.Claim.AttemptID,
		NodeID:         id,
		LeaseID:        n.Claim.LeaseID,
		LeaseExpiresAt: &exp,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// CompleteTask is the only writer of task_completed. It re-validates the
// guard's preconditions inside the current lock, so verify-acceptance completions
// that race with cancel/release/lease-expiry are rejected.
func (tx *Tx) CompleteTask(g CompletionGuard, evidenceIDs []string) (*Event, error) {
	evidenceIDs = normalizeEvidenceIDs(evidenceIDs)
	current, err := completionGuardFromSnapshot(tx.state, g.NodeID, tx.actor)
	if err != nil {
		return nil, err
	}
	n := tx.state.Nodes[g.NodeID]
	if current.AcceptanceKind != g.AcceptanceKind {
		return nil, herr(ExitInvariantBroken, "task #%d acceptance changed under us", g.NodeID)
	}
	if g.AcceptanceKind == AcceptanceVerify && !sameVerifyChecks(current.VerifyChecks, g.VerifyChecks) {
		return nil, herr(ExitInvariantBroken, "task #%d verify checks changed under us", g.NodeID)
	}
	if g.ClaimLeaseID != "" && current.ClaimLeaseID != g.ClaimLeaseID {
		return nil, herr(ExitClaimConflict, "task #%d claim changed under us", g.NodeID)
	}
	if g.ClaimAttemptID != "" && current.ClaimAttemptID != g.ClaimAttemptID {
		return nil, herr(ExitClaimConflict, "task #%d attempt changed under us", g.NodeID)
	}
	for _, evidenceID := range evidenceIDs {
		rec, ok := tx.state.EvidenceIDs[evidenceID]
		if !ok {
			return nil, herr(ExitNotFound, "evidence %s not found", evidenceID)
		}
		if rec.NodeID != g.NodeID {
			return nil, herr(ExitInvariantBroken, "evidence %s belongs to #%d", evidenceID, rec.NodeID)
		}
	}
	if g.AcceptanceKind == AcceptanceVerify {
		hasRunSet := false
		for _, evidenceID := range evidenceIDs {
			if tx.state.EvidenceIDs[evidenceID].Kind == EvidenceAcceptanceRunSet {
				hasRunSet = true
				break
			}
		}
		if !hasRunSet {
			return nil, herr(ExitUsage, "verify acceptance requires acceptance_run_set evidence_id")
		}
	}
	if g.AcceptanceKind == AcceptanceReview && len(evidenceIDs) == 0 {
		return nil, herr(ExitUsage, "review acceptance requires --evidence <event-id> or --note")
	}
	ev := &Event{
		EventID:     NewEventID(),
		Timestamp:   tx.now,
		Actor:       tx.actor,
		Type:        EvTaskCompleted,
		AttemptID:   n.Claim.AttemptID,
		NodeID:      g.NodeID,
		EvidenceIDs: evidenceIDs,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// CancelNode terminates a task or rule. Tasks that are claimed must be
// released or the caller must own the claim.
func (tx *Tx) CancelNode(id int64, reason string) (*Event, error) {
	if reason == "" {
		return nil, herr(ExitUsage, "cancel requires --reason")
	}
	n, ok := tx.state.Nodes[id]
	if !ok {
		return nil, herr(ExitNotFound, "node #%d not found", id)
	}
	if n.Terminal() {
		return nil, herr(ExitInvariantBroken, "node #%d already terminal", id)
	}
	if n.Kind == KindGoal {
		return nil, herr(ExitInvariantBroken,
			"goal #%d cannot be canceled directly; finish or cancel descendant tasks", id)
	}
	if child := tx.state.OpenTaskChild(n); child != nil {
		return nil, herr(ExitInvariantBroken,
			"node #%d has open child task #%d; finish or cancel children first", id, child.ID)
	}
	if n.IsTask() && n.Claim != nil && n.Claim.Actor != tx.actor {
		return nil, herr(ExitClaimConflict, "task #%d held by %s; release or take first", id, n.Claim.Actor)
	}
	ev := &Event{
		EventID:   NewEventID(),
		Timestamp: tx.now,
		Actor:     tx.actor,
		Type:      EvNodeCanceled,
		AttemptID: tx.attemptIDForActorClaim(id),
		NodeID:    id,
		Reason:    reason,
	}
	if err := tx.applyAndQueue(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

func (tx *Tx) requireOpenTask(id int64) (*Node, error) {
	n, ok := tx.state.Nodes[id]
	if !ok {
		return nil, herr(ExitNotFound, "task #%d not found", id)
	}
	if !n.IsTask() {
		return nil, herr(ExitInvariantBroken, "#%d is a rule, not a task", id)
	}
	if n.Terminal() {
		return nil, herr(ExitInvariantBroken, "task #%d already terminal", id)
	}
	return n, nil
}

func (tx *Tx) requireCallerOwnsClaim(n *Node) error {
	if n.Claim == nil {
		return herr(ExitInvariantBroken, "task #%d is not claimed; run `cst take %d` first", n.ID, n.ID)
	}
	if n.Claim.Actor != tx.actor {
		return herr(ExitClaimConflict, "task #%d held by %s, not %s", n.ID, n.Claim.Actor, tx.actor)
	}
	return nil
}

func (tx *Tx) attemptIDForActorClaim(id int64) string {
	n := tx.state.Nodes[id]
	if n == nil || n.Claim == nil || n.Claim.Actor != tx.actor {
		return ""
	}
	return n.Claim.AttemptID
}

func (tx *Tx) validateAfter(nodeID int64, after []int64) error {
	seen := map[int64]bool{}
	for _, refID := range after {
		if refID == 0 {
			return herr(ExitUsage, "--after requires a non-zero node id")
		}
		if refID == nodeID {
			return herr(ExitInvariantBroken, "task #%d cannot depend on itself", nodeID)
		}
		if seen[refID] {
			return herr(ExitInvariantBroken, "task #%d repeats prerequisite #%d", nodeID, refID)
		}
		seen[refID] = true
		ref, ok := tx.state.Nodes[refID]
		if !ok {
			return herr(ExitNotFound, "--after references unknown node #%d", refID)
		}
		if !ref.CanParentWork() {
			return herr(ExitInvariantBroken, "--after references %s #%d; prerequisites must be goals or tasks", ref.Kind, refID)
		}
		if tx.state.isAncestor(refID, nodeID) {
			return herr(ExitInvariantBroken, "task #%d cannot depend on ancestor #%d", nodeID, refID)
		}
		if tx.state.hasPrereqPath(refID, nodeID) {
			return herr(ExitInvariantBroken, "task #%d after #%d would create a prerequisite cycle", nodeID, refID)
		}
	}
	return nil
}

// applyAndQueue both stages the event for commit and applies it to tx.state
// so subsequent primitives in the same Tx see consistent post-event state
// (e.g. CreateTask + TakeClaim in one Tx).
func (tx *Tx) applyAndQueue(ev *Event) error {
	if err := tx.state.applyOne(ev); err != nil {
		return err
	}
	tx.pending = append(tx.pending, ev)
	return nil
}

// renewClaimUnderLock is used by the long-running shell renewer outside any
// Tx. It opens its own lock briefly, reloads state, and writes a single
// claim_renewed event only when the active claim still matches both the
// caller's actor AND the lease id captured at the start of the run. The
// lease-id check prevents an old long-running renewer from extending a new
// claim that was taken after the original lease was released.
func renewClaimUnderLock(paths StorePaths, actor string, id int64, expectedLeaseID string, ttl time.Duration) error {
	if expectedLeaseID == "" {
		return errors.New("renew requires lease id")
	}
	lock, err := AcquireLockAt(paths)
	if err != nil {
		return err
	}
	defer lock.Release()
	events, err := ReplayAt(paths)
	if err != nil {
		return err
	}
	state, err := Apply(events)
	if err != nil {
		return err
	}
	n, ok := state.Nodes[id]
	if !ok || n.Claim == nil || n.Claim.LeaseID != expectedLeaseID || n.Claim.Actor != actor {
		return errors.New("lease lost during renewal")
	}
	now := time.Now()
	exp := now.Add(ttl)
	ev := &Event{
		EventID:        NewEventID(),
		Timestamp:      now,
		Actor:          actor,
		Type:           EvClaimRenewed,
		AttemptID:      n.Claim.AttemptID,
		NodeID:         id,
		LeaseID:        n.Claim.LeaseID,
		LeaseExpiresAt: &exp,
	}
	return AppendAt(paths, ev)
}

// runScriptUnderTx is a thin convenience wrapper used by handlers that need
// to run a shell command and append both the script_run event and possibly a
// completion. Callers still drive the two-phase logic where needed.
func runScript(cfg Config, opts RunOpts) RunResult {
	if opts.StdoutMaxBytes <= 0 {
		opts.StdoutMaxBytes = cfg.RunnerStdoutMaxBytes
	}
	if opts.StderrMaxBytes <= 0 {
		opts.StderrMaxBytes = cfg.RunnerStderrMaxBytes
	}
	if opts.Timeout <= 0 {
		opts.Timeout = cfg.RunnerDefaultTimeout
	}
	return Run(opts)
}

func loadCfg(paths StorePaths) (Config, error) {
	dir, err := EnsureStoreDirAt(paths)
	if err != nil {
		return Config{}, err
	}
	return LoadConfig(dir)
}
