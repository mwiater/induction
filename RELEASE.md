# Induction release guide

This document describes the supported release process for Induction. Releases are
driven by Git tags and built with GoReleaser. The repository's
[.goreleaser.yaml](.goreleaser.yaml) currently produces a Linux AMD64
`induction` binary and runs these checks before building it:

- `go vet ./...`
- `go test ./...`
- `go test -race ./...`

The module currently requires Go 1.26.4 or newer. Run all release commands from
the repository root on a clean checkout.

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

- Patch (`0.1.0` -> `0.1.1`): backwards-compatible bug fixes.
- Minor (`0.1.0` -> `0.2.0`): backwards-compatible features or behavior.
- Major (`0.x.y` -> `1.0.0`): the first stable API, or later breaking changes.

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

1. `.goreleaser.yaml` has `version: 2`.
2. `project_name` is `induction`.
3. `builds.main` points to `./cmd/induction`.
4. The target list is intentional. The current release is Linux AMD64 only.
5. The pre-release hooks pass on the exact code to be released.

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
archive contains the expected `induction` executable. A snapshot is a build
validation artifact; it is not a release and must not be given the production
`v0.1.0` name.

If the machine has a read-only or unusual Go build cache, the repository config
uses `/tmp/induction-gocache` by default. An explicit `GOCACHE` value is still
honored:

```bash
GOCACHE="$PWD/.cache/gocache" goreleaser release --snapshot --clean --skip=publish
```

### 1.3 Prepare the first release commit

Make sure all release-related files are committed, including `.goreleaser.yaml`
and this guide. The release tag must point to the commit whose source and
configuration were tested.

```bash
git status
git diff --check
git log -1 --oneline
```

If the project uses a protected `main` branch, merge the release-ready change
through the normal review process first. Do not tag a local commit that has not
been pushed to the intended default branch.

### 1.4 Create and push `v0.1.0`

Create an annotated tag at the release commit:

```bash
git switch main
git pull --ff-only origin main
git tag -a v0.1.0 -m "Release v0.1.0"
git show --stat --oneline v0.1.0
git push origin v0.1.0
```

The final push is the release trigger when GitHub Actions automation is
configured. Check that the tag points at the expected commit both locally and
on the remote:

```bash
git rev-parse v0.1.0^{commit}
git ls-remote --tags origin refs/tags/v0.1.0
```

### 1.5 Publish locally when automation is not configured

A local publish requires a GitHub token in `GITHUB_TOKEN`. The token needs
permission to create releases in `mwiater/induction`; a fine-grained token
needs repository `Contents: write` access. Do not put the token in a command,
commit, shell history, or this document.

```bash
export GITHUB_TOKEN='...'
goreleaser release --clean
unset GITHUB_TOKEN
```

Run this only after pushing `v0.1.0`. GoReleaser reads the latest Git tag to
derive the release version and publishes the artifacts to GitHub. If the
release already exists, do not rerun with a changed tag or force-push the tag;
inspect the failed run and correct the underlying issue.

### 1.6 Verify the first release

On the GitHub release page, verify all of the following:

- The release is named and tagged `v0.1.0`.
- The release commit is the intended commit.
- The Linux AMD64 archive and checksum file are present.
- The archive extracts to an executable named `induction`.
- The binary starts and reports its help successfully:

  ```bash
  ./induction --help
  ```

- The checksum matches the downloaded archive:

  ```bash
  sha256sum --check checksums.txt --ignore-missing
  ```

Record the release URL and test the documented download path before announcing
the release. Docker builds and documentation that consume the latest public
release should be checked separately because they are downstream of the GitHub
release artifact.

## 2) Subsequent release instructions

Subsequent releases should be sequential, reproducible, and tag-triggered.
Each release is a new commit and a new immutable semantic-version tag.

### 2.1 Sequential manual release process

For every release, start from the default branch and update it before deciding
the next version:

```bash
git switch main
git pull --ff-only origin main
git fetch --tags origin
git tag --list 'v*' --sort=-version:refname | head
```

Choose the next version from the highest existing stable tag. For example:

- `v0.1.0` -> `v0.1.1` for a patch release.
- `v0.1.1` -> `v0.2.0` for a backwards-compatible feature release.
- `v0.9.4` -> `v1.0.0` for the first stable major release.

Then use this sequence:

1. Merge the implementation, tests, documentation, and release-note changes.
2. Run `goreleaser check`, `go vet ./...`, `go test ./...`, and
   `go test -race ./...`.
3. Run `goreleaser release --snapshot --clean --skip=publish` and inspect
   `dist/`.
