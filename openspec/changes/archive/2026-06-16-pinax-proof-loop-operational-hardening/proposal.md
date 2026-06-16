# Proposal: Pinax Proof Loop Operational Hardening

## Why

`pinax-agent-safe-proof-loop` 已证明 Capture → Retrieve → Diagnose → Plan → Apply safely 主路径可运行，但 review 发现四个运营级缺口：

1. `version restore --plan` 只能生成恢复计划，没有 `restore apply --yes` 写路径，坏 apply 后不能通过 CLI 安全回滚。
2. proof loop 没有单一 driver/run id，agent 每次需要从文档重新拼接命令序列。
3. 脱敏边界分散在各命令投影和 testkit evidence runner 中，缺少统一 projection redaction gate。
4. `--events` 与 `--explain` proof-loop contract 覆盖不完整。

本变更把 proof loop 从“已测试的一组命令”升级为“可逆、可调用、可审计的一条本地 agent 工作流”。

## What Changes

- 新增 `pinax version restore apply --yes` 或等价 apply command，基于已生成 restore plan 执行安全恢复。
- 新增统一 projection redaction gate，覆盖 summary/json/agent/events/explain 与 evidence sidecar。
- 新增 `pinax proof loop run` 作为 orchestration command，生成 `proof_loop_run_id`、阶段 receipts、evidence 和 next actions。
- 扩展 proof-loop contract tests：所有 proof-loop stage 覆盖 default、`--json`、`--agent`、`--events`、`--explain`，并递归扫描 note body、token、Authorization、provider payload 泄漏。
- 更新 docs，让用户和 agent 只需从一个主路径进入 proof loop。

## Non-Goals

- 不实现自动无审批 apply。
- 不让 repair/organize 自动处理 manual-review-only 的 link/orphan/attachment 场景。
- 不引入 cloud sync、provider、briefing 或 MCP 写能力。
- 不把 SQLite index 变成 truth source。

## Impact

- `cmd/pinax`: 新 command wiring、output mode tests。
- `internal/app`: restore apply use case、proof loop orchestration service。
- `internal/output` / `internal/redaction`: shared redaction gate。
- `internal/testkit`: integration evidence runner redaction assertions。
- `tests/e2e`: proof loop run and restore apply e2e.
- `docs/commands/*` and README: proof loop entry path.
