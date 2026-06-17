# Spec Delta: Go Dev Toolchain Release 要求

## ADDED Requirements



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
