# cst

Agent-first cross-session task system.

`cst -h` is the primary agent runbook. Keep it useful enough for an agent that
has never seen this repository or any historical plan. This README explains the
same model from an empty store.

## Invariant

`.cst/events.jsonl` is the only task source. It is append-only and should be
reviewable in git.

Chat history, hook state files, generated projections, and external documents
are not task state. If an existing project has another planning artifact,
translate it once into `cst` nodes and stop updating it as a checklist.

## Install

```sh
cd /Users/yansir/code/52/cross-session-todolist
make install
```

For local development:

```sh
make test
```

Do not build generated binaries into the repo root. If a local executable is
needed, build to `/Users/yansir/.local/bin/cst`.

Install the Codex skill that teaches agents the current CST contract:

```sh
make skill
```

## Start From Nothing

Create one root goal. A store has one root for life.

```sh
cst add --intent "Ship the project"
```

Model separate todolists as child goals under the same root.

```sh
cst add --parent 1 --goal --intent "Runtime migration"
cst add --parent 1 --goal --intent "Documentation cleanup"
```

Add rules as inherited context. Rules are for agents to read; they are not a
hidden policy engine.

```sh
cst add --parent 1 --rule "One fact, one location"
cst add --parent 1 --rule "No silent fallback"
```

Add executable tasks under a goal or task. Every task must have exactly one
acceptance kind. Readiness prerequisites are optional `--after` edges.

```sh
cst add --parent 2 --intent "Port SQL boundary" --verify "go test ./..."
cst add --parent 2 --intent "Run narrow gates" --check unit="go test ./..." --check help="go run ./cmd/cst -h >/dev/null"
cst add --parent 2 --intent "Review migration shape" --review self
cst add --parent 2 --intent "Publish after tests" --after 4 --review self
```

## Data Model

```txt
node = id + parent + lifecycle + event log
goal = node + aggregate progress + non-claimable + derived completion
task = node + acceptance + after prerequisites + claimable + completable
rule = node + text + inherited projection + non-claimable + non-completable
attempt = claim-scoped correlation id for run/evidence/completion events
```

Goals are not taken or completed directly. A goal is complete when every
descendant task is completed or canceled and every declared success obligation
in the subtree is covered by descendant leaf task obligation claims.

Tasks are the unit of execution. A task can be claimed, probed, held, completed,
or canceled.

Attempts are not separate task state. `cst take` mints an `attempt_id`; subsequent
claim renewal/release, script runs, evidence, and completion for the same claim
carry that id so projections can reconstruct one execution attempt without any
external current-state file.

Rules are context. `brief` and `show` project inherited rules so the next agent
does not need to rediscover stable constraints.

## Node Context, Boundary, And Obligations

Context is node-local. Put global context high in the tree and local deltas on
the leaf or subtask that owns them. `show`, `take`, `worker-status`, and `ui`
derive a developer briefing by walking root to node; descendants do not store a
workstream pointer.

```sh
cst add --parent 1 --goal --intent "Parser migration" \
  --invariant "Parser API stays source-compatible" \
  --non-goal "Do not rewrite runtime loaders" \
  --success-obligation parser-contract
```

Boundary is also node-local and declared once. It is used both in briefing and
completion validation.

```sh
cst add --parent 2 --intent "Port parser declaration emit" \
  --owned internal/parser --excluded internal/runtime \
  --obligation-claim parser-contract \
  --check unit="go test ./internal/parser"
```

Reducer-checked boundary algebra:

- child `owned` paths must sit inside parent `owned` paths when the parent has
  an owned boundary;
- active sibling `owned` paths must not overlap; completed and canceled sibling
  boundaries remain historical evidence and do not reserve those paths forever;
- verify completion rejects accepted diffs outside the task's `owned` boundary
  or inside its `excluded` boundary.

Success coverage is a named set relation. The union of `success_obligations`
declared in a subtree must be covered by descendant static leaf task
`obligation_claims`; missing coverage is projected in briefing and keeps the
goal open. This proves set coverage, not human understanding.

## Agent Loop

The consumer policy is intentionally one sentence:

```sh
cst next
```

`next` is a read-only projection over the current ledger and worktree. It
returns one of:

