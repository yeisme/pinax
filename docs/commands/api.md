# api 命令

`pinax api` 管理 Pinax 的 REST/RPC projection adapter。它让本地工具、远程 CLI 和 SDK 通过一个明确的 Pinax owner 操作同一座 vault；它不是 Capsa Sync，也不是默认公开到 Internet 的 hosted API。

## 子命令

| 命令 | 用途 | 写入/外部效果 |
| --- | --- | --- |
| `pinax api routes` | 列出 legacy capability/route registry。 | 只读。 |
| `pinax api manifest` | 输出 authoritative transport manifest、真实 binding、schema ref 和 digest。 | 只读。 |
| `pinax api status` | 输出 future client 使用的 workbench status projection。 | 只读。 |
| `pinax api schema export` | 导出由真实 REST binding 生成的 OpenAPI。 | 仅在指定输出路径时写目标文件。 |
| `pinax api serve` | 启动 REST/RPC owner service。 | 启动本地进程；默认只读。 |

先检查 capability、manifest、connection 和六层 readiness：

```bash
pinax api routes --vault ./my-notes --json
pinax api manifest --vault ./my-notes --json
pinax api schema export --format openapi --vault ./my-notes --json
pinax connection inspect --json
pinax connection doctor --json
pinax connection readiness --json
```

`api manifest`、`GET /v1/manifest` 与 `Pinax.Transport.Manifest` 返回同一份 `pinax.transport_manifest.v1` 语义。新客户端应以 manifest 中真实可用的 binding 为准；`api routes` 和旧 capability `surfaces` 字段继续保持原语义，不会因本合同被删除或重解释。

## Owner mode 与 transport

Owner mode 表示谁拥有 vault/domain state；transport 表示如何到达 owner。两者不能混为一个字段。

| Owner mode | 含义 | 常见 transport |
| --- | --- | --- |
| `local-vault` | 当前 Pinax 进程拥有本地 vault。 | `embedded`、`loopback-http`、stdio MCP。 |
| `remote-service` | 一个远程 Pinax owner service 拥有目标 state。 | `https`；未显式声明的旧 non-loopback endpoint 暂按此模式兼容，但 readiness 为 degraded。 |
| `self-hosted-service` | 用户自行部署并运维 Pinax owner service。 | `https`，部署/备份/监控仍由该 owner 证明。 |

远程连接应显式声明 mode：

```bash
pinax --api-url https://pinax.example.test \
  --connection-mode self-hosted-service \
  --api-token-file ~/.config/pinax/owner-api.bearer \
  connection doctor --json
```

也可把 endpoint 与 mode 存为非敏感配置：

```bash
pinax config set remote.api_url https://pinax.example.test --scope user
pinax config set remote.mode self-hosted-service --scope user
pinax connection inspect --json
```

`pinax profile` 仍是 backend/storage compatibility alias，不参与 API connection resolution。

## 启动服务与 token 文件

启动只读 loopback service：

```bash
pinax api serve --vault ./my-notes --readonly --port 8787
```

需要长期 token 时，服务端 canonical 参数是 `--token-store`。它必须指向由 `pinax token` 命令维护的 hashed token registry，例如 vault 内默认 registry：

```bash
pinax token create --label local-agent --scope read,write --groups notes,folders,inbox --vault ./my-notes
pinax api serve --vault ./my-notes --allow-write --port 8787 \
  --token-store ./my-notes/.pinax/tokens/tokens.json
```

服务端旧 `--token-file` 保留为 deprecated compatibility alias，仍然表示同一种 hashed registry；它没有改成读取 plaintext bearer secret。`--token-store` 与服务端 `--token-file` 互斥。

客户端 `--api-token-file` 则只读取一个 plaintext bearer token，并要求 absolute、regular、owner-only `0600` 文件，拒绝 symlink、不安全 ancestor、owner/mode 变化及 open 后 inode 变化。服务端 registry 与客户端 bearer file 必须是两个不同文件：

```bash
pinax --api-url http://127.0.0.1:8787 \
  --api-token-file ~/.config/pinax/owner-api.bearer \
  note list --status active --json
```

不要把 `./my-notes/.pinax/tokens/tokens.json` 传给 `--api-token-file`，也不要把 `~/.config/pinax/owner-api.bearer` 传给服务端 `--token-store`。真实 secret 不得写入仓库、project config、日志、事件、receipt、截图或测试证据。

