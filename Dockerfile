FROM golang:1.26.4-alpine

# Preserve truecolor terminal rendering for interactive sessions.
ENV COLORTERM=truecolor

# Initialize the log file
RUN mkdir -p /test && touch /test/.docker-build.log

# 1. Install prerequisites (including jq) and log versions
RUN set -o pipefail && \
    apk add --no-cache git build-base curl bash jq 2>&1 | tee -a /test/.docker-build.log && \
    echo "Success: Installed Alpine dependencies" >> /test/.docker-build.log && \
    { echo -n "Go version: "; go version; } 2>&1 | tee -a /test/.docker-build.log && \
    { echo -n "jq version: "; jq --version; } 2>&1 | tee -a /test/.docker-build.log

# Install a GoReleaser version compatible with Go 1.26.4 and log version
RUN set -o pipefail && \
    go install github.com/goreleaser/goreleaser/v2@v2.16.0 2>&1 | tee -a /test/.docker-build.log && \
    echo "Success: Installed GoReleaser" >> /test/.docker-build.log
ENV PATH="/root/go/bin:$PATH"
RUN set -o pipefail && \
    { echo -n "GoReleaser version: "; goreleaser --version; } 2>&1 | tee -a /test/.docker-build.log

# 2. Copy the current repository into the build environment. This ensures the
# Docker image contains the same binary sources, fixtures, and pipelines as
# the checkout from which docker build was invoked.
COPY . /src
RUN echo "Success: Copied current Induction repository" >> /test/.docker-build.log

WORKDIR /src

# 3. Build the binary using GoReleaser
RUN set -o pipefail && \
    goreleaser release --snapshot --clean --skip=publish 2>&1 | tee -a /test/.docker-build.log && \
    echo "Success: Built binary via goreleaser" >> /test/.docker-build.log

# 4. Create the test environment
RUN set -o pipefail && \
    mkdir -p /test/dist/induction_linux_amd64_v1 && \
    cp dist/induction_linux_amd64_v1/induction /test/dist/induction_linux_amd64_v1/induction 2>&1 | tee -a /test/.docker-build.log && \
    cp dist/induction_linux_amd64_v1/induction /test/induction 2>&1 | tee -a /test/.docker-build.log && \
    install -m 0755 dist/induction_linux_amd64_v1/induction /usr/local/bin/induction 2>&1 | tee -a /test/.docker-build.log && \
    cp -r data /test/ 2>&1 | tee -a /test/.docker-build.log && \
    cp -r pipelines /test/ 2>&1 | tee -a /test/.docker-build.log && \
    echo "Success: Copied versioned binary, PATH binary, fixtures, and pipelines" >> /test/.docker-build.log

WORKDIR /test

# 5. Generate the induction.yaml configuration file
RUN echo -e "server: http://192.168.0.239:9998\ntimeout: 20m\npoll_interval: 2s\nload_wait_interval: 1s\nenableLiveMetricsOverlay: true\nsidebarWidth: 32\nlog:\n  path: induction.log\n  console: true\n  prefix: \"app: \"\n  microseconds: true" > induction.yaml && \
    echo "Success: Generated induction.yaml" >> /test/.docker-build.log

# 6. BUILD VALIDATION GATES & LOGGING
RUN set -o pipefail && \
    echo -e "\n--- Validation: inspect server ---" | tee -a .docker-build.log && \
    ./induction server inspect 2>&1 | tee -a .docker-build.log && \
    echo -e "\n--- Validation: runtime status ---" | tee -a .docker-build.log && \
    induction runtime status 2>&1 | tee -a .docker-build.log && \
    echo "Success: All validation gates passed" >> .docker-build.log

# 7. PRINT THE LOG TO CONSOLE (also captured on the host by the documented build command)
RUN cat .docker-build.log

CMD ["/bin/bash"]