- `action`: a bound legal action such as take, run acceptance, or complete from
  an acceptance run set;
- `repair`: a minimal command template for init, reconcile, boundary, or
  obligation repair;
- `required=input`: ask for the named input, record it, and rerun `cst next`;
- `phase=no-op`: the root is complete and no work remains.

Then follow this loop:

```sh
cst next                 # project the only legal next procedure step
cst worker-run <id> --action <action-id>
# or execute the returned repair command template, fill required input, then rerun cst next
```

`next`, `take`, `show`, `worker-status`, and `ui` all project the same developer
briefing when a task is selected: root-to-node context fold, local boundary,
direct upstream/downstream edges, acceptance obligation claims, success coverage,
and partition warnings. This makes global context visible before implementation;
it does not prove the developer understood it.

`next` reconcile uses active task `node.boundary` only. A completed task
boundary that once covered a path is historical evidence, not current ownership
for new dirty work. `execution.scope` / `OwnedPaths` is execution identity and
drift detection; it is not task-tree ownership.

If a task cannot continue now:

```sh
cst hold <task-id> --kind blocked --reason "waiting for upstream API"
cst hold <task-id> --kind waiting --reason "PR submitted; waiting review"
cst hold <task-id> --kind deferred --reason "finish smaller workstream first"
```

Clear the hold when it is actionable again:

```sh
cst hold <task-id> --clear
```

If the claim should return to the pool:

```sh
cst release <task-id>
```

If the work is no longer needed:

```sh
cst cancel <id> --reason "merged into #12"
```

Stop only when `cst brief` reports the root goal as `completed` and `claims` is
empty.

## Command Surface

```sh
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
cst done <task-id> --from-acceptance <acceptance-run-set-evidence-id> [--commit <sha>]
cst done <task-id> [--evidence <event-id> ... | --note "..."]
cst cancel <id> --reason "..."
```

JSON is the default output for every command. Use `--human` for manual display.
There is no `--json` compatibility flag.

## Acceptance And Readiness

`--verify <cmd>` is shorthand for one verify check named `default`. `done` runs
the command and completes only on exit 0.

```sh
cst add --parent 1 --intent "Fix parser" --verify "go test ./internal/parser"
cst take 2
cst done 2
```

Use repeatable `--check <name=cmd>` when acceptance has multiple named checks.
`done` runs each check in declaration order, records one `script_run` per check,
and completes only when every check exits 0.

```sh
cst add --parent 1 --intent "Fix parser" \
  --check unit="go test ./internal/parser" \
  --check help="go run ./cmd/cst -h >/dev/null"
cst take 2
cst done 2
```

Verify acceptance records successful `script_run(trigger=acceptance)` events,
then records one `evidence_recorded(kind=acceptance_run_set)` that explicitly
maps each declared check to the script_run event that satisfied it.
`task_completed.evidence_ids` includes that run-set evidence, not the last
script run. Completion replays the run-set and rejects missing checks, failed
runs, mixed execution contexts, stale acceptance digests, and manual `--note` on
verify tasks. Supplemental `--evidence` ids are allowed with
`--from-acceptance` so the completion can bind the full evidence set that
satisfies the task obligations.

Worker checkouts must separate ledger identity from execution identity. For
cross-session completion, persist the execution envelope on the task first:

```sh
cst --store /central/repo revise 12 --exec-cwd /worker/repo --private-exec-cwd --scope internal/parser
cst --store /central/repo worker-status 12 --human
cst --store /central/repo worker-run 12 --action <action-id>
```

`--store` chooses the CST ledger owner. `--exec-cwd` chooses the checkout where
the shell command runs. They are independent axes. `--exec-cwd` on `add` or
`revise` becomes the task default; `cst take <id> --exec-cwd ...` atomically
binds the task envelope and claim in one ledger transaction. `--exec-cwd` on
`run` / `done` is only a one-command override and does not mutate the task
envelope. Without `--store`, CST resolves the ambient ledger root by walking up
to the nearest existing `.cst`; if none exists, it uses the enclosing git root
before falling back to cwd. That discovery is not an explicit store binding and
does not bypass worker-checkout guards. `--scope` paths are relative to the
execution checkout, never absolute and never `..`-escaping. Events record
`store_id` (the root `node_created.event_id`), `exec_cwd`, git
head/branch/status, whole-repo diff hashes, scoped diff hashes when
`--scope` exists, out-of-scope summaries, and full stdout/stderr artifact
references. They do not record absolute `store_root` as ledger identity.

