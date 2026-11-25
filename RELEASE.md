# Induction release guide

This document describes the supported release process for Induction. Releases are
driven by Git tags, built with GoReleaser, and published automatically via
GitHub Actions. The repository's [.goreleaser.yaml](.goreleaser.yaml) produces a
static Linux AMD64 `induction` binary, packages release archives and checksums,
and runs these checks before building:

- `go vet ./...`
- `go test ./...`
- `go test -race ./...`

The module currently requires Go 1.26.4 or newer. Run all release commands from
the repository root on a clean checkout.

## Repository configuration

Ensure both `.goreleaser.yaml` and `.github/workflows/release.yml` are committed
to the repository before tagging a release.

### GoReleaser configuration (`.goreleaser.yaml`)

```yaml
version: 2

project_name: induction

env:
  - GOCACHE={{ if index .Env "GOCACHE" }}{{ .Env.GOCACHE }}{{ else }}/tmp/induction-gocache{{ end }}

before:
  hooks:
    - go vet ./...
    - go test ./...
    - go test -race ./...

builds:
  - id: induction
    main: ./cmd/induction
    binary: induction
    env:
      - CGO_ENABLED=0
    goos:
      - linux
    goarch:
      - amd64
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - id: default
    formats: ['tar.gz']
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    files:
      - LICENSE*
      - README*

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'

release:
  github:
    owner: mwiater
    name: induction
  draft: false
  prerelease: auto

```

### GitHub Actions workflow (`.github/workflows/release.yml`)

```yaml
name: release

on:
  push:
    tags:
      - 'v*.*.*'

permissions:
  contents: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - name: Check out the tagged commit
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Release with GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

```

Key workflow behaviors:

* **`fetch-depth: 0`:** Required so GoReleaser can inspect the full Git tag
and commit history to generate release notes and version metadata.
* **`v*.*.*` tag filter:** Prevents ordinary branch pushes from triggering a
release while matching both standard (`v0.1.0`) and pre-release
(`v0.2.0-rc.1`) semantic tags.
* **`contents: write`:** Grants the built-in `GITHUB_TOKEN` permission to
create the GitHub release and upload build assets without broader repository
permissions.
* **`go-version-file: go.mod`:** Keeps the CI Go toolchain synchronized with
the module's declared Go version automatically.

## Version and tag rules

Use semantic versions with a leading `v` in Git tags:

```text
v0.1.0
v0.1.1
v0.2.0
v1.0.0

```

The `v` is part of the Git tag and is the convention GoReleaser expects. The
release version is the semantic version without the `v` prefix. The first
release should be `v0.1.0`, not `v0.1` or `0.1.0`.

Follow the usual semantic-versioning increments:

* Patch (`0.1.0` -> `0.1.1`): backwards-compatible bug fixes.
* Minor (`0.1.0` -> `0.2.0`): backwards-compatible features or behavior.
* Major (`0.x.y` -> `1.0.0`): the first stable API, or later breaking changes.

Do not reuse, move, or force-push a release tag. A published tag must always
refer to the same commit and artifacts. If a release is wrong, fix the source
and publish the next version.

## 1) Initial release

The initial release establishes the repository configuration, creates the first
immutable version tag, and publishes the `v0.1.0` GitHub release.

### 1.1 Confirm the release configuration

Install GoReleaser using the official installation method, or make sure an
already-installed binary is a current GoReleaser v2 release. Then validate the
configuration:

```bash
goreleaser --version
goreleaser check

```

Confirm the following before tagging:

1. `.goreleaser.yaml` has `version: 2`, `project_name: induction`, and
`builds.main: ./cmd/induction`.
2. `.github/workflows/release.yml` is present and valid.
3. The target list is intentional (currently Linux AMD64 only).
4. The pre-release hooks pass on the exact code to be released.

Run the repository checks directly as a quick failure check:

```bash
go vet ./...
go test ./...
go test -race ./...

```

