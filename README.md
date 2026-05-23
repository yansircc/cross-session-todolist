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
descendant task is completed or canceled.

Tasks are the unit of execution. A task can be claimed, probed, held, completed,
or canceled.

Attempts are not separate task state. `cst take` mints an `attempt_id`; subsequent
claim renewal/release, script runs, evidence, and completion for the same claim
carry that id so projections can reconstruct one execution attempt without any
external current-state file.

Rules are context. `brief` and `show` project inherited rules so the next agent
does not need to rediscover stable constraints.

## Agent Loop

Every agent turn should begin with:

```sh
cst brief
```

Then follow this loop:

```sh
cst take                 # claim the next ready task
cst show <task-id>       # inspect full context
cst run <task-id>        # optional probe; records script_run(trigger=probe)
cst done <task-id>       # verify acceptance runs its command and records evidence
```

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
cst add  --intent "Root goal"
cst add  --parent <id> --goal --intent "Child goal / workstream"
cst add  --parent <id> --intent "Task" (--verify "cmd" | --check <name=cmd>... | --review "who") [--after <node-id> ...]
cst add  --parent <id> --rule "Invariant or context visible to agents"
cst revise <id> [--parent <id>] [--intent "..." | --rule "..."] [--verify "..." | --check <name=cmd>... | --review "..."] [--after <id> ... | --clear-after] [--reason "..."]

cst brief [--within <id>] [--history]
cst show <id>
cst events --for <id>
cst events --attempt <attempt-id>
cst events --since <event-id>
cst events --for <id> --attempt <attempt-id> --since <event-id>
cst events --all --raw

cst take [<task-id>]
cst release <task-id>
cst hold <task-id> --kind blocked|waiting|deferred --reason "..."
cst hold <task-id> --clear
cst run <task-id> [--check <name>] [--cmd "..."]
cst evidence <id> --kind <kind> --summary "..." [--data JSON]
cst evidence <id> --kind note --summary "Process note..."
cst done <task-id> [--evidence <event-id> | --note "..."]
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
and stops at the first failed check without completing the task.

```sh
cst add --parent 1 --intent "Fix parser" \
  --check unit="go test ./internal/parser" \
  --check help="go run ./cmd/cst -h >/dev/null"
cst take 2
cst done 2
```

Verify acceptance uses the successful acceptance run as completion evidence. It rejects
manual `--note` and `--evidence` so evidence cannot be silently ignored.

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
contract artifacts, verifier scripts, manifests, and red-case outputs.
`contract-lock` must rehash that chain; CST does not do that semantic check in
the reducer. Red cases must reject the cheapest plausible lie with executed
failure artifacts, not prose only.

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

For verify acceptance, `done` records successful `script_run(trigger=acceptance)`
events as completion evidence. `cst run` records `script_run(trigger=probe)`
without changing status. If a task has multiple verify checks, `cst run <id>
--check <name>` selects one probe check.

## Tree Correction

Use `revise` when the tree is wrong. It is append-only and preserves the node id,
runs, evidence, and event history.

```sh
cst revise 4 --intent "Fix parser boundary" --reason "scope clarified"
cst revise 4 --parent 9 --reason "belongs under validation phase"
cst revise 4 --verify "go test ./internal/parser" --reason "acceptance was too broad"
cst revise 4 --after 7 --after 8 --reason "must wait for prerequisites"
cst revise 4 --clear-after --reason "prerequisite was folded into the task"
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
- `*_meta`: total/shown/truncated metadata for bounded collections.

Use `cst brief --within <id>` to focus on one child goal/workstream without
changing the global Stop hook condition.

Use `cst brief --history` when you explicitly need completed child subtrees and
historical recent runs/failures. The default brief is an operator view of the
current frontier, not a full history browser.

## Show And Events

`cst show <id>` is a bounded single-node view. It includes scalar node facts,
aggregate progress, inherited rules, and bounded previews of children, recent
runs, and recent evidence. It is not a subtree dump.

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

## Storage

```txt
.cst/events.jsonl   append-only event log; source of truth
.cst/events.lock    advisory transaction lock; do not track as task state
.cst/config.toml    optional budgets, timeouts, lease TTL, actor default
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
