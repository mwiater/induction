# Induction

Induction is a Go client for llama.cpp-compatible servers. It provides
config-driven chat inference, multimodal requests, structured output,
pipelines, telemetry, model inspection, health checks, and MCP tool support.

Induction expects a reachable llama.cpp-compatible server with an OpenAI-style
`/v1` API. Configure the server and runtime settings in `induction.yaml`;
[`induction.example.yaml`](induction.example.yaml) is a starting point.

<img src=".repo/induction-mcp-pipeline-compressed.gif" alt="Induction MCP Pipeline">

## Dependencies

- A running llama.cpp-compatible inference server started with the `--metrics`
  flag and using the expected OpenAI-style `/v1` API.
- Go 1.26.4 or newer to build or run Induction from source.
- GoReleaser to produce release artifacts.
- A Linux AMD64 host for the current release configuration.
- Access to the model and any image, PDF, or pipeline files used by an
  invocation.
- Docker for the easiest quick start: [DOCKER-QUICKSTART.md](docs/DOCKER-QUICKSTART.md).

## Quick Start

### Use Docker — easiest

See [DOCKER-QUICKSTART.md](docs/DOCKER-QUICKSTART.md).

### Build

Build and test the Linux AMD64 release artifact:

```bash
goreleaser release --snapshot --clean --skip=publish
```

For local development:

```bash
go run ./cmd/induction --help
```

## Documentation

The complete code reference is available at
<https://mwiater.github.io/induction/>.

Focused guides:

- [Inference](docs/INFERENCE.md)
- [MCP and application tools](docs/MCP.md)
- [Pipelines](docs/PIPELINES.md)
- [Model manager and inspection](docs/MODELS/MODEL-MANAGER.md)
- [Evaluations](docs/EVALUATIONS.md)
- [CLI reference](docs/CLI-REFERENCE.md)
- [Dashboard metrics](docs/DASHBOARD.md)
- [Development](docs/DEVELOPMENT.md)

## Examples

See [pipeline examples](docs/EXAMPLES.md) grouped by capability and workflow.

- [Document examples](docs/EXAMPLES-DOCUMENT.md)
- [Vision / image examples](docs/EXAMPLES-VISION-IMAGE.md)
- [Image text / OCR examples](docs/EXAMPLES-IMAGE-TEXT-OCR.md)
- [MCP tool examples](docs/EXAMPLES-MCP-TOOLS.md)
- [Decision routing examples](docs/EXAMPLES-DECISION-ROUTING.md)
- [Knowledge graph examples](docs/EXAMPLES-KNOWLEDGE-GRAPH.md)
- [Prompt optimization examples](docs/EXAMPLES-PROMPT-OPTIMIZATION.md)
- [Text examples](docs/EXAMPLES-TEXT.md)
- [Generated task-management example](docs/EXAMPLES-GENERATED-TASK-MANAGEMENT.md)

## Configuration

Config-driven inference reads `induction.yaml` from the current working
directory:

```yaml
server: your-llamacpp-inference-endpoint
timeout: 20m
pollInterval: 2s
loadWaitInterval: 1s
sidebarWidth: 64
log:
  prefix: "induction: "
  microseconds: true
  truncateOnRun: true
mcpServers:
  - mcpServerAllow: true
    mcpServerName: your-mcp-server-name
    mcpServerURL: your-mcp-server-endpoint
# Optional: required only for model-manager commands.
# modelManager:
#   searchResults: 20
#   preferredProviders:
#     - unsloth
#   modelsPath: /path/to/saved/models
#   huggingFaceToken: {optional-hugging-face-token}
resourceBudget:
  reasoning:
    cutoff:
      enabled: true
      maxTokens: 4096
      maxSeconds: 30
      maxContextPercent: 75
```

The checked-in [`induction.example.yaml`](induction.example.yaml) contains the
available configuration fields. Copy it to `induction.yaml`, uncomment the
optional sections you need, and replace placeholder values before use:

| Field | Meaning |
| --- | --- |
| `server` | llama.cpp-compatible inference endpoint. |
| `timeout` | Maximum duration for a request or operation. |
| `pollInterval` | Interval used while polling server state. |
| `loadWaitInterval` | Delay between model-load readiness checks. |
| `sidebarWidth` | Width of the terminal sidebar. |
| `log.prefix` | Prefix written before each log message. |
| `log.microseconds` | Include microseconds in log timestamps. |
| `log.truncateOnRun` | Truncate `induction.log` once when the application starts. |
| `mcpServers[].mcpServerAllow` | Enable that MCP server for inference. |
| `mcpServers[].mcpServerName` | Display name used to identify the MCP server. |
| `mcpServers[].mcpServerURL` | MCP server endpoint. |
| `modelManager.searchResults` | Number of model search results to return. |
| `modelManager.preferredProviders` | Provider order used by model searches and downloads. |
| `modelManager.modelsPath` | Local directory containing downloaded models. |
| `modelManager.huggingFaceToken` | Optional Hugging Face access token for model operations. |
| `resourceBudget.reasoning.cutoff.enabled` | Enable global reasoning cutoff. |
| `resourceBudget.reasoning.cutoff.maxTokens` | Maximum reasoning tokens observed through live slot metrics. |
| `resourceBudget.reasoning.cutoff.maxSeconds` | Maximum elapsed time after reasoning begins. |
| `resourceBudget.reasoning.cutoff.maxContextPercent` | Maximum active context-window utilization. |

At least one threshold is required when `enabled` is `true`; the other
thresholds are optional and may be omitted or left blank. The first configured
threshold reached ends reasoning.

`--nomcp` disables all configured MCP servers for one invocation. Chat sessions
are stored as private JSON under `.sessions/`, and application diagnostics are
written to `induction.log`.


## Common commands

Run these from the repository root or with an installed `induction` binary:

```bash
# Interactive text chat.
induction --model "MODEL"

# Unattended document question-answering.
induction --model "MODEL" --document PATH \
  --userPrompt "Summarize this document." --autosubmit

# Structured output.
induction --model "MODEL" --userPrompt "Return a JSON greeting." \
  --responseFormat json_object --autosubmit --autoexit

# A reusable pipeline.
induction --pipeline pipelines/pipeline.prompt-optimization-01.yaml

# Knowledge graph pipeline.
induction --pipeline pipelines/pipeline.emergent-knowledge-graph-01.yaml

# Inspect the configured server.
induction server inspect --json

# Generate the dashboard projection.
induction dashboard generate
```

Use `induction help` or see the [CLI reference](docs/CLI-REFERENCE.md) for
the complete command set.

For Docker builds, mounts, server checks, pipeline execution, and persistent
logs or assets, see the [Docker Quickstart](docs/DOCKER-QUICKSTART.md).

## Decision pipelines

Decision pipelines classify inputs into bounded semantic candidates and can
conditionally route later steps. For the YAML syntax, thresholds, skipped
steps, and server requirements, see the [pipeline authoring guide](docs/PIPELINES.md#decision-steps-and-conditional-routing).
For the Go API and `DecisionResult` behavior, see [Inference](docs/INFERENCE.md#bounded-decisions).

The [decision-routing examples](docs/EXAMPLES-DECISION-ROUTING.md) show both
single-branch review routing and a multi-level decision tree.
