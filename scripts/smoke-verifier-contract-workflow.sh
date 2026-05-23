#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

go build -o "$tmp/cst" "$repo_root/cmd/cst"
mkdir -p "$tmp/store"

(
  cd "$tmp/store"
  "$tmp/cst" add --intent "Smoke verifier contract workflow" >/dev/null
  "$tmp/cst" add --parent 1 --intent "Freeze verifier contract" --review self >/dev/null
  "$tmp/cst" take 2 >/dev/null

  contract_data=$(cat "$repo_root/testdata/verifier-contract/pass.json")
  evidence_line=$("$tmp/cst" evidence 2 --kind verifier_contract --summary "frozen smoke contract" --data "$contract_data" --human)
  evidence_id=$(printf '%s\n' "$evidence_line" | awk '{print $3}')
  "$tmp/cst" done 2 --evidence "$evidence_id" >/dev/null

  "$tmp/cst" add --parent 1 --intent "Implement against frozen contract" --after 2 \
    --check contract-lock="cd '$repo_root' && scripts/verify-contract-lock --fixture testdata/verifier-contract/pass.json" \
    --check red="cd '$repo_root' && scripts/verify-contract-lock --fixture testdata/verifier-contract/lazy-stub-fails.json" \
    --check real="true" >/dev/null
  "$tmp/cst" take 3 >/dev/null
  "$tmp/cst" done 3 >/dev/null
  "$tmp/cst" brief --human | grep 'root #1 \[completed\]' >/dev/null
)
