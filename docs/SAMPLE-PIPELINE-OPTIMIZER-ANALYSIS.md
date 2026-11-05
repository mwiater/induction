# Pipeline alternative analysis (sample)

Input: `pipelines/pipeline.example.yaml`

Input SHA-256: `<input digest>`

Alternative: `pipelines/pipeline.example-alt-1.yaml`

Alternative SHA-256: `<alternative digest>`

Source revision: `<Git revision>`

Family root: `pipeline.example`.

## Historical completed sessions

- `example-session-id`: exact-hash; completed; two snapshots; one prompt-token sample and one completion-token sample.

## Finding

- **F1** (prompt clarity; medium confidence): The final step requests two different output formats. Evidence: the original YAML final-step prompt and snapshot 2 of `example-session-id`. Hypothesis: one explicit format may reduce malformed outputs.

## Actual change manifest

- Modified step `final`: prompts.

## Expected effect

- Clarified the final-step output instruction — hypothesis, not measured improvement.

## Static validation

- The alternative passed strict YAML decoding and Induction's static pipeline validation.
- Semantic equivalence still requires manual review.

## Manual test

Run `induction --pipeline pipelines/pipeline.example-alt-1.yaml`. Compare completion, output quality, and token use against completed runs of the original using comparable inputs. If the alternative succeeds, optimize `pipeline.example-alt-1.yaml` using its own completed sessions.

**Not executed or performance-tested; run manually to evaluate.**
