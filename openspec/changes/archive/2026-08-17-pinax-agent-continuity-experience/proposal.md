## Why

Pinax 已具备 provider-neutral Agent Memory Runtime、bounded context、proposal、handoff、feedback 与 proof receipt，但用户仍需要理解 `agent`、`memory`、`brain`、`proof` 等底层命令才能完成一次跨 Agent 连续工作。现在需要把这些能力收敛成一个可在五分钟内体验、可重复依赖、可度量验证的产品闭环：让后续 Agent 在不读取完整会话、不绕过来源和审批的前提下继续上一项工作。

本 change 承接 CEO 产品方向“可信任的个人 Agent 记忆与行动层”，但只交付最窄的 Agent Continuity wedge，不扩展为完整编辑器、通用自动化平台或团队 SaaS。底层 runtime 仍由 `openspec/changes/pinax-agent-memory-runtime/` 负责；本 change 只消费其已发布或 experimental 合同。

## What Changes

- 新增 experimental `pinax continue` 意图入口：按 principal、workspace/project/task scope 和 budget 编译可解释的 Continuity Pack，返回目标、关键决策、偏好、未完成事项、失败经验、冲突、来源和下一步命令。
- 新增 experimental `pinax review` 意图入口：聚合待审 memory proposals、冲突、stale/expired 候选和可撤销 receipt，默认只读；任何批准、拒绝、supersede 或 restore 继续调用现有 application service 与 proof gate。
- 新增 Memory Inbox projection，将候选记忆统一分类为 new fact、preference、decision、lesson、commitment、conflict、duplicate 和 stale，提供 reason、source、scope、confidence、expiry、risk 与 proposed action。
- 扩展本地 dashboard 为 Agent Trust Center 的只读视图，展示最近 Agent activity、Memory Inbox、context source coverage、proposal decisions、proof receipts、restore hints 和 adapter health；浏览器不直接执行写操作。
- 新增首次体验与跨 Agent handoff proof flow：fixture vault 上完成 Agent A context/proposal/handoff、用户 review/approve、Agent B continue，并写标准 integration evidence。
- 新增 Continuity 产品指标 projection：context reuse、source resolvability、proposal acceptance、handoff continuation、silent promotion、stale/conflict resolution；不上传明文、raw prompt、provider payload 或完整 transcript。
- 所有 CLI、JSON、`--agent`、`--events`、HTTP projection 均为 additive experimental surface；不删除、重命名或 repurpose `pinax agent`、`pinax memory`、`pinax brain`、`pinax proof`、现有 dashboard 路由或 stored schema。

## Capabilities

### New Capabilities

- `agent-continuity-experience`: `pinax continue`、Continuity Pack、跨 Agent continuation、bounded source-backed context 和五分钟首次价值闭环。
- `memory-review-inbox`: `pinax review`、Memory Inbox 分类、review actions、conflict/stale/duplicate 治理与 proof-gated mutation。
- `agent-trust-center`: dashboard 只读 Agent activity、context coverage、proposal/receipt/restore、adapter health 和本地产品指标 projection。

### Modified Capabilities

- 无。现有 `agent-memory-runtime`、`agent-memory-ledger`、`pinax-agent-brain-layer`、`vault-dashboard-health` 和 `pinax-agent-safe-proof-loop` 行为保持兼容；本 change 通过 additive facade 与 projection 复用它们。

## Impact

- CLI：新增 `pinax continue`、`pinax review` experimental command，不改变现有命令语义。
- Application：新增 continuity orchestration、review inbox aggregation、trust-center projection 和 product metric aggregation service；不得复制 memory lifecycle 或 proof apply 逻辑。
- Dashboard/API：新增只读 `/api/agent-continuity`、`/api/memory-inbox`、`/api/agent-activity`、`/api/trust-metrics` 或等价 additive route；现有 route 保留。
- Output：新增 versioned Continuity Pack、Memory Inbox 和 Trust Metric DTO；default、`--agent`、`--json`、`--events`、`--explain` 遵循现有输出合同。
- Persistence：优先从现有 memory/proposal/handoff/feedback/receipt/activity ledger 聚合；如需新状态，只允许 GORM additive table、nullable field、safe default 或 index。
- Tests：新增 command contract、service integration、dashboard component、cross-Agent e2e、redaction、compatibility、performance 和 integration evidence 验证。
- Compatibility：全部新增 surface 标记 `experimental=true`；本 change 不启动旧入口 deprecation。回滚方式是隐藏新命令与 route，继续使用现有 `pinax agent`、`pinax memory`、`pinax brain` 和 dashboard。

