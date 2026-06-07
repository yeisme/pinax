## Why

Pinax 的根命令已经承载 vault、note、journal、index、storage、sync、MCP、repair、organize 等多类入口，继续增长会让 `pinax --help` 难以扫描，也会让新用户难以判断常用路径和低频管理路径。

本变更通过重组 CLI tree、抽出 command factory 和保留兼容 alias，让 Pinax 的命令结构更贴近笔记用户心智，同时不破坏现有脚本和机器输出合同。

## What Changes

- 新增目标 CLI tree，把高频笔记工作流、vault 管理、journal、配置、storage、index、sync、MCP 等入口分层组织。
- 将 `daily`、`weekly`、`monthly` 规划为 `journal daily|weekly|monthly` 下的主路径，同时保留旧根路径兼容 alias。
- 将 `stats`、`doctor`、`validate` 规划为 `vault stats|doctor|validate` 下的主路径，同时保留旧根路径兼容 alias。
- 将维度浏览从根级 `group/tag/folder/kind list` 收敛为更清晰的 note/view 相关入口，并保留必要兼容 alias。
- 统一计划型命令的主路径和语义，优先使用 `plan -> list -> apply`，保留 `organize suggest` 等兼容入口。
- 将 `storage set-local`、`storage set-s3` 规划为 `storage set local|s3`，保留旧路径兼容 alias。
- 抽出 `internal/cli` command factory 和 shared helpers，逐步瘦身 `cmd/pinax/main.go`。
- 更新 help 展示策略：主 help 只展示推荐路径，兼容 alias 可 hidden 或在说明中标注。
- 保持所有 alias 和新主路径调用同一 app service、同一 projection 和同一 renderer。

## Capabilities

### New Capabilities

- `cli-tree-ux`: 覆盖 Pinax 主命令树、兼容 alias、help 展示、command factory 拆分和输出合同一致性。

### Modified Capabilities

无。本变更新增 CLI tree UX 规范，不直接重写现有 `pinax` 或 `note-command-ux` 基准要求。

## Impact

- 影响 `cmd/pinax/main.go`：逐步从单文件命令树组装迁移到薄 bootstrap。
- 影响 `internal/cli`：新增 root command factory、dependency wiring、command groups、shared flag helpers 和 render helpers。
- 影响 CLI help、completion 和 e2e/golden 输出。
- 影响旧命令路径测试：需要验证旧路径作为 alias 保持行为和机器输出合同。
- 不影响 `internal/app` 业务服务边界；重组命令树不得绕过 app service 写 vault 或 `.pinax` structured assets。