`--private-exec-cwd` means the execution checkout is actor-private. Completion
rejects any context drift between acceptance and done. The default surface is
`shared`; on shared checkouts, scoped drift rejects, while out-of-scope drift is
recorded as `evidence(kind=context_drift)` because git cannot attribute that
change to one actor.

`worker-status` is the worker execution projection. It recomputes the legal
frontier from the event log and returns bound actions with `store_root`, `store_id`,
task id, claim, attempt, execution envelope, and evidence id already filled.
`worker-run` accepts one of those action ids, recomputes the frontier at execution
time, and refuses stale or drifted actions. The previewed `run` / `done` commands
are informational; the wrapper is the closed loop.

When a worker checkout is explicitly bound through a central store and
`--exec-cwd`, CST records a local worker binding outside the git work surface
when possible. The binding is a cache: readers accept it only when its
`store_id` matches the replayed central ledger root. In that detectable worker
checkout, mutating commands without explicit `--store` fail before opening a
local ledger and print a recovery command such as
`cst --store /central/repo take 12 --exec-cwd /worker/repo`.
This guard does not guess central stores; ordinary single-checkout repos without
a worker binding keep normal ambient-store behavior.

`--review <who>` means `done` requires evidence or a note.

```sh
cst done 3 --note "reviewed locally"
```

`--after <id>` means the task is not ready until the referenced node is
completed. It does not prove completion; it only controls readiness.

```sh
cst add --parent 1 --intent "Publish" --after 7 --review self
```

## Verifier Contracts

For risky tasks, the verifier criteria must come from a canonical source of
truth that the implementation task does not edit. A passing validator is not
completion if it only proves the cheapest partial implementation.

Model that as two tasks:

```sh
cst add --parent 2 --intent "Freeze verifier contract" --review self
cst add --parent 2 --intent "Implement against frozen contract" --after 7 \
  --check contract-lock="scripts/verify-contract-lock --contract .artifacts/verifier-contract.json" \
  --check coverage="make verify-coverage" \
  --check red="make verify-red-cases" \
  --check real="make verify"
```

Record the frozen contract as evidence on the contract task:

```sh
cst evidence 7 --kind verifier_contract --summary "frozen acceptance contract" --data '{
  "canonical_source": {"ref": "git:<sha>:<path>", "description": "..."},
  "contract_artifacts": [{"path": "...", "sha256": "..."}],
  "verifier_scripts": [
    {"path": "scripts/verify-contract-lock", "sha256": "..."},
    {"path": "cmd/verify-contract-lock/main.go", "sha256": "..."}
  ],
  "manifest": {"path": "...", "sha256": "...", "count": 0},
  "cheapest_plausible_lie": "...",
  "red_case_runs": [{
    "name": "...",
    "diff_path": "...",
    "diff_sha256": "...",
    "command": "...",
    "expected_exit": 1,
    "observed_exit": 1,
    "stderr_path": "...",
    "stderr_sha256": "..."
  }],
  "blind_spots": [{"axis": "...", "reason": "...", "review": "..."}]
}'
```

If the task is enumerable, freeze a manifest first so partial output fails by
diff or row coverage. If it is not enumerable, derive the verifier from an
external source of truth. If no such source exists, record the axis under
`blind_spots` and do not claim machine-verified completeness. `canonical_source.ref`
must name a stable object such as `git:<sha>:<path>`, `path@<sha>`, or
`url@<version>`. The ref is a declaration: CST and `contract-lock` validate its
shape, not the remote object. Mechanical closure comes from the hash chain over
contract artifacts, verifier scripts, manifests, and red-case outputs. Those
artifact paths are relative to the verifier root and cannot be absolute or
escape with `..`. `contract-lock` must rehash that chain; CST does not do that
semantic check in the reducer. Red cases must reject the cheapest plausible lie
with executed failure artifacts, not prose only.

