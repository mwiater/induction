## 1. Build and Enter the Container

* **Build the image:** Execute the build command with plain progress. `tee` saves the complete Docker build output, including failures, to `.docker-build.log` in the repository root. The image also contains the in-container copy at `/test/.docker-build.log`.


```bash
set -o pipefail
docker build --progress=plain -t induction-test . 2>&1 | tee .docker-build.log
```


* **Launch the interactive session:** Start the container with automatic cleanup enabled so it doesn't leave remnants on your system after testing.
```bash
docker run -it --rm induction-test
```

The image sets `COLORTERM=truecolor`, so the terminal UI uses truecolor inside
the container automatically.

Remove the image:
```bash
docker rmi induction-test:latest
```

## 2. Validate Server and Runtime Connection

* **List all commands:** View every available command and subcommand with their descriptions.


```bash
./induction list commands

```


* **Inspect server health:** Verify that Induction can read the generated `induction.yaml` and communicate with your llama.cpp backend.


```bash
./induction server inspect --json

```


* **Check runtime status:** View the currently loaded model and slot availability on your server.


```bash
./induction runtime status

```



## 3. Test Models

* **List loaded models:** Check which models are currently loaded in the server memory.


```bash
./induction models list --loaded --beta
```

* **Preview local PDF text:** Confirm PDF extraction without contacting the server.

```bash
./induction pdf preview --file data/fixtures/documents/fixture.pdf
```



## 4. Execute Inference Examples and Pipelines

The container includes both the repository-relative binary path and a
PATH-installed `induction` command. It also includes the README-referenced
fixtures under `data/fixtures/` and pipeline files under `pipelines/`.

* **Run interactive chat:**


```bash
induction --model "GLM-4.7-Flash-Q4_K_M"

```

The following README inference commands can be run unchanged inside the
container from `/test`:

```bash
dist/induction_linux_amd64_v1/induction --model "GLM-4.7-Flash-Q4_K_M"
dist/induction_linux_amd64_v1/induction --model "Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL" --image data/fixtures/images/fixture.jpg
dist/induction_linux_amd64_v1/induction --model "Qwen-3.5-9B-MTP-General-Q8_0" --document data/fixtures/documents/fixture.pdf --userPrompt "Summarize this document." --autosubmit
dist/induction_linux_amd64_v1/induction --pipeline pipelines/pipeline.prompt-optimization-01.yaml
dist/induction_linux_amd64_v1/induction --pipeline pipelines/pipeline.image-01.yaml
dist/induction_linux_amd64_v1/induction --pipeline pipelines/pipeline.document-01.yaml
induction --model "Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL" --image data/fixtures/images/fixture.jpg
induction --model "Qwen-3.5-9B-MTP-General-Q8_0" --document data/fixtures/documents/fixture.pdf --userPrompt "Summarize this document." --autosubmit
induction --pipeline pipelines/pipeline.prompt-optimization-01.yaml

```

* **Teardown:** When finished, simply type `exit`. The container will automatically delete itself.
