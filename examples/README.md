# Examples

The examples demonstrate Induction's four output modes—default, stream, chat,
and snapshot—with and without application-managed MCP tools. Two additional
standalone commands demonstrate request parameters and model discovery.

Run every command from the repository root so Induction can load
`induction.yaml` from the current working directory. Each inference example
requires the model ID through the `--model` flag; use an ID exposed by the
configured llama.cpp-compatible server. No example source changes are needed.

Run `go run ./examples/list_models` to print the models exposed by the server.

## Example matrix

| Example | Default | Stream | Chat | Snapshot |
|---|:---:|:---:|:---:|:---:|
| [`infer`](infer/) | Yes | Yes | Yes | Yes |
| [`infer_mcp`](infer_mcp/) | Yes | Yes | Yes | Yes |

Standalone examples:

- [`infer_snapshot_parameters`](infer_snapshot_parameters/) demonstrates
  per-request sampling and generation parameters.
- [`list_models`](list_models/) lists the models exposed by the configured
  server.
- [`infer_tools`](infer_tools/) demonstrates an application-managed function
  tool loop, including validation, allowlisted dispatch, and continuation.
- [`infer_image`](infer_image/) demonstrates ordered text plus local data-URL
  image parts. It uses [`assets/images/fixture.jpg`](assets/images/fixture.jpg).
- [`infer_document`](infer_document/) attaches [`assets/documents/fixture.pdf`](assets/documents/fixture.pdf)
  inline with `FileDataURL`; the example does not require `/v1/files`.

## Function tools

`infer_tools` demonstrates the complete application-managed function-calling
protocol. The model first requests the allowlisted `get_weather` function. The
application validates the JSON arguments, executes the local function, preserves
the tool-call ID, appends an assistant tool-call message and a `role: tool`
result, and sends a second request for the final answer.

```sh
go run ./examples/infer_tools --model "Agents-A1-MTP-Apex-I-Quality"
```

Expected result is one final assistant response followed by a newline. The
example does not allow the model to execute arbitrary functions; add new
functions to the explicit dispatch table
before exposing them to a model.

## Image attachments

`infer_image` sends an ordered multimodal user message containing the detailed
image-analysis prompt and the checked-in local JPEG fixture as a base64 data
URL. The local file is validated and encoded before it is added to the request;
no external image URL is fetched.

```sh
go run ./examples/infer_image --model "Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL"
```

The fixture is [`assets/images/fixture.jpg`](assets/images/fixture.jpg). This
example requires a vision-capable model and a server that supports OpenAI-style
`image_url` content parts. Local attachments are limited to 10 MiB by default,
and unsupported MIME types, empty files, and missing files return actionable
errors.

Expected result is the model's image analysis printed as plain text. The exact
content depends on the selected vision model and the checked-in fixture.

## Document attachments

`infer_document` reads the checked-in PDF locally, extracts its text, and sends
that text in a normal user message before asking for a comprehensive technical
research analysis. This works with servers that do not support PDF/file content
parts and does not require a `/v1/files` endpoint or external network access.

```sh
go run ./examples/infer_document --model "Agents-A1-MTP-Apex-I-Quality"
```

The fixture is [`assets/documents/fixture.pdf`](assets/documents/fixture.pdf).
The example uses `induction.ExtractPDFText` for server-compatible inference.
For servers that do support typed document file content parts, the reusable
`induction.FileDataURL` helper can instead send the PDF inline:

```go
dataURL, filename, err := induction.FileDataURL(
    "examples/assets/documents/fixture.pdf",
    induction.DefaultAttachmentMaxBytes,
)
if err != nil {
    log.Fatal(err)
}
request := induction.ChatRequest{
    Messages: []induction.Message{{Role: "user", Content: []induction.ContentPart{
        {Type: "text", Text: "Summarize the attached PDF."},
        {Type: "file", File: &induction.FileContentPart{
            FileData: dataURL,
            Filename: filename,
        }},
    }}},
}
```

Document contents are not logged. Unsupported document MIME types, oversized
files, empty files, upload failures, and unsupported file endpoints are
returned as errors.

If the server responds with `unsupported content[].type`, it does not implement
document content parts. This is a server capability limitation; the example
reports it explicitly instead of retrying the PDF through an incompatible wire
format.

