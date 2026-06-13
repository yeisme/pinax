## Why

`pinax api` 已经提供本地 REST/RPC projection adapter、route registry 和 OpenAPI 导出，但当前 schema、handler 和 dispatcher 之间仍可能漂移。例如 registry 中 `rest.project.item.plan` 是 `POST`，OpenAPI 导出却可能被硬编码为 `get`。这会让 agent、本机工具和 dashboard 客户端基于错误合同集成。

本变更在已完成的 API discovery UX 之后收紧合同：让 `RemoteRoutes()` 成为 OpenAPI、REST、RPC 和 transport error 行为的可测单一事实来源，并明确 `api serve` 的长生命周期输出边界。

## What Changes

- 修正 OpenAPI 导出，使 REST operation method 从 route registry 的 `method` 派生，而不是在 schema export 中手写固定方法。
- 为 REST route、RPC method 和 OpenAPI paths 增加漂移检测测试，确保 registry、handler、dispatcher 和 schema 保持一致。
- 统一本地 API transport error 语义：404、405、approval gate 和 snapshot gate 仍返回 failed projection envelope，而不是空 body 或错误状态漂移。
- 明确 `pinax api serve --readonly` 的长生命周期输出：默认模式 URL 写 stderr，机器模式不得混入日志或非结构化输出。
- 更新远程 API 合同文档和 `project-board-workspace` spec，保留本地 loopback、readonly/dry-run/gate 边界。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `project-board-workspace`: 收紧本地 REST/RPC route registry、OpenAPI 导出、transport error envelope、remote write gate 和 `api serve` lifecycle 输出要求。

## Impact

- 影响代码：`internal/app/remote.go`、`internal/api/http.go`、`internal/api/rpc.go`、`internal/cli/api_cmd.go`、`cmd/pinax/main_test.go`，以及相关 `internal/app` / `internal/api` 测试。
- 影响接口：`pinax api schema export --format openapi` 的 REST method 输出会更正为 registry method；HTTP error status 更明确，但 response body 继续使用 Pinax projection envelope。
- 影响文档：`docs/interfaces/remote-api-contract.md` 和 `openspec/specs/project-board-workspace/spec.md`。
- 不新增公网 API、非 loopback bind、CORS、TLS、token auth、多用户权限、hosted gateway、真实远程写入或 provider 行为。
