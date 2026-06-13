## Context

Pinax 本地 API 是 REST/RPC projection adapter，当前只绑定 `127.0.0.1`，认证模型是 `ServerOptions{AllowWrite: bool}` 的全局开关。后端连接参数（endpoint、workspace、device、secret-ref）通过 CLI flags 传递，无法命名复用。`BlobStore` 接口只有 Get/Put/Stat/Delete，缺少 List/batch 语义。API 层无缓存、无审计、无 token 管理。

本变更在这四个维度上扩展，保持 Pinax "本地优先、CLI 短生命周期进程" 的定位不变。

## Goals / Non-Goals

**Goals:**

- 引入全局 profile/别名系统，让 sync/api 命令通过名字引用后端配置。
- 实现 temp token + 长期 token 双层认证体系，scope 细粒度到 route group。
- 为只读 API 路由添加 Cache-Control / ETag / 304 语义。
- 扩展 BlobStore 接口，增加 List/Exists/BatchStat。
- 提供 backend ls/cp/stat/du CLI 操作。
- 所有新增能力遵循 ai-native-cli-output-contract。

**Non-Goals:**

- 不实现公网 hosted API、非 loopback bind（`--no-auth` 模式强制检查 RemoteAddr）、CORS、TLS。
- 不实现多用户系统、用户身份、RBAC/ABAC。Token 只存 scope，不存身份。
- 不引入 Redis、memcached 或外部缓存服务。缓存用进程内 `sync.Map` + TTL 清理。
- 不让 token secret 明文落盘。只存 hash（SHA256 + salt）。
- 不改变 application service 的业务逻辑。Auth/caching 是 transport 层 concern。
- 不做 note 级 ACL。权限边界落在 vault 级和 route group 级。

## Decisions

### D1: Profile 存全局，不进 vault

Profile 配置存在 `~/.pinax/profiles.yaml`（或 `$XDG_CONFIG_HOME/pinax/profiles.yaml`），不进 vault 的 `.pinax/`。

**理由**：Profile 描述的是"如何连接远端"，属于用户环境而非 vault 数据。同一用户可能用不同 profile 访问不同 vault，也可能多个 vault 共用同一个远端 backend。Vault 删除时 profile 不应消失。

**配置格式**：

```yaml
# ~/.pinax/profiles.yaml
profiles:
  local:
    endpoint: "file:///home/user/.pinax/store"
    workspace: default
    device: workstation

  my-s3:
    endpoint: "s3://my-bucket?region=us-east-1&prefix=pinax"
    workspace: default
    device: laptop
    secret_ref: "env://PINAX_S3_SECRET"
    default_scope: readonly

  team-shared:
    endpoint: "s3://team-vault?region=ap-southeast-1"
    workspace: engineering
    device: user-device
    secret_ref: "keychain://pinax/team-shared"
    default_scope: read+folders:write

defaults:
  profile: local
  write_profile: ""
```

**CLI**：

```bash
pinax profile add my-s3 --endpoint s3://my-bucket --workspace default
pinax profile list
pinax profile show my-s3
pinax profile remove my-s3
```

**与现有代码的关系**：`sync_cmd.go` 的 `--target` flag 解析逻辑从硬编码字符串改为 `remote.ResolveTarget(target)` —— 如果是 profile name 则从 profiles.yaml 加载；如果是 URI scheme 则直接使用。

### D2: 双层 token——temp（默认）+ 长期（opt-in）

**Temp token**（默认行为）：

- `pinax api serve` 启动时自动生成，存储在进程内存的 `map[string]TokenRecord]`。
- Secret 只输出到 stderr 一次，不写文件。
- 进程退出 token 失效。
- Scope 默认为请求方可以访问所有已暴露的 route group。
- 适用场景：人工 dashboard 使用、SSH tunnel 临时分享。

**长期 token**（`--token-file` 模式）：

- 预先通过 `pinax token create` 生成，存储到 `.pinax/tokens/tokens.json`（vault 级别）。
- 支持自定义 scope、label、过期时间。
- 支持 `pinax token rotate`（创建新 token 并标记旧 token 的 `rotated_from` 链）。
- 支持 `pinax token revoke`。
- Secret 只在 `token create` 时输出一次明文；文件中只存 `SHA256(salt + secret)`。
- 适用场景：agent/MCP 长期连接、CI/CD 自动化。

