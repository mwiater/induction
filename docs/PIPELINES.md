# Pipelines

Pipeline YAML files define ordered inference and deterministic transform steps.
Run one with:

```bash
induction --pipeline pipelines/pipeline.prompt-optimization-01.yaml
```

Reusable fields include `output`, `transform`, `input`, `forEach`, and `as`.
References support `{{ steps.name.output }}`, nested output fields,
`{{ steps.name.items }}`, `{{ inputs.documents.chunks }}`, and the active
fan-out item. Fan-out runs sequentially and preserves input order. Local
attachment paths are resolved relative to the pipeline file; image and PDF
sources may also be HTTP or HTTPS URLs. Remote sources are downloaded with
bounded size and timeout checks, then passed through the same validation and
processing path as local files. Pipeline artifacts are stored under
`.pipeline-artifacts/<run-id>/`.

## Authoring

Pipeline files live under [`pipelines/`](../pipelines/). A minimal pipeline has a
name and one or more model steps:

```yaml
name: summarize
steps:
  - name: summary
    model: MODEL
    userPrompt: Summarize the supplied documents.
```

Structured output uses `output.type: json` and requires an artifact path. For a
multi-step analysis whose intermediate summary is text, add a final formatter
step with `responseFormat.type: json_object` and `output.type: json`; the final
step can serialize the accumulated conversation into the artifact. Model steps
may use `responseFormat`, `jsonSchema`, or a grammar. Transform steps use
registered deterministic operations and receive values through `input`.

## Running the repository pipelines with your own models

The repository examples contain model IDs that reflect the models used while
the examples were developed. Those IDs do not need to be available on an
end-user's inference server. The
[`scripts/autorun-pipelines.sh`](../scripts/autorun-pipelines.sh) convenience
script runs the existing root-level pipeline YAML files while overriding the
models used by ordinary generative steps. This lets users exercise the example
workflows with their own loaded models without editing every pipeline file.

The same script also provides a repeatable way to seed the dashboard with
observations from a selected group of models or from the entire live model
catalog.

### Prerequisites

Before running the script, make sure:

- the repository has a usable `induction.yaml` with a top-level `server` URL;
- the configured inference server is reachable and exposes `/v1/models`;
- the models you want to use are already loaded by that server;
- Bash 4 or newer, Go, `curl`, `jq`, and `rg` are installed.

The script resolves the repository root from its own location, so it can be
started from the repository root or from another working directory:

```bash
./scripts/autorun-pipelines.sh --help
```

### Run with selected user models

By default, the script uses the `models` array near the top of the script. Edit
that array to list the model IDs available on your server:

```bash
models=(
  "my-general-model"
  "my-reasoning-model"
  "my-vision-model"
)
```

Then run:

```bash
./scripts/autorun-pipelines.sh
```

The script verifies every configured ID against `/v1/models` before starting
any pipeline. Each selected model is passed as `--pipeline-model`, so it
replaces the repository model IDs for ordinary generative steps at runtime.
The pipeline YAML files are not modified.

This mode is useful when you want to seed the dashboard with a known,
representative distribution of models. For example, a general model, a
reasoning model, and a vision model can be listed together to compare their
observed pipeline behavior.

### Run with every loaded model

Pass `--all-models` to ignore the script's `models` array and discover the
models from the live server catalog:

```bash
./scripts/autorun-pipelines.sh --all-models
```

Every model with a string `id` in `/v1/models` is considered. This mode is
useful for a broad compatibility pass or for populating the dashboard from the
current server fleet. It can launch many pipeline runs, so it may take
substantial time and generate a large amount of session and telemetry data.

### Vision-model handling

Vision compatibility is applied identically in both modes:

- a model advertising `image` in its input modalities runs every root-level
  pipeline;
- a model without image input support skips pipelines containing an `image:` or
  `images:` input;
- skipped image pipelines are reported and are not counted as failures.

This prevents a text-only model from being sent an image request while still
allowing a vision model to exercise text, document, and image workflows.

### Decision and classification steps

Ordinary generative steps use the selected generation model. Steps using
`decision:` or the legacy `classification:` field use each decision-capable
model discovered from `/v1/models`, identified by
`output_modalities: ["decisions"]`. This automatically rotates the available
decision fleet through decision pipelines without requiring a second model
list in the script.

