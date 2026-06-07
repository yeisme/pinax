## Why

根 `pinax-cloud-storage-backend` 设计已完成云端存储后端架构和跨项目 handoff。本 change 在 `cli/pinax` 内实现云同步客户端：manifest 构建、端侧加密、cloud state 管理、HTTP client、sync plan、冲突处理和 CLI 命令。

## What Changes

- 实现 cloud CLI state 和 config（`pinax cloud login/status/logout/doctor`）。
- 实现 manifest 和 client-side crypto（path hash、encrypted blob envelope、local blob cache、redaction）。
- 实现 cloud sync planner（`pinax sync diff/pull/push` plan、dry-run/yes、conflict queue）。
- 实现 cloud output contract（默认中文摘要、`--agent`、`--json`、`--events`、`--explain`）。
- 使用 fake HTTP server 进行本地开发和测试。

## Capabilities

### New Capabilities

- `pinax-cloud-sync`: Pinax 云同步客户端全链路。

## Impact

- 新增 Go 包：`internal/cloud`、`internal/sync`、`internal/config`（cloud 扩展）。
- 修改 CLI 命令树：新增 `pinax cloud` 和 `pinax sync` 命令组。
- 端侧加密确保明文不离开本地。

## Non-Goals

- 不实现后端 API server（在 `backend-server/pinax-cloud` 中实现）。
- 不改变 Pinax 本地优先工作流，无后端时所有功能正常可用。
