## Why

`pinax api serve --allow-write --no-auth --port 8787 --vault /tmp/pinax-notes` 已经能为一个本地 vault 暴露 REST 投影服务，但另一个 `pinax` 进程目前只能手动用 HTTP 调用接口。普通 CLI 命令仍直接调用本地 `app.Service` 并读取本地 `--vault`，无法显式连接已经启动的 Pinax API 服务。

这会阻塞两个场景：

- 本地前后端分离测试：一个进程持有 `/tmp/pinax-notes`，另一个 CLI 作为客户端验证读取和受控写入。
- Agent/脚本复用 Pinax CLI：希望继续使用 `pinax folder list --json`、`pinax inbox capture --yes` 这类普通命令形状，而不是手写 `curl` 路由。

当前代码中已有可复用基础：`internal/app/remote.go` 维护 REST/RPC capability registry，`internal/api.RPCDispatcher` 已经把 `Pinax.*` RPC method 映射到 `app.Service`，但 HTTP server 尚未暴露 `/v1/rpc`，CLI 也没有 `--api-url` remote mode。

## What Changes

- 为 `pinax api serve` 新增 `POST /v1/rpc`，请求使用 Pinax 轻量 RPC envelope，响应仍是现有 `domain.Projection`。
- 新增 CLI remote mode：当 `--api-url` 或 `PINAX_API_URL` 存在时，受支持的普通命令通过 HTTP RPC 转发到 API 服务。
- 新增远程客户端包，负责 base URL 校验、Bearer token、timeout、非 2xx projection 解码和脱敏错误。
- 首批只接入已有 local API capabilities：project board、note read、project item plan、folder、inbox、draft。
- remote mode 下不支持的命令必须失败，不允许静默 fallback 到本地 vault。
- remote mode 下显式 `--vault` 与 `--api-url` 冲突，第一版返回 `remote_vault_conflict`。

## Non-goals

- 不把 `pinax api serve` 改造成多 vault Cloud 控制面。
- 不实现 `/v1/vaults` discovery，也不复用 `vault remote refresh` 作为本地 API 连接方式。
- 第一版不远程化 `init`、`index rebuild`、`version snapshot`、`git`、`sync`、`cloud`、`backend`、`vault register/use/remote refresh` 或 provider delivery 命令。
- 不把 `--vault` 重新解释为远程 selector。
- 不保存 raw token、Authorization header、cookie、provider payload 或完整 note body 到 stdout、stderr、日志、fixture 或 structured asset。

## Impact

- 受影响包：`internal/api`、`internal/app`、`internal/cli`，新增 `internal/remoteapi`。
- 受影响文档：`docs/interfaces/remote-api-contract.md` 和相关命令说明。
- 输出合同：默认 human、`--json`、`--agent` 继续来自同一个 Projection；remote mode 不是新输出模式。
- 安全边界：服务端 `--allow-write` 与请求级 `yes=true`/`dry_run=true` 仍是写入 gate；客户端不得在远端失败后本地执行。
