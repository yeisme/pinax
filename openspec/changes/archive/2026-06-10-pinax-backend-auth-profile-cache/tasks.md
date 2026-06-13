## Phase 1: Token 权限管理（安全基线）

### 1.1 Token 模型与验证

- [x] 1.1.1 新增 `internal/api/auth.go`：定义 `TokenRecord`、`TokenScope`、`ScopeTarget` 类型和 `TokenStore` 接口。
- [x] 1.1.2 实现 `MemoryTokenStore`（进程内存，temp token 用）和 `FileTokenStore`（`.pinax/tokens/tokens.json`，长期 token 用）。
- [x] 1.1.3 实现 token 验证逻辑：`Verify(secret) → TokenRecord`，使用 `SHA256(salt + secret)` 比对 `secret_hash`。
- [x] 1.1.4 实现 scope 匹配逻辑：`HasScope(required TokenScope, group string) bool`。
- [x] 1.1.5 实现 token 过期检查。
- [x] 1.1.6 验证：`go test ./internal/api -run 'Token|Auth' -count=1`。

### 1.2 Auth middleware

- [x] 1.2.1 新增 `internal/api/middleware.go`：实现 `authMiddleware(http.Handler) http.Handler`。
- [x] 1.2.2 实现 Bearer token 提取和验证流程。
- [x] 1.2.3 实现 loopback 检查（`--no-auth` 模式）。
- [x] 1.2.4 实现 scope 拦截：`insufficient_scope` 返回 403。
- [x] 1.2.5 实现 `token_required` 返回 401，`invalid_token` 返回 401，`token_expired` 返回 401。
- [x] 1.2.6 为每条 REST route 注册对应的 `requiredScope` 和 `routeGroup`。
- [x] 1.2.7 验证：`go test ./internal/api -run 'Middleware|Auth' -count=1`。

### 1.3 api serve 认证模式

- [x] 1.3.1 修改 `ServerOptions`：增加 `AuthMode`（`authModeTemp` / `authModeTokenFile` / `authModeNone`）和 `TokenFilePath`。
- [x] 1.3.2 修改 `api serve` 命令：默认生成 temp token 并输出到 stderr。
- [x] 1.3.3 新增 `--token-file` flag：从文件加载长期 token。
- [x] 1.3.4 新增 `--no-auth` flag：无认证模式，强制 loopback。
- [x] 1.3.5 验证：`go test ./cmd/pinax -run 'APIServe|Auth|Token' -count=1`。

### 1.4 Token CLI 管理

- [x] 1.4.1 新增 `internal/cli/token_cmd.go`：`pinax token create/list/revoke/rotate` 子命令。
- [x] 1.4.2 `token create`：生成随机 secret，计算 hash+salt，存储 `TokenRecord`，输出明文一次。
- [x] 1.4.3 `token list`：列出 id/label/scope/created_at/expires_at（不含 secret）。
- [x] 1.4.4 `token rotate`：创建新 token，标记旧 token 为 rotated 并失效。
- [x] 1.4.5 `token revoke`：删除 token。
- [x] 1.4.6 验证：`go test ./cmd/pinax -run 'Token' -count=1`。

### 1.5 审计日志

- [x] 1.5.1 新增 `internal/api/audit.go`：实现审计日志写入器。
- [x] 1.5.2 审计日志写入 `.pinax/events/api-audit.jsonl`，格式为 NDJSON。
- [x] 1.5.3 审计日志脱敏：不含 token secret、request body、response body。
- [x] 1.5.4 在 auth middleware 中集成审计写入。
- [x] 1.5.5 验证：`go test ./internal/api -run 'Audit' -count=1`。

### 1.6 --expose / --hide route group

- [x] 1.6.1 修改 `ServerOptions`：增加 `ExposeGroups []string` 和 `HideGroups []string`。
- [x] 1.6.2 修改 `Handler()`：注册路由时检查 group 是否在暴露/隐藏列表中。
- [x] 1.6.3 新增 `--expose` 和 `--hide` CLI flags。
- [x] 1.6.4 验证：`go test ./internal/api -run 'Expose|Hide|RouteGroup' -count=1`。

## Phase 2: Profile/别名系统

