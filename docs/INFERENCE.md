# Inference

Induction supports interactive and unattended text, image, and document
inference through the same configured llama.cpp-compatible server.

## Chat API

`InferChat` provides a multi-turn session. `InferStreamChat` provides the same
session while writing generated content as it arrives:

```go
err := induction.InferStreamChat(ctx, &induction.ChatRequest{
    Model: "Qwen-3.5-9B-MTP-General-Q8_0",
    Messages: []induction.Message{{Role: "system", Content: "Be helpful."}},
}, os.Stdin, os.Stdout)
```

Sessions retain conversation history and per-turn `ModelSnapshot` telemetry.
Use `Client.GenerateSnapshot` when an application needs telemetry for one turn.
Reasoning is kept separate from visible content and is rendered as a
`<think>...</think>` block during streaming.

## Bounded decisions

For a bounded decision, set `ChatRequest.Decision` and call
`Client.GenerateSnapshot`. Each candidate key must tokenize as exactly one
token for the selected model. Candidate values are semantic labels, and the
returned `DecisionResult` is application-authored JSON in the first
interaction's `Content`. Candidate probabilities are normalized only across
the configured set; `confidence` is the selected probability and `margin` is
the difference from the runner-up. A decision requires the non-streaming
snapshot API; `GenerateStreamingSnapshot` returns an error for decisions.
Use a Jev decision model for this task. The example model ID
`JEV5K-v0.3-4B-Q8_0` refers to the JevK5 v0.3 4B Q8_0 GGUF in
[the model card](https://huggingface.co/alibiserikbay/JevK5-GGUF); the server
must expose the configured model ID.

```go
import (
    "context"
    "encoding/json"
    "fmt"

    induction "github.com/mwiater/induction"
)

func decide(ctx context.Context, client *induction.Client) (*induction.DecisionResult, error) {
    decision := &induction.DecisionConfig{
        Candidates: map[string]string{
            "A": "relevant",
            "B": "irrelevant",
        },
        TopLogprobs: 20,
    }
    snapshot, err := client.GenerateSnapshot(ctx, &induction.ChatRequest{
        Model: "JEV5K-v0.3-4B-Q8_0",
        Messages: []induction.Message{{Role: "user", Content: "Classify this material."}},
        Decision: decision,
    })
    if err != nil {
        return nil, err
    }
    if len(snapshot.Interaction) == 0 {
        return nil, fmt.Errorf("decision returned no interaction")
    }
    var result induction.DecisionResult
    if err := json.Unmarshal([]byte(snapshot.Interaction[0].Content), &result); err != nil {
        return nil, err
    }
    return &result, nil
}
```

Pipeline users can use `decision:` together with `when:` to route later steps
on the semantic value. The [`pipeline authoring guide`](PIPELINES.md#decision-steps-and-conditional-routing)
covers pipeline syntax, thresholds, skipped steps, and the compatible
`classification:` alias.

## Common commands

```bash
induction --model MODEL
induction --model MODEL --image PATH --userPrompt "Describe this image." --autosubmit
induction --model MODEL --document PATH --userPrompt "Summarize this document." --autosubmit
induction --model MODEL --userPrompt "Return JSON." --responseFormat json_object --autosubmit --autoexit
```

Sampling controls include `--temperature`, `--top-p`, `--top-k`,
`--max-tokens`, `--repeat-penalty`, and `--seed`. Use `--config PATH` for a
non-default configuration and `--nomcp` to disable configured MCP servers for
one invocation.
