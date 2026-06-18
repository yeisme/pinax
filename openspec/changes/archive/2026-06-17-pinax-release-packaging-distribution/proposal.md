# Pinax Multi-Channel Release Packaging

## Why

Pinax currently supports source installation through `go install` and GoReleaser-built GitHub Release archives. That is not enough for users who expect native package-manager installation on macOS, Windows and Linux.

Current gaps:

- no Homebrew tap/cask publication
- no Scoop bucket manifest publication
- no `.deb`, `.rpm` or `.apk` package assets
- no SBOM or source archive contract for user-facing releases
- no cross-repository publisher-token model for tap/bucket updates
- no post-release install smoke for package-manager channels
- release workflow permissions are broader than necessary at the top level
- `.goreleaser.yml` runs `go mod tidy` in a release hook, which can mutate source during release

## What changes

Design and implement a multi-channel release distribution contract for Pinax:

1. keep existing `go install` and GitHub Release archive paths
2. add GoReleaser source archives, SHA-256 checksums and archive SBOMs
3. add Homebrew cask publication to `yeisme/homebrew-tap`
4. add Scoop manifest publication to `yeisme/scoop-bucket`
5. add nFPM `.deb`, `.rpm` and `.apk` assets
6. add guarded Chocolatey package generation with public publishing disabled until credentials and moderation readiness are explicitly approved
7. harden release workflow permissions, tag gating, protected release environment and publisher-token handling
8. add local and CI/post-release install smoke evidence for archives, Homebrew, Scoop and Linux package assets
9. document every supported install path without implying that nFPM assets are full APT/YUM/Alpine repositories

## Out of scope

- hosted Pinax Cloud release operations
- changing Pinax runtime behavior or CLI command contracts
- publishing a real APT/YUM/Alpine repository in the first pass
- publishing Chocolatey packages publicly before package metadata, API key handling and moderation readiness are approved
- changing the `pinax/vX.Y.Z` tag convention

## Impact

- `cli/pinax/.goreleaser.yml`
- `.github/workflows/pinax-release.yml`
- `.github/workflows/pinax-ci.yml`
- optional post-release workflow under `.github/workflows/`
- `cli/pinax/Taskfile.yml`
- `cli/pinax/README.md`
- `cli/pinax/docs/README.md`
- new Pinax release packaging documentation under `cli/pinax/docs/operations/`
- OpenSpec `go-dev-toolchain` release requirements
