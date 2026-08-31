## ADDED Requirements

### Requirement: Continuity experience MUST 支持已绑定 repository 的零配置 resume
当当前 Git worktree 具有唯一有效 continuity binding，且调用者未显式提供 vault/scope 时，experimental `pinax continue` MUST 使用该 binding 编译 Continuity Pack。显式 vault/scope、task、intent 和 handoff MUST 保持现有优先级与语义。

#### Scenario: Codex 在已绑定 Pinax repository 中请求继续
- **WHEN** Codex operator 在已绑定 worktree 中发起 recorded continue，且没有手填 vault/scope
- **THEN** Pinax MUST 返回 `project:pinax` 的 bounded Continuity Pack、resolved binding facts 和 run ID
- **AND** 它 MUST NOT 召回其他 vault 或 scope 的内容。

#### Scenario: 现有脚本继续显式传参
- **WHEN** 旧 consumer 使用既有 `--vault`、`--scope`、`--task`、`--intent` 或 `--handoff`
- **THEN** Pinax MUST 保持这些参数的既有含义与 machine output compatibility
- **AND** binding MUST NOT 静默覆盖显式参数。

### Requirement: Human continuity output MUST 是紧凑、可判断可信度的 Resume Card
默认 human output MUST 优先展示 objective、last state、key decisions、blockers/conflicts、一个 recommended next action 和 evidence status。Card MUST bounded，不能倾倒完整 note、handoff、transcript 或 raw source body。

#### Scenario: Continuity evidence 完整
- **WHEN** handoff、决策和 source refs 均可解析且无冲突
- **THEN** Resume Card MUST 以 success 状态展示六个核心区块，并只突出一个 recommended next action
- **AND** 其他 bounded details MUST 通过 machine data 或 drill-down action 获取。

#### Scenario: 没有匹配 handoff
- **WHEN** 当前 scope 没有可消费 handoff，但存在其他 eligible context
- **THEN** Resume Card MUST 返回 context-only partial result，显示 `handoff_status=missing`
- **AND** MUST 提供 checkpoint next action，而不是失败整个请求或编造 last state。

### Requirement: Continuity freshness MUST 来自 supporting evidence 而非生成时间
Continuity Pack MUST 区分 `generated_at` 与 evidence freshness。Freshness MUST 由支持当前 pack 的 handoff、memory/source observed time 或 revision 推导；生成当前响应的时刻 MUST NOT 被当成内容新鲜度。

#### Scenario: 旧 source 仍可解析
- **WHEN** source 存在但 supporting evidence 的 observed/revision time 超出 freshness policy
- **THEN** Pinax MUST 标记 freshness 为 stale，并保留真实 evidence timestamp
- **AND** MUST NOT 因本次刚生成 pack 而显示为 fresh。

#### Scenario: Repository source revision 漂移
- **WHEN** repository source path 可打开但记录 revision 与当前 revision 不一致
- **THEN** Pinax MUST 将 source 计为 stale，整体 status 至少为 partial
- **AND** 提供 inspect/refresh/review action，不自动替换原 evidence。

### Requirement: Continuity MUST 返回 partial trustworthy result 并只暴露一个关键歧义
当部分来源 missing/stale、存在 conflict 或 scope 信息不足时，Pinax MUST 保留仍可验证的 sections，显式列出 warning categories，并提供一个最关键的 clarification/review action。它 MUST NOT 静默选择冲突事实或把 unresolved 内容渲染为 verified。

#### Scenario: 两个决定冲突
- **WHEN** 当前 scope 同时存在互不兼容且均有来源的 confirmed/proposed decision
- **THEN** Continuity Pack MUST 保留 conflict facts，Resume Card MUST 显示 conflict warning
- **AND** recommended next action MUST 指向一个 review/clarification，不得选取其中一个作为无条件事实。

#### Scenario: 部分 source missing
- **WHEN** 至少一个 supporting source missing，但其他 source 与 handoff 仍可解析
- **THEN** Pinax MUST 返回 partial pack、missing count 和剩余可信内容
- **AND** MUST NOT 因单个 missing source 丢弃全部 continuity value。

### Requirement: Pinax MUST 提供 bounded continuity checkpoint facade
Experimental `pinax continue checkpoint` MUST 自动使用 resolved binding/scope，并复用 canonical handoff service 创建 objective、current state、decisions、completed work、blockers、verification、follow-ups 和 source refs 的 bounded handoff。任何 durable memory candidate MUST 进入 proposal/review，不能由 checkpoint 自动 confirmed。

#### Scenario: Claude Code 暂停 substantial session
- **WHEN** Claude Code operator 提交合法 bounded checkpoint
- **THEN** Pinax MUST 创建可由 Codex 消费的 provider-neutral handoff，并返回 handoff ID、source status 和 next action
- **AND** MUST NOT 保存完整会话或把 handoff 自动提升为 confirmed memory。

#### Scenario: Checkpoint 包含 durable decision
- **WHEN** operator 识别到值得长期保留的 decision、preference 或 lesson
- **THEN** Pinax MUST 只允许通过 canonical proposal service 创建 `proposed` memory，并返回 review action
- **AND** 没有显式 review approval 时 confirmed memory count MUST 保持不变。

#### Scenario: Checkpoint 输入过长或像 transcript dump
- **WHEN** 任一 section 超过 item/character cap，或请求尝试提交 transcript/raw payload
- **THEN** Pinax MUST 拒绝 checkpoint 并返回 bounded validation error
- **AND** MUST NOT 截断后静默写入可能包含私密内容的 handoff。

### Requirement: Codex 与 Claude Code MUST 通过同一 provider-neutral contract 互相继续
Pinax core MUST 只接受 provider-neutral principal/runtime descriptor、scope、handoff、context 和 feedback fields。Codex 与 Claude Code 的安装、自然语言触发和 output formatting MUST 由 split-owner operator Skill 处理，不得进入 memory schema 或 lifecycle policy。

#### Scenario: Codex checkpoint 由 Claude Code 继续
- **WHEN** Codex 通过 operator Skill 创建 checkpoint，Claude Code 随后在同一 binding 中 resume
- **THEN** Claude Code MUST 获得相同 objective、决策、blocker、verification、follow-up 和 source semantics
- **AND** Pinax core schema MUST NOT 包含 Codex-only 或 Claude-only required field。
