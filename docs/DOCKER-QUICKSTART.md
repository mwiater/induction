# Docker Quickstart

This is the preferred way to install and explore the public Induction
release. The root `Dockerfile` installs the latest published Induction binary
in a clean Debian runtime image and copies the repository configuration and
pipeline examples into `/induction`.

The internal source-build image is preserved as [`Dockerfile.local`](../Dockerfile.local)
and is not used by this guide.

Before starting, create an `induction.yaml` configured with a reachable
llama.cpp-compatible server. Build commands below run from the repository root.

## 1. Build the clean runtime image

Build the image using the root `Dockerfile`:

```bash
set -o pipefail
docker build --progress=plain --no-cache -t induction . 2>&1 | tee .docker-build.log
```

The build installs the latest published binary, copies `induction.yaml` and
the complete `pipelines/` directory into `/induction`, and starts with
`induction --help` when no command is supplied.

## 2. Verify the clean runtime

Open a shell in the Debian runtime and confirm the working directory,
configuration, pipelines, and installed binary:

```bash
docker run --rm -it induction bash
```

Inside the container, validate:

```bash
pwd
ls -la
which induction
induction --help
```

Expected values include `/induction` as the working directory,
`induction.yaml` and `pipelines/` in the directory listing, and
`/usr/local/bin/induction` as the binary path.

## 3. Verify the server and runtime

The image does not contact the inference server during the build. The
configured `induction.yaml` is already inside the image, so commands do not
need a configuration bind mount:

```bash
docker run -it --rm \
  induction server inspect --json

docker run -it --rm \
  induction runtime status
```

## 4. Explore models and local assets

The image includes the configured pipeline files but does not copy the
repository's fixture data. Mount `data/` at the same path expected by the
pipeline fixtures when using document or image assets:

```bash
docker run -it --rm \
  induction models list --loaded --beta

docker run -it --rm \
  -v "$PWD/data:/induction/data:ro" \
  induction pdf preview --file /induction/data/fixtures/documents/fixture-01.pdf
```

## 5. Run inference and pipeline examples

All inference commands use the Bubble Tea UI. Run them with `-it` and keep the
terminal attached. Configuration and pipeline files are loaded from the image;
mount `data/` for commands that use repository fixtures.

```bash
# Interactive text chat.
docker run -it --rm \
  induction --model "GLM-4.7-Flash-Q4_K_M"

# Image inference using repository fixture data.
docker run -it --rm \
  -v "$PWD/data:/induction/data:ro" \
  induction --model "Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL" \
  --image /induction/data/fixtures/images/fixture-01.jpg

# Unattended document question-answering.
docker run -it --rm \
  -v "$PWD/data:/induction/data:ro" \
  induction --model "Qwen-3.5-9B-MTP-General-Q8_0" \
  --document /induction/data/fixtures/documents/fixture-01.pdf \
  --userPrompt "Summarize this document." --autosubmit

# Prompt-optimization pipeline.
docker run -it --rm \
  induction --pipeline pipelines/pipeline.prompt-optimization-01.yaml

# Single-image analysis pipeline.
docker run -it --rm \
  -v "$PWD/data:/induction/data:ro" \
  induction --pipeline pipelines/pipeline.image-01.yaml

# Single-document analysis pipeline.
docker run -it --rm \
  -v "$PWD/data:/induction/data:ro" \
  induction --pipeline pipelines/pipeline.document-01.yaml

# Batch image-analysis pipeline.
docker run -it --rm \
  -v "$PWD/data:/induction/data:ro" \
  induction --pipeline pipelines/pipeline.batch-image-analysis-01.yaml
```

The complete categorized inventory is available in
[EXAMPLES.md](EXAMPLES.md), with detailed pages for each retained
pipeline example.

## 6. Persist logs

The container writes diagnostics to `/induction/induction.log`. To persist the
log on the host, create and mount a host file:

```bash
mkdir -p container-data
touch container-data/induction.log
docker run -it --rm \
  -v "$PWD/container-data/induction.log:/induction/induction.log" \
  induction --help
```

## 7. Clean up

Containers started with `--rm` are removed automatically. Remove the image when
you are finished:

```bash
docker rmi induction:latest
```
