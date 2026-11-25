# Application Tools

Application tools are local, read-only helpers exposed by Induction during
inference. They are managed by the host application rather than configured as
MCP servers or declared in pipeline YAML. Models can request them through the
normal tool-call loop, and the application returns the result before the model
continues.

List the currently registered tools with:

```bash
induction list applicationTools
```

The command prints each tool name and description in this form:

```text
tool_name: tool description
```

## Available tools

All tool names use lowercase `snake_case`.

| Tool | Purpose | Arguments |
| --- | --- | --- |
| `get_current_datetime` | Return the current system date and time and timezone. | None |
| `get_free_disk_space` | Return structured filesystem capacity and usage for `/`. | None |
| `get_available_memory` | Return structured memory and swap statistics. | None |
| `list_model_ids` | Return exact model IDs from the configured llama.cpp `/v1/models` endpoint. | None |
| `list_model_profiles` | Return enriched model routing profiles from `/v1/models`, including local paths and Hugging Face metadata. | None |
| `read_text_file` | Read a UTF-8 workspace file with a size limit. | `path`, optional `max_bytes` |
| `list_directory_contents` | List one workspace-relative directory level with metadata. | Optional `path` |
| `search_workspace` | Search workspace text with `rg`. | `query`, optional `path`, `glob`, `max_results` |
| `get_file_metadata` | Return file size, modification time, permissions, type, and path. | `path` |
| `read_json_file` | Read a JSON file, optionally returning a top-level property. | `path`, optional `property` |
| `validate_json` | Validate JSON content or a JSON file. | `content` or `path` |
| `hash_file` | Compute a file digest. | `path`, optional `algorithm` |
| `parse_go_ast` | Parse one or more Go files and return AST representations. | `file` or `files` |

Tool results are returned to the model as JSON text. The structured shapes are
as follows:

- `get_current_datetime`: object with `date_time` and `timezone`.
- `get_free_disk_space`: object with `filesystem`, `total_bytes`,
  `used_bytes`, `available_bytes`, `used_percent`, and `mount`.
- `get_available_memory`: object with memory and swap byte counters.
- `list_model_ids`: JSON array of strings.
- `list_model_profiles`: array of objects containing `id`, `family_slug`,
  `local_path`, `huggingface_url`, `huggingface_author`, `huggingface_repo`,
  `quantization_file`, `context_capacity`, `vision_supported`,
  `inferred_parameters`, and `architecture_flags`.
- `read_text_file`: object containing `path`, `content`, and `bytes`.
- `list_directory_contents`: array of entries containing `name`, `path`,
  `type`, `size`, `modified`, and `mode`.
- `search_workspace`: object containing `query`, `matches`, `count`, and
  `truncated`.
- `get_file_metadata`: object containing `path`, `size`, `modified`,
  `permissions`, and `type`.
- `read_json_file`: the complete JSON value or the selected top-level value.
- `validate_json`: object containing `valid`; invalid JSON also includes an
  error message and, when available, a byte offset.
- `hash_file`: object containing `path`, `algorithm`, and `digest`.
- `parse_go_ast`: array of objects containing `path` and `ast`.

## Workspace and safety boundaries

File-oriented tools accept workspace-relative paths. Absolute paths and paths
that escape the application workspace are rejected, including symlink paths
that resolve outside the workspace. File reads and AST parsing are read-only.

The default `read_text_file` limit is 1 MiB. Search results default to 100
matches unless `max_results` is supplied. Supported hash algorithms are
`sha256`, `sha512`, `sha1`, and `md5`.

The application tools do not perform writes, model loading, process control, or
arbitrary shell execution. `search_workspace` invokes the fixed `rg` command,
and the system resource tools invoke fixed `df` and `free` commands.

## Pipelines

Application-tool validation pipelines are available under
[`pipelines/`](../pipelines/), including one pipeline for each registered
tool. Their filenames follow this pattern:

```text
pipeline.application-tools.{application-tool-name}.yaml
```

Each pipeline calls its application tool, formats the result as a final JSON
object, and writes `final-output.json` under the run's
`.pipeline-artifacts/<run-id>/` directory.

Application tools remain available by default. Direct inference must use
`--usemcp` to opt into configured MCP servers; that does not disable these
host-managed local tools. Pipeline steps must set `usemcp: true` to opt into
configured MCP servers.
