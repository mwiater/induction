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
