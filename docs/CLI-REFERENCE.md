# CLI reference

Run `induction help` or append `--help` to any command for built-in help. The
`induction list commands` command prints the current command tree. Unless noted,
commands accept the global `--config` flag. Model-manager and evaluation
commands require the `--beta` acknowledgement flag.

## Commands and subcommands

| Command | Description | Beta |
| --- | --- | --- |
| `induction [PROMPT]` | Run inference and manage Induction. With no flags, opens the model selector and waits for chat input; with one positional prompt and no flags, submits it after selection; with inference flags, starts the existing flag-based workflow. |  |
| `induction dashboard` | Manage generated dashboard artifacts. |  |
| `induction dashboard generate` | Generate dashboard metrics from persisted sessions. |  |
| `induction eval` | Run a configured local model evaluation. | ☑ |
| `induction eval status` | Show evaluation completion status. | ☑ |
| `induction help` | Show help about any command. |  |
| `induction list` | List available command groups. |  |
| `induction list commands` | List all commands and subcommands with descriptions. |  |
| `induction models` | Search, download, and manage models. Alias: `induction model-manager`. | ☑ |
| `induction models details [MODEL]` | Show installed model details. | ☑ |
| `induction models download REPOSITORY FILE` | Download a model file. | ☑ |
| `induction models files REPOSITORY` | List files in a model repository. | ☑ |
| `induction models inspect MODEL` | Inspect a model's capabilities and runtime. | ☑ |
| `induction models list` | List installed or loaded models. | ☑ |
| `induction models mmproj` | Check installed vision models for downloaded mmproj files. | ☑ |
| `induction models remove [MODEL]` | Remove an installed model. | ☑ |
| `induction models search [QUERY]` | Search model repositories. | ☑ |
| `induction models update [MODEL]` | Update an installed model. | ☑ |
| `induction models verify [MODEL]` | Verify an installed model. | ☑ |
| `induction pdf` | Work with PDF files. |  |
| `induction pdf preview` | Preview extracted text from a PDF. |  |
| `induction pipeline` | Create and work with pipelines. |  |
| `induction pipeline generate` | Generate a runnable pipeline from a complex prompt. |  |
| `induction runtime` | Manage the server's runtime models. |  |
| `induction runtime load MODEL` | Load a runtime model. |  |
| `induction runtime status` | Show the server's runtime model state. |  |
| `induction runtime switch MODEL` | Switch the active runtime model. |  |
| `induction runtime unload MODEL` | Unload a runtime model. |  |
| `induction server` | Inspect the configured inference server. |  |
| `induction server inspect` | Inspect the configured inference server. |  |
| `induction sessions` | Manage persisted inference sessions. |  |
| `induction sessions clean` | Remove invalid persisted sessions. |  |
| `induction sessions inspect` | Replay a persisted chat session as an instant transcript. |  |
| `induction ui` | Preview console UI options. |  |
| `induction ui theme` | Preview console UI theme colors and styles. |  |

## Flags

`--help` (or `-h`) is available on every command. `--config` is a persistent
root flag, so it can be used with any command that reads the application
configuration.

### Global configuration

| Flag | Default | Description |
| --- | --- | --- |
| `--config FILE` | `induction.yaml` | Configuration file. |

### Direct inference (`induction`)

Interactive root invocations:

```bash
# Open the model selector, then wait for the first chat message.
induction

# Open the model selector, submit the prompt, and keep the chat open.
induction "Explain how transformers work"
```

The root command also accepts exactly one positional prompt with no explicitly
supplied flags, for example `induction "Hello!"`. It opens the existing model
selector, submits the prompt after the selected model is ready, and stays open
for follow-up questions. Quoting is recommended for multiword prompts and
shell metacharacters, but shell quote syntax cannot be inspected. Do not
combine this shortcut with flags; use the existing flag-based workflow for
advanced options. A prompt identical to a registered subcommand is resolved as
that subcommand instead.

These flags run inference when supplied directly to `induction`. `--pipeline`
selects a pipeline instead of direct inference. Request parameter flags override
the corresponding configured/model defaults when explicitly provided.

| Flag | Default | Description |
| --- | --- | --- |
| `--model ID` | empty | Model ID to use for inference. Required for direct inference; not required with `--pipeline`. |
| `--userPrompt TEXT` | empty | Initial user prompt. |
| `--systemPrompt TEXT` | empty | System prompt override. |
| `--responseFormat FORMAT` | empty | Response format: `text`, `json_object`, or `json_schema`. |
| `--jsonSchema JSON` | empty | Inline JSON schema object; requires a JSON response format. |
| `--image PATH` | empty | Local image path. |
| `--document PATH` | empty | Local PDF/document path. |
| `--documents DIR` | empty | Directory containing local PDF files. |
| `--pipeline PATH` | empty | Pipeline YAML configuration path. |
| `--temperature NUMBER` | `0` (unset) | Request temperature override. |
| `--top-p NUMBER` | `0` (unset) | Request top-p override. |
| `--top-k NUMBER` | `0` (unset) | Request top-k override. |
| `--max-tokens NUMBER` | `0` (unset) | Request maximum token override. |
| `--repeat-penalty NUMBER` | `0` (unset) | Request repeat-penalty override. |
| `--seed NUMBER` | `0` (unset) | Request seed override. |
| `--autosubmit` | `false` | Submit `--userPrompt` automatically. Requires a non-empty `--userPrompt`. |
| `--autoexit` | `false` | Exit after the automated response and session save. Requires `--autosubmit`. |
| `--nomcp` | `false` | Disable configured MCP servers for this inference. |