## Evidence

Evidence is an event ledger, not a mutable field.

```sh
cst evidence 3 --kind commit --summary "fixed in abc123" --data '{"sha":"abc123"}'
cst done 3 --evidence <event-id>
```

Process notes are evidence too:

```sh
cst evidence 3 --kind note --summary "Investigated parser drift; next attempt should start at reducer validation."
```

Do not add a task note field, invent a `cst note` command, or use CST as a
scratchpad. Notes should be durable facts that help the next agent avoid
repeating work.

For review acceptance, `--note` creates `evidence_recorded(kind=note)` and completes
in one transaction.

Structured review tasks can record `evidence_recorded(kind=review_checklist)`:

```json
{"items":[{"id":"api","criterion":"review API boundary","status":"pass","evidence":"checked handlers"}],"blind_spots":[]}
```

The reducer validates checklist shape and statuses (`pass`, `fail`, `na`); it
does not turn review judgment into a policy engine. Checklist templates should
use a different evidence kind such as `review_checklist_template`;
`review_checklist` is reserved for itemized review results.

For verify acceptance, `done` records successful `script_run(trigger=acceptance)`
events and an `acceptance_run_set` evidence, then completes with an evidence set
that includes that run-set. `cst run` records `script_run(trigger=probe)` without
changing status. If a task has multiple verify checks, `cst run <id> --check
<name>` selects one probe check.

Closure evidence is split by what CST can verify:

- `evidence(kind=boundary)` uses JSON `{"includes":[...],"excludes":[...]}`.
  At completion, CST checks these paths against the actual accepted diff:
  claimed includes must be covered and claimed excludes must not be touched.
- `evidence(kind=rationale)` records non-vacuous structured attestation:
  `invariant`, `failure`, `minimal_fix`, `remaining_risk`, and optional
  `not_doing`. CST validates shape and projects it; it does not prove the
  rationale is true.
- `evidence(kind=contest)` targets a boundary or rationale evidence id and
  marks it contested for later review. Review, not the reducer, resolves the
  truth of rationale.

Each `script_run` keeps bounded `stdout_head` / `stderr_head` for projections.
Full non-empty stdout/stderr is written under `.cst/artifacts/runs/` and the
event stores relative artifact path, sha256, and byte size.

`cst done --commit <sha>` records auxiliary `evidence_recorded(kind=commit)`.
It never replaces verify `acceptance_run_set` or review evidence; CST binds to
an existing commit object and does not execute `git commit`.

## Tree Correction

Use `revise` when the tree is wrong. It is append-only and preserves the node id,
runs, evidence, and event history.

```sh
cst revise 4 --intent "Fix parser boundary" --reason "scope clarified"
cst revise 4 --parent 9 --reason "belongs under validation phase"
cst revise 4 --verify "go test ./internal/parser" --reason "acceptance was too broad"
cst revise 4 --exec-cwd /worker/parser --private-exec-cwd --scope internal/parser --reason "worker checkout and owned boundary"
cst revise 4 --after 7 --after 8 --reason "must wait for prerequisites"
cst revise 4 --clear-after --reason "prerequisite was folded into the task"
cst revise 4 --clear-scope --reason "scope moved to child tasks"
cst revise 7 --rule "No silent fallback" --reason "wording tightened"
```

`revise` refuses claimed nodes, terminal nodes, moving the root goal, cycles,
self/ancestor prerequisites, prerequisite cycles, and no-op revisions.

There are no split or merge verbs:

- Split oversized work by adding child tasks under it.
- Merge duplicate work by canceling the duplicate with a reason.

## Multiple Workstreams

Do not create multiple roots or multiple task stores for one project. Use child
goals:

```txt
#1 root goal
  #2 goal: runtime migration
  #3 goal: docs cleanup
  #4 goal: release work
```

If a large task is temporarily not actionable, hold it:

```sh
cst hold 8 --kind deferred --reason "finish docs cleanup first"
```

Then take a specific ready task from another workstream:

```sh
cst brief
cst take <ready-task-id>
```

Held tasks still keep the root open. They are not completion.

## Brief

