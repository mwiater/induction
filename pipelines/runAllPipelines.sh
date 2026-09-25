#!/usr/bin/env bash

set -u

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

shopt -s nullglob
pipelines=(pipelines/pipeline*.yaml)

if ((${#pipelines[@]} == 0)); then
  echo "No pipeline YAML files found in pipelines/." >&2
  exit 1
fi

failed=0
for pipeline in "${pipelines[@]}"; do
  echo
  echo "=== Running $pipeline ==="
  if ! go run ./cmd/induction --pipeline "$pipeline"; then
    echo "FAILED: $pipeline" >&2
    failed=1
  fi
done

if ((failed)); then
  echo
  echo "One or more pipelines failed." >&2
  exit 1
fi

echo
echo "All pipelines completed successfully."
