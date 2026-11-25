# Induction — Remaining Implementations Needed

This document consolidates the release audit and verification report into one
working list. It records what is complete, what remains, why it remains, and
the evidence needed to close each item.

## Current release position

**Verdict:** READY WITH NON-BLOCKING CAVEATS

**Baseline:** branch `development`, commit `de09532`, module
`github.com/mwiater/induction`, declared Go `1.26.4`, verified on Linux AMD64
with Go `1.26.5`.

No P0 or P1 release blockers are currently known. The current worktree includes
user-owned model redistribution changes in `internal/cli/`; those changes were
preserved and verified with focused tests, vet, build, and formatting checks.

## Completed work

- Updated `scripts/dashboard-browser-check.mjs` to the current ten-view
  dashboard and selectors.
- Updated the README dashboard view list.
- Added GoDoc comments to the current non-test model redistribution
  implementation without changing runtime behavior.
- Applied behavior-preserving lint cleanups for local variable shadowing,
  regexp portability, octal literals, and test-only findings.
- Pinned the Docker builder toolchain and changed the image to build the
  checked-out source, removing the floating `@latest` install path.
- Added focused model-manager failure-path coverage and verified the
  redistribution command package.
- Verified the repository gates: full tests, race tests, vet, build, module
  verification, GoReleaser configuration and Linux AMD64 snapshot, Docker
  build, dashboard generation, and `git diff --check`.

## Remaining implementations

### R01 — Resolve configured golangci-lint findings

**Priority:** P2

`golangci-lint run` is not yet clean. The easy, behavior-preserving findings
are addressed. Remaining findings are primarily production `gocritic` style
warnings such as large value parameters, copied range values, and conditional
style suggestions.

Required implementation:

1. Triage the remaining findings against actual behavior and API compatibility.
2. Fix production findings only when behavior-preserving and supported by
   focused tests; do not change public signatures merely to silence a warning.
3. Re-run lint, the full test suite, vet, and race checks.

### R02 — Expand high-value CLI and model-manager coverage

**Priority:** P2

Aggregate repository coverage is 53.9%. The CLI package is now 34.0% with the
current redistribution test, while several CLI and model-manager entry points
remain at low or zero coverage.

Add deterministic tests for invalid CLI/configuration input, no-server
behavior, model-manager cancellation, token handling, safe destinations, and
persistence failure paths where they are not already covered. Download HTTP
failure and artifact cleanup paths now have focused coverage.
Do not add tests solely to increase a global percentage.

```bash
GOCACHE=/tmp/induction-gocache go test -count=1 ./...
GOCACHE=/tmp/induction-gocache go test -race ./...
GOCACHE=/tmp/induction-gocache go test -coverprofile=/tmp/induction-coverage-final.out ./...
GOCACHE=/tmp/induction-gocache go tool cover -func=/tmp/induction-coverage-final.out
```

### R03 — Run static and vulnerability analyzers

**Priority:** P2 / tooling prerequisite

`staticcheck` and `govulncheck` were not installed in the audit environment.
Run them in CI or a provisioned release environment, record versions and
advisory-data availability, and triage every finding.

```bash
staticcheck ./...
govulncheck ./...
```

Unavailable advisory data must not be reported as a clean security result.

### R04 — Run the browser regression suite

**Priority:** P2 / tooling prerequisite

The browser script is syntactically valid and targets the current dashboard,
but Playwright and Chromium were unavailable locally.

```bash
node scripts/dashboard-browser-check.mjs \
  data/dashboard/dashboard.html \
  /tmp/induction-dashboard-screenshots
```

The run must pass both themes, all configured widths and current views,
overflow checks, console-error checks, scenario controls, recommendations,
Fleet groups, sortable Models behavior, Intelligence layout, inspector access,
Variables isolation, and theme persistence.

### R05 — Make Docker builds reproducible

**Priority:** P2

The Dockerfile now pins the Go builder image to `golang:1.26.4` and builds the
checked-out source instead of installing a floating public `@latest` binary.
The remaining work is to confirm this packaging path in the release CI matrix
and decide whether the Debian runtime image also needs a digest pin.

### R06 — Complete fresh-checkout and repository-wide GoDoc audit

**Priority:** P2

The current redistribution file has name-prefixed GoDoc, but a full repository
comment audit remains. Existing generic comments and inline comments require
package-by-package review; broad automated rewriting would be risky.

Audit all tracked non-test Go package comments, significant declarations, and
inline comments. Relocate important business or safety context into accurate
symbol documentation, remove only redundant body comments, do not touch test
files, and verify the result from a disposable fresh checkout without local
sessions, ignored configuration, generated dashboard output, or private model
data.

## Compatibility rules

- Preserve CLI flags, commands, exit behavior, config defaults, pipeline
  semantics, JSON/session shapes, dashboard contracts, and filesystem paths.
- Do not modify `.sessions/`, private runtime data, user configuration, model
  files, or generated local artifacts during verification.
- Add a regression test before changing behavior-sensitive implementation code.
- Prefer the smallest compatible fix; do not refactor public APIs for style.
- Do not tag, publish, force-push, rewrite history, or claim skipped checks
  passed.

## Verification matrix

| Area | Result |
| --- | --- |
| Full deterministic Go tests | PASS |
| Race tests | PASS |
| Vet, build, module verification | PASS |
| GoReleaser configuration and Linux AMD64 snapshot | PASS |
| Docker build from pinned checked-out source | PASS after the Dockerfile change |
| Dashboard generation and focused dashboard tests | PASS |
| CLI help and invalid flag behavior | PASS |
| Model redistribution focused tests | PASS |
| `golangci-lint` | Findings remain; see R01 |
| Staticcheck and govulncheck | Not installed; see R03 |
| Playwright/Chromium browser run | Not installed; see R04 |
| Fresh-checkout simulation | Not completed; see R06 |