## REST/RPC 调用

读取 discovery、manifest 与 readiness：

```bash
curl http://127.0.0.1:8787/v1/capabilities
curl http://127.0.0.1:8787/v1/manifest
curl http://127.0.0.1:8787/v1/readiness
curl -X POST http://127.0.0.1:8787/v1/rpc \
  -H 'Content-Type: application/json' \
  -d '{"method":"Pinax.Transport.Manifest","params":{}}'
```

普通 read/plan 命令通过已注册 REST/RPC binding 调用 application service。未注册命令返回 `remote_command_unsupported`，remote CLI 不会静默回退本地执行。`config`、`api`、`token`、`profile`、`vault` 等本地控制命令在 endpoint 仅来自持久化 `remote.api_url` 时仍保持本地。

## Remote mutation 与 operation recovery

首批 recovery-enabled mutation 是 `inbox capture` 与 `folder rename`。CLI 默认在首次提交前生成 opaque `operation_id` 和 `Idempotency-Key`；显式恢复时必须同时复用原 pair：

```bash
pinax --api-url http://127.0.0.1:8787 \
  --api-token-file ~/.config/pinax/owner-api.bearer \
  inbox capture "Idea" --body "Draft note" --yes \
  --operation-id op_example_01 --idempotency-key idem_example_01 --json

pinax --api-url http://127.0.0.1:8787 \
  --api-token-file ~/.config/pinax/owner-api.bearer \
  folder rename notes/old notes/new --yes \
  --operation-id op_example_02 --idempotency-key idem_example_02 \
  --expected-revision sha256:replace-with-current-revision --json
```

`folder rename` 要求 current revision precondition；CLI 在未提供 `--expected-revision` 时会先做只读 preflight，并把得到的 revision 绑定到 mutation。

当 timeout、connection reset、5xx 或 malformed 2xx 导致结果不明时，不要使用新 identity 重复 mutation。先检查并 reconcile 同一个 operation：

```bash
pinax --api-url http://127.0.0.1:8787 \
  --api-token-file ~/.config/pinax/owner-api.bearer \
  operation show op_example_01 --json

pinax --api-url http://127.0.0.1:8787 \
  --api-token-file ~/.config/pinax/owner-api.bearer \
  operation reconcile op_example_01 --json
```

SDK/remote CLI 只有在 durable status 明确为 terminal `failed`，且 `retryable=true`、`replay_safe=true` 时，才会使用完全相同的 operation/idempotency/request binding 最多重试一次。`succeeded`、不可见、non-terminal 或 replay-unsafe operation 都不会 blind retry，也不会 fallback 到本地 mutation。

对应 owner API：

```text
GET  /v1/operations/{operation_id}
POST /v1/operations/{operation_id}:reconcile
RPC  Pinax.Operation.Get
RPC  Pinax.Operation.Reconcile
```

REST `inbox capture` 与 `folder rename` 使用 `X-Pinax-Operation-ID`、`Idempotency-Key` headers；RPC mutation 使用 `operation_id`、`idempotency_key` params，folder rename 另带 `expected_revision`。

## 输出和安全边界

`api manifest`、`connection readiness`、`operation show/reconcile` 支持默认 human summary、`--json`、`--agent`、`--events` 与 `--explain`。machine stdout 只承载对应稳定 envelope/event；诊断写 stderr。所有模式都必须省略 note body、raw request、principal/scope digest、idempotency secret、token 与 absolute host path。

`pinax api serve` 默认绑定 loopback 且默认只读。只有显式 `--allow-write` 才启用受控 mutation；approval、snapshot/revision、scope、operation ledger 和 redaction gate 仍然生效。`--no-auth` 只能用于强制 loopback 的受控环境。

Capsa Sync 是独立链路。使用 [`capsa`](./capsa.md) 与 [`sync`](./sync.md) 管理多个本地 vault 的 encrypted revision convergence。

另见 [Remote API Contract](../interfaces/remote-api-contract.md)、[Auth Contract](../interfaces/auth-contract.md)、[Client CLI Parity and Realtime Sync](../interfaces/client-cli-parity-and-sync.md)、[`token`](./token.md) 与 [`mcp`](./mcp.md)。
