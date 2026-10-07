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

## Decision steps and conditional routing

Use `decision:` when the answer is one of a small, known set of values and
downstream steps need to route on that choice. Decision inference is
Jev-style: Induction asks the selected model for one next token with log
probabilities, then constructs the structured result itself. The model is not
asked to generate JSON or an explanation. Ordinary steps without `decision:`
remain normal generative steps.

Use a Jev decision model for decision steps. The example below names the
server preset `JEV5K-v0.3-4B-Q8_0`; it corresponds to the JevK5 v0.3 4B Q8_0
GGUF listed in [the model card](https://huggingface.co/alibiserikbay/JevK5-GGUF).
Configure the llama.cpp server to expose the selected preset under the model ID
in the pipeline. Generative analysis steps can use another general-purpose
model.

Candidate keys are the model-facing token strings; each must tokenize to
exactly one token for the selected model. Candidate values are semantic
labels, which may contain multiple tokens and are what conditions compare.
`topLogprobs` defaults to `20` and must be at least the number of candidates.
Every configured candidate must have a score in the returned next-token
log-probability data; a missing candidate fails the step rather than receiving
an invented score.

```yaml
- name: classify-issue
  model: JEV5K-v0.3-4B-Q8_0
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
for each item.

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
