#!/usr/bin/env bash
set -euo pipefail

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/cst"
central="$tmp/central"
worker="$tmp/worker"
mkdir -p "$central" "$worker"

go build -o "$bin" ./cmd/cst

"$bin" --store "$central" add --intent "root" >/dev/null
mkdir -p "$central/nested/path"
(cd "$central/nested/path" && "$bin" show 1 >/dev/null)

"$bin" --store "$central" add --parent 1 --intent "worker task" \
  --exec-cwd "$worker" \
  --check side-effect="printf worker | tee side-effect.txt" \
  --check real="test -f side-effect.txt" >/dev/null
"$bin" --store "$central" take 2 >/dev/null

runset_json="$tmp/runset.json"
"$bin" --store "$central" run 2 --acceptance >"$runset_json"
runset_id="$(awk -F'"' '/"event_id":/ {print $4; exit}' "$runset_json")"
test -n "$runset_id"

"$bin" --store "$central" done 2 --from-acceptance "$runset_id" >/dev/null

test -f "$central/.cst/events.jsonl"
test ! -e "$worker/.cst/events.jsonl"
test "$(cat "$worker/side-effect.txt")" = "worker"

events="$central/.cst/events.jsonl"
grep -q '"evidence_kind":"acceptance_run_set"' "$events"
grep -q '"exec_cwd":"'"$worker"'"' "$events"
grep -q '"store_id":"178' "$events" || grep -q '"store_id":"' "$events"
grep -q '"stdout_artifact"' "$events"
