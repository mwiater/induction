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
| `induction pipeline optimize` | Analyze completed sessions and propose one validated alternative pipeline. |  |
| `induction runtime` | Manage the server's runtime models. |  |
| `induction runtime load MODEL` | Load a runtime model. |  |
| `induction runtime status` | Show the server's runtime model state. |  |
| `induction runtime switch MODEL` | Switch the active runtime model. |  |
| `induction runtime unload MODEL` | Unload a runtime model. |  |
| `induction server` | Inspect the configured inference server. |  |
| `induction server inspect` | Inspect the configured inference server. |  |
| `induction sessions` | Manage persisted inference sessions. |  |
| `induction sessions clean` | Remove old invalid session JSON and terminal pipeline sessions with null snapshots; preserve active sessions. |  |
| `induction sessions inspect` | Replay a persisted chat session as an instant transcript. |  |
| `induction sessions purge` | Permanently delete local `.sessions/`, `.pipeline-artifacts/`, and `.batches/` directories. |  |
| `induction ui` | Preview console UI options. |  |
| `induction ui theme` | Preview console UI theme colors and styles. |  |

`induction sessions clean` preserves running pipeline sessions, including
their initial `snapshots: null` files. It also preserves legacy pipeline
sessions whose completion state is ambiguous. Invalid JSON and terminal
pipeline sessions with null snapshots must be at least one minute old before
cleanup deletes them. The command reports active or recent files it skipped.

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
| `--min-p NUMBER` | `0` (unset) | Request minimum-probability sampling threshold. |
| `--xtc-probability NUMBER` | `0` (unset) | Request XTC token-exclusion probability. |
| `--xtc-threshold NUMBER` | `0` (unset) | Request XTC probability threshold. |
| `--typical-p NUMBER` | `0` (unset) | Request typical sampling probability mass. |
| `--presence-penalty NUMBER` | `0` (unset) | Penalize tokens based on whether they have appeared. |
| `--frequency-penalty NUMBER` | `0` (unset) | Penalize tokens based on how often they have appeared. |
| `--repeat-last-n NUMBER` | `0` (unset) | Number of recent tokens considered by repeat-penalty sampling. |
| `--autosubmit` | `false` | Submit `--userPrompt` automatically. Requires a non-empty `--userPrompt`. |
| `--autoexit` | `false` | Exit after the automated response and session save. Requires `--autosubmit`. |
| `--nomcp` | `false` | Disable configured MCP servers for this inference. |

`--image` cannot be combined with `--document` or `--documents`; `--document`
and `--documents` cannot be combined. `--pipeline` cannot be combined with the
direct inference flags.

All request parameter flags are optional. An omitted flag is not included in
the inference request, so the model/server default is preserved. Explicit
values of `0` are sent when the flag is supplied.

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

### Pipeline optimization (`induction pipeline optimize`)

```bash
induction pipeline optimize --pipeline pipelines/pipeline.prompt-optimization-01.yaml \
  --sessions auto --source-root . --output-dir pipelines
```

The optimizer reads the target YAML, a matching source checkout, and **completed**
pipeline sessions. It calls the configured local llama.cpp server for analysis
and YAML generation, then validates the candidate with Induction's strict parser
and static validator. It does not run the original or alternative pipeline.
`--model` selects the optimizer model; by default the first model in the input
pipeline is used. Candidate steps may use only model IDs already in the input
pipeline; model swaps are not verified in this MVP.

| Flag | Default | Description |
| --- | --- | --- |
| `--pipeline PATH` | required | Input `.yaml` or `.yml` pipeline. |
| `--sessions auto\|none` | `auto` | Discover completed sessions or use none. |
| `--session ID` | none | Select a completed session by ID; repeatable. Overrides auto discovery unless `--sessions auto` is explicit. |
| `--session-file PATH` | none | Select a completed session JSON file; repeatable. |
| `--max-sessions N` | `10` | Evidence cap, from 1 to 50. |
| `--source-root PATH` | current directory | Matching readable source checkout. |
| `--output-dir PATH` | input directory | Existing directory for output files. |
| `--model ID` | first pipeline model | Local model for optimizer inference. |
| `--focus MODE` | `balanced` | `balanced`, `reliability`, `efficiency`, or `structure`. |
| `--dry-run` | `false` | Perform static inspection only and print the input preview/report; do not contact llama.cpp, infer, load models, or create files. |
| `--verbose` | `false` | Print optimizer stage progress, evidence counts, validation attempts, output warnings, and paths. |

Auto matching prioritizes the exact pipeline SHA-256 saved in new sessions.
Same-path and same-name matches are labeled unverified. Sessions marked
`pipeline_status: running` are excluded. Sessions from older binaries without a
terminal status are considered only after the file has been stable for at least
one minute and contains a terminal error or all expected snapshots plus final
output. An explicit in-flight session is rejected.

Outputs are `{stem}-alt-{N}.yaml` and `{stem}-alt-{N}.analysis.md`, where N is
one greater than the highest existing alternative for the family. Optimizing
an alternative continues the same family number sequence. The analysis report
contains evidence and a manual test plan; projected benefits are hypotheses.
Run the candidate manually with `induction --pipeline <alternative path>` and
use its completed sessions for the next iteration. See the
[sample analysis report](SAMPLE-PIPELINE-OPTIMIZER-ANALYSIS.md).

If no completed sessions match, the optimizer proceeds from pipeline and
source evidence and says so in the report. It fails if the source checkout is
missing, the candidate remains invalid after two repair attempts, the local
model is unavailable, or the output directory is unsafe or unwritable. A dry
run creates no output files. Session content and source comments are treated
as untrusted data; reports contain bounded metadata, not full transcripts.

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
