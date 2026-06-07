# Proposal: Pinax Backend Provider CLI

## Why

当前 `pinax backend` 不存在，用户运行时会得到 `unknown command "backend" for "pinax"`。Pinax 已有 `storage set-local/set-s3/status/doctor` 和 `sync diff/push/pull --target git|s3|cloud`，但这会把“后端配置”“provider 能力诊断”“同步计划”“外部 CLI 依赖”分散在多个命名空间里。随着 S3、rclone、OneDrive 和未来 Pinax Cloud 增加，用户和 Agent 需要一个统一入口来回答：当前 vault 接了哪些 backend、每个 backend 能做什么、凭据从哪里来、下一步能不能 pull/push、失败原因是什么。

本 change 设计 `pinax backend` 子命令，作为 Pinax CLI 内的统一后端接口交互层。它不把 Pinax 改成云笔记，也不让远端成为笔记真源；本地 Markdown vault 仍是用户资产真源。`backend` 只管理 provider profile、capability probe、doctor、sync plan 和受控执行入口。

## What Changes

- 新增 `pinax backend` 命名空间，统一管理 `local`、`s3`、`rclone`、`onedrive` 和未来 `pinax-cloud` provider profile。
- 将现有 `pinax storage` 作为兼容别名或过渡命令保留，并在帮助文案中引导到 `pinax backend`。
- 定义 provider contract：每个 backend 必须提供 profile schema、capabilities、doctor、diff、pull plan、push plan、apply gate 和 redaction policy。
- S3 首期继续只保存 profile 和诊断，不保存 access key/secret；真实网络动作通过后续 adapter 和 fake/MinIO 测试落地。
- rclone 首期作为外部 CLI-backed provider：通过 `rclone config show`、`rclone lsjson`、`rclone copy --dry-run` 等 fake executable 测试 capability，不直接保存 remote secret。
- OneDrive 首期不直接接 Microsoft Graph；默认通过 rclone remote 类型 `onedrive` 接入，避免在 Pinax 内新增 OAuth/token 生命周期。
- 所有 `backend` 输出遵守 AI-native CLI 输出合同：默认中文摘要，`--agent` key=value，`--json` envelope，`--events` NDJSON，`--explain` 中文可审查摘要。
- 结构化资产由 CLI/service 写入，例如 `.pinax/backends.json`、backend event、sync receipt、conflict queue；Agent 不直接手写 metadata。

## Non-Goals

- 不实现 Microsoft Graph 原生 OneDrive SDK、OAuth 授权服务或长期 token refresh daemon。
- 不在 MVP 执行真实公网上传/下载；第一阶段落地 profile、doctor、capability 和 dry-run plan。
- 不把远端 backend 作为 vault 真源；本地 `notes/**/*.md` 仍可在无网络、无 provider credential 时完整使用。
- 不让 `pinax init`、`note`、`search`、`index`、`template`、`metadata`、`organize`、`git` 或只读 `mcp` 隐式依赖 backend。
- 不保存 S3 secret、rclone config secret、OneDrive token、Authorization header、raw provider payload 或完整外部命令输出到 stdout、事件、fixture 或 Git。
- 不绕过 application service 直接写 `.pinax/*.json`、event JSONL、sync-state 或 receipt。

## Impact

- CLI 入口：`cmd/pinax/main.go` 增加 `backend` 命令树，`storage` 进入兼容/迁移状态。
- 应用层：`internal/app` 增加 backend profile、doctor、capability、sync plan service。
- 领域层：`internal/domain` 增加 backend kind、profile、capability、operation plan、risk、credential source 类型。
- 输出层：`internal/output` 复用 projection renderer，增加 backend command contract tests。
- 脱敏层：`internal/redaction` 扩展 provider secret、remote URL、外部 CLI payload 脱敏。
- 测试：Go unit + command tests + testscript e2e，使用 fake rclone/fake provider/fake vault，不依赖真实公网或真实 token。

## Handoff

实现前先验证本 change：

```bash
cd cli/pinax
openspec validate pinax-backend-provider-cli --strict
```

实现完成后至少运行：

```bash
cd cli/pinax
task check
```

没有安装 `task` 时运行：

```bash
cd cli/pinax
gofmt -w cmd internal
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
```
