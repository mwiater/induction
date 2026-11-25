# Prompt Optimization Pipeline Examples

## Prompt optimization experiment

### 1. Overview

Compares a baseline answer with an answer produced from an optimized prompt and evaluates the difference. It is useful for prompt-engineering experiments, regression comparisons, and testing whether clearer instructions improve quality.

### 2. Steps

- **baseline:** Answers the raw creative-writing request directly.
- **generate-optimized-prompt:** Rewrites the raw request into a self-contained optimized prompt while preserving intent and constraints.
- **optimized-answer:** Uses the generated prompt to produce the requested story without discussing the experiment.
- **evaluate:** Scores and compares baseline and optimized responses and states whether optimization helped.

### 3. Expected final output

The optimized answer plus a concise evaluation scoring both responses, identifying improvements or harms, and avoiding unsupported additions or a second answer to the original request.

### 4. Example YAML

[Open pipelines/pipeline.prompt-optimization-01.yaml in the repository](../pipelines/pipeline.prompt-optimization-01.yaml)

### 5. YAML source

```yaml
name: prompt-optimization-07-code-review

steps:
  - name: baseline
    model: Qwen-3-Coder-Next-Q4_K_M
    systemPrompt: You are a helpful software-engineering assistant. Answer directly and do not discuss this experiment.
    userPrompt: &raw_user_prompt |
      [RAW USER PROMPT]
      Review this Go function for correctness and maintainability, then
      suggest a minimal fix. Focus on the bug rather than rewriting unrelated
      code:
      ```go
      func first(items []string) string {
          if len(items) > 0 {
              return items[1]
          }
          return ""
      }
      ```

  - name: generate-optimized-prompt
    model: Qwen-3-Coder-Next-Q4_K_M
    systemPrompt: |
      You optimize coding-review prompts. Rewrite only the raw request into a
      self-contained review task. Preserve the language, code, requested scope,
      and minimal-fix constraint. Require a precise explanation, corrected
      snippet, and relevant edge-case note without inventing unrelated issues.
      Return only the rewritten prompt.
    userPrompt: *raw_user_prompt

  - name: optimized-answer
    model: Qwen-3-Coder-Next-Q4_K_M
    systemPrompt: Follow the optimized coding-review prompt. Return the review and minimal fix only, without discussing prompt optimization.
    userPrompt: Produce the corrected code review requested by the optimized prompt.

  - name: evaluate
    model: Qwen-3-Coder-Next-Q4_K_M
    systemPrompt: Compare both reviews for finding the actual bug, explaining the index and boundary issue correctly, respecting scope, and providing a valid minimal fix. Score each out of 10.
    userPrompt: Evaluate the baseline and optimized code reviews.
```

