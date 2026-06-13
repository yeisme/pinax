## Why

Pinax 的本地 API server（`pinax api serve`）当前只有 `--readonly` / `--allow-write` 全局开关，没有认证、没有 token、没有资源级权限控制。每次 sync 操作需要重复传入 `--endpoint`、`--workspace`、`--device`、`--secret-ref` 等参数，无法命名和复用连接配置。API 层缺少 HTTP 缓存语义和审计能力，不适合 agent/MCP 持久接入场景。

本变更引入四项能力：

1. **Profile/别名系统**（类 mc alias）—— 命名后端连接配置，全局持久化，sync/api 命令通过别名引用。
2. **Token 权限管理** —— 双层 token 体系（temp token + 长期 token），scope 细粒度到 route group，支持创建、轮转和撤销。
3. **API 缓存中间件** —— 只读路由返回 `Cache-Control` / `ETag`，客户端可 `If-None-Match` 304。
4. **后端操作扩展** —— `BlobStore` 增加 `List`/`Exists`/`BatchStat`，CLI 暴露 `backend ls/cp/diff` 等操作。

## What Changes

### Profile/别名系统

- 新增全局配置文件 `~/.pinax/profiles.yaml`，存储命名后端连接配置（endpoint、workspace、device、secret_ref、默认 scope）。
- 新增 `pinax profile add/list/remove/show` CLI 命令。
- `sync --target` 从硬编码 `cloud/git/s3` 改为解析 profile name 或直接传 endpoint URI。
- 配置层（`internal/config`）新增 `ProfilesConfig`，支持 Viper 合并。

### Token 权限管理

- 新增 `internal/api/auth.go`：`TokenRecord` 模型、scope 定义、验证逻辑、auth middleware。
- 新增 `internal/api/audit.go`：API 审计日志 NDJSONL 输出到 `.pinax/events/api-audit.jsonl`。
- 新增 `pinax token create/list/revoke/rotate` CLI 命令。
- `pinax api serve` 新增三种认证模式：
  - 默认：生成 temp token（进程内存，退出失效，stderr 输出一次）。
  - `--token-file`：加载长期 token（文件持久化，按 scope 控制）。
  - `--no-auth`：无认证（强制 loopback）。
- `ServerOptions` 扩展为包含 token store 和 route group 暴露配置。
- 每个路由根据注册的 scope 要求经过 auth middleware 验证。

### API 缓存

- 新增 `internal/api/cache.go`：只读路由的 `Cache-Control` header 设置和 ETag 304 响应。
- 缓存策略从 route registry 的默认配置派生，不引入外部缓存依赖。
- 写入路由和 RPC 不缓存。

### 后端操作扩展

- `BlobStore` 接口新增 `List(prefix)` / `Exists(key)` / `BatchStat(keys)`。
- 新增 `pinax backend ls/cp/stat/du` CLI 命令，支持 `profile:path` 格式。
- 新增 `internal/remote/cached_store.go`：`BlobStore` 缓存装饰器，本地缓存远端 blob。

## Capabilities

### New Capabilities

- `pinax-auth-token-management`：本地 API token 创建、验证、轮转、撤销和 scope 管理。
- `pinax-profile-management`：全局后端连接配置的命名别名管理。
- `pinax-api-caching`：只读 API 路由的 HTTP 缓存语义。

### Modified Capabilities

- `pinax-cloud-sync`：`sync --target` 支持解析 profile name；`BlobStore` 接口扩展。
- `pinax-backend-provider-cli`：新增 `backend ls/cp/stat/du` 命令。
- `project-board-workspace`：API handler 增加 auth middleware；`ServerOptions` 扩展。
- `configuration-layer`：新增 `ProfilesConfig` 和 `APIAuthConfig` 配置段。

## Impact

- 影响代码：`internal/api/http.go`、`internal/api/rpc.go`、`internal/remote/store.go`、`internal/remote/registry.go`、`internal/config/config.go`、`internal/cli/sync_cmd.go`、`internal/cli/api_cmd.go`，以及新增的 `internal/api/auth.go`、`internal/api/cache.go`、`internal/api/audit.go`、`internal/remote/cached_store.go`、`internal/cli/profile_cmd.go`、`internal/cli/token_cmd.go`、`internal/cli/backend_cmd.go`。
- 影响接口：`pinax api serve` 新增 `--token-file`、`--no-auth`、`--expose`、`--hide` 参数；`pinax sync --target` 接受 profile name；`BlobStore` 接口新增方法（通过嵌入接口保持向后兼容）。
- 影响文档：新增 `docs/interfaces/auth-contract.md`、`docs/interfaces/cache-contract.md`；更新 `docs/interfaces/remote-api-contract.md`。
- 不新增公网 hosted API、非 loopback bind（`--no-auth` 强制 127.0.0.1）、CORS、TLS、多用户权限或 hosted gateway。
- 不改变 note/project board application service 的业务语义。
- 不在 handler/service 层硬编码 SQL；token 存储使用 GORM repository。
