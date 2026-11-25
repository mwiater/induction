# `induction-image-meta` appliance

> **Beta / work in progress:** This appliance is not a finished or stable
> feature. Interfaces, safeguards, model requirements, and behavior may
> change; validate it in an isolated environment before relying on it.

The image metadata appliance is a Linux-only, zero-argument executable built from the normal Induction codebase. It embeds the three-step image-analysis workflow from `pipelines/pipeline.image-01.yaml`, simplified to the three required output fields, plus its tested endpoint configuration; the source YAML files are not needed beside the executable at runtime.

## Build

From the repository root:

```bash
goreleaser build --config appliances/image-meta/.goreleaser.yaml --snapshot --clean
```

The binary is written under `dist/appliances/image-meta/`. A direct reproducible build is also supported:

```bash
go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o induction-image-meta ./cmd/induction-image-meta
```

The embedded files are `appliances/image-meta/pipeline.yaml` and `config.yaml`. Change them only as a reviewed appliance change and rebuild the binary. The endpoint is compiled into the binary; there are no runtime config, flag, or environment overrides.

## Invocation and output

Run the binary from the directory containing images, with no arguments:

```bash
./induction-image-meta
```

Supported inputs are top-level regular `.jpg`, `.jpeg`, `.png`, and `.webp` files. Processing is deterministic by filename. Each successful image produces exactly `<original filename>.json`, for example `mountain.jpg.json`, containing the simplified three-field schema from the appliance pipeline.

```json
{
  "subjects": "string",
  "composition": "string",
  "environmental_context": "string"
}
```

The only other permitted durable file is `induction-image-meta.log`, an append-only JSONL audit/recovery journal. Every event written to that file is also emitted immediately to the console. Events include asset and Landlock preflight checks, endpoint/model readiness, discovery summaries, per-image state, every pipeline step start/completion with model, duration, and error details, publication, recovery, and the final run summary. Existing valid outputs are skipped byte-for-byte. Existing malformed, symlinked, hardlinked, or otherwise unknown outputs are preserved and reported as conflicts.

## Requirements and failure behavior

The appliance requires a reachable llama.cpp-compatible endpoint, the exact embedded models `Gemma-4-E4B-UNC-Q8_K_P`, `Granite-4.1-4B-Q4_K_M`, and `MiniCPM-4.7-35B-A3B-Q4_K_M`, verified image input capability for each, JSON output support, and Linux Landlock filesystem confinement with ABI 3 or newer. It fails before inference if any required preflight check fails. It does not use MCP, shell commands, sessions, snapshots, metrics exports, artifacts, caches, temporary directories, or ordinary Induction persistence.

Exit codes are: `0` success; `10` arguments rejected; `11` embedded assets invalid; `12` sandbox unavailable; `13` log/lock failure; `14` preflight/server/model failure; `15` image-level partial failure; `16` runtime integrity failure.

Landlock ABI 3 or newer confines filesystem access to the startup working directory; controlled file operations additionally use Linux `openat2` with `RESOLVE_BENEATH`, `RESOLVE_NO_SYMLINKS`, and `RESOLVE_NO_MAGICLINKS`. The publisher uses exclusive staging plus `renameat2(RENAME_NOREPLACE)` and directory fsync. Verified staging files are retired from the append-only journal on restart; files without a matching device/inode/size record remain in place and are logged as ambiguous. The guarantee does not protect against a hostile same-UID process modifying shared directory entries, nor does Landlock restrict network access to a malicious inference server.

## Smoke test

Use a temporary directory containing two valid images and an unrelated file, copy in the binary, and run it twice against an explicitly approved compatible llama.cpp server or a mock server configured at build time. Do not point the binary at an unapproved endpoint because image bytes are sent to the configured server. The first run must create two JSON files and one log; the second must preserve both JSON files byte-for-byte and only append log events. Add a third image to verify incremental processing. Replace one output with malformed JSON and verify it is not overwritten. Run `./induction-image-meta --help` and verify a nonzero exit with an `arguments_rejected` log event.
