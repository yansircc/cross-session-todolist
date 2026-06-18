---
name: cst
description: Use when working in a repo that uses CST (`.cst/events.jsonl`) as the only task source, including creating/resuming tasks, interpreting claims, running verify/review acceptance, recording evidence, inspecting attempt-correlated events, or installing/updating the CST CLI.
---

# CST

Use `cst -h` as the canonical runbook. This skill keeps only the operating
contract an agent needs before touching a CST-backed repo.

## Invariant

`.cst/events.jsonl` is the only task source. Do not create sidecar task state,
mutable task notes, or plan/checklist fallbacks.

State is `fold(events)`. Use write commands to append facts and read commands to
project them.

## First Commands

```sh
cst brief
```

`cst brief` is frontier-first by default: it expands active child subtrees and
summarizes completed child subtrees. Use `cst brief --history` only when you
need completed child subtrees or historical recent runs/failures.

If claims exist, inspect them:

```sh
cst claims
cst show <task-id>
```

If no claim exists, take a ready task:

```sh
cst take
```

## Work Loop

1. `cst brief`
2. `cst show <claimed-task-id>`
3. Do the repo work.
4. Optional probe:

   ```sh
   cst run <task-id>
   cst run <task-id> --check <name>
   cst run <task-id> --cmd "custom probe"
   ```

5. Finish according to acceptance:

   ```sh
   cst done <task-id>
   cst done <task-id> --note "reviewed locally"
   cst done <task-id> --evidence <event-id>
   cst hold <task-id> --kind blocked --reason "..."
   cst release <task-id>
   cst cancel <task-id> --reason "..."
   ```

Stop only when `cst brief` reports the root as `completed` and no claims remain.

## Boundary Identity

Do not let process cwd imply identity across a session or worker boundary.

- `--store <repo-root>` selects the central CST ledger owner.
- `--exec-cwd <checkout-root>` on `add` / `revise` sets the task execution
  envelope; on `run` / `done` it is only a one-command override.
- `--private-exec-cwd` marks the checkout as actor-private. Without it the
  surface is shared.
- `--scope <path>` declares owned paths for scoped drift checks and projection
  noise reduction. Scope is a view, not truth: out-of-scope changes are still
  recorded.
- Events record `store_id` (root `node_created.event_id`), `exec_cwd`, git
  checkout identity, whole-repo and scoped diff hashes, out-of-scope summaries,
  and full log artifact references. They do not record absolute `store_root` as
  durable identity.

Worker acceptance flow:

```sh
cst --store /central/repo revise 12 --exec-cwd /worker/repo --private-exec-cwd --scope internal/parser
cst --store /central/repo worker-status 12 --human
cst --store /central/repo worker-run 12 --action <action-id>
```

For verify tasks, ordinary `--note` / `--evidence` completion is still invalid.
Completion evidence must be `acceptance_run_set`, which explicitly maps every
declared check to the successful `script_run` event that satisfied it.
Private execution surfaces reject any final context drift. Shared surfaces
reject scoped drift but record `evidence(kind=context_drift)` and allow
completion for out-of-scope drift because shared checkouts cannot attribute that
change to one actor.

`worker-status` is read-only and derives bound legal actions from the event log.
`worker-run` reprojects the frontier at execution time and refuses stale action
ids. The previewed `run --acceptance` / `done --from-acceptance` commands are
informational; the action id is the executable worker handoff.

## Modeling

Create one root goal per store, then child goals/workstreams:

```sh
cst add --intent "Project goal"
cst add --parent 1 --goal --intent "Workstream"
cst add --parent 1 --rule "One fact, one location"
```

Tasks need exactly one acceptance kind:

```sh
cst add --parent 2 --intent "Implement" --verify "go test ./..."
cst add --parent 2 --intent "Implement with named gates" \
  --check unit="go test ./..." \
  --check help="go run ./cmd/cst -h >/dev/null"
cst add --parent 2 --intent "Implement in worker checkout" \
  --exec-cwd /worker/repo --private-exec-cwd --scope internal/parser \
  --check unit="go test ./internal/parser"
cst add --parent 2 --intent "Review" --review self
```

