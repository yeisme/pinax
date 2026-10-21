# 设计

## 统一 metadata

`pinax.transport_manifest.v1` 保持版本不变，能力和 binding 增加可选字段：

- `execution_location`: `vault_owner`、`remote_service`、`client_local`
- `write_strategy`: `readonly`、`dry_run_then_apply`、`proposal_then_apply`、`snapshot_then_apply`
- `recovery`: `identity_required`、`status_command`、`reconcile_command`
- `owner_action`: 当前 surface 不可执行时的 bounded command template 和原因

字段由现有 `readonly`、`approval_required`、`snapshot_required`、`write_gate`、`local_only_reason` 和 error vocabulary 推导。命令、API routes、OpenAPI、MCP manifest 和 `pkg/pinaxclient.Manifest` 使用同一投影。

## Remote write

- Remote API 默认只读；`--allow-write`、token scope、`yes=true` 或 `dry_run=true` 仍是必要条件。
- `inbox.capture` 和 `folder.rename` 使用 `operation_id`、`idempotency_key`；folder rename 另外绑定 `expected_revision` 和 snapshot gate。
- writer apply 使用 preview digest、caller authorization、snapshot 和 operation receipt。
- ambiguous timeout、5xx 或 malformed 2xx 先查询原 operation，再按 `retryable && replay_safe` 使用原 identity 最多重试一次。
- Remote CLI 不支持的命令继续返回 `remote_command_unsupported`，不回退到本地写入。

## MCP

`pinax mcp serve --vault ./my-notes` 继续只读。MCP 通过 `pinax://manifest`、`pinax://readiness`、`tools/list` 和 resources discovery 返回能力、执行位置和 owner action；不新增 MCP 自我批准工具，不返回 token、绝对路径或正文。

## Verification

新增 profile：

```bash
go run ./tools/testkit/integrationevidence --profile remote-write-parity
```

profile 使用临时 vault、fake HTTP/token 边界和已有 recovery tests，验证 CLI、REST/RPC、OpenAPI、MCP manifest 与 SDK 的核心 metadata 一致。最终门禁仍使用：

```bash
task check
openspec validate --all
```

本 change 不运行 race、不部署、不执行真实远程写入。
