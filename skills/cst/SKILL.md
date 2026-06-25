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
cst next
```

`cst next` is the repo-level procedure projection. It is read-only and returns a
phase plus either one bound legal action, one minimal repair contract, required
input, or `phase=no-op`. The consumer policy is: run `cst next`, execute its
returned action or repair command template, then rerun `cst next`.

For a worker checkout, bind the execution envelope and claim atomically:

```sh
cst --store /central/repo take 12 --exec-cwd /worker/repo --private-exec-cwd --scope internal/parser
```

## Work Loop

1. `cst next`
2. If `action` is present, execute the returned bound action. Worker actions use:

   ```sh
   cst worker-run <task-id> --action <action-id>
   ```

3. If `repair` is present, fill only the required placeholders in the returned
   command template, execute it, then rerun `cst next`.
4. If `required=input`, ask for the named input, record it in CST, then rerun
   `cst next`.
5. If `phase=no-op`, confirm no claims remain and stop.
6. During implementation, optional probes still use:

   ```sh
   cst run <task-id>
   cst run <task-id> --check <name>
   cst run <task-id> --cmd "custom probe"
   ```

Use `cst brief`, `cst show <task-id>`, and `cst claims` as diagnostic
projections. They are not the primary procedure loop. Stop only when `cst next`
returns `phase=no-op` and no claims remain.

## Boundary Identity

Do not let process cwd imply identity across a session or worker boundary.

- `--store <repo-root>` selects the central CST ledger owner.
- `--exec-cwd <checkout-root>` on `add` / `revise` sets the task execution
  envelope; on `run` / `done` it is only a one-command override.
- `--private-exec-cwd` marks the checkout as actor-private. Without it the
  surface is shared.
- `cst take <id> --exec-cwd ...` binds the task envelope and claim in one
  transaction, which is the worker setup path when the worker path is known.
- `--scope <path>` declares owned paths for scoped drift checks and projection
  noise reduction. Scope is a view, not truth: out-of-scope changes are still
  recorded.
- Events record `store_id` (root `node_created.event_id`), `exec_cwd`, git
  checkout identity, whole-repo and scoped diff hashes, out-of-scope summaries,
  and full log artifact references. They do not record absolute `store_root` as
  durable identity.
- Detectable worker checkouts reject mutating commands without explicit
  `--store` before opening a local ledger. Use the printed recovery command; do
  not rerun from worker cwd with ambient store identity.

Worker acceptance flow:

```sh
cst --store /central/repo revise 12 --exec-cwd /worker/repo --private-exec-cwd --scope internal/parser
cst --store /central/repo worker-status 12 --human
cst --store /central/repo worker-run 12 --action <action-id>
```

For verify tasks, ordinary `--note` / `--evidence` completion is still invalid.
Completion evidence must be `acceptance_run_set`, which explicitly maps every
declared check to the successful `script_run` event that satisfied it.
Completions bind `evidence_ids`; use repeatable `--evidence` with
`--from-acceptance` only for supplemental structured evidence that belongs to
the same task.
Private execution surfaces reject any final context drift. Shared surfaces
reject scoped drift but record `evidence(kind=context_drift)` and allow
completion for out-of-scope drift because shared checkouts cannot attribute that
change to one actor.

`worker-status` is read-only and derives bound legal actions from the event log.
`worker-run` reprojects the frontier at execution time and refuses stale action
ids. The previewed `run --acceptance` / `done --from-acceptance` commands are
informational; the action id is the executable worker handoff.

`next` reuses the same bound action generator at repo level. `brief`, `show`,
and `ui` project completed evidence ids, closure summaries, and contested state.
Use those bounded projections before reading raw events.

## Modeling

Create one root goal per store, then child goals/workstreams:

```sh
cst add --intent "Project goal"
cst add --parent 1 --goal --intent "Workstream"
cst add --parent 1 --rule "One fact, one location"
```

Node-local context/boundary/obligations are one generator. Put durable global
context high in the tree and local deltas on the node that owns them; descendants
derive briefing by root-to-node projection, not by storing a workstream pointer:

```sh
cst add --parent 1 --goal --intent "Parser migration" \
  --invariant "Parser API stays source-compatible" \
  --non-goal "Do not rewrite runtime loaders" \
  --success-obligation parser-contract
```

Boundary is a task-tree partition, not the execution `--scope`. Declare it once
on the node and let CST reuse it for briefing and verify completion checks:

```sh
cst add --parent 2 --intent "Port parser declaration emit" \
  --owned internal/parser --excluded internal/runtime \
  --obligation-claim parser-contract \
  --check unit="go test ./internal/parser"
```

- child `owned` paths must be inside parent `owned` paths when the parent has a
  declared owned boundary.
- active sibling `owned` paths cannot overlap; completed and canceled sibling
  boundaries remain historical evidence and do not reserve those paths forever.
- verify completion rejects accepted diffs outside the task `owned` boundary or
  inside its `excluded` boundary.
- named `success_obligations` in a subtree must be covered by descendant static
  leaf task `obligation_claims`; missing coverage is projected and keeps goals
  open.

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

Before implementation, use `cst show`, `cst take`, `cst worker-status`, or
`cst ui` to read the developer briefing. `cst next` also includes the briefing
when it selects a task. The briefing includes root-to-node
context fold, local boundary, upstream/downstream edges, local acceptance,
obligation claims, success coverage, and partition warnings. This projection
makes global context recoverable; it does not prove the agent understood prose.

`cst next` reconcile uses active task `boundary.owned` as task-tree ownership.
Completed task boundaries are historical evidence, not current ownership for
new dirty work. `execution.scope` / `OwnedPaths` is only execution identity and
drift detection; do not use it to decide whether a diff belongs to a task.

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

Closure evidence is first-class but split by verifiability:

- `boundary`: JSON `{"includes":[...],"excludes":[...]}`. CST checks it against
  the accepted diff at completion.
- `rationale`: non-vacuous `invariant`, `failure`, `minimal_fix`,
  `remaining_risk`, optional `not_doing`. This is attestation, not proof.
- `contest`: targets a boundary/rationale evidence id and marks it contested for
  review.

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
