## ADDED Requirements

### Requirement: Codex 与 Claude Code MUST 共用一个 Pinax continuity source Skill
Yeisme MUST 在 `.skills/yeisme/pinax-agent/` 维护一个 `pinax-continuity-operator` source Skill，并通过 root 与 `cli/pinax` 受管 profile/sync workflow 生成对应 workspace 的 Codex 与 Claude Code runtime copies。所有 runtime copies MUST 消费相同 Pinax CLI contract，不得维护独立 workflow 或 canonical state。

#### Scenario: Source Skill 同步到两个 workspace 和两个 runtime
- **WHEN** root 与 `cli/pinax` profile sync 成功执行
- **THEN** root/Pinax 的 `.agents/skills` 与 `.claude/skills` MUST 包含语义一致的 `pinax-continuity-operator`
- **AND** generated-copy validation MUST 证明四个 copies 来自同一 source，而非手工漂移版本。

### Requirement: Operator MUST 只通过 Pinax provider-neutral CLI contract 操作 continuity
Operator Skill MUST 使用 `cli/pinax/openspec/changes/pinax-trusted-continuity-dogfood-v1/` 验收的 machine-mode commands 执行 status/binding recovery、resume、checkpoint、review、feedback 和 report。它 MUST NOT 读取 Pinax 内部数据库、手写 registry/vault metadata 或复制 memory lifecycle。

#### Scenario: 用户请求继续当前项目
- **WHEN** 用户在 Codex 或 Claude Code 中表达“继续这个项目”或等价意图
- **THEN** Operator MUST 先调用 Pinax status/resume contract，并只渲染 Pinax 返回的 bounded Resume Card facts
- **AND** MUST NOT 根据目录名或聊天历史自行猜测 vault、scope、decision 或 source status。

#### Scenario: Pinax CLI contract 未 ready
- **WHEN** 所需 command/version/fact 不存在或 Pinax 返回 compatibility blocker
- **THEN** Operator MUST 报告具体 blocker 和 canonical recovery command
- **AND** MUST NOT 回退为无来源的伪 continuity memory。

### Requirement: Missing 或 ambiguous binding MUST 最多触发一个关键问题
当 Pinax 返回 missing、invalid 或 ambiguous binding 时，Operator MUST 停止自动 resume，只提出一个能解除当前歧义的问题或展示一个 canonical recovery command。它 MUST NOT 搜索所有 vault、自动创建 project、切换 project 或选择任意候选。

#### Scenario: 当前 worktree 有多个候选 binding
- **WHEN** Pinax 返回 `continuity_binding_ambiguous`
- **THEN** Operator MUST 展示 bounded candidates/status 并询问用户选择或执行 disable/fix command
- **AND** 在用户解决前 MUST NOT 读取任何候选 scope 的 continuity content。

### Requirement: Operator closeout MUST 创建 bounded checkpoint 且禁止 silent memory promotion
Substantial session 结束、暂停或切换 Agent 时，Operator MUST 使用 Pinax checkpoint contract 提交 bounded objective、state、decision、completed work、blocker、verification、follow-up 和 source refs。任何 durable candidate MUST 进入 Pinax proposal/review；Operator MUST NOT 直接创建 confirmed memory或提交 transcript/raw prompt。

#### Scenario: Codex 完成实现并切换 Claude Code
- **WHEN** 用户明确结束或切换一个 substantial Codex session
- **THEN** Operator MUST 创建 provider-neutral checkpoint，使 Claude Code 可通过同一 binding resume
- **AND** 没有显式 review approval时 confirmed memory MUST 不变。

#### Scenario: 普通只读问答结束
- **WHEN** session 没有 substantial state change、handoff need 或用户 pause/switch intent
- **THEN** Operator MUST NOT 强制创建 checkpoint 或 run feedback
- **AND** 普通问答体验保持轻量。

### Requirement: Continuity outcome MUST 由用户显式四选一
Recorded substantial loop closeout 后，Operator MUST 向用户提供且只提供 `trusted`、`corrected`、`wrong_project`、`insufficient` 四个 outcome，并把用户选择提交给 Pinax。用户未选择时 MUST 保持 missing，Operator 不得推断。

#### Scenario: 用户没有回答 outcome
- **WHEN** Operator 已询问但用户继续其他工作或结束对话
- **THEN** feedback MUST 保持 missing
- **AND** Operator MUST NOT 因任务成功、用户未纠正或正向语气自动记录 `trusted`。

### Requirement: Operator MUST 组织 relevant-inline 与 weekly bounded review
Operator MUST 只在 Pinax 标记某个 pending item 会改变当前 next action 时 inline 提示；其余 item 通过每周 bounded review 呈现。所有 approve/reject MUST 使用 Pinax canonical review/proof service 和显式确认。

#### Scenario: 无关 proposal 存在
- **WHEN** Pinax inbox 有 pending item，但 continuity projection 标记其不影响当前 objective/decision/blocker
- **THEN** Operator MUST 不在 Resume Card 打断用户
- **AND** item MUST 保留到 weekly review。

### Requirement: Root handoff MUST 保持 owner 与验证边界
Root change MUST 只跟踪 source Skill、profile、runtime generation 和跨 runtime contract acceptance。Pinax command、GORM、source resolver 和 package tests MUST 由 `cli/pinax/openspec/changes/pinax-trusted-continuity-dogfood-v1/` 跟踪。

#### Scenario: Pinax implementation 尚未通过 contract gate
- **WHEN** child OpenSpec 或 machine contract 未通过其 owner validation
- **THEN** root Operator rollout MUST 保持 disabled 或 fixture-only
- **AND** root tasks MUST NOT 把 Pinax 内部代码完成状态伪装为 root handoff 已完成。
