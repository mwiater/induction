#!/usr/bin/env bash

# Run root-level pipelines against configured model overrides, or every model
# currently reported by the inference server. Decision-capable models are
# automatically rotated through decision/classification steps.
#
# Default mode is intended for dashboard seeding and repeatable local runs:
# edit the models array below and each model is applied to every root-level
# pipeline. Use --all-models to discover the model list from /v1/models.
#
# Vision models run every compatible pipeline. Non-vision generation models
# skip pipelines with image/images inputs. Decision models are checked for
# vision support separately when a decision pipeline has image inputs.

set -u -o pipefail

models=(
  "Inkling-2-Mini-26B-A3B-Q4_K_XL"
)

usage() {
  cat <<'EOF'
Usage: scripts/autorun-pipelines.sh [--all-models]

Without --all-models, run root-level pipelines using the model IDs in the
models array. With --all-models, discover ordinary and decision models from
/v1/models and rotate them through the relevant pipelines.
EOF
}

all_models=0
for argument in "$@"; do
  case "$argument" in
    --all-models)
      all_models=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $argument" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if (( ! all_models )) && ((${#models[@]} == 0)); then
  echo "Configure at least one model in the models array or use --all-models" >&2
  exit 1
fi

for command in curl jq rg go; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "$command is required to run this script" >&2
    exit 1
  fi
done

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

config_file="induction.yaml"
if [[ ! -f "$config_file" ]]; then
  echo "Configuration file not found: $config_file" >&2
  exit 1
fi

server="$(awk '$0 ~ /^server:[[:space:]]*/ { print $2; exit }' "$config_file")"
server="${server#\"}"
server="${server%\"}"
server="${server#\'}"
server="${server%\'}"
if [[ -z "$server" ]]; then
  echo "No top-level server value found in $config_file" >&2
  exit 1
fi

models_endpoint="${server%/}/v1/models"
if ! models_json="$(curl --fail --silent --show-error --location \
    --connect-timeout 10 --max-time 60 "$models_endpoint")"; then
  echo "Failed to query model catalog: $models_endpoint" >&2
  exit 1
fi
if ! jq -e '(.data // .) | type == "array"' >/dev/null <<<"$models_json"; then
  echo "Invalid model catalog response from $models_endpoint" >&2
  exit 1
fi

catalog_model_ids() {
  jq -r '(.data // .)[] | select(.id? | type == "string") | .id' <<<"$models_json"
}

is_decision_model() {
  local model="$1"
  jq -e --arg model "$model" '
    any((.data // .)[];
      .id == $model and
      (((.architecture.output_modalities // .output_modalities // .outputModalities // [])
        | map(ascii_downcase)
        | index("decisions")) != null)
    )
  ' <<<"$models_json" >/dev/null
}

if (( all_models )); then
  mapfile -t models < <(catalog_model_ids | sort -u)
else
  for model in "${models[@]}"; do
    if ! jq -e --arg model "$model" \
        'any((.data // .)[]; .id == $model)' <<<"$models_json" >/dev/null; then
      echo "Configured model not found at $models_endpoint: $model" >&2
      exit 1
    fi
  done
fi

if ((${#models[@]} == 0)); then
  echo "No models found at $models_endpoint" >&2
  exit 1
fi

generation_models=()
decision_models=()
for model in "${models[@]}"; do
  if is_decision_model "$model"; then
    decision_models+=("$model")
  else
    generation_models+=("$model")
  fi
done

if ((${#generation_models[@]} == 0)); then
  echo "No ordinary generation models found at $models_endpoint" >&2
  exit 1
fi

is_vision_model() {
  local model="$1"
  jq -e --arg model "$model" '
    any((.data // .)[];
      .id == $model and
      ((.architecture.input_modalities // .input_modalities // .inputModalities // [])
        | map(ascii_downcase)
        | index("image"))
    )
  ' <<<"$models_json" >/dev/null
}

mapfile -d '' pipelines < <(
  find pipelines -maxdepth 1 -type f -name '*.yaml' -print0 | sort -z
)
if ((${#pipelines[@]} == 0)); then
  echo "No root-level pipeline YAML files found in pipelines/." >&2
  exit 1
fi

failed=0
attempted=0
skipped=0
decision_pipelines=0

pipeline_has_decisions() {
  local pipeline="$1"
  rg -q '^[[:space:]]+(decision|classification):' "$pipeline"
}

run_pipeline() {
  local pipeline="$1"
  local generation_model="$2"
  local decision_model="${3:-}"
  local command=(go run ./cmd/induction --pipeline "$pipeline" --pipeline-model "$generation_model")
  if [[ -n "$decision_model" ]]; then
    command+=(--pipeline-decision-model "$decision_model")
  fi

  echo
  if [[ -n "$decision_model" ]]; then
    echo "=== Running $pipeline with generation=$generation_model decision=$decision_model ==="
  else
    echo "=== Running $pipeline with $generation_model ==="
  fi
  attempted=$((attempted + 1))
  if ! "${command[@]}"; then
    if [[ -n "$decision_model" ]]; then
      echo "FAILED: $pipeline [generation=$generation_model decision=$decision_model]" >&2
    else
      echo "FAILED: $pipeline [$generation_model]" >&2
    fi
    failed=1
  fi
}

for generation_model in "${generation_models[@]}"; do
  generation_vision=0
  if is_vision_model "$generation_model"; then
    generation_vision=1
  fi

  for pipeline in "${pipelines[@]}"; do
    vision_pipeline=0
    if rg -q '^[[:space:]]+(image|images):' "$pipeline"; then
      vision_pipeline=1
    fi

    if (( vision_pipeline && ! generation_vision )); then
      echo "=== Skipping $pipeline for non-vision model $generation_model ==="
      skipped=$((skipped + 1))
      continue
    fi

    if ! pipeline_has_decisions "$pipeline"; then
      run_pipeline "$pipeline" "$generation_model"
      continue
    fi

    decision_pipelines=$((decision_pipelines + 1))
    if ((${#decision_models[@]} == 0)); then
      echo "=== Skipping $pipeline: no decision-capable models found ===" >&2
      failed=1
      continue
    fi

    for decision_model in "${decision_models[@]}"; do
      decision_vision=0
      if is_vision_model "$decision_model"; then
        decision_vision=1
      fi
      if (( vision_pipeline && ! decision_vision )); then
        echo "=== Skipping $pipeline for non-vision decision model $decision_model ==="
        skipped=$((skipped + 1))
        continue
      fi
      run_pipeline "$pipeline" "$generation_model" "$decision_model"
    done
  done
done

echo
echo "Autorun summary: attempted=$attempted skipped=$skipped generation_models=${#generation_models[@]} decision_models=${#decision_models[@]} pipelines=${#pipelines[@]} decision_pipelines=$decision_pipelines"
if ((failed)); then
  echo "One or more pipelines failed." >&2
  exit 1
fi

echo "All applicable pipelines completed successfully."

