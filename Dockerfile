# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.4
ARG TARGETOS=linux
ARG TARGETARCH=amd64

# Build stage. GoReleaser and the repository's development tools are not
# carried into the runtime image.
FROM golang:${GO_VERSION}-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/induction ./cmd/induction

# Small runtime image. The inference server and model files remain external;
# this image contains only Induction and the repository assets used by its
# example pipelines.
FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates

ENV COLORTERM=truecolor
WORKDIR /app

COPY --from=builder /out/induction /usr/local/bin/induction
COPY data /app/data
COPY pipelines /app/pipelines
COPY induction.example.yaml /app/induction.yaml

ENTRYPOINT ["/usr/local/bin/induction"]