### 1.2 Build a local snapshot

Before publishing anything, build the same configuration without creating a
GitHub release or uploading artifacts:

```bash
goreleaser release --snapshot --clean --skip=publish

```

This writes local artifacts under `dist/`. Inspect the output and verify the
archive contains the expected `induction` executable and `checksums.txt`. A
snapshot is a build validation artifact; it is not a release and must not be
given the production `v0.1.0` name. Remove `dist/` or ensure it is ignored by
Git before committing.

If the machine has a read-only or unusual Go build cache, the repository config
uses `/tmp/induction-gocache` by default. An explicit `GOCACHE` value is still
honored:

```bash
GOCACHE="$PWD/.cache/gocache" goreleaser release --snapshot --clean --skip=publish

```

### 1.3 Prepare and push the first release commit

Make sure all release-related files are committed, including `.goreleaser.yaml`,
`.github/workflows/release.yml`, and this guide. The release commit must be
pushed to `main` before tagging so the remote branch and tag point to the same
commit.

```bash
git switch main
git status
git diff --check
git log -1 --oneline
git push origin main

```

If the project uses a protected `main` branch, merge the release-ready change
through a pull request first, then pull `main` locally before tagging. Do not
tag a commit that has not been pushed to `main`.

### 1.4 Create and push `v0.1.0`

Create an annotated tag at the pushed `main` commit:

```bash
git switch main
git pull --ff-only origin main
git tag -a v0.1.0 -m "Release v0.1.0"
git show --stat --oneline v0.1.0
git push origin v0.1.0

```

Pushing `v0.1.0` automatically triggers the GitHub Actions release workflow.
Verify that the tag points at the expected commit both locally and on the
remote, and monitor the **Actions** tab on GitHub until the `release` workflow
completes:

```bash
git rev-parse v0.1.0^{commit}
git ls-remote --tags origin refs/tags/v0.1.0

```

### 1.5 Fallback: Manual local publish

If GitHub Actions is unavailable and you must publish locally, export a GitHub
token with `Contents: write` access to `mwiater/induction`. Do not put the
token in a command, commit, shell history, or this document:

```bash
export GITHUB_TOKEN='...'
goreleaser release --clean
unset GITHUB_TOKEN

```

Run this only after pushing `main` and `v0.1.0`. GoReleaser reads the latest Git
tag to derive the release version and publishes the artifacts to GitHub. If
the release already exists, do not rerun with a changed tag or force-push the
tag; inspect the failed run and correct the underlying issue.

### 1.6 Verify the first release

A green GitHub Actions workflow is not a substitute for testing the published
artifacts. On the GitHub release page, verify all of the following:

* The release is named and tagged `v0.1.0`.
* The release commit is the intended commit on `main`.
* The Linux AMD64 archive (`induction_0.1.0_linux_amd64.tar.gz`) and
`checksums.txt` are present.
* The checksum matches the downloaded archive:
```bash
sha256sum --check checksums.txt --ignore-missing

```


* The archive extracts to an executable named `induction`, and the binary
starts and reports its help successfully:
```bash
./induction --help

```



Record the release URL and test the documented download path before announcing
the release. Docker builds and documentation that consume the latest public
release should be checked separately because they are downstream of the GitHub
release artifact.

## 2) Subsequent release instructions

Subsequent releases should be sequential, reproducible, and tag-triggered.
Each release is a new commit and a new immutable semantic-version tag.

### 2.1 Standard automated release process

For every release, start from the default branch and update it before deciding
the next version:

```bash
git switch main
git pull --ff-only origin main
git fetch --tags origin
git tag --list 'v*' --sort=-version:refname | head

```

Choose the next version from the highest existing stable tag. For example:

* `v0.1.0` -> `v0.1.1` for a patch release.
* `v0.1.1` -> `v0.2.0` for a backwards-compatible feature release.
* `v0.9.4` -> `v1.0.0` for the first stable major release.

