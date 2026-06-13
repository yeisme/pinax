# Pinax GitHub CI/CD 设计优化提案

## 背景与目的
目前的 Pinax 项目尚未在 GitHub CI/CD 层面对齐本地质量门禁。依据 `AGENTS.md` 对本地 `task check` 和 `openspec` 的要求，我们需要设计并优化与独立仓库 / monorepo 一致的 CI 及 Release 工作流，以强化交付的一致性。

## 方案设计
主要包含在根仓库（或子仓库内部）维护两个 GitHub Action Workflow：
1. **Pinax CI (`pinax-ci.yml`)**：监听 `cli/pinax/**` 下的代码变更，在 `push` 及 `pull_request` 阶段，设置 Go / Node / Task 运行环境，并完整执行针对 Pinax 的质量门禁。
2. **Pinax Release (`pinax-release.yml`)**：当打上符合 `pinax/v*.*.*` 约定的 Tag 时触发构建与发布。利用 `goreleaser` 完成 `snapshot` 以及真正的 Release 发版。

## 关键增强 (优化点)
- **质量门禁对齐**：使用 `task ci` 同步调用包含 `golangci-lint`、`task test` 以及 `openspec validate --all` 在内的完整检查（为此需预先配置 Node 环境安装 `@fission-ai/openspec`）。
- **运行性能与缓存**：配置 `setup-go` 依赖缓存和 `golangci-lint-action` 以缩短构建与检查时间。
- **集成测试规范**：保留对后续运行基于 `testscript` 的 e2e 工具并输出证据 (`integrationevidence`) 到 CI artifact 的扩展能力。
