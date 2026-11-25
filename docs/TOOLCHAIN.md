# Go Toolchain

This guide documents the standalone Go tools used to lint, inspect, refactor,
test, and profile Induction. Run commands from the repository root.

## Installation

Go 1.26.4 or newer is required to build Induction. Install the requested tools
into the standard Go binary directory:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/deadcode@latest
go install golang.org/x/tools/cmd/gorename@latest
```

`staticcheck` and `go-critic` are run through `golangci-lint`, so they do not
need separate installations. Verify the binaries and your `PATH` with:

```bash
go env GOPATH GOBIN
command -v golangci-lint deadcode gorename
golangci-lint version
```

Example output:

```text
/home/matt/go

/home/matt/go/bin/golangci-lint
/home/matt/go/bin/deadcode
/home/matt/go/bin/gorename
golangci-lint has version v1.64.8 built with go1.26.5
```

If a command is not found, add `$(go env GOPATH)/bin` to `PATH`, or set
`GOBIN` to a directory already on `PATH` before running `go install`.

## Linting

The repository's [`.golangci.yml`](../.golangci.yml) runs these linters
concurrently:

| Linter | Purpose |
| --- | --- |
| `staticcheck` | SSA-based correctness and simplification checks. |
| `gocritic` | Performance, diagnostic, and style checks. |
| `errcheck` | Unchecked error handling. |
| `revive` | Fast, configurable Go style checks. |

Run the complete configured analysis:

```bash
golangci-lint run ./...
```

```bash
golangci-lint run ./... > .local/LINTING_ISSUES.txt
```

A clean run produces no diagnostic output and exits with status 0. Findings
include the source location, rule, and message, for example:

```text
internal/appliance/appliance_test.go:87:16: Error return value of `os.Chdir` is not checked (errcheck)
internal/core/api.go:174:14: http.NoBody should be preferred to the nil request body (gocritic)
```

Fix the code, or make a narrowly scoped configuration exception when a finding
is intentional. Keep the enabled linter set small and actionable.

## Dead-code analysis

`deadcode` follows the execution graph and reports unreachable functions and
types. Analyze the Induction CLI entry point with:

```bash
deadcode ./cmd/induction
```

Or, to generate a file:

```bash
deadcode ./cmd/induction > .local/DEADCODE.txt
```

Example output:

```text
github.com/mwiater/induction/internal/core.unusedHelper
github.com/mwiater/induction/internal/cli.legacyCommand
```

Treat results as removal candidates. Check build tags, generated code,
reflection, plugin entry points, and public API usage before deleting code.

## Cross-package renaming

`gorename` performs type-aware renames across packages. Inspect the blast
radius with `-d` first. For example:

```bash
gorename -d \
  -from '"github.com/mwiater/induction/internal/core".Client' \
  -to 'InferenceClient'
```

Example dry-run output:

```text
Would rename github.com/mwiater/induction/internal/core.Client to InferenceClient
  internal/core/client.go:42
  internal/core/client_test.go:18
  internal/cli/inference.go:27
```

After reviewing the affected files, execute the rename without `-dryrun`:

```bash
gorename \
  -from '"github.com/mwiater/induction/internal/core".Client' \
  -to 'InferenceClient'
```

Run formatting, tests, and linting after a rename.

## Custom AST tooling

Place repository-specific source analysis or reorganization programs under
`tools/` so they stay separate from the main application binary. This example
parses a Go file with comments preserved and reports top-level declarations:

```go
// tools/reorganizer/main.go
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
)

func main() {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "internal/core/pipeline.go", nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			fmt.Println("general declaration:", d.Tok)
		case *ast.FuncDecl:
			fmt.Println("function:", d.Name.Name)
		}
	}
}
```

Run it with:

```bash
go run ./tools/reorganizer
```

Example output:

```text
general declaration: import
function: RunPipelineItem
function: RunPipelineItemWithObserver
```

Use `go/format` or `go/printer` when writing modified syntax trees back to
disk, and add tests for transformations that change source files.

## Verification workflow

For a normal change or refactor, format changed Go files, run the test suite,
and then run the configured linter:

```bash
gofmt -w path/to/changed.go
go test ./...
golangci-lint run ./...
```

To generate and inspect the complete coverage profile, use:

```bash
go test ./... \
  -coverpkg=./... \
  -covermode=atomic \
  -coverprofile=coverage.out
```

```go
go tool cover -func=coverage.out
go tool cover -html=coverage.out
```

Example successful test output:

```text
?    github.com/mwiater/induction/cmd/induction    [no test files]
ok   github.com/mwiater/induction/internal/core  0.233s
ok   github.com/mwiater/induction/internal/cli   0.024s
```

Run `deadcode` and `gorename` when the change calls for them; they are
maintenance tools rather than required steps for every build.
