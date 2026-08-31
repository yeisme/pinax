# Token 与 credential contract

## 服务端认证模式

`pinax api serve` 支持三种 additive auth composition：

| 模式 | 参数 | 内容与边界 |
| --- | --- | --- |
| Temp token | 默认 | secret 只存在于进程内，只在启动 stderr 输出一次，进程退出后失效。 |
| Token store | `--token-store <path>` | canonical 长期认证输入；读取 hashed token registry。 |
| No auth | `--no-auth` | 不验证 bearer，但强制 loopback；non-loopback 返回 `loopback_required`。 |

服务端旧 `--token-file <path>` 至少保留两个 minor release，作为 `--token-store` 的 deprecated compatibility alias。两者读取完全相同的 hashed registry，互相排斥，也都与 `--no-auth` 排斥。使用旧 alias 时，human stderr 输出一次不含实际 path 的迁移 warning；JSON/agent/events stdout 不被 warning 污染。

Canonical 启动示例：

```bash
pinax token create --label local-agent --scope read,write --groups notes,folders,inbox --vault ./my-notes
pinax api serve --vault ./my-notes --allow-write --port 8787 \
  --token-store ./my-notes/.pinax/tokens/tokens.json
```

`.pinax/tokens/tokens.json` 是 CLI-authored structured asset，只保存 salted hash、scope、label、expiry、rotation 与 audit metadata，不保存 plaintext secret。不要手写或用客户端 bearer file 替换它。

## 客户端 bearer source

客户端 credential source 与服务端 token store 是两类不同资产：

| 参数/变量 | 内容 | 典型用途 |
| --- | --- | --- |
| `--api-token-file` / `PINAX_API_TOKEN_FILE` | 只含一个 plaintext bearer token 的 secure local file。 | 推荐的长期本地 client source。 |
| `--api-token` / `PINAX_API_TOKEN` | 进程级 plaintext bearer。 | CI、临时 automation、兼容调用；不持久化到 Pinax config。 |

`--api-token-file` 由 `pkg/pinaxclient` 按 secure file contract 读取：

- path 必须是 absolute path，目标必须是 regular file；拒绝 symlink。
- Unix-like 系统要求 current-user owner 与精确 owner-only `0600` mode。
- ancestor 不得是会让其他用户替换目标的 unsafe writable directory。
- 打开文件后重新检查 owner、mode、device/inode 与目标 identity，阻止 check/use 间替换。
- token 读取后去掉首尾空白；空值和 oversized value 被拒绝。
- 不支持 Unix owner/mode 的平台必须使用等价 owner-only policy，不能静默放宽为 world-readable。

调用示例：

```bash
pinax --api-url http://127.0.0.1:8787 \
  --api-token-file ~/.config/pinax/owner-api.bearer \
  connection doctor --json
```

服务端 hashed registry 与客户端 plaintext bearer file 必须分开：

```text
server: ./my-notes/.pinax/tokens/tokens.json
client: ~/.config/pinax/owner-api.bearer
```

禁止把 server registry 传给 `--api-token-file`，也禁止把 client bearer file 传给 `--token-store` 或旧服务端 `--token-file`。

## Token model 与 scope

Server registry 中每条 record 至少保存以下 bounded fields：

```go
type TokenRecord struct {
    ID          string
    SecretHash  string
    Salt        string
    Scope       map[TokenScope]ScopeTarget
    Label       string
    CreatedAt   string
    ExpiresAt   string
    LastUsedAt  string
    RotatedFrom string
    CreatedBy   string
}
```

Stable scope 包括：

| Scope | 含义 |
| --- | --- |
| `read` | 允许目标 route groups 的只读 route/RPC。 |
| `write` | 允许目标 route groups 的 mutation，但仍需 `--allow-write`、approval、snapshot/revision 与 operation gate。 |
| `admin` | token management 等明确管理操作。 |

`ScopeTarget.Groups` 和 `ScopeTarget.Actions` 为空表示对应维度不再缩小；准确 route groups 与 required scope 以 `pinax api manifest --json` 和 route registry 为准。

Verification flow：

1. 读取 `Authorization: Bearer <secret>`。
2. 对每条 record 计算并 constant-time 比较 salted hash。
3. 检查 expiration。
4. 解析 route group 与 required scope。
5. 验证 group/action target。
6. 绑定 operation principal/scope digest，但不输出或记录 secret。
7. 写 bounded audit entry 并允许请求。

## Token lifecycle

```bash
pinax token create --label my-agent --scope read,write --groups notes,folders --expires 30d --vault ./my-notes
pinax token list --vault ./my-notes --json
pinax token rotate <token-id> --label my-agent-v2 --vault ./my-notes
pinax token revoke <token-id> --vault ./my-notes --json
```

`create`/`rotate` 的 plaintext secret 只在交互 human output 交付一次；machine projection 只报告 `secret_delivery=interactive_only`，避免 secret 进入 automation evidence。用户应把一次性 secret 保存到 user-level secret store 或 owner-only client token file，不得写 shell credential script 或仓库文件。

## Audit、错误与脱敏

API audit 写入 `.pinax/events/api-audit.jsonl`，只记录 bounded token id、method、route/group、scope、status 与时间。它不得包含 token secret、Authorization/Cookie、request/response body、idempotency key、provider payload 或 absolute credential path。

| 场景 | HTTP status | error code |
| --- | --- | --- |
| 缺少 bearer | `401` | `token_required` |
| token 校验失败 | `401` | `invalid_token` |
| token 已过期 | `401` | `token_expired` |
| scope 不足 | `403` | `insufficient_scope` |
| `--no-auth` 接收 non-loopback | `403` | `loopback_required` |

所有 human、JSON、agent、events、explain、日志、fixture、截图与 integration evidence 都只能显示 credential source type、env var name、redacted digest 或 user-level path category，不能显示真实 token、Authorization header 或完整 credential path。
