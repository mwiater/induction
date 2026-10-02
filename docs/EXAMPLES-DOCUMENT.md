# Document Pipeline Examples

Examples:

- [Structured extraction](#document-structured-extraction-and-normalization)
- [Batch analysis](#batch-document-analysis)
- [Combined analysis](#combined-multi-document-analysis)

## Document structured extraction and normalization

### 1. Overview

Reads fixture-01.pdf as the authoritative source and converts unstructured content into a conservative machine-readable record. It is useful for document intake, archival extraction, and downstream systems that need exact wording, typed values, and provenance without invented facts.

### 2. Steps

- **extract-records:** Ingests the PDF and inventories explicit names, dates, numbers, units, categories, and cited entities, preserving context and marking absent or ambiguous values unknown.
- **normalize-document-data:** Converts the extraction to JSON with entities and facts arrays; each item retains source wording, normalized type, and source context, with null for undetermined values.

### 3. Expected final output

JSON only with entities and facts arrays. Each item should contain exact source wording, normalized type, and source context; unknown values should be null, with no silent correction or outside information.

### 4. Example YAML

[Open pipelines/pipeline.document-01.yaml in the repository](../pipelines/pipeline.document-01.yaml)

### 5. YAML source

```yaml
name: document-05-structured-extraction

config: induction.yaml

steps:
  - name: extract-records
    model: Muse-Glimmer-30B-Q4_K_XL
    nomcp: true
    systemPrompt: |
      You are a conservative document data-extraction assistant. Extract only
      values explicitly present in the supplied document and preserve their
      wording, units, dates, and qualifiers.
    userPrompt: |
      Read the document and inventory every useful structured value you can
      find, including names, dates, numbers, units, categories, and cited
      entities. Group related values, retain their surrounding context, and
      mark absent or ambiguous values as unknown rather than guessing.
    document: ../data/fixtures/documents/fixture-01.pdf

  - name: normalize-document-data
    model: Muse-Glimmer-30B-Q4_K_XL
    nomcp: true
    systemPrompt: |
      You normalize a preceding document extraction for downstream processing.
      The ingested document is authoritative; never silently correct or infer
      a value.
    userPrompt: |
      Convert the extracted values into a clear JSON object with an `entities`
      array and a `facts` array. Each entity or fact must include its exact
      source wording, normalized type, and source context. Use null for values
      that are not present or cannot be determined, and return JSON only.
    responseFormat:
      type: json_object
```

## Batch document analysis

### 1. Overview

Defines two independent batch items, each containing one PDF. Every item receives a complete run and a separate result, making this appropriate for queue-based per-document processing where documents must not be blended.

### 2. Steps

- **extract:** Identifies the subject, purpose, and key facts in the current batch document.
- **summarize:** Produces a concise structured analysis for that same document.

### 3. Expected final output

One separate result for document-001 and one for document-002, each containing document-specific subject, purpose, key facts, and structured summary.

### 4. Example YAML

[Open pipelines/pipeline.batch-document-analysis-01.yaml in the repository](../pipelines/pipeline.batch-document-analysis-01.yaml)

### 5. YAML source

```yaml
name: batch-document-analysis
# `batch.items` is the batch boundary: each document gets a separate complete
# pipeline run and a separate result.
batch:
  items:
    - id: document-001
      documents: [../data/fixtures/documents/fixture-01.pdf]
    - id: document-002
      documents: [../data/fixtures/documents/fixture-02.pdf]
steps:
  - name: extract
    model: Qwen-3.5-9B-MTP-General-Q8_0
    userPrompt: Identify the subject, purpose, and key facts in the document.
  - name: summarize
    model: Qwen-3.5-9B-MTP-General-Q8_0
    userPrompt: Produce a concise structured analysis.
```

## Combined multi-document analysis

### 1. Overview

Supplies two PDFs as one input set and produces one combined analysis. Unlike batching, this intentionally reasons across the documents and is useful for synthesis, comparison, corpus themes, and combined reporting.

### 2. Steps

- **compare:** Analyzes all supplied documents together and identifies similarities, differences, and key themes.
- **report:** Turns the accumulated comparison into one concise combined report.

### 3. Expected final output

One report covering both documents collectively, with meaningful similarities and differences and clear source distinctions where needed, rather than independent per-document results.

### 4. Example YAML

[Open pipelines/pipeline.multi-document-analysis-01.yaml in the repository](../pipelines/pipeline.multi-document-analysis-01.yaml)

### 5. YAML source

```yaml
name: multi-document-analysis
# `inputs.documents` is one input set: both documents are analyzed together in
# one complete pipeline run and produce one combined report.
inputs:
  documents:
    - ../data/fixtures/documents/fixture-01.pdf
    - ../data/fixtures/documents/fixture-02.pdf
steps:
  - name: compare
    model: Qwen-3.5-9B-MTP-General-Q8_0
    userPrompt: Compare the provided documents and identify their key themes.
  - name: report
    model: Qwen-3.5-9B-MTP-General-Q8_0
    userPrompt: Produce one concise combined report.
```
