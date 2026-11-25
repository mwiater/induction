# SIA: shuffle-invariance evaluator — beta / work in progress

> **Beta / work in progress:** SIA is experimental diagnostic tooling. Its metrics, question banks, reports, and dashboard may change without backward compatibility and should not be treated as production benchmark results.

SIA measures how consistently a model answers a multiple-choice question when the answer choices are repositioned. The evaluator reads one JSON object per line, validates the complete bank, freezes all rotations before contacting a model, and evaluates requested models sequentially against the same plan.

All commands below run from the repository root. The default question bank is `data/sia/shuffle_questions_320.jsonl`.

## Recommended command

Run the beta evaluation through the Cobra CLI:

```bash
induction run sia
```

The command reads the server URL from `induction.yaml`, evaluates all models by default with 100 top probabilities, reads `data/sia/shuffle_questions_320.jsonl`, and writes its only result file to `data/dashboard/sia.json`. It resumes from a compatible existing report and replaces that report atomically after each update. Complete model runs are skipped; within incomplete models, successful records are skipped and only missing or failed records are retried. Unsupported models receive one retry request and remain unsupported if the server still returns HTTP 501.

## Question banks

- `questions.jsonl`: five-question smoke test for local validation.
- `shuffle_questions_320.jsonl`: full 320-item bank; 80 easy, 160 medium, and 80 hard across 32 question families.
- `shuffle_questions_pilot_40.jsonl`: stratified 40-item pilot; 8 easy, 24 medium, and 8 hard.
- `shuffle_questions_smoke_12.jsonl`: 12-item smoke bank; four questions per difficulty with balanced original answer slots.

Each line is one JSON object with `id`, `question`, four distinct `options`, a zero-based `correct` index, and a `difficulty` of `easy`, `medium`, or `hard`. The evaluator creates the rotations; do not add rotated questions to the bank.

The 320-item bank has SHA-256 `bdecede878b602f36a4efd1c03c7af9a0ea4bd8a642f5a261bca3f7584bb1924`. The banks are committed fixtures and should be treated as frozen evaluation inputs. They are synthetic, procedurally generated, and not a calibrated public benchmark; avoid making precise model-ranking claims from them alone.

Example:

```bash
go run ./internal/sia/eval.go \
  --url http://192.168.0.239:9998 \
  --questions data/sia/shuffle_questions_320.jsonl \
  --model LFM-2.5-24B-A2B-UD-Q6_K_XL \
  --nprobs 100 \
  --out data/dashboard/sia.json
```

Use `--models 'MODEL_A,MODEL_B'` for sequential multi-model runs. `--model` and `--models` are mutually exclusive. `--control all` evaluates baseline, balanced few-shot, and no-few-shot prompts separately; `--repeat N` repeats the same frozen plan. `--verbose 0|1|2` selects errors, concise diagnostics, or bounded HTTP bodies.

Use `--models-all` to load IDs from `data/.local/v1_models.json` (override the catalog with `--models-file`). It cannot be combined with `--model` or `--models`:

```bash
go run ./internal/sia/eval.go \
  --url http://192.168.0.239:9998 \
  --questions data/sia/shuffle_questions_320.jsonl \
  --models-all \
  --nprobs 100 \
  --out data/dashboard/sia.json
```

At four cyclic rotations per item, a full run makes 1,280 completion calls per model. The pilot makes 160 calls per model and the 12-item smoke bank makes 48. Validate server model selection and failure handling before running the full bank.

Reports use schema version 4 and are written atomically to `data/dashboard/sia.json` by default. The compact report keeps the question manifest, mappings, letter masses, capture mass, timing subset, errors, and derived model metrics; prompts, generated tokens, full timing maps, and top-token arrays are omitted. Model IDs are keys under `models`; incomplete or unverified models are retained in the report and cause a nonzero exit. A server must list the requested model from `GET /models` unless `--allow-unverified-model` is supplied.

Rerunning the same command is safe: the report is rebuilt from a fresh evaluation plan and replaced atomically, so results are never appended or duplicated. A rerun is a new model evaluation and may have different metrics if the server or model is nondeterministic; idempotence here applies to filesystem state, not model responses.

Headline `sia` is conditional on the captured candidate letters and is computed only when every selected item has four valid cyclic rotations. Each item also receives four `distractor-reversed` rotations: the correct answer remains in the same A–D slot while the three incorrect alternatives reverse order. `mean_distractor_order_sensitivity` compares paired correct-letter probabilities across those two sets, and `distractor_top_pick_change_rate` compares paired content picks. `unconditional_correct_letter_probability` uses the raw probability mass of the correct displayed letter. `answer_format_adherence` measures whether the generated token is exactly one uppercase A–D token with optional leading whitespace. The report also includes top-pick accuracy, strict content consistency, always-correct rate, order sensitivity, observed off-letter leakage, unseen top-N tail mass, and per-model output fingerprints. Identical fingerprints are grouped under `duplicate_output_groups` as a diagnostic for aliased or behaviorally identical configurations.

If the server returns HTTP 501 for `/completion`, that model is marked `unsupported` and its remaining requests are skipped. The evaluator also supports `--control all` for baseline, balanced few-shot, and no-few-shot runs, plus `--repeat N` for repeated calls against the same frozen plan.

Add `--raw` to write a separate gzip archive containing the full request/response diagnostics. Use `--raw-out` to choose its path. To recompute metrics without contacting the server, use `--rescore input.json --out output.json`; schema 3 reports are migrated when the question bank hash matches.

The five bundled questions are a smoke test, not a reliable model ranking. Relative paths are resolved from the process working directory. Raw prompts and top-token data remain available through `--raw`; debug console bodies are bounded.

For conservative uncertainty estimates, bootstrap by question or preferably by question family because the bank contains related templates. Cyclic rotations balance the correct answer across A–D, but they do not cover all 24 distractor permutations; the distractor-reversed set provides a targeted order-sensitivity check rather than exhaustive permutation testing.
