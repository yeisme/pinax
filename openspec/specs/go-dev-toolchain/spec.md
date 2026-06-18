# go-dev-toolchain Specification

## Purpose
TBD - created by archiving change pinax-go-dev-toolchain. Update Purpose after archive.
## Requirements
### Requirement: Go 开发工具链标准化落地

本项目 SHALL 按照根 go-dev-toolchain-quality 设计落地 golangci-lint v2 配置、Taskfile 任务语义对齐和可选热加载。

#### Scenario: lint 配置覆盖基线

- **WHEN** 本项目完成 .golangci.yml 配置
- **THEN** 配置 SHALL 覆盖基线 linters（errcheck、govet、ineffassign、staticcheck、unused、misspell、revive）和 formatters（gofmt、goimports）
- **AND** golangci-lint run --new-from-rev=HEAD~ SHALL 退出码 0

#### Scenario: Taskfile 任务语义对齐

- **WHEN** 本项目完成 Taskfile 更新
- **THEN** Taskfile SHALL 提供 deps、mod-check、fmt、fmt-check、lint、test、build、ci/check 任务
- **AND** task ci SHALL 至少执行格式检查、lint、测试和构建

#### Scenario: 项目特有任务保留

- **WHEN** 本项目已有特有任务
- **THEN** 工具链落地 MUST 保留这些特有任务
- **AND** 只允许为命名一致性增加别名或补充说明

### Requirement: Pinax GitHub CI mirrors the local quality gate

Pinax SHALL provide a GitHub Actions CI workflow for repository changes that touch the Pinax subproject or its workflow definition, and the workflow SHALL run from the Pinax subproject working directory using the same quality gate expected locally.

#### Scenario: CI runs the Pinax check gate for subproject changes

- **GIVEN** a push or pull request changes files under `cli/pinax/**` or the Pinax CI workflow file
- **WHEN** the Pinax CI workflow runs
- **THEN** it SHALL set up Go, Task, golangci-lint, and OpenSpec dependencies
- **AND** it SHALL run the Pinax quality gate that covers formatting, linting, tests, build, and `openspec validate --all`
- **AND** it SHALL NOT require provider credentials, user vaults, or network calls to external note providers.

#### Scenario: CI scope stays inside the Pinax subproject

- **GIVEN** the workflow is triggered from the repository root
- **WHEN** it runs Pinax commands
- **THEN** command execution SHALL use `cli/pinax` as the working directory
- **AND** generated build artifacts, coverage files, and test evidence SHALL NOT be committed by the workflow.

### Requirement: Pinax release workflow uses Pinax tags

Pinax SHALL provide a GitHub Actions release workflow that is triggered only by Pinax release tags and builds release artifacts through the Pinax Go CLI release toolchain.

#### Scenario: Release workflow triggers on Pinax semantic tags

- **GIVEN** a tag matching `pinax/v*.*.*` is pushed
- **WHEN** the Pinax release workflow runs
- **THEN** it SHALL validate the tag as `pinax/vX.Y.Z`
- **AND** it SHALL set up the Go release environment from the Pinax module
- **AND** it SHALL run a release check or packaging step for the Pinax CLI
- **AND** it SHALL keep non-Pinax tags from triggering Pinax release packaging.

#### Scenario: Release workflow separates validation from publishing

- **GIVEN** the release workflow is being changed or reviewed
- **WHEN** validation is required without publishing a real release
- **THEN** the workflow SHALL support a snapshot path that validates release configuration without publishing artifacts
- **AND** real publishing SHALL require the Pinax release tag trigger
- **AND** snapshot runs SHALL NOT update Homebrew taps, Scoop buckets, Chocolatey packages, Linux package repositories or other public package-manager channels.

#### Scenario: Release workflow uses least privilege

- **GIVEN** the release workflow has both snapshot and publish jobs
- **WHEN** GitHub evaluates workflow permissions
- **THEN** the workflow-level default SHALL be read-only
- **AND** only the publish job SHALL request GitHub Release write permissions
- **AND** real publishing SHALL run in the protected `release` environment
- **AND** cross-repository Homebrew or Scoop publication SHALL use a release-environment publisher token scoped to the exact tap and bucket repositories
- **AND** signing or attestation permissions SHALL be added only when those release steps are implemented and verified.

### Requirement: Preview Release 配置

Pinax release SHALL 使用 GoReleaser v2 配置，支持 preview/pre-release 标记。

#### Scenario: GoReleaser 配置包含 prerelease 标记

- **WHEN** 运行 `goreleaser check`
- **THEN** `.goreleaser.yml` 包含 `release.prerelease: true`
- **AND** 包含 `release.name_template`
- **AND** 配置校验通过

#### Scenario: Release workflow 最小权限

- **WHEN** 检查 `.github/workflows/pinax-release.yml`
- **THEN** 顶层 permissions 为 read-only
- **AND** 只有 publish job 拥有 `contents: write`
- **AND** 不发布到 Homebrew/Scoop/Chocolatey

#### Scenario: LICENSE 文件存在