`cst brief` is the bounded projection for both agents and users. It includes:

- `revision`: event count and latest event id.
- `root`: root goal and derived status.
- `summary`: total/open/ready/claimed/held/completed/canceled counts.
- `scope`: selected subtree when using `--within`.
- `subtrees`: frontier-first child progress; by default this expands only child
  work nodes that still contain open tasks.
- `completed_subtrees_meta`: completed/canceled child work hidden from the
  default frontier view.
- `ready`: tasks eligible for `take`.
- `review_ready`: ready tasks with review acceptance.
- `waiting_on`: tasks paused by incomplete `--after` prerequisites.
- `dependency_failed`: tasks whose prerequisite was canceled.
- `held`, `claims`, `recent_failures`, `recent_runs`, `recent_done`.
- completed task evidence sets and closure summaries when they are present.
- `*_meta`: total/shown/truncated metadata for bounded collections.

Use `cst brief --within <id>` to focus on one child goal/workstream without
changing the global Stop hook condition.

Use `cst brief --history` when you explicitly need completed child subtrees and
historical recent runs/failures. The default brief is an operator view of the
current frontier, not a full history browser.

## Show And Events

`cst show <id>` is a bounded single-node view. It includes scalar node facts,
aggregate progress, inherited rules, completed evidence ids, closure
boundary/rationale/contest projection, and bounded previews of children, recent
runs, and recent evidence. It is not a subtree dump.

`cst worker-status <task-id>` is a bounded worker view over one task. It derives
legal bound actions from the same admissibility predicate used by completion, and
it records subagents only as external observations. `cst worker-run` must
re-read this frontier before executing an action id.

`cst ui` renders the same bounded frontier, completed evidence ids, closure
summary, and contested state for browser review. It is a projection, not another
task source.

Use explicit event ranges for history:

```sh
cst events --for <id>
cst events --attempt <attempt-id>
cst events --since <event-id>
cst events --for <id> --attempt <attempt-id> --since <event-id>
cst events --all --raw
```

`events --all --raw` is the export path. It is intentionally explicit and should
not be part of the normal Agent loop.

Projection commands return a consistent snapshot at their reported `revision`.
Concurrent writes after that snapshot are not reflected; re-run `cst brief` after
any mutation or confusing claim state.

## Claims And Recovery

`cst claims [--within <id>]` and `cst recover [--within <id>]` are read-only
operator projections. They list current claim holder, task, attempt id, lease
expiry, stale status, task envelope, path-overlap warnings, and latest
execution identity observed for that attempt. They do not auto-release claims.
`run --acceptance` and verify `done` can renew an active or expired claim only
for the same explicit actor (`--actor`, `CST_ACTOR`, or `actor.default`). Other
actors must take over explicitly; CST does not silently decide liveness or
ownership.

## Storage

```txt
.cst/events.jsonl   append-only event log; source of truth
.cst/events.lock    advisory transaction lock; do not track as task state
.cst/config.toml    optional budgets, timeouts, lease TTL, actor default
.cst/artifacts/     hash-checked run witness attachments referenced by events
```

State is rebuilt by replaying `events.jsonl` through a checked reducer on every
command. Corrupt histories fail loudly: duplicate ids, multiple roots,
nil-acceptance tasks, rule-under-rule, double terminal, prerequisite cycles,
claim/lease/attempt drift, duplicate verify check names, and invalid completion
evidence are rejected.

## Config

```toml
[brief]
max_tasks = 10
max_rules = 20
max_recent = 5

[runner]
default_timeout_seconds = 300
stdout_max_bytes = 4096
stderr_max_bytes = 4096

[claim]
lease_ttl_seconds = 600
renew_every_seconds = 120

[actor]
default = ""
```

`CST_ACTOR` overrides `actor.default`. If neither is set, cst uses
`user@hostname`.

## Boundaries

No init command, no physical event deletion, no priority score, no due date, no
dependency DSL, no interactive mode, no hook surface, no cross-repo aggregation.

Append facts. Read projections. Correct tree shape with `revise`. Do not infer
workstream scope from `attempt_id`; task parent and brief scope stay explicit
through `--parent` and `--within`.