For a pipeline containing decision steps, the script runs the relevant
generation-model × decision-model combinations. Pipelines without decision
steps run once per generation model. Decision models must satisfy the
pipeline's decision candidate constraints, and image-based decision pipelines
also require the selected decision model to advertise image input support.

### Example output

A normal run reports each pipeline/model combination, including the generation
and decision model for decision pipelines, and explicitly identifies
vision-incompatible skips:

```text
=== Running pipelines/pipeline.document-01.yaml with my-general-model ===
... pipeline output ...

=== Skipping pipelines/pipeline.image-01.yaml for non-vision model my-general-model ===

=== Running pipelines/pipeline.decision-routing-01.yaml with generation=my-general-model decision=Kev-4B-Q8_0 ===
... pipeline output ...

=== Running pipelines/pipeline.image-01.yaml with generation=my-vision-model decision=Kev-4B-Q8_0 ===
... pipeline output ...

Autorun summary: attempted=68 skipped=8 generation_models=3 decision_models=2 pipelines=20 decision_pipelines=12
All applicable pipelines completed successfully.
```

If an individual run fails, the script continues with the remaining
combinations and returns a failing exit status at the end:

```text
=== Running pipelines/pipeline.decision-routing-01.yaml with generation=my-general-model decision=Kev-9B-Q8_0 ===
FAILED: pipelines/pipeline.decision-routing-01.yaml [generation=my-general-model decision=Kev-9B-Q8_0]

Autorun summary: attempted=68 skipped=8 generation_models=3 decision_models=2 pipelines=20 decision_pipelines=12
One or more pipelines failed.
```

The failed pipeline session remains available under `.sessions/` for review.
This makes the script suitable for both interactive exploration and automation
that relies on its exit status.

### What the script discovers

The script runs YAML files directly under `pipelines/` in sorted order. It does
not recursively execute files under `pipelines/generated/`. Run a generated
pipeline separately when you want to test it:

```bash
induction --pipeline pipelines/generated/your-pipeline.yaml \
  --pipeline-model my-general-model
```

The autorun script does not change pipeline definitions, model catalogs, or
server configuration. Its persistent effects are the normal sessions,
artifacts, logs, and dashboard telemetry produced by the pipeline runs.

### MCP tool access

Pipeline steps do not discover or call configured MCP server tools by default.
Add `usemcp: true` to a step when that step needs MCP access:

```yaml
steps:
  - name: fetch-live-data
    model: MODEL
    usemcp: true
    userPrompt: Use the configured MCP tools to fetch the requested data.
```

The setting is per step, so a pipeline may mix MCP-enabled and ordinary
steps. The legacy `nomcp` pipeline key is no longer accepted; remove it from
non-MCP steps and replace it with `usemcp: true` only where MCP is required.

### Generation parameters

A model step can optionally override generation parameters with `parameters`:

```yaml
steps:
  - name: answer
    model: MODEL
    userPrompt: Answer the question.
    parameters:
      temperature: 1.2
      topP: 0.98
      topK: 40
      maxTokens: 2048
      repeatPenalty: 1.05
      seed: 42
      minP: 0.02
      xtcProbability: 0.35
      xtcThreshold: 0.10
      typicalP: 1.0
      presencePenalty: 0.0
      frequencyPenalty: 0.0
      repeatLastN: 64
```

The supported parameter names and meanings are:

| YAML field | Definition |
| --- | --- |
| `temperature` | Sampling randomness. |
| `topP` | Nucleus sampling probability mass. |
| `topK` | Limits sampling to the highest-scoring K tokens. |
| `maxTokens` | Maximum number of generated tokens. |
| `repeatPenalty` | Penalty applied to repeated tokens. |
| `seed` | Random seed for reproducible sampling where supported. |
| `minP` | Minimum token probability relative to the most likely token. |
| `xtcProbability` | Probability of applying XTC token exclusion. |
| `xtcThreshold` | XTC probability threshold used when excluding tokens. |
| `typicalP` | Typical-sampling probability mass. |
| `presencePenalty` | Penalizes a token if it has appeared previously. |
| `frequencyPenalty` | Penalizes a token according to how often it has appeared. |
| `repeatLastN` | Number of recent tokens considered by repeat-penalty sampling. |

