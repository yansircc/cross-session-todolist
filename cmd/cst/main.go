package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/yansir/cst/internal/cst"
)

const usage = `cst - agent-first cross-session task system

Invariant:
  .cst/events.jsonl is the only task source. It is append-only.
  Every command emits JSON by default. Use --human only for manual display.
  There is no --json flag.

Mental model:
  node = id + parent + lifecycle + event log
  goal = node + aggregate progress; non-claimable; completion is derived
  task = node + acceptance + after prerequisites; claimable when ready
  rule = node + inherited text; non-claimable; not a policy engine
  context/boundary/obligation = node-local declarations; global means high in tree
  attempt = claim-scoped correlation id for run/evidence/completion events

Start from an empty repo:
  cst add --intent "Project goal"
  cst add --parent 1 --goal --intent "Workstream A"
  cst add --parent 1 --rule "One fact, one location"
  cst add --parent 2 --intent "Implement first slice" --verify "go test ./..."
  cst add --parent 2 --intent "Implement second slice" --check unit="go test ./..." --check help="go run ./cmd/cst -h >/dev/null"
  cst add --parent 2 --intent "Review first slice" --review self
  cst add --parent 2 --intent "Publish after tests" --after 3 --review self

Command surface:
  cst next
  cst add  --intent "Root goal"
  cst add  --parent <id> --goal --intent "Child goal / workstream"
  cst add  --parent <id> --intent "Task" (--verify "cmd" | --check <name=cmd>... | --review "who") [--exec-cwd <path>] [--private-exec-cwd] [--scope <path> ...] [--after <node-id> ...]
  cst add  ... [--invariant "..."] [--non-goal "..."] [--success-obligation <name>] [--owned <path>] [--excluded <path>] [--obligation-claim <name>]
  cst add  --parent <id> --rule "Invariant or context visible to agents"
  cst revise <id> [--parent <id>] [--intent "..." | --rule "..."] [--verify "..." | --check <name=cmd>... | --review "..."] [--exec-cwd <path>] [--private-exec-cwd|--shared-exec-cwd] [--scope <path> ... | --clear-scope] [--invariant "..."] [--non-goal "..."] [--success-obligation <name>] [--clear-context] [--owned <path>] [--excluded <path>] [--clear-boundary] [--obligation-claim <name> | --clear-obligation-claims] [--after <id> ... | --clear-after] [--reason "..."]

  cst brief [--within <id>] [--history]
  cst claims [--within <id>]
  cst recover [--within <id>]
  cst show <id>
  cst events --for <id>
  cst events --attempt <attempt-id>
  cst events --since <event-id>
  cst events --for <id> --attempt <attempt-id> --since <event-id>
  cst events --all --raw
  cst ui [--within <id>] [-o <path>] [--no-open] [--stdout]

  cst take [<task-id>] [--exec-cwd <path>] [--private-exec-cwd] [--scope <path> ...]
  cst release <task-id>
  cst hold <task-id> --kind blocked|waiting|deferred --reason "..."
  cst hold <task-id> --clear
  cst run <task-id> [--exec-cwd <checkout-root>] [--check <name>] [--cmd "..."]
  cst run <task-id> [--exec-cwd <checkout-root>] --acceptance
  cst worker-status <task-id>
  cst worker-run <task-id> --action <action-id> [--commit <sha>]
  cst evidence <id> --kind <kind> --summary "..." [--data JSON]
  cst evidence <id> --kind note --summary "Process note..."
  cst done <task-id> [--exec-cwd <checkout-root>] [--commit <sha>]
  cst done <task-id> --from-acceptance <evidence-id> [--commit <sha>]
  cst done <task-id> [--evidence <event-id> ... | --note "..."]
  cst cancel <id> --reason "..."

Agent loop:
  Run cst next, then execute the returned action or repair contract. If next
  returns required=input, ask for the named input and rerun cst next after
  recording it. Stop only when cst next returns phase=no-op.

Read semantics:
  cst next is the repo-level procedure projection. It is read-only: no procedure
  state is stored. It returns a phase, a single recommended bound action when
  one is legal, or a minimal repair contract with copyable command templates.
  Its reconcile phase checks uncommitted paths against active task node.boundary
  only; completed task boundaries are historical evidence, not current
  ownership. Execution scope is not task ownership.
  cst brief is the bounded work projection. By default it is frontier-first:
  active child subtrees are expanded and completed child subtrees are counted.
  Use cst brief --history to inspect completed child subtrees and historical
  recent runs/failures. It reports total/shown/truncated metadata for bounded
  collections.
  cst show is a bounded single-node view with aggregate progress, completed
  evidence ids, closure projection, and previews.
  cst claims and cst recover are read-only claim recovery projections. They
  show actor, task, attempt, lease staleness, and latest execution identity.
  cst events is the event-source reader and always requires an explicit range:
  --for, --since, or --all --raw. Do not use --all in the normal Agent loop.

Node briefing:
  Use --invariant, --non-goal, and --success-obligation on any goal/task to
  declare node-local context. Use --owned and --excluded to declare a node
  boundary once; active sibling owned paths cannot overlap, while terminal
  boundaries remain historical evidence. Boundary is reused for briefing and
  accepted-diff validation. Use --obligation-claim on static leaf tasks to claim
  named success obligations.
  show, take, worker-status, and ui derive the same developer briefing by
  walking root->node: context fold, local boundary, upstream/downstream edges,
  local acceptance, obligation claims, success coverage, and partition warnings.
  The reducer checks named set coverage and boundary path algebra; it cannot
  prove prose understanding, so rationale remains review-only attestation.

Acceptance and readiness:
  --verify <cmd>   shorthand for one verify check named "default".
  --check <n=cmd>  named verify check; repeatable; done runs all checks in order.
  --review <who>   done requires --note or --evidence.
  --after <id>     task is not ready until each prerequisite node is completed.
  Risky work should freeze a verifier contract before implementation: create a
  review task that records evidence kind=verifier_contract with
  canonical_source.ref, contract_artifacts, verifier_scripts, manifest,
  cheapest_plausible_lie, red_case_runs, and blind_spots; then make the
  implementation task depend on it with --after and named checks such as
  contract-lock, coverage, red, and real. contract-lock must rehash the frozen
  artifacts/scripts outside the reducer; include the shim and real implementation
  such as cmd/verify-contract-lock/main.go. canonical_source.ref is a
  declaration; closure comes from the hash chain. CST records shape, not
  verifier truth. Contract artifact paths are relative to the verifier root and
  cannot be absolute or escape with '..'.

Evidence and scripts:
  cst run records script_run(trigger=probe) and does not change status.
  cst run --acceptance records acceptance script_runs and acceptance_run_set
  without completing the task.
  cst done on a verify task records script_run(trigger=acceptance) for each check,
  then records acceptance_run_set and includes it in task_completed.evidence_ids.
  cst worker-status projects bound legal worker actions. cst worker-run reprojects
  before execution and refuses stale action ids.
  Execution envelopes keep ledger and execution identities separate:
    cst --store /central/repo revise 12 --exec-cwd /worker/repo --private-exec-cwd --scope internal/parser
    cst --store /central/repo worker-status 12 --human
    cst --store /central/repo worker-run 12 --action <action-id>
  Without --store, CST walks up to the nearest existing .cst; if none exists,
  it uses the enclosing git root before falling back to cwd. This is ambient
  discovery, not an explicit store binding.
  --exec-cwd on add/revise becomes the task default. --exec-cwd on run/done is
  a one-command override. Private exec surfaces reject any final context drift.
  Shared surfaces reject scoped drift but record context_drift evidence and
  allow out-of-scope drift because shared checkout attribution is unknowable.
  --scope paths are relative to the execution checkout, never absolute and never
  ..-escaping.
  Detectable worker checkouts reject mutating commands without explicit --store
  before opening a local ledger and print the bound recovery command. Worker
  binding sidecars are accepted only when their store_id matches the replayed
  central ledger root.
  cst evidence records structured evidence; --data must be JSON.
  boundary evidence has {"includes":[],"excludes":[]} and is checked against the
  accepted diff. rationale evidence is structured attestation, projected and
  contestable, not reducer-proved. contest evidence marks boundary/rationale
  evidence as contested for review.
  claim, script_run, evidence, and completion events from one claim share attempt_id.
  Process notes are evidence: use --kind note. Do not add task note fields or
  use cst as a scratchpad.
  Evidence and script runs are facts: append-only, never revised or deleted.

Tree rules:
  A store has one root goal for life. No init command is needed.
  Goals are not taken or done; they complete when descendant tasks are terminal.
  Multiple todolists are child goals under the root, not multiple stores.
  Oversized work should become child tasks or a child goal.
  Temporarily non-actionable work should use hold blocked|waiting|deferred.
  A final review is a task under the goal, not a claim on the goal.
  If the tree is wrong, use revise; it preserves id, runs, and evidence.
  Split by adding children. Merge by canceling the duplicate with a reason.

Safety rules:
  Tasks must have exactly one acceptance kind: verify or review.
  Readiness prerequisites are optional and repeatable with --after.
  Rules are inherited context for agents; cst does not parse them as policy.
  Claimed tasks cannot be revised or held; release first.
  Terminal nodes cannot be revised.
  Cancel is semantic deletion; events are never physically deleted.

Storage:
  .cst/events.jsonl   append-only event log; source of truth
  .cst/events.lock    advisory transaction lock; do not track as task state
  .cst/config.toml    optional budgets, timeouts, lease TTL, actor default
  .cst/artifacts/     hash-checked run witness attachments referenced by events

Global flags:
  --human   emit human-readable text
  --actor <agent-id>
            set explicit actor identity for claim ownership and same-actor lease renewal
  --store <repo-root>
            read/write the CST ledger under this repo root
  --help    show this help

Exit codes:
  0  ok
  1  generic error
  2  acceptance / acceptance failed
  3  claim conflict
  4  invariant broken
  5  not found
 64  usage error
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(int(cst.ExitUsage))
	}
	args := os.Args[1:]
	asJSON := true
	storeRoot := ""
	actor := ""
	var err error
	args, asJSON, storeRoot, actor, err = extractLeadingGlobalFlags(args, asJSON)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cst:", err)
		os.Exit(int(cst.ExitUsage))
	}
	if storeRoot != "" {
		if err := cst.SetStoreRoot(storeRoot); err != nil {
			fmt.Fprintln(os.Stderr, "cst:", err)
			os.Exit(int(cst.ExitUsage))
		}
	}
	if actor != "" {
		cst.SetActor(actor)
	}
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(int(cst.ExitUsage))
	}
	cmd := args[0]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		fmt.Print(usage)
		return
	}
	args = args[1:]
	if storeRoot == "" && commandMutates(cmd) {
		if binding, ok, err := cst.DetectWorkerStoreBinding(""); err != nil {
			fmt.Fprintln(os.Stderr, "cst:", err)
			os.Exit(int(cst.ExitGenericError))
		} else if ok {
			recovery := cst.WorkerRecoveryCommand(cmd, args, binding)
			fmt.Fprintln(os.Stderr, "cst:", cst.WorkerStoreGuardError(binding, recovery))
			os.Exit(int(cst.ExitInvariantBroken))
		}
	}

	switch cmd {
	case "add":
		err = runAdd(args, asJSON)
	case "next":
		err = runNext(args, asJSON)
	case "revise":
		err = runRevise(args, asJSON)
	case "take":
		err = runTake(args, asJSON)
	case "release":
		err = runRelease(args, asJSON)
	case "hold":
		err = runHold(args, asJSON)
	case "run":
		err = runRun(args, asJSON)
	case "worker-status":
		err = runWorkerStatus(args, asJSON)
	case "worker-run":
		err = runWorkerRun(args, asJSON)
	case "evidence":
		err = runEvidence(args, asJSON)
	case "done":
		err = runDone(args, asJSON)
	case "cancel":
		err = runCancel(args, asJSON)
	case "brief":
		err = runBrief(args, asJSON)
	case "claims":
		err = runClaims(args, asJSON)
	case "recover":
		err = runRecover(args, asJSON)
	case "show":
		err = runShow(args, asJSON)
	case "events":
		err = runEvents(args, asJSON)
	case "ui":
		err = runUI(args, asJSON)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n%s", cmd, usage)
		os.Exit(int(cst.ExitUsage))
	}

	if err != nil {
		var hErr *cst.HandlerError
		if errors.As(err, &hErr) {
			fmt.Fprintln(os.Stderr, "cst:", hErr.Msg)
			os.Exit(int(hErr.Code))
		}
		fmt.Fprintln(os.Stderr, "cst:", err)
		os.Exit(int(cst.ExitGenericError))
	}
}

func runNext(args []string, asJSON bool) error {
	fs := flag.NewFlagSet("next", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	var err error
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("next takes only flags")
	}
	return cst.DoNext(os.Stdout, asJSON)
}

func commandMutates(cmd string) bool {
	switch cmd {
	case "add", "revise", "take", "release", "hold", "run", "worker-run", "evidence", "done", "cancel":
		return true
	default:
		return false
	}
}

func extractLeadingGlobalFlags(args []string, current bool) ([]string, bool, string, string, error) {
	val := current
	storeRoot := ""
	actor := ""
	for len(args) > 0 {
		if args[0] == "--store" {
			if len(args) < 2 || args[1] == "" {
				return args, val, storeRoot, actor, fmt.Errorf("--store requires a path")
			}
			storeRoot = args[1]
			args = args[2:]
			continue
		}
		if strings.HasPrefix(args[0], "--store=") {
			storeRoot = strings.TrimPrefix(args[0], "--store=")
			if storeRoot == "" {
				return args, val, storeRoot, actor, fmt.Errorf("--store requires a path")
			}
			args = args[1:]
			continue
		}
		if args[0] == "--actor" {
			if len(args) < 2 || args[1] == "" {
				return args, val, storeRoot, actor, fmt.Errorf("--actor requires an id")
			}
			actor = args[1]
			args = args[2:]
			continue
		}
		if strings.HasPrefix(args[0], "--actor=") {
			actor = strings.TrimPrefix(args[0], "--actor=")
			if actor == "" {
				return args, val, storeRoot, actor, fmt.Errorf("--actor requires an id")
			}
			args = args[1:]
			continue
		}
		next, ok, err := formatFlagValue(args[0], val)
		if err != nil {
			return args, val, storeRoot, actor, err
		}
		if !ok {
			break
		}
		val = next
		args = args[1:]
	}
	return args, val, storeRoot, actor, nil
}

func extractFormatFlag(args []string, current bool) ([]string, bool, error) {
	out := args[:0]
	val := current
	for _, a := range args {
		next, ok, err := formatFlagValue(a, val)
		if err != nil {
			return nil, val, err
		}
		if ok {
			val = next
			continue
		}
		out = append(out, a)
	}
	return out, val, nil
}

func formatFlagValue(arg string, current bool) (bool, bool, error) {
	switch arg {
	case "--human":
		return false, true, nil
	default:
		return current, false, nil
	}
}

type commandFormatFlags struct {
	human *bool
}

type idListFlag []int64

func (f *idListFlag) String() string {
	if f == nil {
		return ""
	}
	return fmt.Sprint([]int64(*f))
}

func (f *idListFlag) Set(raw string) error {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid id %q", raw)
	}
	*f = append(*f, id)
	return nil
}

func (f idListFlag) Values() []int64 {
	return append([]int64(nil), f...)
}

type checkListFlag []cst.VerifyCheck

func (f *checkListFlag) String() string {
	if f == nil {
		return ""
	}
	return fmt.Sprint([]cst.VerifyCheck(*f))
}

func (f *checkListFlag) Set(raw string) error {
	name, cmd, ok := strings.Cut(raw, "=")
	name = strings.TrimSpace(name)
	cmd = strings.TrimSpace(cmd)
	if !ok || name == "" || cmd == "" {
		return fmt.Errorf("invalid check %q; expected name=cmd", raw)
	}
	*f = append(*f, cst.VerifyCheck{Name: name, Cmd: cmd})
	return nil
}

func (f checkListFlag) Values() []cst.VerifyCheck {
	return append([]cst.VerifyCheck(nil), f...)
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty value")
	}
	*f = append(*f, raw)
	return nil
}

func (f stringListFlag) Values() []string {
	return append([]string(nil), f...)
}

func addCommandFormatFlags(fs *flag.FlagSet) commandFormatFlags {
	return commandFormatFlags{
		human: fs.Bool("human", false, "emit human-readable text"),
	}
}

func resolveCommandFormat(fs *flag.FlagSet, inherited bool, format commandFormatFlags) (bool, error) {
	seenHuman := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "human" {
			seenHuman = true
		}
	})
	if seenHuman && *format.human {
		return false, nil
	}
	return inherited, nil
}

func runAdd(args []string, asJSON bool) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	parent := fs.Int64("parent", 0, "parent node id")
	goal := fs.Bool("goal", false, "create aggregate goal node")
	intent := fs.String("intent", "", "task intent")
	rule := fs.String("rule", "", "rule text (creates a rule node)")
	verify := fs.String("verify", "", "verify acceptance command")
	review := fs.String("review", "", "review acceptance reviewer")
	execCWD := fs.String("exec-cwd", "", "default checkout root for task execution")
	privateExec := fs.Bool("private-exec-cwd", false, "mark exec-cwd as actor-private mutable surface")
	invariant := fs.String("invariant", "", "node-local invariant context")
	var after idListFlag
	var checks checkListFlag
	var scope stringListFlag
	var nonGoals stringListFlag
	var successObligations stringListFlag
	var owned stringListFlag
	var excluded stringListFlag
	var obligationClaims stringListFlag
	fs.Var(&after, "after", "readiness prerequisite node id (repeatable)")
	fs.Var(&checks, "check", "named verify check in name=cmd form (repeatable)")
	fs.Var(&scope, "scope", "owned path under exec checkout (repeatable)")
	fs.Var(&nonGoals, "non-goal", "node-local non-goal context (repeatable)")
	fs.Var(&successObligations, "success-obligation", "named success obligation declared by this node (repeatable)")
	fs.Var(&owned, "owned", "node boundary owned repository path (repeatable)")
	fs.Var(&excluded, "excluded", "node boundary excluded repository path (repeatable)")
	fs.Var(&obligationClaims, "obligation-claim", "named success obligation claimed by this task acceptance (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var err error
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	envelope, err := buildAddEnvelope(*execCWD, *privateExec, scope.Values())
	if err != nil {
		return err
	}
	if envelope != nil && (*goal || *rule != "") {
		return fmt.Errorf("execution envelope flags apply only to tasks")
	}
	invariantSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "invariant" {
			invariantSet = true
		}
	})
	return cst.DoAdd(os.Stdout, cst.AddArgs{
		Parent:           *parent,
		Intent:           *intent,
		Rule:             *rule,
		Goal:             *goal,
		AcceptanceVerify: *verify,
		VerifyChecks:     checks.Values(),
		AcceptanceReview: *review,
		After:            after.Values(),
		Envelope:         envelope,
		Context:          buildNodeContext(invariantSet, *invariant, nonGoals.Values(), successObligations.Values()),
		Boundary:         buildNodeBoundary(owned.Values(), excluded.Values()),
		ObligationClaims: obligationClaims.Values(),
	}, asJSON)
}

func buildNodeContext(invariantSet bool, invariant string, nonGoals []string, successObligations []string) *cst.NodeContext {
	if !invariantSet && len(nonGoals) == 0 && len(successObligations) == 0 {
		return nil
	}
	return &cst.NodeContext{
		Invariant:          invariant,
		NonGoals:           nonGoals,
		SuccessObligations: successObligations,
	}
}

func buildNodeBoundary(owned []string, excluded []string) *cst.NodeBoundary {
	if len(owned) == 0 && len(excluded) == 0 {
		return nil
	}
	return &cst.NodeBoundary{Owned: owned, Excluded: excluded}
}

func buildAddEnvelope(execCWD string, private bool, ownedPaths []string) (*cst.ExecutionEnvelope, error) {
	if !private && execCWD == "" && len(ownedPaths) == 0 {
		return nil, nil
	}
	if private && execCWD == "" {
		return nil, fmt.Errorf("--private-exec-cwd requires --exec-cwd")
	}
	surface := cst.ExecSurfaceShared
	if private {
		surface = cst.ExecSurfacePrivate
	}
	return &cst.ExecutionEnvelope{
		ExecCWD:     execCWD,
		ExecSurface: surface,
		OwnedPaths:  ownedPaths,
	}, nil
}

func runRevise(args []string, asJSON bool) error {
	id, rest, err := splitIDArg(args, "revise")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("revise", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	parent := fs.Int64("parent", 0, "new parent node id")
	intent := fs.String("intent", "", "new goal/task intent")
	rule := fs.String("rule", "", "new rule text")
	verify := fs.String("verify", "", "new verify acceptance command")
	review := fs.String("review", "", "new review acceptance reviewer")
	execCWD := fs.String("exec-cwd", "", "new default checkout root for task execution")
	privateExec := fs.Bool("private-exec-cwd", false, "mark exec-cwd as actor-private mutable surface")
	sharedExec := fs.Bool("shared-exec-cwd", false, "mark exec-cwd as shared mutable surface")
	invariant := fs.String("invariant", "", "replace node-local invariant context")
	var after idListFlag
	var checks checkListFlag
	var scope stringListFlag
	var nonGoals stringListFlag
	var successObligations stringListFlag
	var owned stringListFlag
	var excluded stringListFlag
	var obligationClaims stringListFlag
	fs.Var(&after, "after", "replace readiness prerequisites with node id (repeatable)")
	fs.Var(&checks, "check", "replace verify acceptance with named check in name=cmd form (repeatable)")
	fs.Var(&scope, "scope", "replace owned path under exec checkout (repeatable)")
	fs.Var(&nonGoals, "non-goal", "replace node-local non-goal context (repeatable)")
	fs.Var(&successObligations, "success-obligation", "replace named success obligations for this node (repeatable)")
	fs.Var(&owned, "owned", "replace node boundary owned repository path (repeatable)")
	fs.Var(&excluded, "excluded", "replace node boundary excluded repository path (repeatable)")
	fs.Var(&obligationClaims, "obligation-claim", "replace named success obligation claims for this task acceptance (repeatable)")
	clearAfter := fs.Bool("clear-after", false, "clear readiness prerequisites")
	clearScope := fs.Bool("clear-scope", false, "clear owned path scope")
	clearContext := fs.Bool("clear-context", false, "clear node-local context")
	clearBoundary := fs.Bool("clear-boundary", false, "clear node boundary")
	clearObligationClaims := fs.Bool("clear-obligation-claims", false, "clear task acceptance obligation claims")
	reason := fs.String("reason", "", "revision reason")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	parentSet := false
	afterSet := *clearAfter
	execCWDSet := false
	scopeSet := *clearScope
	invariantSet := false
	nonGoalsSet := false
	successObligationsSet := false
	ownedSet := false
	excludedSet := false
	obligationClaimsSet := *clearObligationClaims
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "parent" {
			parentSet = true
		}
		if f.Name == "after" {
			afterSet = true
		}
		if f.Name == "exec-cwd" {
			execCWDSet = true
		}
		if f.Name == "scope" {
			scopeSet = true
		}
		if f.Name == "invariant" {
			invariantSet = true
		}
		if f.Name == "non-goal" {
			nonGoalsSet = true
		}
		if f.Name == "success-obligation" {
			successObligationsSet = true
		}
		if f.Name == "owned" {
			ownedSet = true
		}
		if f.Name == "excluded" {
			excludedSet = true
		}
		if f.Name == "obligation-claim" {
			obligationClaimsSet = true
		}
	})
	if *clearAfter && len(after) > 0 {
		return fmt.Errorf("revise uses either --after or --clear-after, not both")
	}
	if *clearScope && len(scope) > 0 {
		return fmt.Errorf("revise uses either --scope or --clear-scope, not both")
	}
	if *privateExec && *sharedExec {
		return fmt.Errorf("revise uses either --private-exec-cwd or --shared-exec-cwd, not both")
	}
	if *clearContext && (invariantSet || nonGoalsSet || successObligationsSet) {
		return fmt.Errorf("revise uses either --clear-context or context fields, not both")
	}
	if *clearBoundary && (ownedSet || excludedSet) {
		return fmt.Errorf("revise uses either --clear-boundary or boundary fields, not both")
	}
	if *clearObligationClaims && len(obligationClaims) > 0 {
		return fmt.Errorf("revise uses either --obligation-claim or --clear-obligation-claims, not both")
	}
	patch := cst.ExecutionEnvelopePatch{
		ExecCWDSet:     execCWDSet,
		ExecCWD:        *execCWD,
		ExecSurfaceSet: *privateExec || *sharedExec || execCWDSet,
		OwnedPathsSet:  scopeSet,
		OwnedPaths:     scope.Values(),
	}
	if *privateExec {
		patch.ExecSurface = cst.ExecSurfacePrivate
	} else {
		patch.ExecSurface = cst.ExecSurfaceShared
	}
	return cst.DoRevise(os.Stdout, id, cst.ReviseArgs{
		ParentSet:        parentSet,
		Parent:           *parent,
		Intent:           *intent,
		Rule:             *rule,
		AcceptanceVerify: *verify,
		VerifyChecks:     checks.Values(),
		AcceptanceReview: *review,
		EnvelopeSet:      patch.ExecCWDSet || patch.ExecSurfaceSet || patch.OwnedPathsSet,
		EnvelopePatch:    patch,
		ContextSet:       *clearContext || invariantSet || nonGoalsSet || successObligationsSet,
		ContextPatch: cst.NodeContextPatch{
			InvariantSet:          invariantSet,
			Invariant:             *invariant,
			NonGoalsSet:           nonGoalsSet,
			NonGoals:              nonGoals.Values(),
			SuccessObligationsSet: successObligationsSet,
			SuccessObligations:    successObligations.Values(),
			Clear:                 *clearContext,
		},
		BoundarySet: *clearBoundary || ownedSet || excludedSet,
		BoundaryPatch: cst.NodeBoundaryPatch{
			OwnedSet:    ownedSet,
			Owned:       owned.Values(),
			ExcludedSet: excludedSet,
			Excluded:    excluded.Values(),
			Clear:       *clearBoundary,
		},
		ObligationClaimsSet: obligationClaimsSet,
		ObligationClaims:    obligationClaims.Values(),
		AfterSet:            afterSet,
		After:               after.Values(),
		Reason:              *reason,
	}, asJSON)
}

func runHold(args []string, asJSON bool) error {
	id, rest, err := splitIDArg(args, "hold")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("hold", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	kind := fs.String("kind", "", "hold kind")
	reason := fs.String("reason", "", "hold reason")
	clear := fs.Bool("clear", false, "clear hold")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	return cst.DoHold(os.Stdout, id, *kind, *reason, *clear, asJSON)
}

func runTake(args []string, asJSON bool) error {
	id, rest, err := optionalIDArg(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("take", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	execCWD := fs.String("exec-cwd", "", "default checkout root for claimed task execution")
	privateExec := fs.Bool("private-exec-cwd", false, "mark exec-cwd as actor-private mutable surface")
	var scope stringListFlag
	fs.Var(&scope, "scope", "owned path under exec checkout (repeatable)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("take accepts at most one positional id before flags")
	}
	envelope, err := buildAddEnvelope(*execCWD, *privateExec, scope.Values())
	if err != nil {
		return err
	}
	return cst.DoTakeWithArgs(os.Stdout, id, cst.TakeArgs{Envelope: envelope}, asJSON)
}

func runRelease(args []string, asJSON bool) error {
	var err error
	args, asJSON, err = extractFormatFlag(args, asJSON)
	if err != nil {
		return err
	}
	id, err := requiredIDArg(args, "release")
	if err != nil {
		return err
	}
	return cst.DoRelease(os.Stdout, id, asJSON)
}

func runRun(args []string, asJSON bool) error {
	id, rest, err := splitIDArg(args, "run")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	cmd := fs.String("cmd", "", "shell command override")
	check := fs.String("check", "", "verify check name to run")
	execCWD := fs.String("exec-cwd", "", "checkout root for shell execution")
	acceptance := fs.Bool("acceptance", false, "run the full verify acceptance and record acceptance_run_set without completing")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	return cst.DoRunWithArgs(os.Stdout, id, cst.RunArgs{
		Override:   *cmd,
		CheckName:  *check,
		Acceptance: *acceptance,
		ExecCWD:    *execCWD,
	}, asJSON)
}

func runDone(args []string, asJSON bool) error {
	id, rest, err := splitIDArg(args, "done")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("done", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	var evidence stringListFlag
	note := fs.String("note", "", "inline review note evidence")
	execCWD := fs.String("exec-cwd", "", "checkout root for verify shell execution")
	fromAcceptance := fs.String("from-acceptance", "", "acceptance_run_set evidence id")
	commitSHA := fs.String("commit", "", "git commit sha to bind as auxiliary evidence")
	fs.Var(&evidence, "evidence", "evidence event id (repeatable)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	return cst.DoDone(os.Stdout, id, cst.DoneArgs{
		EvidenceIDs:      evidence.Values(),
		Note:             *note,
		ExecCWD:          *execCWD,
		FromAcceptanceID: *fromAcceptance,
		CommitSHA:        *commitSHA,
	}, asJSON)
}

func runWorkerStatus(args []string, asJSON bool) error {
	id, rest, err := splitIDArg(args, "worker-status")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("worker-status", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("worker-status takes only flags after the task id")
	}
	return cst.DoWorkerStatus(os.Stdout, id, asJSON)
}

func runWorkerRun(args []string, asJSON bool) error {
	id, rest, err := splitIDArg(args, "worker-run")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("worker-run", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	actionID := fs.String("action", "", "worker frontier action id")
	commitSHA := fs.String("commit", "", "git commit sha to bind as auxiliary evidence")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("worker-run takes only flags after the task id")
	}
	return cst.DoWorkerRun(os.Stdout, id, cst.WorkerRunArgs{ActionID: *actionID, CommitSHA: *commitSHA}, asJSON)
}

func runEvidence(args []string, asJSON bool) error {
	id, rest, err := splitIDArg(args, "evidence")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("evidence", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	kind := fs.String("kind", "", "evidence kind")
	summary := fs.String("summary", "", "evidence summary")
	data := fs.String("data", "", "JSON evidence payload")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	return cst.DoEvidence(os.Stdout, id, cst.EvidenceArgs{
		Kind:    *kind,
		Summary: *summary,
		Data:    *data,
	}, asJSON)
}

func runCancel(args []string, asJSON bool) error {
	id, rest, err := splitIDArg(args, "cancel")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	reason := fs.String("reason", "", "cancel reason")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	asJSON, err = resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	return cst.DoCancel(os.Stdout, id, *reason, asJSON)
}

func runBrief(args []string, asJSON bool) error {
	fs := flag.NewFlagSet("brief", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	within := fs.Int64("within", 0, "scope brief to a goal/task subtree")
	history := fs.Bool("history", false, "include completed child subtrees and historical recent runs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	asJSON, err := resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("brief takes only flags")
	}
	return cst.DoBriefWithOptions(os.Stdout, cst.BriefOptions{ScopeID: *within, History: *history}, asJSON)
}

func runClaims(args []string, asJSON bool) error {
	fs := flag.NewFlagSet("claims", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	within := fs.Int64("within", 0, "scope claims to a goal/task subtree")
	if err := fs.Parse(args); err != nil {
		return err
	}
	asJSON, err := resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("claims takes only flags")
	}
	return cst.DoClaims(os.Stdout, cst.ClaimsArgs{Within: *within}, asJSON)
}

func runRecover(args []string, asJSON bool) error {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	within := fs.Int64("within", 0, "scope recovery view to a goal/task subtree")
	if err := fs.Parse(args); err != nil {
		return err
	}
	asJSON, err := resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("recover takes only flags")
	}
	return cst.DoRecover(os.Stdout, cst.ClaimsArgs{Within: *within}, asJSON)
}

func runShow(args []string, asJSON bool) error {
	var err error
	args, asJSON, err = extractFormatFlag(args, asJSON)
	if err != nil {
		return err
	}
	id, err := requiredIDArg(args, "show")
	if err != nil {
		return err
	}
	return cst.DoShow(os.Stdout, id, asJSON)
}

func runEvents(args []string, asJSON bool) error {
	fs := flag.NewFlagSet("events", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	forID := fs.Int64("for", 0, "node id to read events for")
	attemptID := fs.String("attempt", "", "attempt id to read events for")
	since := fs.String("since", "", "event id cursor; returns events after it")
	all := fs.Bool("all", false, "read the full event stream")
	raw := fs.Bool("raw", false, "print raw JSONL events")
	if err := fs.Parse(args); err != nil {
		return err
	}
	asJSON, err := resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("events takes only flags")
	}
	return cst.DoEvents(os.Stdout, cst.EventsArgs{
		NodeID:       *forID,
		AttemptID:    *attemptID,
		SinceEventID: *since,
		All:          *all,
		Raw:          *raw,
	}, asJSON)
}

func runUI(args []string, asJSON bool) error {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	format := addCommandFormatFlags(fs)
	within := fs.Int64("within", 0, "scope ui to a goal/task subtree")
	output := fs.String("o", "", "output HTML path (default .cst/ui.html)")
	noOpen := fs.Bool("no-open", false, "skip launching the browser")
	stdout := fs.Bool("stdout", false, "write HTML to stdout instead of a file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	asJSON, err := resolveCommandFormat(fs, asJSON, format)
	if err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("ui takes only flags")
	}
	if *stdout && *output != "" {
		return fmt.Errorf("ui --stdout and -o are mutually exclusive")
	}
	return cst.DoUI(os.Stdout, cst.UIArgs{
		Within: *within,
		Output: *output,
		NoOpen: *noOpen || *stdout,
		Stdout: *stdout,
	}, asJSON)
}

// argument parsing helpers — the positional id must be the first non-flag arg
// when present.
func splitIDArg(args []string, name string) (int64, []string, error) {
	if len(args) == 0 {
		return 0, nil, fmt.Errorf("%s requires an id", name)
	}
	if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
		return id, args[1:], nil
	}
	return 0, nil, fmt.Errorf("%s requires an id as first argument", name)
}

func requiredIDArg(args []string, name string) (int64, error) {
	if len(args) == 0 {
		return 0, fmt.Errorf("%s requires an id", name)
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid id %q", name, args[0])
	}
	return id, nil
}

func optionalIDArg(args []string) (int64, []string, error) {
	if len(args) == 0 {
		return 0, args, nil
	}
	if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
		return id, args[1:], nil
	}
	return 0, args, nil
}
