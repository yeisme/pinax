## Context

Pinax 现在已经能通过 `stats`、`doctor` 和 dashboard 发现 vault 健康问题。产品下一步不应该跳去 provider 或 agent 平台，而是把这些问题转成可审查、可回滚、可脚本化的维护动作，让本地 Markdown note CLI 的价值落到“长期维护成本下降”。

现有边界仍然成立：Markdown 文件是真源，`.pinax/` 机器资产必须由 CLI/service 写入，真实写入必须显式审批，涉及批量改动时需要 Git snapshot 保护。

## Goals / Non-Goals

**Goals:**

- 提供 `pinax repair plan`，从 `VaultHealthService` 输出生成 repair plan。
- 提供 `pinax repair apply`，只在 `--yes` 和 Git snapshot 保护后应用低风险修复。
- 定义 CLI-authored repair plan 资产，记录 schema version、plan id、issue snapshot、operations、risk、expiry 和 source facts。
- 将 dashboard 从“只看问题”增强到“只读查看 repair plan 和 issue drilldown”。
- 保持所有输出模式来自同一 projection，`--json` 和 `--agent` 可稳定被脚本和 agent 消费。

**Non-Goals:**

- 不自动改写正文、总结内容或调用 LLM。
- 不删除用户笔记，不自动合并重复笔记。
- 不让 dashboard 提供写入 API。
- 不绕过 Git snapshot 保护执行批量写入。
- 不接 firecrawl、agent-browser、Lark、Notion、Pinax Cloud 或其它 provider。

## Decisions

### 1. repair plan 是 CLI-authored structured asset

`pinax repair plan` 默认在 stdout 返回 projection，并可通过 `--save` 写入 `.pinax/repair-plans/<plan_id>.json`。计划文件包含 `schema_version=pinax.repair_plan.v1`、`plan_id`、`created_at`、`vault_root`、`source_command`、`issue_snapshot`、`operations`、`expires_at` 和 `status`。

替代方案是让 agent 或用户直接编辑 JSON。该方案违反 Pinax 机器资产边界，也会让计划可追溯性变差，因此不采用。

### 2. apply 只执行低风险、可解释动作

MVP 自动 apply 的操作限定为：补齐 Pinax metadata、为空 tags 写入 `[]` 或默认 `inbox`、重建 index、写入 archive marker/status、记录 manual-review receipt。以下操作只生成 manual review：删除空笔记、合并重复标题、重写正文、自动建立语义链接。

这样牺牲了一些自动化程度，但保护用户 vault，符合 note CLI 的本地优先和可回滚定位。

### 3. 批量写入复用 Git snapshot guard

`repair apply` 与 `organize apply` 一样需要 `--yes` 和最近 Pinax Git snapshot，或通过 `--snapshot-message` 先创建 snapshot。无保护时返回稳定错误 `snapshot_required` 和可运行 next action。

### 4. dashboard 保持只读，只展示 plan/drilldown

Dashboard 可以展示 repair plan 摘要、issue drilldown、operation risk 和对应 CLI 命令，但不暴露 POST/PUT/DELETE 或写入路由。用户要落地仍运行 CLI。

### 5. 计划过期和 issue drift 必须显式处理

repair plan 生成时记录 issue snapshot 和 note facts。apply 前重新运行 doctor 或校验 target 文件 mtime/hash；如果 facts 变化，计划返回 `plan_stale`，要求重新生成计划。

## Risks / Trade-offs

- 自动修复误伤笔记 -> 只允许低风险操作，批量写入需要 snapshot，复杂问题只 manual review。
- 计划文件过期导致错误 apply -> 记录 expiry 和 issue snapshot，apply 前做 drift check。
- repair 与 metadata/organize 命令重复 -> repair 只编排既有能力，具体写入仍调用相同 service/helper，避免双写逻辑。
- dashboard 诱导用户以为可点击修复 -> UI 只展示 CLI 命令和只读状态，不提供写入按钮。
- 大型 vault plan 生成慢 -> 输出 scan duration 和 operation count；后续再基于实测加 cache。

## Migration Plan

1. 新增 domain model 和 plan service，不改变已有 stats/doctor 行为。
2. 新增 `repair plan`，默认只输出 projection。
3. 新增 plan save/load repository，写入 `.pinax/repair-plans/`。
4. 新增 `repair apply`，先支持 metadata/index/archive/manual-review receipt 等低风险动作。
5. 增强 dashboard 只读 endpoint 和 HTML view。
6. 更新 README/docs 示例和 OpenSpec 验证证据。

## Open Questions

- 默认补空 tag 应写 `[]` 还是 `inbox`？建议先写 `[]`，需要用户显式 `--default-tag inbox` 才写入标签。
- archive stale note 是移动文件到 `archive/`，还是只写 `status: archived`？建议 MVP 只写 frontmatter `status: archived`，避免路径移动和 organize 产生边界重叠。
- repair plan 默认是否落盘？建议默认不落盘，`--save` 才写 `.pinax/repair-plans/`。
