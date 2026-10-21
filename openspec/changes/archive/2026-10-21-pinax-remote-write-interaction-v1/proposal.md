# 远程写入与 MCP/CLI 交互收敛

## Why

Pinax 已有 `inbox.capture`、`folder.rename`、writer proposal/apply、operation ledger 和 typed client。当前能力分别存在于 Remote API、Remote CLI、OpenAPI、MCP manifest 与 SDK projection 中，调用方仍需要自行推断执行位置、写入顺序和恢复动作。

## What Changes

- 为 transport manifest、API routes、OpenAPI 和 Remote CLI 增加统一的执行位置、写入策略和恢复 metadata。
- 为 `pkg/pinaxclient` 提供 inbox capture、folder rename 的 typed mutation 方法，复用原 operation identity。
- 保持 MCP 默认只读；manifest 和 owner action 告诉无 CLI 客户端如何把写入交给 vault owner 或 Remote API。
- 新增 `remote-write-parity` integration evidence profile。
- 更新 Remote API、MCP、CLI parity 与中文 README 使用路径。

## Scope

首批 mutation 为 `inbox.capture`、`draft.create`、writer propose/apply、`folder.rename`、`folder.move` 和 operation show/reconcile。`note.add`、publish、plugin、Cloud Sync、daemon 控制继续维持 local-only 或 plan-first 语义。

所有变更保持 additive，不删除旧 CLI、REST/RPC、MCP 工具、schema 或错误码。
