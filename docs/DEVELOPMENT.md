# Development

Build and test the Linux AMD64 release artifacts with:

```bash
goreleaser release --snapshot --clean --skip=publish
```

The release configuration runs formatting, vet, the test suite, and the race
detector before producing the binary. For local development, use:

```bash
go run ./cmd/induction --help
```