- **WHEN** 检查 `cli/pinax/` 根目录
- **THEN** 存在标准开源 LICENSE 文件
- **AND** README License 段与文件一致

#### Scenario: Snapshot 不创建 Release

- **WHEN** 通过 `workflow_dispatch` 触发 snapshot
- **THEN** 只构建 artifacts
- **AND** 不创建 GitHub Release
- **AND** 不发布到任何分发 channel

### Requirement: Pinax multi-channel release packaging

Pinax SHALL publish installable release artifacts for Go users, direct archive users, Homebrew users, Scoop users and Linux package users from one GoReleaser configuration.

#### Scenario: Universal archives remain available

- **WHEN** a real Pinax release is published
- **THEN** GoReleaser SHALL build `pinax` from `./cmd/pinax` for Linux, macOS and Windows on amd64 and arm64 unless a platform is explicitly unsupported
- **AND** Linux and macOS artifacts SHALL be archives suitable for direct extraction
- **AND** Windows artifacts SHALL be zip archives
- **AND** release assets SHALL include SHA-256 checksums.

#### Scenario: Source and SBOM artifacts are generated

- **WHEN** Pinax release packaging runs for a real release
- **THEN** the release SHALL include a source archive
- **AND** the release SHALL include SBOM documents for archive artifacts
- **AND** these artifacts SHALL be generated by GoReleaser rather than hand-maintained scripts.

#### Scenario: Homebrew cask is published from tagged releases

- **GIVEN** the release job has a publisher token for the Yeisme Homebrew tap
- **WHEN** a `pinax/vX.Y.Z` release is published
- **THEN** GoReleaser SHALL update the Pinax Homebrew cask under the configured Yeisme tap repository
- **AND** the cask SHALL install the `pinax` binary
- **AND** snapshot releases SHALL NOT update the tap.

#### Scenario: Scoop manifest is published from tagged releases

- **GIVEN** the release job has a publisher token for the Yeisme Scoop bucket
- **WHEN** a `pinax/vX.Y.Z` release is published
- **THEN** GoReleaser SHALL update the Pinax Scoop manifest under the configured Yeisme bucket repository
- **AND** the manifest SHALL install the `pinax` binary
- **AND** snapshot releases SHALL NOT update the bucket.

#### Scenario: Linux package assets are published without repository overclaim

- **WHEN** a Pinax release is published
- **THEN** GoReleaser SHALL create `.deb`, `.rpm` and `.apk` package assets for supported Linux architectures
- **AND** those packages SHALL install `pinax` under `/usr/bin`
- **AND** packages SHALL include standard documentation or license metadata where supported
- **AND** Pinax documentation SHALL describe them as release assets, not as an APT, YUM, DNF or Alpine repository.

#### Scenario: Chocolatey is guarded until publish readiness exists

- **WHEN** Chocolatey packaging is added
- **THEN** public Chocolatey publishing SHALL remain disabled unless an approved API key, package metadata, moderation readiness and smoke verification are present
- **AND** snapshot releases SHALL NOT publish Chocolatey packages.

### Requirement: Pinax release packaging is locally verifiable

Pinax SHALL provide local validation for packaging changes without requiring real package-manager publication.

#### Scenario: Snapshot packaging proves archive usability

- **WHEN** maintainers run `task release:package:validate`
- **THEN** it SHALL validate the GoReleaser configuration
- **AND** it SHALL run a no-publish snapshot release
- **AND** it SHALL verify generated checksums against at least one generated archive
- **AND** it SHALL extract a generated archive and run `pinax version` and `pinax --help` from the extracted binary
- **AND** it SHALL verify that an archive SBOM artifact exists.

#### Scenario: Local packaging validation is safe by default

- **WHEN** package validation runs on a developer machine or CI runner
- **THEN** it SHALL NOT require real provider credentials, user vaults, tap write tokens, bucket write tokens, Chocolatey API keys or public package-manager publication
- **AND** unavailable host package inspection tools SHALL be reported as skipped rather than hidden or treated as successful checks.

### Requirement: Pinax release install smoke covers published channels

Pinax SHALL define post-release smoke checks for each published install channel.

#### Scenario: Archive install smoke verifies checksums and execution

- **WHEN** a tagged Pinax release has been published
- **THEN** release smoke SHALL download at least one archive and the checksum file
- **AND** it SHALL verify the archive checksum
- **AND** it SHALL run `pinax version` and `pinax --help` from the downloaded binary.

#### Scenario: Package-manager smoke verifies published manifests

- **WHEN** Homebrew or Scoop publication is enabled for a tagged release
- **THEN** release smoke SHALL install Pinax from the published Homebrew cask or Scoop manifest when the runner supports that package manager
- **AND** it SHALL run `pinax version` and `pinax --help` after installation.

#### Scenario: Linux package smoke verifies package assets

- **WHEN** Linux package assets are published
- **THEN** release smoke SHALL install or inspect `.deb`, `.rpm` and `.apk` assets where the runner supports the package format
- **AND** it SHALL run `pinax version` and `pinax --help` after any successful package install.