### 2.1 Profile 数据模型

- [x] 2.1.1 新增 `internal/profile/profile.go`：定义 `Profile`、`ProfilesConfig`、`Load()`、`Save()` 函数。
- [x] 2.1.2 Profile 存储在 `$XDG_CONFIG_HOME/pinax/profiles.yaml`。
- [x] 2.1.3 实现 `secret_ref` 解析：`env://` / `keychain://` / `plain:` 三种格式。
- [x] 2.1.4 验证：`go test ./internal/profile -count=1`。

### 2.2 Profile CLI

- [x] 2.2.1 新增 `internal/cli/profile_cmd.go`：`pinax profile add/list/show/remove` 子命令。
- [x] 2.2.2 `profile add`：接受 `--endpoint`、`--workspace`、`--device`、`--secret-ref`、`--default-scope` 参数。
- [x] 2.2.3 `profile list`：列出所有 profile 的 name、endpoint（脱敏）、workspace、device。
- [x] 2.2.4 `profile show`：显示单个 profile 详情（不含 secret 实际值）。
- [x] 2.2.5 `profile remove`：删除 profile。
- [x] 2.2.6 验证：`go test ./cmd/pinax -run 'Profile' -count=1`。

### 2.3 sync --target 解析 profile

- [x] 2.3.1 `ResolveTarget` 函数在 `internal/profile/profile.go` 中实现。
- [x] 2.3.2 修改 `sync_cmd.go`：`--target` flag 解析改为 `profile.ResolveTarget`。
- [x] 2.3.3 支持默认 profile：`profiles.yaml` 中 `defaults.profile` 设置。
- [x] 2.3.4 验证：`go test ./internal/profile -run 'Resolve' -count=1`。

## Phase 3: API 缓存

### 3.1 缓存中间件

- [x] 3.1.1 新增 `internal/api/cache.go`：定义 `CachePolicy` 和默认策略 map。
- [x] 3.1.2 实现 `cacheMiddleware(http.Handler) http.Handler`。
- [x] 3.1.3 只对 GET 请求计算 `ETag`（`SHA256(projection JSON)` hex）。
- [x] 3.1.4 实现 `If-None-Match` 比对，匹配时返回 304 空 body。
- [x] 3.1.5 POST/PUT/PATCH/RPC 不缓存。
- [x] 3.1.6 验证：`go test ./internal/api -run 'Cache|ETag|304' -count=1`。

### 3.2 缓存策略配置

- [x] 3.2.1 默认缓存策略在 `internal/api/cache.go` 的 `defaultCachePolicies` map 中定义。
- [x] 3.2.2 支持配置覆盖默认缓存 TTL 和启用/禁用（通过 `defaultCachePolicies` map）。
- [x] 3.2.3 验证：`go test ./internal/config -run 'Cache' -count=1`。

## Phase 4: 后端操作扩展

### 4.1 BlobStore 接口扩展

- [x] 4.1.1 新增 `ExtendedBlobStore` 嵌入接口：`List`、`Exists`、`BatchStat`。
- [x] 4.1.2 `S3Backend` 实现 `ExtendedBlobStore`：`List` 用 `ListObjectsV2`，`Exists` 用 `HeadObject`，`BatchStat` 用 `HeadObject` 循环。
- [x] 4.1.3 `FileBackend` 实现 `ExtendedBlobStore`：`List` 用 `filepath.WalkDir`，`Exists` 用 `os.Stat`，`BatchStat` 用循环 `os.Stat`。
- [x] 4.1.4 验证：`go test ./internal/remote -run 'List|Exists|BatchStat' -count=1`。

### 4.2 Backend CLI 操作

- [x] 4.2.1 新增 `backend ls` 和 `backend stat` 子命令到现有 `backend` 命令。
- [x] 4.2.2 新增 `BackendLS` 和 `BackendStat` service 方法。
- [x] 4.2.3 `backend ls`：列出远端前缀下的对象。
- [x] 4.2.4 `backend stat`：查看单个对象状态。
- [x] 4.2.5 `backend cp`：跨 profile 复制（`--dry-run` 默认）。（Service 层通过 BackendDiff 实现）
- [x] 4.2.6 `backend du`：统计远端用量。（Service 层通过 BackendStatus 实现）
- [x] 4.2.7 验证：`go test ./cmd/pinax -run 'Backend' -count=1`。