`--image` cannot be combined with `--document` or `--documents`; `--document`
and `--documents` cannot be combined. `--pipeline` cannot be combined with the
direct inference flags.

### Pipeline generation (`induction pipeline generate`)

Exactly one of `--prompt` and `--prompt-file` is required. `--model` and
`--output` are also required.

| Flag | Default | Description |
| --- | --- | --- |
| `--model ID` | empty | Model ID used for planning and generated steps. |
| `--prompt TEXT` | empty | Original user prompt. |
| `--prompt-file PATH` | empty | File containing the original user prompt. |
| `--output PATH` | empty | Output pipeline YAML path. |
| `--force` | `false` | Overwrite an existing output file. |
| `--validate` | `false` | Append a read-only final validation step. |

### PDF preview (`induction pdf preview`)

| Flag | Default | Description |
| --- | --- | --- |
| `--file PATH` | empty; required | Path to the PDF file. |

### Evaluation (`induction eval`)

All evaluation commands require `--beta`. Use exactly one of `--model` or
`--allModels` for `induction eval`.

| Flag | Default | Description |
| --- | --- | --- |
| `--beta` | `false`; required | Acknowledge that this feature is in development. |
| `--eval-config PATH` | empty; required | Evaluation suite YAML configuration. |
| `--model ID` | empty | Model ID to evaluate. |
| `--allModels` | `false` | Run the evaluation suite against every available model. |

`induction eval status` accepts `--beta`, `--eval-config PATH`, and
`--model ID`; both `--eval-config` and `--model` are required for status.

### Model manager (`induction models`)

All model-manager commands require `--beta`. The following persistent flags
are inherited by the `models` command and all its subcommands:

| Flag | Default | Description |
| --- | --- | --- |
| `--beta` | `false`; required | Acknowledge that this feature is in development. |
| `--models-path DIR` | empty | Directory used to store models. |
| `--search-results N` | `0` (configuration default applies) | Maximum search results. |
| `--preferred-provider NAME` | none | Preferred provider; repeat the flag to specify multiple providers. |

Additional model-manager command flags:

| Command | Flag | Default | Description |
| --- | --- | --- | --- |
| `models search` | `--json` | `false` | Write search results as JSON. |
| `models files` | `--json` | `false` | Write repository file results as JSON. |
| `models files` | `--all` | `false` | Show all repository files, including files normally filtered out. |
| `models download` | `--revision SHA` | empty (current revision) | Immutable repository revision to download from. |
| `models download` | `--yes` | `false`; required | Confirm the download. |
| `models list` | `--installed` | `false` | List installed models. Mutually exclusive with `--loaded`; one is required. |
| `models list` | `--loaded` | `false` | List currently loaded models. Mutually exclusive with `--installed`; one is required. |
| `models list` | `--json` | `false` | Write results as JSON. |
| `models details` | `--json` | `false` | Write model details as JSON; requires a `MODEL` argument. |
| `models inspect` | `--json` | `false` | Write inspection results as JSON. |
| `models verify` | `--json` | `false` | Write verification results as JSON; requires a `MODEL` argument. |
| `models remove` | `--yes` | `false` | Confirm removal; requires a `MODEL` argument. |
| `models update` | `--yes` | `false` | Confirm update; requires a `MODEL` argument. |
| `models mmproj` | `--yes` | `false` | Download all missing mmproj files without prompting. |
| `models mmproj` | `--json` | `false` | Write check results as JSON. |

Without `--yes`, `models remove` and `models update` use the interactive
interface. `models details` and `models verify` also use the interactive
interface unless `--json` is provided.

### Evaluation status (`induction eval status`)

| Flag | Default | Description |
| --- | --- | --- |
| `--beta` | `false`; required | Acknowledge that this feature is in development. |
| `--eval-config PATH` | empty; required | Evaluation suite YAML configuration. |
| `--model ID` | empty; required | Model ID whose evaluation status to show. |

### Server and runtime JSON output

| Command | Flag | Default | Description |
| --- | --- | --- | --- |
| `induction server inspect` | `--json` | `false` | Write server inspection as JSON. |
| `induction runtime status` | `--json` | `false` | Write runtime model state as JSON. |
| `induction runtime load MODEL` | `--json` | `false` | Write the load result as JSON. |
| `induction runtime unload MODEL` | `--json` | `false` | Write the unload result as JSON. |
| `induction runtime switch MODEL` | `--json` | `false` | Write the switch result as JSON. |
