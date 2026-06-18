## Context

Pinax is a Go CLI with an existing release baseline:

- binary name: `pinax`
- entrypoint: `./cmd/pinax`
- current official release tag shape: `pinax/vX.Y.Z`
- current release workflow: root `.github/workflows/pinax-release.yml`
- current local release tasks: `task release:check`, `task snapshot`, `task release:local`, `task release:build`, `task release`
- current GoReleaser output: linux/darwin/windows archives for amd64/arm64 plus `checksums.txt`

The new design should extend this baseline, not create a second release system.

## Decisions

### 1. One release source of truth

GoReleaser remains the source of truth for release artifacts and package-manager manifests. Pinax will not add hand-maintained Homebrew formulae, Scoop manifests, Linux packages or Chocolatey nuspecs.

The implementation should reshape `cli/pinax/.goreleaser.yml` instead of adding a parallel config:

- add build/archive ids so downstream package sections reference a stable artifact set
- replace the mutating `go mod tidy` release hook with `go mod download` and `go mod verify`
- keep `CGO_ENABLED=0`, `-trimpath`, `main.version={{ .Version }}` and current OS/arch coverage
- set checksum algorithm to SHA-256
- enable source archive generation
- enable archive SBOM generation

### 2. Supported install channels

Pinax should advertise and verify these channels:

| Channel | Contract |
| --- | --- |
| Go module install | `go install github.com/yeisme/pinax/cmd/pinax@<version>` remains documented for Go users. |
| GitHub Release archives | Linux/macOS tarballs and Windows zip archives remain primary universal artifacts, with checksums and SBOMs. |
| Direct install scripts | Optional convenience scripts may download the matching archive and verify checksums before installing `pinax`; they must not write Pinax vault/config state. |
| Homebrew | Publish a cask to `yeisme/homebrew-tap/Casks` using GoReleaser `homebrew_casks`; snapshots must not update the tap. |
| Scoop | Publish a manifest to `yeisme/scoop-bucket/bucket` using GoReleaser `scoops`; snapshots must not update the bucket. |
| Linux packages | Publish `.deb`, `.rpm` and `.apk` assets with nFPM; install binary to `/usr/bin/pinax`; include README/license/release packaging docs under standard doc/license paths. |
| Chocolatey | Generate a package only if GoReleaser can do so without public publish by default; keep public Chocolatey publishing disabled until package metadata, `CHOCOLATEY_API_KEY`, moderation readiness and smoke checks are explicitly added. |

nFPM package assets are not an APT/YUM/Alpine repository. If repository-backed Linux package installs become required later, use a separate approved publisher such as Cloudsmith or another explicit repository pipeline with its own token and smoke tests.

### 3. Cross-repository publisher credentials

Same-repository GitHub Release creation should continue to use `GITHUB_TOKEN` with release-job `contents: write`.

Homebrew and Scoop are cross-repository writes. The preferred credential is a per-project GitHub App token minted inside a protected `release` environment:

- environment variable: `RELEASE_APP_CLIENT_ID`
- environment secret: `RELEASE_APP_PRIVATE_KEY`
- token step: `actions/create-github-app-token`
- GoReleaser env: `PUBLISHER_TOKEN`
- installation repositories: `homebrew-tap` and `scoop-bucket`
- app permissions: `contents: write` only

A fine-grained PAT named `PUBLISHER_TOKEN` is allowed only as a fallback and must be restricted to the exact repository set.

### 4. Workflow hardening

The root release workflow should keep the current `pinax/vX.Y.Z` trigger but tighten permissions and validation:

- top-level `permissions: contents: read`
- snapshot job remains manual `workflow_dispatch` and runs `goreleaser release --snapshot --clean --skip=publish`
- publish job runs only for tag pushes matching `pinax/vX.Y.Z`
- publish job uses `environment: release`
- publish job grants `contents: write`; add `id-token: write` only when cosign keyless signing is implemented
- GoReleaser Action should pin a major action/version instead of `version: latest`
- release job should run the Pinax quality gate or an equivalent pre-publish gate before `goreleaser release --clean`
- CI path filters should include release workflow changes so packaging edits run the normal Pinax gate

If signing or attestations are added in the same implementation pass, permissions and smoke tests must cover them explicitly. If not, the design still requires SBOMs and checksums in the first pass.

### 5. Local packaging verification

Pinax should add a release packaging validation target instead of relying on manual inspection. The target should be safe on developer machines and CI:

- validate GoReleaser config
- run snapshot release without publishing
- verify `checksums.txt` against at least one generated archive
- extract an archive and run `pinax version` and `pinax --help`
- inspect nFPM package metadata when host tools are available; skip with an explicit message when unavailable
- never require real provider credentials, user vaults, public network access beyond release download steps, or writable tap/bucket credentials for local validation

### 6. Post-release smoke

After a real tagged release, a post-release workflow or documented manual gate should prove installability:

- download release archive and `checksums.txt`, verify checksum, run `pinax version` and `pinax --help`
- install Homebrew cask from `yeisme/tap/pinax` or the repository's chosen tap alias, run smoke
- add the Yeisme Scoop bucket on Windows, install `pinax`, run smoke
- install direct Linux package assets where runner support is practical, then run smoke
- verify SBOM artifacts exist; verify signatures/attestations if those features are enabled

## Risks / checks

- Package-manager writes can update the wrong repository. Check: use a release environment GitHub App token scoped only to `homebrew-tap` and `scoop-bucket`.
- Snapshots can accidentally publish package-manager manifests. Check: `skip_upload: "{{ .IsSnapshot }}"` on Homebrew/Scoop and no Chocolatey public publish in first pass.
- Linux package messaging can overclaim APT/YUM support. Check: docs say `.deb/.rpm/.apk` release assets, not repositories.
- Release hook can mutate the worktree. Check: use `go mod download` and `go mod verify`, not `go mod tidy`.
- Archive naming changes can break package templates. Check: add stable archive ids and smoke checksum verification.
- Signing/attestation can fail on forks or local runs. Check: gate signing behind release job permissions and keep local snapshot verification usable without credentials.
