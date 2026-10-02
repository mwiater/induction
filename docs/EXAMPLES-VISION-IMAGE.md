# Vision / Image Pipeline Examples

Examples:

- [Multi-aspect analysis](#multi-aspect-image-analysis-with-json-synthesis)
- [Batch analysis](#batch-image-analysis)
- [Combined comparison](#combined-multi-image-comparison)
- [Custom parameters](#custom-model-parameters-for-structured-image-analysis)

## Multi-aspect image analysis with JSON synthesis

### 1. Overview

Analyzes one image through specialist lenses before serializing the findings. It is useful for structured visual inspection, accessibility or catalog records, and evaluations that need separate subject, composition, and environmental evidence.

### 2. Steps

- **analyze-image-subjects:** Describes visually supported subjects, actions, poses, appearance, spatial relationships, and salient background elements without inventing identity or cause.
- **analyze-image-composition:** Examines framing, balance, focal points, perspective, depth, leading lines, negative space, color, contrast, lighting, horizon, and reflections when visible.
- **analyze-image-environmental:** Describes the visible setting and labels cautious interpretations about terrain, water, vegetation, weather, season, built environment, and activity.
- **return-json:** Combines prior analyses into the required JSON schema, retaining uncertainty and emitting only JSON.

### 3. Expected final output

Exactly one JSON object with required string properties subjects, composition, and environmental_context, no extra properties, and a clear distinction between observation and inference.

### 4. Example YAML

[Open pipelines/pipeline.image-01.yaml in the repository](../pipelines/pipeline.image-01.yaml)

### 5. YAML source

```yaml
name: image-analysis

config: induction.yaml

steps:
  - name: analyze-image-subjects
    model: Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL
    nomcp: true
    systemPrompt: |
      You are a careful visual analyst. Identify the primary subjects and
      describe only what is visually supported by the image. Distinguish
      clearly between people, animals, objects, and important background
      elements. Describe observable actions, poses, appearance, spatial
      relationships, and salient details. Do not invent identities, locations,
      clothing details, or causes that cannot be established from the image.
    userPrompt: |
      Analyze the provided image and describe its primary subjects in detail.
      Organize the analysis by subject and include the visual evidence for each
      description. State uncertainty when a detail is ambiguous.
    image: ../data/fixtures/images/fixture-01.jpg

  - name: analyze-image-composition
    model: Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL
    nomcp: true
    systemPrompt: |
      You are an expert photography and visual-composition analyst. Focus on
      formal visual structure rather than guessing the photographer's intent.
      Base the analysis on observable evidence and use precise spatial language.
      Consider framing, visual weight, balance, focal points, hierarchy,
      perspective, depth, leading lines, negative space, color, contrast,
      lighting, horizon placement, and reflections when they are present.
    userPrompt: |
      Analyze the provided image's composition in detail. Explain how the major
      visual elements are arranged and how that arrangement guides attention.
      Mention only compositional features that are actually visible.

  - name: analyze-image-environmental
    model: Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL
    nomcp: true
    systemPrompt: |
      You are a cautious environmental-context analyst. Separate direct visual
      observations from reasonable interpretations. Discuss terrain, water,
      vegetation, weather, atmosphere, season, built environment, lighting,
      and apparent human activity when relevant. Never identify a specific
      place, species, material, altitude, time, or weather condition as fact
      unless the image provides strong evidence; label inferences explicitly.
    userPrompt: |
      Analyze the provided image's environmental context in detail. Describe
      the visible setting first, then explain cautious interpretations about
      geography, climate, weather, or human activity. Clearly label anything
      that cannot be determined with confidence.

  - name: return-json
    model: Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL
    nomcp: true
    systemPrompt: |
      You are a precise JSON serialization assistant. Use the accumulated
      analyses in the conversation as source material and preserve their useful
      details without adding unsupported claims. Return exactly one JSON object
      that conforms to the supplied schema. Use the exact required property
      names, include every required property, use string values, and do not add
      properties. Return JSON only: no Markdown fences, explanations, or
      trailing commentary.
    userPrompt: |
      Synthesize all accumulated image analyses into one concise but
      information-rich JSON object matching the supplied schema. Preserve
      uncertainty where the earlier analyses identified it; do not turn guesses
      into facts.
    responseFormat:
      type: json_object
    jsonSchema:
      type: object
      properties:
        subjects:
          type: string
          description: Detailed description of the primary subjects.
        composition:
          type: string
          description: Detailed description of the image composition.
        environmental_context:
          type: string
          description: Detailed description of the environmental context.
      required:
        - subjects
        - composition
        - environmental_context
      additionalProperties: false
```

## Batch image analysis

### 1. Overview

Processes two images as separate batch items. Each item runs an independent description-and-summary sequence, which is useful for per-image workers, evaluation sets, and workflows requiring isolated results.

### 2. Steps

- **describe:** Describes the current image and identifies its primary subjects.
- **summarize:** Returns a concise structured analysis for that image.

### 3. Expected final output

Two independent results, one for image-001 and one for image-002, with no cross-image observations or combined summary.

### 4. Example YAML

[Open pipelines/pipeline.batch-image-analysis-01.yaml in the repository](../pipelines/pipeline.batch-image-analysis-01.yaml)

### 5. YAML source

```yaml
name: batch-image-analysis
# `batch.items` is the batch boundary: each item gets a separate complete
# pipeline run and a separate result. This is not a combined multi-image run.
batch:
  items:
    - id: image-001
      images: [../data/fixtures/images/fixture-01.jpg]
    - id: image-002
      images: [../data/fixtures/images/fixture-02.jpg]
steps:
  - name: describe
    model: Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL
    userPrompt: Describe the provided image and identify its primary subjects.
  - name: summarize
    model: Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL
    userPrompt: Return a concise structured analysis.
```

## Combined multi-image comparison

### 1. Overview

Supplies two images as one input set and creates one comparison. It is useful for visual QA, before-and-after review, side-by-side inspection, and any workflow requiring one result spanning multiple images.

### 2. Steps

- **compare:** Records meaningful similarities and differences across all supplied images.
- **summarize:** Produces one concise combined analysis from the comparison.

### 3. Expected final output

One combined analysis covering all images, explicitly identifying important similarities and differences rather than returning unrelated single-image outputs.

### 4. Example YAML

[Open pipelines/pipeline.multi-image-analysis-01.yaml in the repository](../pipelines/pipeline.multi-image-analysis-01.yaml)

### 5. YAML source

```yaml
name: multi-image-analysis
# `inputs.images` is one input set: both images are analyzed together in one
# complete pipeline run and produce one combined result.
inputs:
  images:
    - ../data/fixtures/images/fixture-01.jpg
    - ../data/fixtures/images/fixture-02.jpg
steps:
  - name: compare
    model: Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL
    userPrompt: Compare all provided images, noting similarities and differences.
  - name: summarize
    model: Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL
    userPrompt: Produce one concise combined analysis of the images.
```

## Custom model parameters for structured image analysis

### 1. Overview

Shows a compact multimodal pipeline with an image attachment, MCP disabled, explicit sampling controls, and a JSON schema. It is useful as a controlled starting point for reproducible vision experiments and strict output contracts.

### 2. Steps

- **analyze-input:** Analyzes the image in detail using custom temperature, topP, topK, token, repeat-penalty, and seed settings.
- **return-json:** Serializes the accumulated analysis into the exact schema using a separate lower-temperature parameter set.

### 3. Expected final output

Exactly one JSON object with only subjects, composition, and environmental_context string properties; all are required and must describe the image without unsupported claims.

### 4. Example YAML

[Open pipelines/pipeline.custom-model-paramers-01.yaml in the repository](../pipelines/pipeline.custom-model-paramers-01.yaml)

### 5. YAML source

```yaml
name: full-pipeline-example

# Optional. Defaults to induction.yaml when omitted.
config: induction.yaml

steps:
  - name: analyze-input
    model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
    nomcp: true
    systemPrompt: |
      You are a careful multimodal analysis assistant.
    userPrompt: |
      Analyze the provided image and produce a detailed analysis that can be
      transformed into the final structured response by the next step.
    image: ../data/fixtures/images/fixture-01.jpg
    # Use document instead of image when analyzing a document. Image and
    # document are mutually exclusive, and attachments are only allowed here.
    parameters:
      temperature: 0.7
      topP: 0.95
      topK: 40
      maxTokens: 2048
      repeatPenalty: 1.1
      seed: 42

  - name: return-json
    model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
    nomcp: true
    systemPrompt: |
      You are a precise data-formatting assistant.
    userPrompt: |
      Convert all accumulated analysis into one valid JSON object matching the
      supplied schema. Return JSON only, without Markdown fences, commentary,
      or additional keys.
    responseFormat:
      type: json_object
    jsonSchema:
      type: object
      properties:
        subjects:
          type: string
          description: The primary subjects identified in the input.
        composition:
          type: string
          description: The composition and visual arrangement of the input.
        environmental_context:
          type: string
          description: The environmental or contextual setting of the input.
      required:
        - subjects
        - composition
        - environmental_context
      additionalProperties: false
    parameters:
      temperature: 0.2
      topP: 0.9
      topK: 20
      maxTokens: 2048
      repeatPenalty: 1.0
      seed: 42
```
