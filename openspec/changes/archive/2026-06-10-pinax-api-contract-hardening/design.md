## Context

Pinax 的本地 API 是 REST/RPC projection adapter：handler 和 dispatcher 只解析参数、调用 application service、映射 transport status，并输出同一份 command projection。现有 registry 位于 `internal/app/remote.go` 的 `RemoteRoutes()` / `RemoteCapabilities()`；HTTP server 位于 `internal/api/http.go`；RPC dispatcher 位于 `internal/api/rpc.go`；CLI wiring 位于 `internal/cli/api_cmd.go`。

当前 API discovery UX 已完成：`pinax api routes` 默认输出可扫读 route evidence，隐藏 root `pinax schema export` 兼容路径也已建立。本变更继续处理更底层的合同硬化：OpenAPI method 不得与 registry 漂移，REST/RPC handler 不得与 registry 漂移，错误 transport status 不得吞掉 failed projection，`api serve` 不得把日志混入机器 stdout。

## Goals / Non-Goals

**Goals:**

- 让 `RemoteRoutes()` 成为 REST path、RPC method、OpenAPI operation 和 capability metadata 的可测单一事实来源。
- 修正 OpenAPI method 派生，确保 `POST /v1/project-items/{ref}:{action}` 不再被导出为 `get`。
- 为 REST/RPC drift 增加 table-driven contract tests，覆盖 route 可达性、method/status 映射和 projection envelope。
- 保持本地 API 只绑定 loopback，远程 write 只返回 plan/gate projection，不执行真实写入。
- 明确 `pinax api serve --readonly` 在默认、`--json`、`--agent`、`--events` 下的 stdout/stderr 边界。

**Non-Goals:**

- 不实现公网 hosted API、非 loopback bind、CORS、TLS、token auth、多用户权限或 hosted gateway。
- 不引入新的 route framework、OpenAPI generator、web server 依赖或自动 router。
- 不改变 note/project board application service 的业务语义。
- 不让 REST/RPC 默认执行 Markdown、`.pinax/`、Git、provider 或远端写入。

## Decisions

1. **OpenAPI 从 registry 派生 method 和 metadata**

   `APISchemaExport` 遍历 `RemoteRoutes()` 时只处理 `surface=rest` 且有 `path` 的 route。operation method 使用 `strings.ToLower(route.Method)`，并保留 `operationId`、`x-pinax-command`、`x-pinax-capability`、`x-pinax-readonly`、`x-pinax-body-allowed`、`x-pinax-approval-required` 和 `x-pinax-snapshot-required`。这样 schema export 不再手写第二套路由表。

   备选方案是引入 OpenAPI generator 或 schema DSL；当前 route 数量很小，新增依赖会增加维护面，不符合 Pinax boring 方案偏好。

2. **先用 drift tests，不急着自动生成 HTTP/RPC routing**

   HTTP/RPC handler 当前逻辑很薄，且需要根据 path shape 解析 project slug、note ref、item action 等参数。直接把 registry 变成自动 router 会引入新的 path matching 抽象。先增加 table-driven tests：每条 registered REST route 有代表性 fixture path 能命中对应 handler；每条 registered RPC method 能被 dispatcher 接受；OpenAPI paths/methods 与 registry 一致。

   如果后续 route 数量明显增长，再考虑窄接口 router；本变更不提前抽象。

3. **transport status 与 projection error 分层**

   HTTP status 只表达 transport 层语义；response body 必须继续是 Pinax failed projection envelope。`route_not_found` 使用 404，method mismatch 使用 405，approval gate 和 snapshot gate 使用明确非 2xx status，同时保留 `error.code`、中文 `error.message`、可运行 hint/action。

   这避免客户端只能靠 HTTP status 猜业务错误，也避免 `405` 空 body 破坏 agent/SDK 的统一解析。

4. **`api serve` 是长生命周期命令，输出必须按模式定义**

   默认人类模式继续把 local URL 写 stderr。机器模式不得把日志、banner 或 progress 写 stdout。`--events` 是最自然的 long-running 机器模式，应输出 `start`、`ready`、`shutdown` 或 `error` NDJSON。如果 `--json` / `--agent` 继续支持，只允许输出一次 startup projection 后保持 stdout 静默；如果实现成本或语义不清，允许返回稳定 `unsupported_output_mode`，但必须通过同一 failed projection 渲染。

## Risks / Trade-offs

- **Risk: registry tests 只覆盖 representative path，不等于完整 path parser。** → 用 route id 到 fixture path 的显式表驱动测试覆盖当前所有 route，并在新增 route 时强制补 fixture。
- **Risk: HTTP status 变更影响已有本机客户端。** → projection body、command、error code 保持稳定；文档记录 status mapping，测试覆盖 gate 行为。
- **Risk: `api serve --json` 长生命周期语义不如普通命令直观。** → 优先用 `--events` 表达 lifecycle；若支持 `--json` / `--agent`，只输出 startup projection，不输出持续日志。
- **Risk: OpenAPI schema 仍然比较轻量。** → 本变更只修 route/method/metadata 漂移，不声明完整 request/response JSON schema；完整 schema 可以后续单独设计。
