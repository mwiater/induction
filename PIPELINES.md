# Pipelines

Pipeline YAML files define ordered inference and deterministic transform steps.
Run one with:

```bash
induction --pipeline pipelines/pipeline.prompt-optimization-01.yaml
```

Reusable fields include `output`, `transform`, `input`, `forEach`, and `as`.
References support `{{ steps.name.output }}`, nested output fields,
`{{ steps.name.items }}`, `{{ inputs.documents.chunks }}`, and the active
fan-out item. Fan-out runs sequentially and preserves input order. Attachment
paths are resolved relative to the pipeline file. Pipeline artifacts are stored
under `.pipeline-artifacts/<run-id>/`.

## Authoring

Pipeline files live under [`pipelines/`](pipelines/). A minimal pipeline has a
name and one or more model steps:

```yaml
name: summarize
steps:
  - name: summary
    model: MODEL
    userPrompt: Summarize the supplied documents.
```

Structured output uses `output.type: json` and requires an artifact path. Model
steps may use `responseFormat`, `jsonSchema`, or a grammar. Transform steps use
registered deterministic operations and receive values through `input`.

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
`.sessions/`. Completed items are skipped when a batch is resumed; invalid
items are recorded while valid items continue by default.

## Knowledge graph pipelines

Induction can build an evidence-backed knowledge graph from one or more
documents through the normal Bubble Tea pipeline UI:

```bash
induction --pipeline pipelines/pipeline.emergent-knowledge-graph.yaml
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

Reusable step fields are `output`, `transform`, `input`, `forEach`, and `as`.
References intentionally support forms such as `{{ steps.name.output }}`,
`{{ steps.name.output.field }}`, `{{ steps.name.items }}`,
`{{ inputs.documents.chunks }}`, and the active fan-out item. Fan-out executes
sequentially in the existing Bubble Tea pipeline and preserves input order.

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
