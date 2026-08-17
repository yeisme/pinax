## Why

Pinax 当前默认帮助同时暴露本地笔记、知识检索、Agent runtime、API、发布、插件、Capsa、多设备同步和运维能力，用户必须先理解产品内部模块才能完成个人笔记任务，导致命令发现成本过高、日常使用频率过低。现在需要把默认产品面重新收敛为“个人本地知识工具”，让第一次运行 `pinax --help` 的用户立即看见最常用的本地工作流，同时保留已有脚本和高级命令的兼容性。

本变更承接根级 [`docs/architecture/subproject-product-boundaries.md`](../../../../../docs/architecture/subproject-product-boundaries.md) 对 Pinax 的 Notes、indexing、local knowledge workflow 边界，以及 [`docs/workflows/local-first-backup-sync.md`](../../../../../docs/workflows/local-first-backup-sync.md) 中“实时同步不是默认能力”的原则。

## What Changes

- 将 Pinax 默认 root help 收敛为个人本地知识核心入口：`init`、`vault`、`note`、`inbox`、`journal`、`search`、`project`、`backup`。
- 新增 `pinax backup` 个人备份 facade，复用现有本地 version/Git service 提供 status、create、history 和 restore；`pinax version` 保留为高级兼容入口，不删除、不弃用。
- 新增 `pinax commands` 命令，通过同一 Projection 输出完整命令目录；默认 human 输出用于发现，高级自动化可使用 `--json`、`--agent`、`--events` 和 `--explain`。
- 将 `sync`、`capsa`、`api`、`agent`、`publish`、`plugin`、`backend` 等能力标记为高级命令，仅从默认 root help 降级，不删除命令、不改名、不改变 flags 或机器输出合同。
- 更新 root help 文案、快速开始和命令文档，使 Pinax 的首要定位回到本地 Markdown、个人 capture、search、project 和 version safety。
- 个人默认采用现有本地 Git/version snapshot；S3/rclone/Capsa 继续作为高级远端同步或归档能力。本变更不把实验性远端写入包装成稳定备份，也不修改凭据和远端状态。
- 删除没有任何生产调用方的 `internal/research` 原型。该包即使配置 external research endpoint 也只返回 fake fixture，实际 briefing 已由 `internal/briefing` 独立实现；删除不改变 CLI、SDK、配置读取路径或 vault 数据。
- **兼容说明**：默认 help 的可见命令集合会改变，但所有既有命令路径继续执行；本阶段不构成命令删除或机器合同 breaking change。

## Capabilities

### New Capabilities

- `personal-local-cli-surface`: 定义 personal-first 默认命令面、完整命令目录、核心与高级能力分层，以及各输出模式的发现合同。

### Modified Capabilities

- `cli-tree-ux`: 将 root help 从“展示所有产品域”调整为“只展示个人本地知识核心入口”，并要求高级命令继续兼容可执行。

## Impact

- `internal/cli/root.go`、`internal/cli/backup_cmd.go`：root help 分组、命令可见性标注、个人备份 facade、完整命令目录注册。
- `internal/cli/*_test.go`、`cmd/pinax/*_test.go`：默认 help、完整目录、旧命令兼容和输出合同测试。
- `README.md`、`README.zh-CN.md`、`docs/quickstart.md`、`docs/commands/README.md`：personal-first 定位和真实命令示例。
- 稳定表面分类：新增 `backup`/`backup.*` 为 additive CLI 合同；`version`、`sync`、`capsa` 的命令名、flags、JSON envelope、`--agent` keys、`--events` types 均保持不变。
- 内部清理分类：`internal/research` 不在 `cmd/pinax` 依赖图中，没有生产 import，也没有可执行 CLI/API/SDK 消费者；删除属于 internal dead-code cleanup，不需要用户迁移或 deprecation shim。
- 回滚：移除核心可见性过滤与 `commands` 注册即可恢复原 root help；现有命令实现和存储状态不需要迁移或回滚。