Then execute this sequence:

1. Merge the implementation, tests, documentation, and workflow/config updates
into `main`, and ensure `main` is pushed to `origin`:
```bash
git switch main
git push origin main

```


2. Validate locally before tagging:
```bash
goreleaser check
goreleaser release --snapshot --clean --skip=publish

```


3. Confirm the working tree is clean and local `main` matches `origin/main`
(`git status` reports `Your branch is up to date with 'origin/main'`).
4. Create and push one annotated tag, for example:
```bash
git tag -a v0.1.1 -m "Release v0.1.1"
git push origin v0.1.1

```


5. Monitor the triggered GitHub Actions `release` workflow until it succeeds.
6. Download and verify the GitHub release archive, `checksums.txt`, and
`induction` executable.
7. Announce the release using the exact tag and release URL.

Never create `v0.1.2` from a stale checkout just because `v0.1.1` failed. Fix
the release problem first, then decide whether the same tag can be safely
re-run. If the tag or GitHub release has already been published, use the next
patch version rather than changing history.

### 2.2 Pre-releases and automated version selection

Do not use a moving tag such as `latest` as the immutable release identifier.
For a pre-release, push a semver-compatible tag such as `v0.2.0-rc.1` from a
pushed commit. Because `.goreleaser.yaml` sets `prerelease: auto` and
`.github/workflows/release.yml` triggers on `v*.*.*`, GitHub Actions will build
and mark the GitHub release as a pre-release automatically. Promote the final
release with a new stable tag (for example `v0.2.0`); do not retag the release
candidate.

If a separate release tool calculates versions from commits, it must still
ensure the target commit is pushed and create one annotated `vX.Y.Z` tag on that
exact commit. The tag remains the source of truth for GoReleaser. Do not
combine a moving branch build, a manually edited version string, and a Git tag
as three different version sources.

### 2.3 Failure handling and recovery

When a release workflow fails:

1. Identify whether it failed during validation/build, while uploading assets,
or after the GitHub release was created.
2. Inspect the GitHub Actions workflow logs and GoReleaser output from the
failed run.
3. Fix the configuration, source, permissions, or workflow on a new commit and
push the fix to `main`.
4. Re-run the failed workflow job only when the existing tag and release state
are known to be safe (for example, a transient network error before any
release asset was published).
5. Otherwise, publish the next patch version and note the correction in its
release notes.

Common causes include a shallow checkout (`fetch-depth` not set to `0`), a
missing `contents: write` permission, a tag that is not valid semver, failed
race tests, or an artifact built for the wrong target. Never delete and
recreate a published tag merely to make a failed release appear green.

### 2.4 Release checklist

Before pushing a release tag:

* [ ] The next version is based on the highest existing release tag.
* [ ] The release commit is merged and pushed to `origin/main`, and the local checkout is up to date.
* [ ] `.goreleaser.yaml` and `.github/workflows/release.yml` are up to date.
* [ ] `goreleaser check` passes.
* [ ] `go vet ./...`, `go test ./...`, and `go test -race ./...` pass.
* [ ] `goreleaser release --snapshot --clean --skip=publish` passes.
* [ ] The working tree is clean (`dist/` removed or ignored).
* [ ] The tag is annotated, uses `vX.Y.Z`, and has not been used before.

After publishing:

* [ ] The GitHub Actions `release` workflow completed successfully.
* [ ] The GitHub release has the expected tag, commit, archive, and checksum.
* [ ] The archive extracts and `induction --help` works.
* [ ] A checksum was verified with `sha256sum`.
* [ ] The release URL and exact version were recorded in the announcement.

For additional reference, see the [GoReleaser quick start](https://goreleaser.com/getting-started/quick-start/), [GoReleaser GitHub Actions documentation](https://goreleaser.com/customization/ci/actions/), and [GitHub Actions workflow permissions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions).
