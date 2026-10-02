# Build/install the publicly released Induction
FROM golang:latest AS builder

RUN go install github.com/mwiater/induction/cmd/induction@latest

# Clean runtime environment
FROM debian:bookworm-slim

# Install the globally accessible Induction binary
COPY --from=builder /go/bin/induction /usr/local/bin/induction

ENV COLORTERM=truecolor

# Create a clean working directory
WORKDIR /induction

# Copy runtime configuration and example pipelines
COPY induction.yaml ./induction.yaml
COPY pipelines/ ./pipelines/

ENTRYPOINT ["induction"]
CMD ["--help"]
