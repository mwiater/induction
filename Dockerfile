# Build the checked-out release source with a pinned toolchain.
FROM golang:1.26.4 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/induction ./cmd/induction

# Clean runtime environment
FROM debian:bookworm-slim

# Install the checked-out Induction binary
COPY --from=builder /out/induction /usr/local/bin/induction

ENV COLORTERM=truecolor

# Create a clean working directory
WORKDIR /induction

# Copy runtime configuration and example pipelines
COPY induction.yaml ./induction.yaml
COPY pipelines/ ./pipelines/

ENTRYPOINT ["induction"]
CMD ["--help"]