Expected result is a plain-text technical analysis of the extracted PDF. If the
fixture is missing or unreadable, the command exits nonzero before inference.

## Default

Default mode waits for inference to finish and then prints the complete
OpenAI-compatible response as formatted JSON. The `infer` program precedes it
with the system prompt, user prompt, and model headings.

### Standard inference

```sh
go run ./examples/infer --model "Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL" --mode default
```

Representative output:

```text
System Prompt: You are a precise technical assistant.
User Prompt:   Explain the purpose of an atomic pointer in Go in two detailed paragraphs.
Model:         Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL

{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "choices": [{"message": {"content": "An atomic pointer allows..."}}]
}
```

The response object varies by server and model; the headings are emitted by
the example itself.

```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1786464000,
  "model": "Qwen3.6-35B-A3B-MTP-UD-Q6_K_XL",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "An atomic pointer allows a pointer value to be read and replaced safely..."
      },
      "finish_reason": "stop"
    }
  ]
}
```

### MCP inference

```sh
go run ./examples/infer_mcp --model "Agents-A1-MTP-Apex-I-Quality" --mode default
```

Induction discovers the enabled MCP tools, lets the model select and call
read-only tools, returns tool results to the model, and prints the model's final
response as JSON. When live metrics are enabled and stdout is a terminal, MCP
activity is shown in a light-blue footer above the green metrics footer:

```text
[Induction: MCP] search_documents · completed · text×2 + structured · 184ms
[Induction: Live Metrics] model · Stage: MCP · Prefill (tok/s): 42.1 · Decode (tok/s): 31.8 ...
```

The MCP footer can report discovery, requested, running, completed, tool-error,
denied, and failed states. It summarizes returned content types without mixing
tool diagnostics into the model response.

Expected result is a JSON `InferenceResponse` containing the final answer after
any selected read-only tool calls. MCP status lines may appear in the terminal
footer while the request is running.

## Stream

Stream mode prints generated reasoning and content as it arrives. It does not
print the surrounding response JSON.

### Standard streaming

```sh
go run ./examples/infer --model "Agents-A1-MTP-Apex-I-Quality" --mode stream
```

Representative output appears incrementally:

```text
An atomic pointer provides synchronized access to a pointer value without requiring a mutex...
```

Reasoning returned separately by the server is delimited in the output:

```text
<think>
I should distinguish atomicity from ownership and memory reclamation.
</think>

An atomic pointer provides...
```

### MCP streaming

```sh
go run ./examples/infer_mcp --model "Agents-A1-MTP-Apex-I-Quality" --mode stream
```

Tool-call fragments are reconstructed internally. Tool status remains in the
sticky MCP footer while only the final generated content is written to the main
console area:

```text
The available documentation tool returned two relevant entries. Together they show...

[Induction: MCP] search_documents · completed · text×2 · 96ms
[Induction: Live Metrics] model · Stage: Complete · Prefill (tok/s): 39.4 ...
```

## Chat

Chat mode starts an interactive multi-turn session. The complete conversation
is retained between turns. Press Ctrl-C to finish the session.

### Standard chat

```sh
go run ./examples/infer --model "Agents-A1-MTP-Apex-I-Quality" --mode chat
```

When stdin and stdout are terminals, Induction presents its console UI with a
scrolling transcript, user-input field, model-information sidebar, and live
metrics footer. Responses stream into the transcript.

Representative interaction:

```text
Chat Session

You: What is an atomic pointer?

Assistant: It is a pointer value accessed through atomic operations...

User Input
You: When would I use one instead of a mutex?
```

Press Ctrl-1 to toggle the model-information sidebar.

### MCP chat

```sh
go run ./examples/infer_mcp --model "Agents-A1-MTP-Apex-I-Quality" --mode chat
```

MCP chat retains earlier user and assistant messages while making the configured
tools available on every turn. Like standard chat, it uses the full-screen
Bubble Tea interface with a scrolling transcript, input field, and model
sidebar. Its MCP and metrics footer rows show current tool activity and
inference metrics without inserting status messages into the chat:

```text
You: Search the documentation for atomic pointer guidance.
Assistant: The documentation highlights three relevant considerations...

[Induction: MCP] search_documents · completed · text×3 · 121ms
[Induction: Live Metrics] model · Stage: MCP · Prefill (tok/s): 37.2 ...
```