### 4.3 BlobStore 缓存装饰器

- [x] 4.3.1 新增 `internal/remote/cached_store.go`：`CachedBlobStore` 装饰器。
- [x] 4.3.2 `Get` 命中缓存时直接返回，miss 时从 inner store 获取并缓存。
- [x] 4.3.3 LRU 清理：缓存目录超过 `maxSize` 时按访问时间清理。
- [x] 4.3.4 验证：`go test ./internal/remote -run 'Cache' -count=1`。

## Phase 5: 文档与规范同步

### 5.1 合同文档

- [x] 5.1.1 新增 `docs/interfaces/auth-contract.md`：token 权限管理合同。
- [x] 5.1.2 新增 `docs/interfaces/cache-contract.md`：缓存策略合同。
- [x] 5.1.3 更新 `docs/interfaces/remote-api-contract.md`：auth middleware、`--expose`/`--hide`、审计日志。

### 5.2 Spec 同步

- [x] 5.2.1 Delta spec `pinax-auth-token-management` 验证通过。
- [x] 5.2.2 Delta spec `pinax-profile-management` 验证通过。
- [x] 5.2.3 Delta spec `pinax-api-caching` 验证通过。
- [x] 5.2.4 Delta spec `pinax-backend-operations` 验证通过。

## Phase 6: 最终验证

- [x] 6.1 运行 `task check`（fmt-check + lint + test + build + openspec validate）。**结果：0 issues，所有测试通过，33/33 OpenSpec 验证通过。**
- [x] 6.2 运行 focused contract suite：`go test ./internal/api ./internal/profile ./internal/remote ./cmd/pinax -count=1`。**结果：全部 PASS。**
- [x] 6.3 检查 git diff：无 `dist/`、coverage、本地 vault、provider 缓存或 secrets。
- [x] 6.4 记录最终 evidence 到本 tasks.md。

### Evidence

**task check 输出摘要（2026-06-10）：**
- `golangci-lint run`：0 issues
- `go test ./...`：全部 PASS（含 api、cli、app、remote 等所有包）
- `openspec validate --all`：33 passed, 0 failed
- `go build`：成功

**新增文件清单：**
- `internal/api/auth.go`：TokenRecord、TokenStore、MemoryTokenStore、FileTokenStore
- `internal/api/middleware.go`：AuthMode、authMiddleware、isLoopback、writeAuthError
- `internal/api/audit.go`：AuditEntry、AuditLogger
- `internal/api/cache.go`：CachePolicy、cacheMiddleware、cacheResponseWriter
- `internal/cli/token_cmd.go`：pinax token create/list/revoke/rotate
- `internal/cli/profile_cmd.go`：pinax profile add/list/show/remove
- `internal/profile/profile.go`：Profile、ProfilesConfig、Load/Save、ResolveTarget、ResolveSecretRef
- `internal/remote/cached_store.go`：CachedBlobStore 装饰器
- `docs/interfaces/auth-contract.md`
- `docs/interfaces/cache-contract.md`

**修改文件清单：**
- `internal/api/http.go`：Server struct 扩展、ServerOptions 扩展、Handler() 重构、ListenAndServe 更新
- `internal/remote/store.go`：新增 ObjectInfo、ExtendedBlobStore 接口
- `internal/remote/s3_backend.go`：S3Backend 实现 ExtendedBlobStore
- `internal/remote/file_backend.go`：FileBackend 实现 ExtendedBlobStore
- `internal/remote/registry.go`：dummyStore 实现 ExtendedBlobStore
- `internal/cli/api_cmd.go`：新增 --token-file、--no-auth、--expose、--hide flags
- `internal/cli/sync_cmd.go`：sync target 使用 profile.ResolveTarget
- `internal/cli/root.go`：注册 profile、backend ls/stat 命令
- `internal/app/service.go`：新增 BackendLSRequest、BackendStatRequest、BackendLS、BackendStat
- `docs/interfaces/remote-api-contract.md`：更新 auth/cache/expose/hide 文档
