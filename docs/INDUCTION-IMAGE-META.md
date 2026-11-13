# `induction-image-meta` appliance

> **Beta / work in progress:** This appliance is not a finished or stable
> feature. Interfaces, safeguards, model requirements, and behavior may
> change; validate it in an isolated environment before relying on it.

The image metadata appliance is a Linux-only, zero-argument executable built from the normal Induction codebase. It embeds the image-analysis workflow from `pipelines/pipeline.image-01.yaml` and its tested endpoint configuration; the source YAML files are not needed beside the executable at runtime.

## Build

From the repository root:

```bash
goreleaser build --config appliances/image-meta/.goreleaser.yaml --snapshot --clean
```

The binary is written under `dist/appliances/image-meta/`. A direct reproducible build is also supported:

```bash
go build -trimpath -ldflags='-s -w' -o induction-image-meta ./cmd/induction-image-meta
```

The embedded files are `appliances/image-meta/pipeline.yaml` and `config.yaml`. Change them only as a reviewed appliance change and rebuild the binary. The endpoint is compiled into the binary; there are no runtime config, flag, or environment overrides.

## Invocation and output

Run the binary from the directory containing images, with no arguments:

```bash
./induction-image-meta
```

Supported inputs are top-level regular `.jpg`, `.jpeg`, `.png`, and `.webp` files. Processing is deterministic by filename. Each successful image produces exactly `<original filename>.json`, for example `mountain.jpg.json`, containing the full twenty-field schema from the embedded `image-01` pipeline: descriptive strings, string arrays, and evidence-backed location, camera, film, and capture-date estimate objects.

```json
{
  "subjects": "string",
  "composition": "string",
  "environmental_context": "string",
  "title": "string",
  "caption": "string",
  "image_category": "string",
  "activities": [],
  "keywords": [],
  "visible_text": [],
  "distinctive_details": [],
  "lighting": "string",
  "color_palette": [],
  "photographic_technique": "string",
  "image_quality": "string",
  "editing_or_reproduction_clues": "string",
  "location_estimate": {},
  "camera_estimate": {},
  "film_estimate": {},
  "capture_date_estimate": {}
}
```

The only other permitted durable file is `induction-image-meta.log`, an append-only, human-readable JSONL audit/recovery journal. Every event written to that file is also emitted immediately to the console. Events include asset and Landlock preflight checks, endpoint/model readiness, discovery summaries, per-image state, every pipeline step start/completion with model, duration, and error details, publication, and the final run summary. Existing valid outputs are skipped byte-for-byte. Existing malformed, symlinked, hardlinked, or otherwise unknown outputs are preserved and reported as conflicts.

## Requirements and failure behavior

The appliance requires a reachable llama.cpp-compatible endpoint, the exact embedded models `Gemma-4-E4B-UNC-Q8_K_P`, `Granite-Vision-4.1-4B-Q4_K_M`, and `MiniCPM-V-4.7-35B-A3B-Q4_K_M`, verified image input capability for each, JSON output support, and Linux Landlock filesystem confinement with ABI 3 or newer. It fails before inference if any required preflight check fails. It does not use MCP, shell commands, sessions, snapshots, metrics exports, artifacts, caches, temporary directories, or ordinary Induction persistence.

Exit codes are: `0` success; `10` arguments rejected; `11` embedded assets invalid; `12` sandbox unavailable; `13` log/lock failure; `14` preflight/server/model failure; `15` image-level partial failure; `16` runtime integrity failure.

Landlock confines filesystem access to the startup working directory. The application additionally rejects symlink traversal and uses exclusive staging plus Linux `renameat2(RENAME_NOREPLACE)` publication. The guarantee does not protect against a hostile same-UID process modifying shared directory entries, nor does Landlock restrict network access to a malicious inference server. A crash orphan is left in place when ownership cannot be proven.

## Smoke test

Use a temporary directory containing two valid images and an unrelated file, copy in the binary, and run it twice. The first run must create two JSON files and one log; the second must preserve both JSON files byte-for-byte and only append log events. Add a third image to verify incremental processing. Replace one output with malformed JSON and verify it is not overwritten. Run `./induction-image-meta --help` and verify a nonzero exit with an `arguments_rejected` log event.
