<!-- induction-optimizer-v1 -->
# Pipeline alternative analysis

Input: `/home/matt/ai/bin/induction/pipelines/pipeline.batch-document-analysis-01.yaml`

Input SHA-256: `050207c88a8594388f4d571869d75d4c7d07a552d9785ec2bd4d4b9c36287680`

Alternative: `/home/matt/ai/bin/induction/pipelines/pipeline.batch-document-analysis-01-alt-1.yaml`

Alternative SHA-256: `12d1fee53afa96368c12a602873154c597fe699ef007fe9e8f722ff4be3ae8ad`

Source revision: `5cf133cd99084aed0f0fc546e2f64a0de983cf56 (dirty)`

Focus: `reliability`

Family root: `pipeline.batch-document-analysis-01`. The input above is the immediate predecessor of this alternative.

## Source inspection

Inspected: go.mod, pipeline.go, pipeline_references.go, pipeline_retry.go, pipeline_test.go, pipeline_references_test.go, session_persistence.go, console_ui.go, README.md, docs/CLI-REFERENCE.md. The checked-out parser and validator are the static authority.

Supported keys: Pipeline fields: batch, config, inputs, name, prompt, steps
Step fields: as, classification, contract, decision, document, forEach, image, input, jsonSchema, model, name, nomcp, output, parameters, quality, responseFormat, retry, systemPrompt, temperature, transform, userPrompt, when

## Historical completed sessions (8)

- `113c7878-051b-4d4b-acd1-14d66de95fdf` (`/home/matt/ai/bin/induction/.sessions/113c7878-051b-4d4b-acd1-14d66de95fdf.json`): exact-hash; failed; 2 snapshots; steps extract, summarize; models Qwen-3.5-9B-MTP-General-Q8_0; snapshot_count=2.
- `392d8eb4-c835-4457-a1cf-2d427f5d1656` (`/home/matt/ai/bin/induction/.sessions/392d8eb4-c835-4457-a1cf-2d427f5d1656.json`): same-name-unverified; failed; 2 snapshots; steps extract, summarize; models Qwen-3.5-9B-MTP-General-Q8_0; snapshot_count=2.
- `d720ab8d-0c68-4def-b1f5-b8112b25ec26` (`/home/matt/ai/bin/induction/.sessions/d720ab8d-0c68-4def-b1f5-b8112b25ec26.json`): exact-hash; failed; 2 snapshots; steps extract, summarize; models Qwen-3.5-9B-MTP-General-Q8_0; snapshot_count=2.
- `c1ce4531-d428-4677-80b9-79c52d0cf209` (`/home/matt/ai/bin/induction/.sessions/c1ce4531-d428-4677-80b9-79c52d0cf209.json`): exact-hash; failed; 2 snapshots; steps extract, summarize; models Qwen-3.5-9B-MTP-General-Q8_0; snapshot_count=2.
- `563f7186-8374-43b3-948e-2d4ac66095c6` (`/home/matt/ai/bin/induction/.sessions/563f7186-8374-43b3-948e-2d4ac66095c6.json`): exact-hash; failed; 2 snapshots; steps extract, summarize; models Qwen-3.5-9B-MTP-General-Q8_0; snapshot_count=2.
- `ffe1dfc2-8a34-435c-8597-843d9b74a994` (`/home/matt/ai/bin/induction/.sessions/ffe1dfc2-8a34-435c-8597-843d9b74a994.json`): exact-hash; failed; 2 snapshots; steps extract, summarize; models Qwen-3.5-9B-MTP-General-Q8_0; snapshot_count=2.
- `3e7f6d1e-33a5-4a4d-b7e3-a6748520625c` (`/home/matt/ai/bin/induction/.sessions/3e7f6d1e-33a5-4a4d-b7e3-a6748520625c.json`): exact-hash; failed; 2 snapshots; steps extract, summarize; models Qwen-3.5-9B-MTP-General-Q8_0; snapshot_count=2.
- `708c5f7b-3a4e-4322-9869-25cc1e455aab` (`/home/matt/ai/bin/induction/.sessions/708c5f7b-3a4e-4322-9869-25cc1e455aab.json`): same-name-unverified; failed; 2 snapshots; steps extract, summarize; models Qwen-3.5-9B-MTP-General-Q8_0; snapshot_count=2.

## Findings

- **F1** (Reliability_Failure_Consistency; confidence High): Nine verified sessions consistently fail at the 'return-json' step with a 400 inference error indicating sampler initialization failure. Evidence: S:113c7878-051b-4d4b-acd1-14d66de95fdf, S:392d8eb4-c835-4457-a1cf-2d427f5d1656. Hypothesis: The model backend (Qwen-3.5-9B-MTP-General-Q8_0) or MCP tool infrastructure rejects the specific grammar schema required by the responseFormat configuration.. Remedy: Inspect the 'responseFormat' and prompt content against model constraints; simplify JSON schema or switch to a compatible model..
- **F2** (Reliability_Fault_Tolerance; confidence Medium): The pipeline YAML defines steps without 'retry' configuration blocks, while the source code supports retry logic. Evidence: P1, C:pipeline_retry.go:79. Hypothesis: Missing retry configuration prevents any attempt to recover from transient inference or parsing errors during step execution.. Remedy: Add 'retry' configuration with 'maxAttempts' and validation rules to allow recovery for parseable errors..
- **F3** (Reliability_Model_Configuration; confidence Medium): Session logs report 'Failed to initialize samplers: failed to parse grammar' during the inference step. Evidence: P1, S:563f7186-8374-43b3-948e-2d4ac66095c6. Hypothesis: The combination of 'json_object' response format and MCP tool invocation triggers strict grammar validation incompatible with the current model version.. Remedy: Verify if 'responseFormat' can be relaxed or if the step requires a raw response text transformation prior to MCP tooling..

## Actual change manifest

- Modified step `return-json`: validation/retry.

## Expected effects

- Added nomcp: true to all steps (extract, summarize, return-json) to ensure consistent CLI routing and satisfy retry logic requirements. — hypothesis, not measured improvement.
- Added retry block with maxAttempts: 2 and validate.json: true to the return-json step to enable fault tolerance against transient inference failures. — hypothesis, not measured improvement.

## Static validation

- Strict YAML decoding, existing pipeline validation, references, and original objective fields passed.
- Candidate was not executed or benchmarked. Semantic equivalence requires manual review.

- Warning: skipped session 697c3051-ab10-4e23-8b26-9ba1faad32f5.json: session exceeds 8 MiB read limit
- Warning: skipped session 741f4e33-d0b9-49d4-912a-37ae6d24d0d0.json: session exceeds 8 MiB read limit
- Warning: skipped session 88d4829b-c561-47ad-950d-53eb45abc695.json: session exceeds 8 MiB read limit
- Warning: Source checkout is dirty; capabilities reflect inspected files rather than a clean Git revision.
- Warning: Source excerpts were bounded and truncated; the full checked-out parser and validator still gate candidate YAML.
- Warning: Candidate model IDs are restricted to those in the input pipeline; runtime availability of every candidate model was not independently verified.

## Manual test

Run the alternative with `induction --pipeline <alternative path>`. Compare final output quality, failures, step snapshots, token counts, and elapsed time against completed sessions for the same inputs. Treat any quality regression or new failure as a failed experiment. Then optimize the alternative using its own completed sessions.

**Not executed or performance-tested; run manually to evaluate.**