Every field is optional. If a field is absent, it is omitted from the request
and the model/server default is used. Explicit zero values are retained and
sent; they are not treated as absent. The server/model ultimately determines
which parameter values are valid.

`temperature` is also supported as a legacy top-level step field. Prefer the
`parameters` mapping when configuring multiple generation parameters.

### Pipeline failures and sessions

When a pipeline step fails—for example, because it returns empty output,
invalid structured output, fails validation, or cannot load its model—Induction
records the terminal error in the pipeline session's `error` field, persists
the session under `.sessions/<session-id>.json`, and ends that console run.
The process returns a failure status so wrappers can report the failed pipeline.
The [`scripts/autorun-pipelines.sh`](../scripts/autorun-pipelines.sh) convenience
script continues with the next pipeline after a failure while retaining the
failed session for review. By default it runs root-level pipelines against the
model IDs in its `models` array, allowing users to override repository model
IDs without editing pipeline YAML and giving maintainers a repeatable way to
seed dashboard content. Pass `--all-models` to run against every model in the
live `/v1/models` catalog.

The script applies `--pipeline-model` to ordinary generative steps and
`--pipeline-decision-model` to decision and classification steps. Vision models
run every compatible root-level pipeline. Non-vision models skip pipelines with
`image` or `images` inputs, so text-only models are never used for image tasks.

## Decision steps and conditional routing

Use `decision:` when the answer is one of a small, known set of values and
downstream steps need to route on that choice. Decision inference uses a
model that advertises `output_modalities: ["decisions"]` in `/v1/models`.
The model is not asked to generate JSON or an explanation. Ordinary steps without `decision:`
remain normal generative steps.

The examples use `Kev-4B-Q8_0` for bounded decisions and `Kev-9B-Q8_0` for
larger decision review steps; replace these with any current
decision-capable model from `/v1/models`. Generative analysis steps can use
another general-purpose model.

Candidate keys are the model-facing token strings; each must tokenize to
exactly one token for the selected model. Candidate values are semantic
labels, which may contain multiple tokens and are what conditions compare.
`topLogprobs` defaults to `20` and must be at least the number of candidates.
Every configured candidate must have a score in the returned next-token
log-probability data; a missing candidate fails the step rather than receiving
an invented score.

```yaml
- name: classify-issue
  model: Kev-4B-Q8_0
  userPrompt: |
    Classify the primary concern. Return only A, B, C, or D.
    A = security
    B = performance
    C = correctness
    D = other
  decision:
    candidates:
      A: security
      B: performance
      C: correctness
      D: other
    topLogprobs: 20

- name: security-review
  model: Qwen-3.5-9B-MTP-General-Q8_0
  when:
    decision: classify-issue
    equals: security
    minConfidence: 0.5
    minMargin: 0.2
  systemPrompt: You are a careful software security reviewer.
  userPrompt: Review the supplied report for security risks and mitigations.
```

`when.decision` must name an earlier step configured with `decision:` or
legacy `classification:`. `when.equals` must exactly match one of that step's
semantic values. Optional `minConfidence` and `minMargin` must be in `[0,1]`;
they pass when the decision's confidence and margin are greater than or equal
to the thresholds. All checks are combined with AND. Conditions cannot refer
to generative steps, future steps, or candidate token keys unless a token key
is also a semantic value.

A condition that fails records a skipped step in the pipeline session and
continues sequentially without loading the step's model or sending an
inference request. This supports escalation by mapping a candidate to a value
such as `uncertain` and gating an ordinary generative review on
`equals: uncertain`. There is no less-than confidence operator or general
expression language; use an explicit uncertainty candidate when the pipeline
needs a review path.

