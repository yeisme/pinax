# pinax-agent-note-proposal-v1 Design

## Context

[Agent 优化设计](../../../docs/product/agent-optimization-design.md) 定下六条线，本 change 落 §3.2（note 域 propose-only）与 §3.5 的指标挂钩。现状：proof loop 已是唯一安全写入路径（plan→snapshot→apply→receipt→restore）；`RejectHandwrittenMetadata` 已在手写元数据边界 fail-closed；`pinax-agent-memory-runtime` 已实现 proposal 基型、policy、review 应用服务与 `pinax agent` facade（该 change 活跃，CLI 树已合并 develop）。缺的是把这些纪律延伸到 note 域——不 fork 实现，只加 note payload 与 note 特有的版本钉住语义。

## Goals / Non-Goals

**Goals**

- Agent 经 MCP/CLI 提出 note 域变更，apply 全部走 proof loop，产出可审计 receipt。
- 中风险变更必须用户确认；低风险白名单 auto-apply 有 receipt 与回滚路径。
- note_edit 的并发安全用 expected revision 钉住，漂移显式拒绝并给下一步。
- 审阅体验按 [交互体验规格](../../../docs/product/rendered-preview-interaction-ux.md) §7：list→show diff→approve，四输出模一致。

**Non-Goals**

- 不实现 agent 自主审批（"agent 批 agent"不存在）。
- 不改 dsh-pane gated actions、不动 memory-runtime 合同、不做配置化 auto-apply 白名单。
- 不做 workbench/pane 图形审阅面（后续）。

## Architecture

```text
MCP propose_note_* (capability-gated) / CLI propose
        ▼
internal/app/agent_note_proposal.go
  ├─ validate：principal/scope/rationale 长度/RiskTier 判定
  ├─ note_edit：expected revision 校验 ──漂移──► plan_stale + next action
  ├─ policy：low → auto-apply 白名单；medium → pending review；high → 既有 plan 流
  └─ apply：proof loop service（snapshot → apply → receipt）
        ▼
receipt（redacted：rationale≤500、diff 摘要行数上限、无正文全文）
        ▼
pinax agent note proposal list|show|approve|reject  ◄── 用户审阅
```

## Decisions

- **D1 复用不 fork**：proposal 存储、review 流、policy 骨架全部来自 memory-runtime 既有实现；note 域只新增 payload DTO 与 note 特有校验（expected revision、目标 note 的 scope 检查）。
- **D2 expected revision 必填**：note_edit 不允许 floating 目标——版本钉住是 identity-first 的写入侧对偶；漂移报 `plan_stale`，错误文案必含下一步（重读 card → 重新 propose），不是裸错误码。
- **D3 auto-apply 白名单硬编码**：`inbox_capture`、`journal_append`、`note_draft` 三项；不提供配置键（设计稿开放问题 1 的裁决：首版显式、有真实需求再配置化）。
- **D4 MCP 默认无写**：propose 工具在 capability 关闭时不进 tools/list；开启后 propose 返回 proposal id + approval facts，中风险不会"propose 即 apply"。list/show 审阅工具只读、默认可见。
- **D5 receipt redaction 上限**：rationale 截断 500 字符；diff 只存行数摘要与 hunk 计数，不存全文（全文由 proposal 的结构化 payload 持有，receipt 只引用 proposal id）。
- **D6 指标无正文**：dogfood 计数器（proposed/auto_applied/approved/rejected/stale）进 evidence profile；`proposal_accept_rate` 过高/过低的复盘动作写进文档不在代码强制。

## Risks / Migration / Rollback

- additive only：不改 ledger schema 中既有行；proposal 行为 memory-runtime 已建表的 note 扩展或同构新表，迁移前先固化旧 fixture。
- 回滚：关闭 capability + 移除新命令注册；已产生 receipt 保留为审计事实。
- 风险：与 memory-runtime 共享 `internal/app` 文件的并行编辑——任务 0.2 要求其 CLI facade 合并后再动，或独立新文件避让。
- 风险：auto-apply 误判（draft 命中同名）——目标名带 operation/ proposal 派生前缀，不覆盖既有文件（沿 DriveBridge `<operation-id>-<name>` 同型决策）。

## Security

- agent principal 无直写 capability；一切写路径终点是 proof loop service。
- `RejectHandwrittenMetadata` 对 proposal payload 全量生效。
- MCP capability 关闭是默认态；capability 开启记录进审计。
- proposal/ receipt 不含正文全文、token、provider payload。
