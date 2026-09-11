# Refactor Cleanup Implementation Guide

This guide covers the cleanup identified during the unused/duplicated-code review. Complete the work in small, testable changes so existing inference, MCP, session, dashboard, CLI, and model-manager behavior remains intact.

## Goals and compatibility rules

The implementation should:

- preserve all existing user-facing commands and successful inference flows;
- preserve public APIs unless a compatibility shim is retained;
- preserve sessions, snapshots, telemetry, dashboards, attachments, and model management;
- remove stale references without deleting supported fixtures;
- stop creating `application.log` and `induction-model-manager.log`;
- add regression tests before or alongside each cleanup.

Do not combine unrelated behavior changes with this cleanup. Keep each change small enough to revert independently.

## Recommended implementation order

### 1. Establish a clean baseline

Record the current results before editing:

```bash
GOCACHE=/tmp/induction-gocache go test ./... -run '^$'
GOCACHE=/tmp/induction-gocache go test ./internal/...
git status --short
git diff --stat
```

Run the full suite in an environment that permits `httptest` to bind a local listener. In restricted environments, API tests may fail before exercising application code because local socket creation is prohibited.

### 2. Fix stale pipeline references

Update the stale image-pipeline references at:

- `README.md:135`
- `DOCKER-BUILD-TESTING.md:88`

Use a tracked replacement such as `pipelines/pipeline.image-01.yaml` or `pipelines/pipeline.image-02.yaml`, depending on the intended example.

Update the obsolete command comment at `pipelines/pipeline.prompt-optimization.yaml:4`. It references the removed `./examples/infer` program and an ignored generated pipeline. Show the current CLI and a tracked pipeline instead.

Add a documentation/reference check:

```bash
rg -n 'examples/infer|pipeline\\.prompt-optimization-[0-9]+' \\
  README.md DOCKER-BUILD-TESTING.md pipelines
```

The check should find no stale references, or explicitly allow references that are intentionally historical.

### 3. Correct and consolidate MCP streaming behavior

`InferMCPStreamChat` in `mcp_modes.go` currently delegates to `InferMCPChatWithApproval`, although `mcp_stream.go` contains the streaming implementation. Point it at the streaming path while preserving its signature.

Also change `InferMCPStreamWithApproval` to use the option-aware configuration path (`loadConfigForOptions(options)`) rather than `LoadConfig()`, so `WithConfigPath` works consistently.

Add tests verifying:

- `InferMCPStreamChat` emits content incrementally;
- streamed tool-call fragments accumulate correctly;
- multiple tool-call indexes remain ordered;
- malformed arguments are rejected;
- read-only tools run automatically;
- side-effecting tools require approval;
- MCP tool errors retain existing behavior in both modes;
- custom configuration paths are honored.

The non-streaming and streaming MCP loops duplicate tool preparation, validation, approval, timeout, and result-message logic. After behavior is covered, extract shared helpers to:

1. convert bound tools into request tools and a name-to-binding map;
2. validate a model tool call and resolve its binding;
3. approve and execute one call;
4. append its tool result message.

Keep inference orchestration separate: non-streaming receives a full response, while streaming receives chunks and builds an assistant message. Shared helpers must not buffer streamed assistant content.

### 4. Remove redundant session-loading indirection

`loadChatSession` in `session_persistence.go` only forwards to `loadChatSessionFromPath`. Replace its single call site with the target helper, then remove the wrapper.

Retain tests for valid loading, malformed JSON, invalid IDs, invalid structure, and message cloning/isolation. Do not remove exported `LoadChatSession`.

### 5. Simplify command construction

`NewRootCommand` calls `newInspectCommands` twice. Construct the pair once and register the returned server and model commands in their respective locations.

Add CLI structure tests asserting that `inspect server`, `models inspect MODEL`, session inspection, and the `model-manager` alias still work, with no duplicate registered commands. This should not change command names, flags, aliases, or output.

### 6. Remove application file logging safely

Core logging is implemented in `logging.go` and configured through `LogConfig`. The requested `application.log` file is ignored by Git but is created when a file destination is selected.

Implement in this order:

1. Remove `application.log` from local workspaces and ensure it is not tracked.
2. Remove that filename from tests and documentation; use a temporary test path only when testing compatibility behavior.
3. Decide whether the public `LogConfig` and `NewConfiguredLogger` API must be retained. If external callers may use it, retain a shim that writes to stderr or `io.Discard` and never opens a file. Otherwise remove the API and its plumbing.
4. Remove file creation, truncation, append, and file `MultiWriter` behavior.
5. Preserve all non-logging behavior in model loading, telemetry, inference, UI, and session saving.
6. Update `Config.validate` and YAML examples so `log.path` is not required if file logging no longer exists.

Add tests proving that logger construction never creates or truncates `application.log`, inference still succeeds with diagnostics disabled, and CLI errors/output remain unchanged. If console diagnostics remain, verify they go only to the console and never create a file.

### 7. Remove model-manager interaction logging

The model-manager audit feature consists of:

- `internal/modelmanager/log.go`;
- `InteractionLogPath`, `LogInteraction`, and `recordInteraction`;
- call sites in `internal/cli/root.go` and `internal/modelmanager/model.go`;
- `internal/modelmanager/modelmanager_test.go:TestInteractionLog`.

Delete the generated root and internal log files, remove the implementation and call sites, and preserve command execution and model-manager results.

Replace `TestInteractionLog` with tests covering behavior formerly surrounding those calls: command completion, download/update error propagation, cancellation, and user-facing output. Then verify:

```bash
rg -n 'InteractionLogPath|LogInteraction|recordInteraction|induction-model-manager\\.log' \\
  --glob '*.go' --glob '*.md' .
```

Do not remove unrelated Docker build logging (`.docker-build.log`); it is a build-validation artifact, not one of the requested application logs.

### 8. Review other duplication candidates conservatively

Package-level `Chat`, `Complete`, `StreamChat`, `StreamComplete`, `CheckHealth`, `ListLoadedModels`, and `GenerateSnapshot` wrap client methods and are likely intentional public convenience APIs. Do not remove them without checking downstream consumers or retaining forwarding shims.

Likewise, keep attachment helpers and exported inspection/session APIs unless usage analysis confirms they are not supported library surface.

## Test and validation checklist

Run formatting and static checks:

```bash
gofmt -w <changed-go-files>
GOCACHE=/tmp/induction-gocache go test ./... -run '^$'
GOCACHE=/tmp/induction-gocache go test ./internal/...
go vet ./...
```

Run the full suite with local socket permissions and inspect coverage:

```bash
GOCACHE=/tmp/induction-gocache go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

Exercise the important CLI surfaces:

```bash
induction --help
induction list commands
induction inspect server
induction models inspect MODEL
induction sessions inspect --session .sessions/<session-id>.json
induction runtime status
induction dashboard generate
```

Finally verify cleanup and stale references:

```bash
test ! -e application.log
test ! -e induction-model-manager.log
test ! -e internal/modelmanager/induction-model-manager.log
rg -n 'examples/infer|InteractionLogPath|LogInteraction|recordInteraction' \\
  --hidden -g '!.git/**' -g '!dist/**' .
git diff --check
git status --short
```

Confirm that only intended source, test, documentation, and configuration changes remain, and that no generated log file is recreated by tests or CLI runs.