For the `DecisionResult` structure and programmatic decision API, see
[`docs/INFERENCE.md`](INFERENCE.md#bounded-decisions). `classification:`
remains supported as a legacy alias for `decision:`; both keys on one step
are invalid. Decision steps cannot use `responseFormat` or `jsonSchema`, and
`parameters.maxTokens`, if supplied, must equal `1`.

Text, image, and document-derived context use the same decision mode. The
server must support OpenAI-compatible chat-completion log probabilities;
Induction requests one output token and the configured top alternatives. No
additional llama.cpp logits startup flag is required for this supported path.
Image decisions require a vision-capable model and its required multimodal
projector. See [`pipeline.decision-routing-01.yaml`](../pipelines/pipeline.decision-routing-01.yaml)
for a complete sequential routing example.

Generated pipelines can be created and validated with:

```bash
induction pipeline generate --model MODEL --prompt "Compare two rollout plans." \
  --output pipelines/generated/rollout.yaml --validate
```

## Batch pipelines

`inputs:` creates one pipeline run containing all listed files. `batch.items:`
creates one independent child run per item and executes the complete pipeline
for each item. Batch items may provide `files`, `images`, `documents`, or a
text value exposed to pipeline references as `{{inputs.text}}`.

```yaml
name: image-review
batch:
  items:
    - id: image-001
      images: [./images/one.jpg]
    - id: image-002
      images: [./images/two.jpg]
steps:
  - name: review
    model: vision-model
  userPrompt: Review the provided image.
```

For text batches, bind the item-specific value in the first step:

```yaml
batch:
  items:
    - id: question-001
      text: "First question"
steps:
  - name: answer
    model: MODEL
    userPrompt: "{{inputs.text}}"
```

Batch state is persisted under `.batches/`, with child sessions under
`.sessions/`. Completed items are skipped when an incomplete batch is resumed.
Invalid and failed items are recorded while valid items continue by default. If
the batch finishes with errors, rerunning the same pipeline starts a fresh
attempt for every item, overwrites the persisted batch record, and regenerates
all pipeline artifacts. A fully completed batch remains resumable without
rerunning its completed items.

## Knowledge graph pipelines

Induction can build an evidence-backed knowledge graph from one or more
documents through the normal Bubble Tea pipeline UI:

```bash
induction --pipeline pipelines/pipeline.emergent-knowledge-graph-01.yaml
```

The corpus profile supplies ontology hints; it is not a closed schema. Model
steps extract observations and make semantic identity decisions. Deterministic
Go transforms generate IDs, verify evidence, resolve conservative candidates,
aggregate edges, calculate support-density weights, and serialize the final
artifact. Source observations are retained when canonical entities or edges
are created.

Relations are accepted only when both endpoints are grounded in the source
chunk, either in the relation's verbatim evidence or in another exact source
substring. The extractor is instructed to return those endpoints as entities
in the same chunk; if an endpoint is present in the chunk but was omitted from
the entity list, the pipeline recovers it as an `Unclassified` entity.
Relations with unsupported endpoints are discarded rather than creating
untraceable graph edges. During graph construction, canonical names and aliases
are matched against the chunk-local entity observations.

The semantics step must return one object with `facets`,
`structural_hierarchy`, and `traversal_filters` arrays. Facets and structural
roles are checked against the graph's node IDs, and roles are limited to
`hub`, `intermediate`, or `leaf`. Invalid entries are filtered while valid
partial output is retained. A model response containing only a facet array is
also retained as partial semantics, with the missing arrays reported. The final
artifact metadata records `semantics_status` (`valid`, `partial`, or `invalid`)
and any `semantics_issues`, so a completed run makes schema problems visible.

The reusable step fields and reference forms for this pipeline are defined in
the [authoring section](#authoring). They apply to the knowledge-graph steps as
well, including sequential fan-out over document chunks and candidates.

Document and chunk IDs are derived from source bytes and exact chunk content.
Evidence must occur verbatim in its source chunk, apart from CRLF/LF
normalization. Edge weight is a deterministic support-density measure derived
from the number of supporting observations; it is not a probability that the
relationship is true.

The initial registered transforms are:

- `knowledgeGraph.entityCandidates`
- `knowledgeGraph.canonicalizeEntities`
- `knowledgeGraph.predicateCandidates`
- `knowledgeGraph.buildGraph`
- `knowledgeGraph.finalize`

JSON outputs are persisted under `.pipeline-artifacts/<run-id>/` using atomic
writes and SHA-256 content hashes. Existing pipeline fields and examples remain
supported. The knowledge-graph result includes `nodes`, `edges`, retained
observations, unresolved relations, and the validated semantics arrays.
