# Tasks

## 0. Baseline audit

- [x] 0.1 Record current Pinax release surfaces: `.goreleaser.yml`, `Taskfile.yml`, release workflow, CI workflow, README/docs install text, and `go-dev-toolchain` spec.
- [x] 0.2 Confirm the release tag convention remains `pinax/vX.Y.Z` and no standalone `vX.Y.Z` Pinax release tag is introduced.

## 1. GoReleaser distribution config

- [x] 1.1 Replace the release-time `go mod tidy` hook with non-mutating dependency verification.
- [x] 1.2 Add stable build/archive ids and keep the existing `pinax` binary, `./cmd/pinax` main package, `CGO_ENABLED=0`, `-trimpath`, version ldflag and OS/arch matrix.
- [x] 1.3 Add SHA-256 checksums, source archive generation and archive SBOM generation.
- [x] 1.4 Add Homebrew cask publishing to `yeisme/homebrew-tap/Casks` using `PUBLISHER_TOKEN` and snapshot skip upload.
- [x] 1.5 Add Scoop publishing to `yeisme/scoop-bucket/bucket` using `PUBLISHER_TOKEN` and snapshot skip upload.
- [x] 1.6 Add nFPM `.deb`, `.rpm` and `.apk` assets with `/usr/bin/pinax` and standard doc/license files.
- [x] 1.7 Add guarded Chocolatey package generation only if it can remain non-publishing by default; otherwise document it as a deferred channel with explicit prerequisites.

## 2. Workflow hardening

- [x] 2.1 Tighten `.github/workflows/pinax-release.yml` top-level permissions to read-only and grant write permissions only to the publish job.
- [x] 2.2 Add protected `release` environment to the publish job.
- [x] 2.3 Pin GoReleaser Action/tool version consistently within the Pinax release workflow instead of `latest`.
- [x] 2.4 Add GitHub App token minting for Homebrew/Scoop cross-repository publishing and pass it as `PUBLISHER_TOKEN`.
- [x] 2.5 Keep `workflow_dispatch` snapshot-only and ensure snapshots cannot update tap, bucket or public package channels.
- [x] 2.6 Add pre-publish Pinax quality gate or equivalent release gate before `goreleaser release --clean`.
- [x] 2.7 Update Pinax CI path filters so release workflow edits trigger the normal Pinax gate.

## 3. Local package validation

- [x] 3.1 Add Taskfile target(s) for package validation that run GoReleaser check and snapshot release without publishing.
- [x] 3.2 Verify generated checksums against at least one archive and smoke `pinax version` plus `pinax --help` from the extracted archive.
- [x] 3.3 Inspect Linux package metadata when `dpkg-deb`, `rpm` or `tar`/`apk` tooling exists, while skipping unavailable host tools explicitly.
- [x] 3.4 Ensure local validation never requires real provider credentials, user vaults, tap/bucket write tokens or public package-manager writes.

## 4. Post-release install smoke

- [x] 4.1 Add a post-release smoke path or documented release checklist for archive download/checksum verification.
- [x] 4.2 Add Homebrew install smoke for the published cask when `brew` is available.
- [x] 4.3 Add Scoop install smoke on Windows when the bucket is reachable.
- [x] 4.4 Add direct Linux package install smoke where the runner supports it.
- [x] 4.5 Check SBOM artifact presence and, if signing/attestation is implemented, verify signatures or attestations.

## 5. Docs and OpenSpec sync

- [x] 5.1 Update README installation docs with `go install`, GitHub archives, Homebrew, Scoop and Linux package options.
- [x] 5.2 Add Pinax release packaging docs under `docs/operations/` describing channels, credentials, tag flow, local validation and post-release smoke.
- [x] 5.3 Update docs to say nFPM publishes `.deb/.rpm/.apk` assets, not APT/YUM/Alpine repositories.
- [x] 5.4 Update `go-dev-toolchain` release requirements through this OpenSpec change.

## 6. Verification

- [x] 6.1 Run `openspec validate pinax-release-packaging-distribution --strict`.
- [x] 6.2 Run `openspec validate --all --strict`.
- [x] 6.3 Run `task release:check`.
- [x] 6.4 Run the new local package validation target.
- [x] 6.5 Run `task check` after docs/config/workflow changes.

## Validation notes

- `task release:check` passed with GoReleaser v2.12.6 and validated `.goreleaser.yml`.
- `task release:package:validate` passed. It generated snapshot archives for linux/darwin/windows amd64+arm64, source archive, archive SBOMs, `checksums.txt`, nFPM `.deb/.rpm/.apk` assets, Homebrew cask metadata, and Scoop manifest metadata; checksum verification passed; extracted `dist/pinax_0.0.1-next_linux_x86_64.tar.gz`; ran `pinax version` and `pinax --help`; checked `.deb` metadata with `dpkg-deb`; reported `rpm` metadata inspection skipped because `rpm` is not installed; checked `.apk` archive metadata with `tar`.
- `openspec validate pinax-release-packaging-distribution --strict` passed.
- YAML diagnostics for `.github/workflows/pinax-release.yml` and `.github/workflows/pinax-ci.yml` passed.
- `task check` passed after docs/config/workflow changes; its OpenSpec gate validated 40 items with 0 failures, lint reported 0 issues, tests passed, and build succeeded.