## User Research Script (Task 0.2)

### Target user screening

- Uses two or more coding Agents (e.g. Codex + Ordo) for the same ongoing project.
- Maintains a Markdown vault or note collection with ≥50 notes.
- Has experienced "re-explaining context" when switching between Agents within the same week.

### Five-minute first-value script

1. Agent A runs `pinax continue --project <user-project> --task "<real task>" --vault <vault> --json` and creates a handoff.
2. Owner reviews and approves one low-risk proposal via `pinax review --project <user-project> --vault <vault> --json`.
3. Agent B runs `pinax continue --project <user-project> --task "<continuation task>" --vault <vault> --json`.
4. Measure: time-to-first-continuity-pack, source resolvability, and whether Agent B received objective/blocker/commitment without re-explanation.

### Re-explanation baseline

- Before: count how many times the user manually re-explains project context per Agent switch (self-reported, weekly).
- After: count how many times `pinax continue` replaced manual re-explanation.

### Cross-Agent continuation success criteria

- Agent B receives ≥1 approved decision, ≥1 open commitment, ≥1 blocker or verification note.
- Source refs are resolvable (≥95%).
- Silent promotion = 0 (no memory auto-confirmed without owner approval).

### Willingness-to-pay interview questions

- "How much time per week do you spend re-explaining project context to different Agents?"
- "If this tool eliminated 80% of that re-explanation, what would that be worth per month?"
- "Would you pre-pay for a version that also synced context across machines?"

### Go/No-Go metrics

- ≥7/10 users complete the five-minute flow without help.
- ≥5/10 users reuse `pinax continue` within one week.
- ≥3/10 users express willingness to pay or pre-pay.

## Product Validation Operating Contract

### Scope freeze

在 8.1-8.7 产品验证完成前，本 change 进入 wedge scope freeze。只允许修复阻塞首次体验、数据安全、权限、脱敏、来源解析、continuation 正确性、恢复链路和兼容性的缺陷；不新增新的顶层意图命令、provider 类型、团队协作、编辑器、发布分发或自动化平台能力。兴趣、下载量和新增命令调用量不能解除 scope freeze。

### Evidence cohort

- 首轮样本为 10 名符合筛选条件的目标用户，或 10 个由不同真实项目构成、可独立复核的等价任务；同一用户的重复运行不能伪装成多个样本。
- 每个样本必须记录脱敏 participant/run ID、vault 规模区间、Agent 组合、首次设置时间、首次 Continuity Pack 时间、是否需要人工帮助、失败阶段、source coverage、proposal decision、handoff continuation、七日内复用和付费意愿。
- 每次失败必须归入 onboarding、scope/permission、retrieval/context、source resolution、proposal/review、handoff、adapter、trust UI 或 recovery；不能只记录总体成功率。
- Evidence 只保存计数、区间、枚举、redacted ID 和用户明确允许的短评，不保存正文、raw prompt、完整 transcript、provider payload、secret 或私有工具参数。

### CEO decision outcomes

- **Go**：达到首次完成、七日复用、continuation/source/safety 和付费信号门槛；下一 change 只允许进入 onboarding、两个 reference adapter、可靠性与 stable-from 准备。
- **Iterate**：核心问题和复用信号存在，但失败集中在最多两个可修复阶段；下一 change 必须只针对证据中的主要失败类别，不扩大产品面。
- **Stop**：七日复用低于 3/10、用户没有高频重复解释问题、continuation 未形成可观察价值，或安全门槛不能满足；停止扩大 facade，保留底层 runtime 和兼容读取面，记录停止原因。

Go/Iterate/Stop 决策必须附样本数、失败分布、指标分母、已知偏差和后续 change 名称或停止动作。没有该决策 receipt，不得将 capability 标记 stable，也不得以“工程已完成”为由启动团队 SaaS、更多 provider 或通用知识工作台。
