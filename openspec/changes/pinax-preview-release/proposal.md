# Pinax v0.1.0-preview Release

## Why

Pinax CLI 已完成核心功能、打包链路（GoReleaser v2）、和全部 OpenSpec 归档。CEO review（2026-06-17）建议做低调 preview release 支撑 5 个真实用户验证，而不是大发布。

当前 gap：
- 无 LICENSE 文件（阻塞开源传播）
- 无 GitHub Release workflow（只能本地 `task release:local`）
- 无 Quickstart（README 有详细命令但没有 5 分钟最小流程）
- 已有的 `pinax-release-packaging-distribution` 变更范围太大（含 Homebrew/Scoop/nFPM multi-channel），CEO 判断目前过早

## What changes

1. 添加 LICENSE 文件（MIT 或 Apache-2.0）
2. 创建 `.github/workflows/pinax-release.yml`：tag `pinax/v*` 触发 GoReleaser release，最小权限，pre-release 标记
3. 新增 `docs/quickstart.md`：5 分钟从安装到 proof loop run 的最小流程
4. 更新 README 安装段加入 GitHub Release archive 下载方式
5. 更新 `.goreleaser.yml` 添加 `release.prerelease: true` 默认标记
6. 更新 `go-dev-toolchain` spec release 要求

## Out of scope

- Homebrew tap/Scoop bucket/nFPM packages（由 `pinax-release-packaging-distribution` 处理，CEO 判断暂缓）
- SBOM/签名/attestation（preview 阶段不需要）
- 文档站
- Chocolatey
- 正式 v1.0 发布

## Impact

- `cli/pinax/LICENSE`（新增）
- `.github/workflows/pinax-release.yml`（新增）
- `cli/pinax/.goreleaser.yml`（添加 prerelease 配置）
- `cli/pinax/README.md`（安装段更新）
- `cli/pinax/docs/quickstart.md`（新增）
- OpenSpec `go-dev-toolchain` spec
