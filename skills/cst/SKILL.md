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
cst add --parent 2 --intent "Review" --review self
```

Use `--after <id>` for internal sequencing. Reserve `hold` for external pauses.

Correct tree shape with `revise`; do not create duplicate replacement tasks when
identity should be preserved.

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

Do not infer workstream scope from `attempt_id`; use explicit `--parent` and
`--within`.

## Install

From the CST repo:

```sh
cd /Users/yansir/code/52/cross-session-todolist
make install
make skill
```