**无认证模式**（`--no-auth`）：

- 不验证任何 token。
- 强制检查 `RemoteAddr` 是 loopback，非 loopback 请求直接 403。
- 适用场景：纯本地开发/调试。

### D3: Token 数据模型

```go
// internal/api/auth.go

type TokenScope string

const (
    ScopeRead  TokenScope = "read"   // 所有 GET 路由
    ScopeWrite TokenScope = "write"  // 所有 mutation 路由
    ScopeAdmin TokenScope = "admin"  // token 管理本身
)

type ScopeTarget struct {
    Groups  []string  // ["folders", "notes", "inbox", "drafts", "projects", "capabilities"]
    Actions []string  // ["list", "read", "create", "update", "delete"]
}

type TokenRecord struct {
    ID          string                      `json:"id"`            // pt_<hex16>
    SecretHash  string                      `json:"secret_hash"`   // SHA256(salt + secret)
    Salt        string                      `json:"salt"`          // 随机 hex
    Scope       map[TokenScope]ScopeTarget  `json:"scope"`
    Label       string                      `json:"label"`
    CreatedAt   string                      `json:"created_at"`
    ExpiresAt   string                      `json:"expires_at,omitempty"`
    LastUsedAt  string                      `json:"last_used_at,omitempty"`
    RotatedFrom string                      `json:"rotated_from,omitempty"`
    CreatedBy   string                      `json:"created_by"`    // "auto" | "manual" | "rotate"
}
```

**验证流程**：

```
请求 → 提取 Bearer token → SHA256(salt + token) 比对 secret_hash
     → 检查 ExpiresAt → 查 scope 是否覆盖当前 route group + action
     → 记录 LastUsedAt → 写审计日志 → 放行
```

**Scope 匹配规则**：

- `ScopeRead` + `Groups: ["notes", "folders"]` → 允许 GET `/v1/notes/` 和 GET `/v1/folders`
- `ScopeWrite` + `Groups: ["inbox"]` + `Actions: ["create"]` → 允许 POST `/v1/inbox:capture`
- 空 `Groups` 表示所有 group；空 `Actions` 表示所有 action

### D4: Auth middleware 是 transport 层 concern

Auth middleware 在 `api.Server.Handler()` 的 mux 外面包一层，不侵入 handler 逻辑：

```go
func (s *Server) Handler() http.Handler {
    mux := http.NewServeMux()
    // ... 注册路由不变 ...
    return s.authMiddleware(s.cacheMiddleware(mux))
}
```

- `authMiddleware`：验证 token → 检查 scope → 写审计日志 → 调用 next handler。
- `cacheMiddleware`：只对 GET 请求设置 Cache-Control / ETag / 304。
- 两个 middleware 都是 `func(http.Handler) http.Handler` 形式，可独立测试。

### D5: 缓存策略从 route registry 派生

```go
var defaultCachePolicies = map[string]CachePolicy{
    "/v1/capabilities": {MaxAge: 300, Scope: "public"},
    "/v1/folders":      {MaxAge: 60,  Scope: "private"},
    "/v1/folders/":     {MaxAge: 60,  Scope: "private"},
    "/v1/notes/":       {MaxAge: 30,  Scope: "private"},
    "/v1/inbox":        {MaxAge: 10,  Scope: "private"},
    "/v1/inbox/":       {MaxAge: 10,  Scope: "private"},
    "/v1/drafts":       {MaxAge: 10,  Scope: "private"},
    "/v1/drafts/":      {MaxAge: 10,  Scope: "private"},
    "/v1/projects/":    {MaxAge: 30,  Scope: "private"},
}
```

- ETag 使用 projection JSON 的 SHA256 hex。
- `If-None-Match` 匹配时返回 304 空 body。
- POST/PUT/PATCH/RPC 不缓存。
- 缓存策略可通过 config 覆盖：`api.cache.policies` 配置段。
- **不用 sync.Map**：直接用 `http.ServeMux` 的 path matching + handler 内部计算 ETag，无需进程级缓存存储。真正的缓存发生在外部（浏览器、HTTP 客户端、反向代理）。

