---
name: owner-remote-cli
description: Use when setting up, diagnosing, or documenting an owner-hosted remote CLI connection (remote connect / remote doctor / remote profile / client access key) for any Yeisme domain CLI, including Eikona, Scaena, Sonora, Anatomia, Auctra, Pinax, DriveBridge, or manju-hotlist. Covers the unified contract, auth levels, REMOTE_* error recovery, and the no-local-CLI client path. 使用场景：给另一台电脑配远程 CLI、签客户端 key、排 401/403、判断某项目是否适用远程 CLI 契约。
---

# Owner-hosted 远程 CLI 统一契约操作

一台 owner 主机配好服务后，其他电脑复用该域能力而不持有 provider key。统一的是**契约形状**（secret-free sidecar、命令族、鉴权分级、错误码、slice、doctor），不是网关、元 CLI 或凭据池。权威规格：`openspec/changes/owner-remote-cli-contract-v2/`；契约正文：仓库根 `docs/architecture/owner-remote-cli-contract.md`。

## 先判断适用性

不要给每个项目都套 `remote connect`。先读能力账本（`docs/architecture/owner-remote-cli-contract.md` 的「能力账本」）判 `fit / split-owner / reject-now`：

- `fit`（域 owner，如 Eikona / Scaena / Sonora / Anatomia / Auctra / Pinax / DriveBridge / manju-hotlist）：可进入接线闭环；其中 Anatomia、Auctra 等尚未交付 `remote` 命令族，**不得**把命令写成当前可执行。
- `split-owner`（网关/平台、纯消费端）：只消费公开合同，不新增第二套 auth、不把 domain state 搬进客户端。
- `reject-now`（本地工具/库、无 owner-hosted HTTP 面的 adapter）：明确写「本契约不适用 + 重新进入条件」，不伪造支持。

用户明确要求的「全部纳入」是 `required` 能力：逐项判定并保留，不静默删除、不永久降级。

## 七步闭环

1. 读 capability / scope / project / 版本 / readiness。
2. zero-write preflight（health + 授权探针），取得 plan digest 与 blockers。
3. 展示目标、数量、输入版本、成本状态与影响范围。
4. 用户确认后以稳定幂等键提交。
5. 用 status 或事件游标观察原 operation。
6. 结果未知时只 reconcile 原 operation，**不重投**。
7. 经 owner review/adopt 后才进入固定版本或交付包。

接线顺序与真实命令见 `docs/workflows/owner-remote-cli-onboarding.md`。只使用当前确实存在的子命令；不含 `remote` 的旧安装版先升级。

## 鉴权分级（必须显式声明）

| Level | 形态 | 约束 |
| --- | --- | --- |
| L0 | loopback 本地静态 token | 仅 dev/fixture，禁 production / service-profile |
| L1 | 明文 LAN private-beta | IP literal + 显式端口、精确 Host、RFC1918 peer、忽略 forwarded headers、purpose/scope Bearer、有界请求；TLS/public/production readiness 恒 `degraded|blocked` |
| L2 | scoped access-key 台账 | hash-only、issue/list/revoke/rotate、私密渠道交付到新建 0600 文件 |
| L3 | production TLS | 独立 gate/change，未承诺 |

三类凭据严格分离：client access key / workload identity / provider credential。L0/L1 绝不标 production-ready；health 200、fixture、canary 不等于 provider 或真实远程可用。

## `REMOTE_*` 错误恢复

| Code | 恢复动作 |
| --- | --- |
| `REMOTE_PROFILE_NOT_CONFIGURED` | 补齐 `connect` 的 endpoint/key 对 |
| `REMOTE_PROFILE_INVALID` | 绝对 key 路径 + 显式 `--allow-private-http` |
| `REMOTE_PROFILE_UNSUPPORTED_COMMAND` | 查该域 slice 或用 `--no-remote` |
| `REMOTE_INPUT_REQUIRES_UPLOAD` | 先 `upload` 取 owner 侧引用（`eikona://` / `sonora://`），不传客户端本地路径 |
| `REMOTE_SERVICE_UNREACHABLE` | 检查 endpoint / 网络 / unit；可重试 |
| `REMOTE_OPTION_UNSUPPORTED` | 去掉无法保真的 flag，**零 HTTP 提交** |
| `REMOTE_OUTCOME_UNKNOWN` | 保留 operation ref，只回查/reconcile，**绝不重投** |
| `REMOTE_KEY_REJECTED` (401) | owner 轮换 key |
| `REMOTE_KEY_PURPOSE_DENIED` (403) | owner 签发匹配 purpose 的 key |

## 无本机 CLI 的客户端

遵循 `docs/workflows/mcp-client-without-cli.md`：从 tools/list、inputSchema 与可读资源取参数与恢复合同；CLI help/doctor/capabilities 只是已安装客户端的可选路径。区分 MCP 控制面、客户端文件访问与 HTTP 数据传输；不把客户端本地路径当远端可读文件，不虚构上传动作，不把 planned/readonly 升级为可写。

## 硬红线

- 不建统一网关、不建 `yeisme remote` 元 CLI、不合并多域凭据。
- 凭据 / Authorization / token / 明文 key 不进仓库、文档、日志、stdout、trace、证据、聊天；明文只写新建 0600 私有文件并经私密渠道交付。
- 不删/不重命名既有命令、flag、错误码、purpose；兼容 ≥2 个 minor release + rollback。
- 不把 inventory、planned 能力冒充可执行；不把低层可达性升级高层 readiness。
- 不替用户签发/轮换凭据、不改 systemd unit、不重启实时服务、不部署；这些属高影响动作，逐项授权后执行。
- 不运行 race 检测；不以根级静态检查代替部署机真实验收。

## 验证命令

```bash
openspec validate owner-remote-cli-contract-v2 --strict --no-interactive
scripts/openspec.sh validate
scripts/skills.sh validate-custom
scripts/skills.sh validate-profiles
scripts/skills.sh validate-runtime
```
