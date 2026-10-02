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