Use `--after <id>` for internal sequencing. Reserve `hold` for external pauses.

Correct tree shape with `revise`; do not create duplicate replacement tasks when
identity should be preserved.

## Verifier Contracts

For risky tasks, acceptance criteria must derive from a canonical source of
truth that the implementation task does not edit. Do not treat a self-authored
validator as enough unless it has a frozen source or manifest, red cases, and
explicit blind spots.

Use two tasks:

```sh
cst add --parent 2 --intent "Freeze verifier contract" --review self
cst add --parent 2 --intent "Implement against frozen contract" --after 7 \
  --check contract-lock="scripts/verify-contract-lock --contract .artifacts/verifier-contract.json" \
  --check coverage="..." \
  --check red="..." \
  --check real="..."
```

Record the contract as evidence:

```sh
cst evidence 7 --kind verifier_contract --summary "frozen acceptance contract" --data '{"canonical_source":{"ref":"git:<sha>:<path>","description":"..."},"contract_artifacts":[{"path":"...","sha256":"..."}],"verifier_scripts":[{"path":"scripts/verify-contract-lock","sha256":"..."},{"path":"cmd/verify-contract-lock/main.go","sha256":"..."}],"manifest":{"path":"...","sha256":"...","count":0},"cheapest_plausible_lie":"...","red_case_runs":[{"name":"...","diff_path":"...","diff_sha256":"...","command":"...","expected_exit":1,"observed_exit":1,"stderr_path":"...","stderr_sha256":"..."}],"blind_spots":[{"axis":"...","reason":"...","review":"..."}]}'
```

Enumerable work must freeze a manifest first. Non-enumerable work must name an
external source of truth or record `blind_spots`; do not claim
machine-verified completeness for uncovered axes. `canonical_source.ref` must
name a stable object such as `git:<sha>:<path>`, `path@<sha>`, or
`url@<version>`. The ref is a declaration: CST and `contract-lock` validate its
shape, not the remote object. Mechanical closure comes from the hash chain over
contract artifacts, verifier scripts, manifests, and red-case outputs. Include
both the lock shim and its real implementation, such as
`cmd/verify-contract-lock/main.go`, in `verifier_scripts`. Red cases must reject
the cheapest plausible lie with executed failure artifacts, not prose only.

## Evidence And Attempts

Durable process notes are evidence:

```sh
cst evidence <id> --kind note --summary "..."
```

Do not add task note fields or invent `cst note`.

`cst take` mints an `attempt_id`. Claim renewal/release, script runs, evidence,
and completion from the same claim carry that id. Inspect one attempt with:

```sh
cst events --attempt <attempt-id>
```

Use `cst claims` or `cst recover` when a session restarts or multiple agents
have touched the store. They are read-only views: actor, task, attempt, lease
staleness, task envelope, path-overlap warnings, and latest execution identity.
They do not auto-release stale claims.

Use an explicit actor for long verify runs:

```sh
cst --actor agent-parser run 12 --acceptance
cst --actor agent-parser done 12 --from-acceptance <acceptance-run-set-evidence-id>
```

Only the same explicit actor may auto-renew an active or expired claim during
`run --acceptance` / verify `done`. Other actors must take over explicitly with
`cst take`; CST does not infer liveness.

Review tasks can record structured coverage with
`evidence(kind=review_checklist)` using an `items` array. Checklist templates
must use a different kind such as `review_checklist_template`; do not put
template-shaped data under `review_checklist`. `done --commit <sha>` records an
auxiliary commit edge only; it does not replace verify or review evidence.

Do not infer workstream scope from `attempt_id`; use explicit `--parent` and
`--within`.

## Install

From the CST repo:

```sh
cd /Users/yansir/code/52/cross-session-todolist
make install
make skill
```
