# Induction

Induction is a Go client for llama.cpp-compatible servers. It provides
config-driven chat inference, OpenAI-compatible request and response types,
live terminal metrics, per-turn telemetry snapshots, model inspection, health
checks, and MCP tool support.

## Configuration

Config-driven inference reads `induction.yaml` from the current working
directory:

```yaml
server: http://localhost:9998
timeout: 20m
poll_interval: 2s
load_wait_interval: 1s
enableLiveMetricsOverlay: true
sidebarWidth: 32
log:
  path: induction.log
  console: true
  prefix: "app: "
  microseconds: true
```

`ChatRequest.Model` selects the model for each request. Chat sessions are
stored as private JSON under `.sessions/`; each session contains its transcript
and the telemetry snapshots collected for completed turns.

## Chat inference

`InferChat` runs a multi-turn non-streaming session. `InferStreamChat` runs the
same session while writing generated content as it arrives:

```go
err := induction.InferStreamChat(ctx, &induction.ChatRequest{
    Model: "Your-Model-Name",
    Messages: []induction.Message{{Role: "system", Content: "Be helpful."}},
}, os.Stdin, os.Stdout)
```

Both functions retain the conversation and per-turn `ModelSnapshot` in the
session object. Snapshots include the interaction, complete message history,
model properties, slot samples, metrics, and request metadata. The lower-level
`Client.GenerateSnapshot` and `GenerateStreamingSnapshot` methods are
available when an application needs telemetry for a single turn.

Reasoning remains separate from visible content. Streaming output renders it
as a `<think>...</think>` block.

## MCP chat

`InferMCPChat` provides the same multi-turn chat experience with configured MCP
servers. Read-only tools run automatically; other tools may use an approval
callback through `InferMCPChatWithApproval`.

## Examples

See [`examples/README.md`](examples/README.md) for runnable examples. The
interactive inference examples launch directly into chat and accept only the
model flag; there is no mode selector.

## Model manager and inspection

Install the modern Hugging Face CLI for model-manager operations:

```bash
pip install -U huggingface_hub
```

The command-line tools include model discovery and downloads, plus read-only
server and model inspection. See the source under `internal/modelmanager` and
`internal/cli` for the complete command set.

## Dashboard metrics

Generate a rebuildable, server-free metrics projection from all valid saved and
unsaved sessions:

```bash
induction generate dashboard
```

The source is `.sessions/` and the outputs are
`data/dashboard/session_metrics.json` and the self-contained
`data/dashboard/dashboard.html`. Raw transcript, reasoning, response,
properties, metrics, and slot payloads are not copied; the session files remain
the source of truth.

## Build and test

```bash
goreleaser release --snapshot --clean --skip=publish
```
