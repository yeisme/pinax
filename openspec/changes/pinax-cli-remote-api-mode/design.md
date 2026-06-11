## Context

Pinax 已有三块相关能力：

1. `internal/app/remote.go` 的 `RemoteCapabilities()` / `RemoteRoutes()` 描述本地 API 的 REST/RPC capabilities。
2. `internal/api/http.go` 暴露 REST 投影路由、root discovery 和 `/v1/capabilities`。
3. `internal/api.RPCDispatcher` 能在 Go 进程内调用 `Pinax.*` RPC method，但没有 HTTP transport。

目标不是新增第二套业务逻辑，而是让普通 CLI 命令在 remote mode 下把已解析的命令参数转换为 RPC method + params，由远端 API 服务继续通过 `app.Service` 执行业务，并返回标准 Projection envelope。

## Design

### HTTP RPC transport

新增：

```text
POST /v1/rpc
```

请求类型：

```go
type HTTPRPCRequest struct {
    ID     string         `json:"id,omitempty"`
    Method string         `json:"method"`
    Params map[string]any `json:"params,omitempty"`
}
```

响应仍是 `domain.Projection`，不是 JSON-RPC 2.0 error envelope。这样 `--json`、`--agent`、默认 human renderer 都继续复用现有输出合同。

### RPC metadata lookup

在 route registry 附近新增 helper：

```go
func FindRemoteRPCMethod(method string) (domain.RemoteRoute, bool)
```

它只查找 `Surface == "rpc"` 且 `RPCMethod == method` 的 route。用途：

- `/v1/rpc` handler 判定 method 是否存在。
- 根据 route metadata 判定 readonly/write、capability、error contract。
- CLI client 或测试验证 capability parity。
- 防止 RPC dispatcher 与 registry 漂移。

### Server handler

`/v1/rpc` handler 只做 transport 层职责：

1. 仅接受 `POST`。
2. 限制并解析 JSON body。
3. 校验 `method` 非空。
4. 用 `FindRemoteRPCMethod` 查找 method metadata。
5. 按 method metadata 做 exposure/auth/write gate 判定。
6. 调用 `RPCDispatcher.Call(ctx, req)`。
7. 将 projection error 映射到 HTTP status。
8. 输出 Projection JSON。

业务逻辑必须继续在 `app.Service`，handler 不直接读写 Markdown、`.pinax/`、SQLite/GORM、Git 或 provider。

### Write gates

远端写入必须同时满足：

- server 启动时使用 `--allow-write`；
- 请求本身提供 `yes=true` 或 `dry_run=true`；
- 对 snapshot-required 的 mutation，仍按现有 service 返回 `snapshot_required`；
- `dry_run=true` 不得写 vault、provider 或远端服务。

### CLI remote mode

新增全局 flag 与环境变量：

```text
--api-url string
--api-token string
--api-token-file string

PINAX_API_URL
PINAX_API_TOKEN
PINAX_API_TOKEN_FILE
```

优先级：explicit flag > env > empty。第一版不写 profile/config，避免引入 token 持久化语义。

当 remote mode 激活：

- 受支持命令将 Cobra 已解析参数转换为 RPC params。
- 受支持命令调用 `internal/remoteapi.Client.Call`。
- 不支持命令返回 `remote_command_unsupported`。
- 显式 `--vault` 与 `--api-url` 同时出现返回 `remote_vault_conflict`。
- 任意远端错误都不允许 fallback 到本地执行。

### Remote client

新增包：

```text
internal/remoteapi/
  client.go
  rpc.go
  capabilities.go
  errors.go
```

核心接口：

```go
type Client struct {
    BaseURL string
    Token   string
    HTTP    *http.Client
}

func (c *Client) Ping(ctx context.Context) (domain.Projection, error)
func (c *Client) Capabilities(ctx context.Context) (domain.Projection, error)
func (c *Client) Call(ctx context.Context, method string, params map[string]any) (domain.Projection, error)
```

客户端约束：

- `BaseURL` 必须是 `http` 或 `https`。
- 默认 HTTP client 必须有 timeout。
- token 只写入请求 header，不写入 error、projection、日志或测试快照。
- 非 2xx 响应仍优先 decode Projection。
- 无法解析 projection 时返回 `remote_api_invalid_response`。

### First supported command set

第一版只支持 registry 已存在的 capabilities：

```text
project.board.show
note.show / note.read
project.item.plan
folder.list/show/create/rename/move/delete/adopt/repair
inbox.list/show/capture/promote/discard
draft.list/show/create/promote/archive/discard
```

`init`、`index rebuild`、`version snapshot`、`git`、`sync`、`cloud`、`backend`、`vault register/use/remote refresh` 和 provider delivery 命令第一版返回 unsupported。

## Risks and mitigations

- 风险：远程失败后误写本地 vault。缓解：remote mode 下不支持命令和远端错误都不得 fallback。
- 风险：RPC handler 绕过 REST auth/write gate。缓解：method metadata lookup + registry/dispatcher parity tests + write gate tests。
- 风险：token 泄漏到输出或 fixture。缓解：client error redaction、server audit/log 不记录 raw body/header、测试覆盖 token 不出现在 stdout/stderr。
- 风险：用户混淆 local API remote mode 与 Cloud/vault remote discovery。缓解：文档明确 `/v1/rpc` 是单 vault local API transport，不提供 `/v1/vaults`。

## Verification

- 服务端：覆盖 `/v1/rpc` happy path、unknown method、bad JSON、readonly/write gate、scope、hidden group、registry parity。
- 客户端：覆盖 URL 校验、Authorization header、非 2xx projection decode、invalid response、unreachable、timeout、redaction。
- CLI：覆盖 `--api-url` 读写、`--vault` 冲突、unsupported 不 fallback、`--json`/`--agent` stdout 合同、token 不泄漏。
- 最终运行 `openspec validate pinax-cli-remote-api-mode --strict`、相关 Go 测试和 `task check`。
