# Proposal: Pinax Local Vault Organize

## 背景

Pinax 已完成 Go/Cobra 开发底座，但还不能作为真实本地 Markdown vault 的日常工具。下一步必须先形成自用闭环：接入现有笔记库、补齐机器可识别 metadata、生成整理计划，并在显式 Git 保护后允许真实落地。

## 目标

- 提供 `pinax init`、`pinax validate`、`pinax note list/show`、`pinax search` 的本地 vault 基础能力。
- 提供 `pinax metadata plan/apply`，为 Markdown 笔记补 `note_id`、`title`、`tags` 和 `schema_version` frontmatter。
- 提供 `pinax organize plan/apply`，生成并应用安全路径内的重命名和目录整理计划。
- 提供 `pinax git snapshot`，在真实整理落地前建立显式保护提交。
- 所有机器输出遵守 Pinax CLI 输出合同，所有 `.pinax/` 机器资产由 CLI/service 写入。

## 非目标

- 不接入 Notion、Lark 或其它 provider。
- 不实现远端写入、sync push/pull 或 conflict queue 完整状态机。
- 不实现长期 daemon、cloud 或 briefing 工作流。

## 影响

- 子项目代码：`cmd/pinax`、`internal/app`、`internal/domain`、`internal/output`、`internal/vault`、`internal/git`。
- 子项目文档：`README.md`、`docs/product/mvp-scope.md`、`docs/operations/local-development.md`。
