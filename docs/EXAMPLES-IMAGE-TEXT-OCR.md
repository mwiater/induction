# Image Text / OCR Pipeline Examples

## Form OCR and validation

### 1. Overview

Extracts visible content from a photographed form and validates the resulting record. It is useful for data entry, scanned forms, manual-review queues, and workflows where blanks and uncertain characters must remain explicit.

### 2. Steps

- **extract-form-content:** Reads visible text, preserves labels, values, punctuation, and reading order, associates values with labels, and marks blank, obscured, or unreadable fields.
- **validate-form-record:** Checks the authoritative extraction and reports captured fields, manual-review fields, ambiguous text, and reasons for review.

### 3. Expected final output

A concise validation report containing captured fields, fields needing manual review, and ambiguous transcription. It must preserve source wording and never invent or silently correct values.

### 4. Example YAML

[Open pipelines/pipeline.image-text-01.yaml in the repository](../pipelines/pipeline.image-text-01.yaml)

### 5. YAML source

```yaml
name: image-text-05-form-processing

config: induction.yaml

steps:
  - name: extract-form-content
    model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
    nomcp: true
    systemPrompt: |
      You are a careful form-processing assistant. Read only text visibly
      present in the supplied image and preserve labels, values, punctuation,
      and uncertainty. Do not infer personal data or fill missing fields.
    userPrompt: |
      Extract the visible text as a form record. Preserve the reading order,
      associate each value with its visible label, and mark blank, obscured, or
      unreadable fields explicitly. Keep the original wording rather than
      correcting it.
    image: ../data/fixtures/images/fixture-text-01.jpg

  - name: validate-form-record
    model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
    nomcp: true
    systemPrompt: |
      You validate a preceding image-text extraction for data entry. Treat the
      extraction as authoritative and never invent or silently correct values.
    userPrompt: |
      Review the extracted form record for consistency. Return a concise report
      with captured fields, fields needing manual review, and a transcription
      of any ambiguous text. Explain why each field needs review when possible.
```