## Snapshot

Snapshot mode performs inference while collecting model telemetry. It prints a
formatted `ModelSnapshot`, including the interaction, model-load timing, slot
samples, model properties, and metrics when available.

### Standard snapshot

```sh
go run ./examples/infer --model "Agents-A1-MTP-Apex-I-Quality" --mode snapshot
```

Representative output (abbreviated):

```json
{
  "ModelID": "Agents-A1-MTP-Apex-I-Quality",
  "LoadTime": 125000000,
  "CollectedAt": "2026-08-11T12:00:00Z",
  "Interaction": [
    {
      "content": "An atomic pointer provides...",
      "response": "{\"id\":\"chatcmpl-123\",...}"
    }
  ],
  "messages": [
    {"role": "user", "content": "Explain atomic pointers."},
    {"role": "assistant", "content": "An atomic pointer provides..."}
  ],
  "Props": {
    "total_slots": 4
  },
  "Slots": [],
  "Metrics": {
    "raw": "...",
    "entries": {}
  }
}
```

When `persistSnapshots` is enabled in `induction.yaml`, snapshots are also
written beneath `snapshots/{model-id}/`.

### MCP snapshot

```sh
go run ./examples/infer_mcp --model "Agents-A1-MTP-Apex-I-Quality" --mode snapshot
```

This mode executes the MCP tool loop and returns telemetry for its final model
turn. The snapshot's interaction contains the final response produced after
the model has received any selected tool results.

Expected result is one JSON `ModelSnapshot` for the final model turn. Its
`Messages` include the tool-call conversation and its `Interaction` contains
the final assistant response.

### Parameterized snapshot

The parameterized snapshot is intentionally a standalone example rather than a
mode:

```sh
go run ./examples/infer_snapshot_parameters --model "GLM-4.7-Flash-Q4_K_M"
```

It demonstrates request-level overrides for temperature, top-p, top-k, maximum
tokens, repeat penalty, and seed. Its output has the same `ModelSnapshot` shape
as snapshot mode.

Expected result is one JSON snapshot containing the final interaction and
telemetry collected for the parameterized request. Unlike `infer`, this
standalone program writes only the indented snapshot JSON.

## MCP configuration and safety

`infer_mcp` implements application-managed MCP rather than native Responses API
remote-MCP objects. It reads every server from `MCPServers` in
`induction.yaml`:

```yaml
MCPServers:
  - MCPServerAllow: true
    MCPServerName: FEXR
    MCPServerURL: http://127.0.0.1:4002/mcp
```

Only entries with `MCPServerAllow: true` are contacted or exposed to the model.
Enabled servers are initialized and their discovered tools are combined. Tool
names must be unique across enabled servers so calls can be routed
unambiguously.

Read-only tools run automatically. Potentially side-effecting tools are denied
by default. Applications that need them must call the corresponding
`WithApproval` package function and provide an explicit approval callback.

The MCP mode functions used by the example are:

| Mode | Package function |
|---|---|
| Default | `induction.InferMCP` |
| Stream | `induction.InferMCPStream` |
| Chat | `induction.InferMCPChat` |
| Snapshot | `induction.InferMCPSnapshot` |

## List models

To inspect the models exposed by the server configured in `induction.yaml`:

```sh
go run ./examples/list_models
```

Representative output:

```text
MODEL                                  LOADED
Qwen3.6-35B-A3B-MTP-UD-Q6_K_XL        true
```

The exact rows depend on the running server. With no loaded models, the command
prints the table header and no model rows; configuration or server failures exit
nonzero.

## Cleanup and redirected output

The examples call `induction.Cleanup(os.Stdout)` before exiting. Cleanup removes
active sticky footers and restores the terminal scroll region. Applications
should do the same before `log.Fatal` or `os.Exit`, because those functions do
not run deferred calls.

Sticky footers and the full-screen chat UI are used only with interactive
terminals. When output is redirected, terminal escape sequences are omitted,
but `infer` and `infer_mcp` still print their headings before response data:

```sh
go run ./examples/infer --model "your-model-id" --mode default > response.json
go run ./examples/infer_mcp --model "your-model-id" --mode snapshot > snapshot.txt
```

Use `infer_snapshot_parameters` when a machine-readable JSON-only snapshot file
is required.