### D6: BlobStore 向后兼容扩展

使用 Go 接口嵌入保持向后兼容：

```go
type BlobStore interface {
    Get(ctx context.Context, key string) (data []byte, rev string, err error)
    Put(ctx context.Context, key string, data []byte, baseRev string) (newRev string, err error)
    Stat(ctx context.Context, key string) (rev string, err error)
    Delete(ctx context.Context, key string) error
}

type ExtendedBlobStore interface {
    BlobStore
    List(ctx context.Context, prefix string) ([]ObjectInfo, error)
    Exists(ctx context.Context, key string) (bool, error)
    BatchStat(ctx context.Context, keys []string) (map[string]string, error)
}

type ObjectInfo struct {
    Key         string
    Size        int64
    Revision    string
    LastModified time.Time
}
```

现有 `S3Backend` 和 `FileBackend` 升级实现 `ExtendedBlobStore`。`registry.go` 的工厂函数签名不变，返回类型从 `BlobStore` 改为 `ExtendedBlobStore`。`sync` 模块通过类型断言或 capabilities probe 检查是否支持扩展操作。

### D7: 审计日志格式

```jsonl
{"ts":"2026-06-10T12:34:56Z","token_id":"pt_a8f3c2","method":"GET","path":"/v1/notes/note-001","scope":"read","group":"notes","status":200}
{"ts":"2026-06-10T12:34:57Z","token_id":"pt_b1e4d5","method":"POST","path":"/v1/folders?path=new-folder","scope":"write","group":"folders","status":202}
```

- 写入 `.pinax/events/api-audit.jsonl`。
- 不记录 request body、response body、token secret。
- 只在 auth middleware 层写入，不侵入 handler。
- `--no-auth` 模式也记录（使用 `token_id: "no-auth"`）。

### D8: `--expose` / `--hide` route group 过滤

```bash
pinax api serve --expose notes,inbox        # 只暴露这两个 group
pinax api serve --hide drafts,projects       # 隐藏这些 group
```

实现方式：在 `ServerOptions` 中增加 `ExposeGroups []string` 和 `HideGroups []string`。`Handler()` 注册路由时检查 group 是否在允许列表中。不在列表中的路由返回 `route_not_found`（与现有行为一致）。

### D9: Profile secret_ref 解析

`secret_ref` 支持三种格式：

| 格式 | 解析方式 | 示例 |
|------|----------|------|
| `env://VAR_NAME` | `os.Getenv("VAR_NAME")` | `env://PINAX_S3_SECRET` |
| `keychain://service/account` | 调用系统 keychain（macOS Keychain / Linux secret-service） | `keychain://pinax/team-shared` |
| `plain:text` | 直接使用（仅用于测试） | `plain:test-secret` |

`plain:` 前缀在非测试环境下打印 warning。实际 secret 值不存入 profiles.yaml、不输出到 stdout/stderr/events。

## Risks / Trade-offs

- **Risk: BlobStore 接口扩展破坏第三方实现。** → 使用嵌入接口 `ExtendedBlobStore`，现有 `BlobStore` 不变。第三方代码继续实现 `BlobStore` 即可，扩展操作通过类型断言可选使用。
- **Risk: Token 文件权限泄露。** → `.pinax/tokens/tokens.json` 创建时设置 0600 权限。启动时检查文件权限，非 0600 打印 warning。
- **Risk: 审计日志膨胀。** → 审计日志追加写入，不自动轮转。用户可通过 `pinax vault doctor` 检查大小并手动清理。后续可加日志轮转。
- **Risk: ETag 计算开销。** → SHA256(projection JSON) 对于 Pinax API 的响应量（通常 < 100KB）开销可忽略。如果后续出现性能问题可改用 xxHash。
- **Risk: Profile YAML 与 Viper 合并复杂度。** → Profile 独立于 Viper 配置链，用独立的 `profiles.Load()` 函数加载。不与现有 config 耦合。
- **Risk: `--no-auth` + 非 loopback 绕过。** → `--no-auth` 模式下 handler 强制检查 `r.RemoteAddr` 是 loopback（`127.0.0.1` 或 `[::1]`），非 loopback 直接返回 403。
