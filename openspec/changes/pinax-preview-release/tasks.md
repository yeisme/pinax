# Tasks: Pinax v0.1.0-preview Release

Owner: `cli/pinax`  
Priority: P0（CEO 建议的首要行动）  
Execution rule: 这是 preview release，不做 multi-channel 分发。Homebrew/Scoop/nFPM 由 `pinax-release-packaging-distribution` 独立处理。

## 0. 决策和基线

- [ ] 0.1 Owner: `cli/pinax`; Lane: sequential; Depends on: none; Scope: 确认 License 选择（MIT 推荐）并记录在 design.md。Acceptance: design.md 记录 License 决策和理由。Validation: 人工确认。Failure re-check: 如果 CEO 选择 Apache-2.0，更新 design.md。
- [ ] 0.2 Owner: `cli/pinax`; Lane: sequential; Depends on: 0.1; Scope: 确认 `pinax-release-packaging-distribution` 变更保持 active 但不阻塞本变更；记录两个变更的边界。Acceptance: proposal.md out of scope 已说明边界。Validation: `openspec validate pinax-preview-release --strict`。Expected: 校验通过。

## 1. LICENSE 文件

- [ ] 1.1 Owner: `cli/pinax`; Lane: A; Depends on: 0.1; Scope: 在 `cli/pinax/` 根目录创建 LICENSE 文件，使用选择的 License 全文。Acceptance: LICENSE 文件存在且是标准 License 文本。Validation: `test -f cli/pinax/LICENSE`。Expected: 文件存在。
- [ ] 1.2 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: 更新 `README.md` License 段落：删除"No open-source license has been selected"声明，替换为实际 License 名称和链接。Acceptance: README License 段与实际 LICENSE 文件一致。Validation: 人工审阅。Expected: 一致。

## 2. Release workflow

- [ ] 2.1 Owner: `cli/pinax`; Lane: B; Depends on: 0.2; Scope: 创建 `.github/workflows/pinax-release.yml`：触发 `pinax/v*` tag，最小权限（顶层 read-only，publish job contents:write），setup-go 1.26，task check 质量门禁，goreleaser release --clean。Acceptance: workflow YAML 语法正确，权限最小化。Validation: `goreleaser check` + workflow YAML lint。Expected: 配置有效。
- [ ] 2.2 Owner: `cli/pinax`; Lane: B; Depends on: 2.1; Scope: 添加 `workflow_dispatch` snapshot 支持：手动触发只构建 artifacts 不创建 GitHub Release。Acceptance: snapshot 不能发布到任何 channel。Validation: 检查 workflow 配置。Expected: snapshot 安全。
- [ ] 2.3 Owner: `cli/pinax`; Lane: B; Depends on: 2.1; Scope: 更新 `.goreleaser.yml` 添加 `release.prerelease: true` 和 `release.name_template`。Acceptance: goreleaser check 通过。Validation: `goreleaser check`。Expected: 配置有效。

## 3. Quickstart 文档

- [ ] 3.1 Owner: `cli/pinax`; Lane: C; Depends on: 1.2; Scope: 创建 `docs/quickstart.md`：5 分钟从安装到 proof loop run/plan/snapshot/apply/restore 的最小流程。Acceptance: 只使用真实可运行命令；不覆盖 Cloud Sync/MCP/Templates/Boards。Validation: 人工审阅 + 命令可运行性检查。Expected: Quickstart 独立可完成。
- [ ] 3.2 Owner: `cli/pinax`; Lane: C; Depends on: 3.1; Scope: 更新 README 安装段：新增 GitHub Release archive 下载方式（curl + tar + checksum verify），保留 `go install` 和 `task release:local`。Acceptance: 三种安装方式都真实可用。Validation: 人工审阅。Expected: 安装段完整。

## 4. Spec 更新

- [ ] 4.1 Owner: `cli/pinax`; Lane: D; Depends on: 2.3; Scope: 更新 `go-dev-toolchain` spec 的 release 要求：添加 preview release 的 prerelease 标记、最小权限 workflow、LICENSE 要求。Acceptance: spec delta 只描述本次变化。Validation: `openspec validate pinax-preview-release --strict`。Expected: 校验通过。

## 5. 验证

- [ ] 5.1 Owner: `cli/pinax`; Lane: sequential; Depends on: all previous; Scope: 运行 `task check`、`goreleaser check`、`task release:local`。Acceptance: 全绿，archives + checksums 生成。Validation: 同上。Expected: 6 archives + checksums.txt。
- [ ] 5.2 Owner: `cli/pinax`; Lane: sequential; Depends on: 5.1; Scope: 运行 `openspec validate pinax-preview-release --strict` 和 `openspec validate --all --strict`。Acceptance: 全绿。Validation: 同上。Expected: 全部通过。
- [ ] 5.3 Owner: `cli/pinax`; Lane: sequential; Depends on: 5.2; Scope: 检查 `git status --short`：无构建产物、coverage、evidence、credentials 被提交。Acceptance: 只有源码/docs/workflow/LICENSE/openspec 变更。Validation: `git status --short`。Expected: 干净。
