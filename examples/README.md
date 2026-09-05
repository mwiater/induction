# Examples

All inference examples use chat inference. The interactive examples accept a
model ID with `--model`; inference starts directly in chat.

## Interactive chat

```bash
go run ./examples/infer --model "Agents-A1-MTP-Apex-I-Quality"
go run ./examples/infer --model "Agents-A1-MTP-Apex-I-Quality" --prompt "Give me a detailed 5 paragraph biography on Claude Shannon." --autosubmit --autoexit
go run ./examples/infer_mcp --model "Ornith-1.0-35B-UD-Q4_K_M" --prompt "What is the current local weather in Portland, OR?" --autosubmit --autoexit
go run ./examples/infer_param_overrides --model "Agents-A1-MTP-Apex-I-Quality" --prompt "Give me a detailed 5 paragraph biography on Claude Shannon." --autosubmit --autoexit
```

The interactive examples start chat sessions. Use `--prompt` to prefill
the first user input after the model is ready; add `--autosubmit` to send it
automatically. `--autosubmit` requires a non-empty `--prompt`. The MCP variant
starts a chat session with configured MCP tools. Add `--autoexit` to exit only
after the automated response and session snapshot have been saved; it requires
`--autosubmit`.

The parameter-overrides example starts chat with explicit sampling and
generation settings on its `ChatRequest`, demonstrating request-level tuning.

## Multimodal requests

Analyze the bundled image:

```bash
go run ./examples/infer_image --model "Qwen-3.5-9B-MTP-General-Q8_0"
```

Analyze the bundled PDF:

```bash
go run ./examples/infer_document --model "Phi-4-Mini-Reasoning-Q4_K_M"
```

## Application-managed tools

The tools example demonstrates a function-calling loop where the application
executes local system tools for the current date/time, free disk space on `/`,
and available RAM. Disk and RAM requests also chain the system-time tool:

```bash
go run ./examples/infer_tools --model "Agents-A1-MTP-Apex-I-Quality"
```

## Model listing

```bash
go run ./examples/list_models
```

All examples read the server and runtime settings from `induction.yaml` when
the selected library function requires configuration.
