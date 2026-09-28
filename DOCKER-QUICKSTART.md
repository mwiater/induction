# Docker Quickstart

This is the preferred way to build and explore Induction. It gives you a small
runtime image containing the application, pipeline examples, and fixture data,
while keeping the Go toolchain and build dependencies out of the final image.

Before starting, make sure a reachable llama.cpp-compatible server is available
and create an `induction.yaml` configured with its endpoint. The commands below
are run from the repository root.

## 1. Build the runtime image

The Dockerfile uses a multi-stage build. Only the stripped Induction binary, CA
certificates, pipeline files, and fixture data are copied into the final Alpine
runtime image.

```bash
set -o pipefail
docker build --progress=plain -t induction . 2>&1 | tee .docker-build.log
```

The runtime image does not contain a shell. Mount the local configuration file
when starting the container:

```bash
docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction --help
```

The image sets `COLORTERM=truecolor`, so Bubble Tea uses truecolor rendering
inside the container.

## 2. Verify the server and runtime

The image build itself does not contact the inference server. Validate the
runtime connection after starting the container:

```bash
docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction server inspect --json

docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction runtime status
```

## 3. Explore models and local assets

The image includes the repository fixtures and pipeline examples:

```bash
docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction models list --loaded --beta

docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction pdf preview --file data/fixtures/documents/fixture-01.pdf
```

## 4. Run inference and pipeline examples

All inference commands use the Bubble Tea UI. Run them with `-it` and keep the
terminal attached.

```bash
docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction --model "GLM-4.7-Flash-Q4_K_M"

docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction --model "Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL" \
  --image data/fixtures/images/fixture-01.jpg

docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction --model "Qwen-3.5-9B-MTP-General-Q8_0" \
  --document data/fixtures/documents/fixture-01.pdf \
  --userPrompt "Summarize this document." --autosubmit

docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction --pipeline pipelines/pipeline.prompt-optimization-01.yaml

docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction --pipeline pipelines/pipeline.image-01.yaml

docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction --pipeline pipelines/pipeline.document-01.yaml

docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  induction --pipeline pipelines/pipeline.batch-image-analysis.yaml
```

The container writes diagnostics to `/app/induction.log`. To persist the log,
create and mount a host file:

```bash
mkdir -p container-data
touch container-data/induction.log
docker run -it --rm \
  -v "$PWD/induction.yaml:/app/induction.yaml:ro" \
  -v "$PWD/container-data/induction.log:/app/induction.log" \
  induction --help
```

## 5. Clean up

Containers started with `--rm` are removed automatically. Remove the image when
you are finished:

```bash
docker rmi induction:latest
```
