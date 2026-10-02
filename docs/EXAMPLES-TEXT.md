# Text Pipeline Examples

## Structured text log analysis

### 1. Overview

Parses fixed application log lines and converts observations into an incident summary. It is useful for conservative operational reporting where exact values matter but root cause must not be asserted without evidence.

### 2. Steps

- **parse-log:** Extracts timestamps, levels, request IDs, latencies, statuses, and observable patterns.
- **produce-incident-summary:** Returns JSON with summary, observations, affected_requests, and next_checks, separating observations from hypotheses.

### 3. Expected final output

A JSON object with summary, observations, affected_requests, and next_checks, preserving exact values and counts and listing diagnostic checks without claiming an unverified cause.

### 4. Example YAML

[Open pipelines/pipeline.text-01.yaml in the repository](../pipelines/pipeline.text-01.yaml)

### 5. YAML source

```yaml
name: text-05-structured-log-analysis

config: induction.yaml

steps:
  - name: parse-log
    model: LFM-2.5-8B-A1B-UD-Q8_K_XL
    nomcp: true
    systemPrompt: You are a conservative incident-data analyst. Treat the supplied log lines as the only source and preserve exact values.
    userPrompt: |
      Analyze these application log lines:
      2026-09-21T10:04:12Z WARN api request_id=abc latency_ms=842 status=503
      2026-09-21T10:04:15Z INFO api request_id=abd latency_ms=91 status=200
      2026-09-21T10:04:19Z ERROR api request_id=abe latency_ms=1204 status=503
      Extract timestamps, levels, request IDs, latency, and status. Identify
      the observable pattern without claiming a root cause.

  - name: produce-incident-summary
    model: LFM-2.5-8B-A1B-UD-Q8_K_XL
    nomcp: true
    systemPrompt: You produce a concise incident summary from the preceding structured log analysis. Separate observations from hypotheses.
    userPrompt: |
      Return a JSON object with `summary`, `observations`, `affected_requests`,
      and `next_checks`. Include exact counts and values from the logs, use an
      empty array when appropriate, and list diagnostic checks rather than
      asserting an unverified cause.
    responseFormat:
      type: json_object
```