4. Confirm the working tree is clean and the release commit is on `main`.
5. Create one annotated tag, for example:

   ```bash
   git tag -a v0.1.1 -m "Release v0.1.1"
   git push origin v0.1.1
   ```

6. Let the tag-triggered workflow publish the release, or run
   `goreleaser release --clean` locally with `GITHUB_TOKEN`.
7. Verify the GitHub release, downloaded archive, checksum, and executable.
8. Announce the release using the exact tag and release URL.

Never create `v0.1.2` from a stale checkout just because `v0.1.1` failed.
Fix the release problem first, then decide whether the same tag can be safely
re-run. If the tag or GitHub release has already been published, use the next
patch version rather than changing history.

### 2.2 Automated GitHub Actions releases

The recommended automation is a workflow that runs only when a tag is pushed.
Create `.github/workflows/release.yml` with a repository-specific version of
the following:

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
        uses: actions/checkout@v7
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v7
        with:
          go-version: '1.26.4'

      - name: Release with GoReleaser
        uses: goreleaser/goreleaser-action@v7
        with:
          distribution: goreleaser
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Important details:

- `fetch-depth: 0` is required so GoReleaser can inspect the complete tag and
  commit history.
- The `v*.*.*` filter prevents ordinary branch pushes from publishing a
  release. It also expects the `v0.1.0`-style tag format used by this project.
- `contents: write` allows the workflow token to create the GitHub release and
  upload its assets. Keep permissions no broader than necessary.
- The workflow must check out the tag that triggered it. Do not build the
  default branch separately and attach those artifacts to the tagged release.
- Pin action versions according to the repository's action-maintenance policy;
  the versions above illustrate the current GoReleaser v2 workflow shape.

With this workflow, the normal release operation is deliberately small:

```bash
git switch main
git pull --ff-only origin main
# edit, test, review, and merge the release-ready change
git tag -a v0.1.1 -m "Release v0.1.1"
git push origin v0.1.1
```

GitHub Actions runs the same GoReleaser configuration, executes its hooks, and
publishes the release if all checks pass. Review the workflow logs and the
GitHub release page after every tag push. A green workflow is not a substitute
for downloading and testing the published artifact.

### 2.3 Optional pre-release and automated version selection

Do not use a moving tag such as `latest` as the immutable release identifier.
For a pre-release, use a semver-compatible tag such as `v0.2.0-rc.1` only after
confirming that the workflow filter and GoReleaser configuration support the
chosen pre-release policy. Promote the final release with a new stable tag,
for example `v0.2.0`; do not retag the release candidate.

If a separate release tool calculates versions from commits, it must still
create one annotated `vX.Y.Z` tag on the exact commit being released and push
that tag. The tag remains the source of truth for GoReleaser. Do not combine a
moving branch build, a manually edited version string, and a Git tag as three
different version sources.

### 2.4 Failure handling and recovery

When a release fails:

1. Identify whether it failed before publishing, while uploading, or after the
   GitHub release was created.
2. Inspect the GoReleaser logs and the `dist/` artifacts from the failed run.
3. Fix the configuration, source, permissions, or workflow on a new commit.
4. Re-run only when the existing tag and release state are known to be safe.
5. Otherwise publish the next patch version and note the correction in its
   release notes.

Common causes include a shallow checkout, a missing `contents: write`
permission, a tag that is not valid semver, an absent `GITHUB_TOKEN`, failed
race tests, or an artifact that was built for the wrong target. Never delete
and recreate a published tag merely to make a failed release appear green.

### 2.5 Release checklist

Before pushing a release tag:

- [ ] The next version is based on the highest existing release tag.
- [ ] The release commit is merged into `main` and the checkout is current.
- [ ] `goreleaser check` passes.
- [ ] `go vet ./...` passes.
- [ ] `go test ./...` passes.
- [ ] `go test -race ./...` passes.
- [ ] `goreleaser release --snapshot --clean --skip=publish` passes.
- [ ] The working tree is clean.
- [ ] The tag is annotated, uses `vX.Y.Z`, and has not been used before.

After publishing:

- [ ] The workflow completed successfully.
- [ ] The GitHub release has the expected tag, commit, archive, and checksum.
- [ ] The archive extracts and `induction --help` works.
- [ ] A checksum was verified.
- [ ] The release URL and exact version were recorded in the announcement.

For additional reference, see the [GoReleaser quick start](https://goreleaser.com/getting-started/quick-start/), [GoReleaser GitHub Actions documentation](https://goreleaser.com/customization/ci/actions/), and [GitHub Actions workflow permissions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions).

