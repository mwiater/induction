# Development

Use this guide for local builds, tests, and release verification. The project
uses Go tooling for development and GoReleaser for the Linux AMD64 release
artifact.

Build and test the Linux AMD64 release artifacts with:

```bash
goreleaser release --snapshot --clean --skip=publish
```

The release configuration runs formatting, vet, the test suite, and the race
detector before producing the binary. For local development, use:

```bash
go run ./cmd/induction --help
```

For the complete command and flag surface, see the [CLI reference](CLI-REFERENCE.md).

## Documentation maintenance

Exported Go declarations are part of the public API and must have GoDoc-style
comments that begin with the declaration name. Keep comments beside the code
they describe, and update the focused guides when a command, configuration
field, persistence path, or pipeline behavior changes. The CLI reference must
include every command exposed by `induction list commands`; the checked-in
examples should continue to point at existing pipeline files.

Before submitting documentation or API changes, run:

```bash
gofmt -w $(git diff --name-only -- '*.go')
go vet ./...
go test ./...
```
