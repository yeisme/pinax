## Why

目录是 Pinax vault 的核心组织结构，但如果 agent 或远程调用方直接 `mkdir`、`mv` 或删除目录，Pinax 就无法统一做 vault 边界校验、审批、snapshot gate、事件记录、索引刷新和后续 hook。

这次变更把目录生命周期提升为一等 Pinax 能力：以 `pinax folder` 作为 CLI 主入口，并通过同一 application service 暴露 REST/RPC projection adapter，让本地和远程操作共享体验、输出合同和安全钩子。

## What Changes

- 新增一级命令组 `pinax folder`，作为目录创建、查看、移动、重命名、删除、采纳和修复计划的主入口。
- 保留 `pinax note folders` 作为 note folder 维度浏览入口；不再把目录生命周期管理塞在 `note folders` 下面。
- 定义目录操作 application service：命令层、REST handler、RPC dispatcher 都只能调用 service，不得直接 `mkdir`、`rename`、`remove` 或手写 `.pinax` metadata。
- 定义 CLI-authored folder registry，用于记录空目录、目录 purpose、管理状态、hook evidence 和远程幂等写入证据。
- 定义 folder REST/RPC capabilities：list/show 为只读；create/rename/move/delete/adopt 通过 plan、approval、snapshot 和 idempotency gate 控制写入。
- 定义 folder index projection 和 lifecycle events，让目录操作可以触发索引更新、事件审计和后续 hook。

## Capabilities

### New Capabilities

- `folder-operations`: 覆盖 Pinax vault 目录的 CLI、API、service、registry、写入 gate、hook 和输出合同。

### Modified Capabilities

- `cli-tree-ux`: 将 `pinax folder` 从旧维度兼容路径调整为一级目录操作主入口，同时保留 `pinax note folders` 作为维度浏览。
- `incremental-vault-index-maintenance`: 补充 folder lifecycle events 对本地索引和关联 note/asset 投影的更新要求。

## Impact

- 影响代码：`internal/cli` 新增 folder command factory，`internal/app` 新增 folder service/use case，`internal/api` 新增 REST/RPC route，`internal/index` 新增 folder projection 或 folder index refresh，`internal/domain` 新增 folder operation/plan/registry model。
- 影响文档：`docs/interfaces/remote-api-contract.md`、`docs/interfaces/cli-output-contract.md`、命令帮助示例。
- 影响命令：新增 `pinax folder ...`；`pinax note folders` 继续保留维度列表和兼容行为。
- 不新增长期 daemon、不开放公网 hosted API、不让 agent 直接写 `.pinax/folders.json` 或绕过 service 操作文件系统。
