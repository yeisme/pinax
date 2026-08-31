## ADDED Requirements

### Requirement: Continuity run receipt MUST 是显式 opt-in 且不改变默认读取语义
Pinax MUST 只在调用者显式请求 recorded run 时创建 continuity run receipt。普通 `pinax continue` MUST 保持 read-only；recorded run MUST 返回 opaque `continuity_run_id`，并仅保存运行、来源、handoff、runtime、task class 和安全计数等最小元数据。

#### Scenario: 普通 continue 不记录 run
- **WHEN** 调用者执行现有 `pinax continue` 且没有启用 run recording
- **THEN** Pinax MUST 编译 continuity pack，但 MUST NOT 写 continuity run 或 feedback table
- **AND** 现有 machine output consumers 不需要采用新字段。

#### Scenario: Agent operator 开始 substantial continuation loop
- **WHEN** Codex 或 Claude Code operator 显式启用 run recording，并提交合法 runtime 与 task class
- **THEN** Pinax MUST 创建一个 opaque run receipt，返回 additive `continuity_run_id`
- **AND** receipt MUST NOT 保存 task title、prompt、正文、完整路径、provider payload 或私有工具参数。

### Requirement: 用户 MUST 通过四值 outcome 提交 continuation 结果
Pinax MUST 接受且只接受 `trusted`、`corrected`、`wrong_project`、`insufficient` 四种 user outcome。Outcome MUST 来自用户明确选择，Agent 不得根据任务是否完成、语气或工具日志推断。

#### Scenario: 用户信任 continuation pack
- **WHEN** 用户对 recorded run 提交 `trusted`
- **THEN** Pinax MUST 追加 immutable feedback event，并将该 run 计入 valid outcome denominator
- **AND** report MUST 使用用户提交值而非 Agent inference。

#### Scenario: 用户修改已有 outcome
- **WHEN** 用户对同一 run 再次提交一个合法 outcome
- **THEN** Pinax MUST 追加 superseding feedback event，保留旧 event 供本地审计
- **AND** report MUST 使用最新有效 outcome，不能原地覆盖而丢失变更事实。

#### Scenario: Outcome 不合法或 run 不存在
- **WHEN** outcome 不属于四值枚举，或 `run_id` 不存在
- **THEN** Pinax MUST 拒绝写入并返回 stable validation/not-found error
- **AND** report denominator MUST 保持不变。

### Requirement: Dogfood evidence MUST 覆盖 runtime、task class、review burden 和 silent write
每个 recorded run MUST 可选记录 runtime、三类 task class、source coverage、handoff status、checkpoint/proposal count、weekly review seconds 和由 canonical lifecycle receipt 计算的 silent confirmed write count。Task class MUST 只允许 `implementation_debugging`、`product_spec_docs` 和 `release_operations`。

#### Scenario: 三类 Pinax 任务形成样本
- **WHEN** 六周窗口中三个 task class 均有 completed run
- **THEN** report MUST 分别显示每类 run/outcome 分母和 trusted 数
- **AND** 不得把 synthetic fixture、无 outcome run 或普通命令调用伪装为真实 continuity loop。

#### Scenario: Weekly review 被记录
- **WHEN** operator 完成一次 bounded weekly review 并提交 review seconds
- **THEN** Pinax MUST 保存非负整数 duration，并按观察周聚合
- **AND** MUST NOT 保存被 review 的完整 proposal body 或用户对话。

#### Scenario: Lifecycle 发生 silent confirmed write
- **WHEN** canonical receipts 显示某个 recorded run 在没有显式 review approval 时产生 confirmed memory
- **THEN** report MUST 增加 `silent_confirmed_write_count` 并使 safety gate 失败
- **AND** Agent 自报的 outcome MUST NOT 覆盖该失败。

### Requirement: 六周报告 MUST 以明确分母执行 Go/Iterate/Stop 门禁
Pinax MUST 提供 CLI-authored local report，至少输出总 completed loops、四值 outcome 分布、trusted rate、source resolvability、runtime coverage、task-class coverage、weekly review duration、silent confirmed writes、missing feedback、known bias 和唯一 decision readiness。

#### Scenario: 全部 Go 门槛达到
- **WHEN** 六周报告包含至少 30 个真实 completed loops，Codex 与 Claude Code 均有样本，三个 task class 均有样本，trusted rate ≥80%，source resolvability ≥95% 且来源分母大于 0，每周 review ≤300 秒，silent confirmed writes = 0
- **THEN** report MUST 标记 `go_ready=true`
- **AND** 它仍 MUST 标记 `cross_project_routing=unvalidated`，因为首轮只有 Pinax repository。

#### Scenario: 安全通过但质量门槛未全部达到
- **WHEN** silent confirmed writes 为 0，但 trust、source、review 或 sample gate 未达到
- **THEN** report MUST 标记 `go_ready=false` 并列出失败门槛和分母
- **AND** CEO/product 只能在失败集中于最多两个可修复类别时选择 Iterate，否则选择 Stop 或继续收集未完成窗口证据。

#### Scenario: 缺少 outcome 或 source 分母
- **WHEN** recorded runs 没有足够 user outcome，或 source total 为 0
- **THEN** 相应指标 MUST 为 `not_measured`，不得按 100% 或成功处理
- **AND** Go gate MUST 失败。

### Requirement: Dogfood evidence MUST 保持 local、redacted 和 append-only
Continuity runs、feedback events 和 reports MUST 使用 GORM additive tables/index 或 Pinax application service 写入。Evidence MUST NOT 包含 secrets、Authorization/Cookie、raw prompt、hidden system prompt、provider payload、完整 transcript、chain-of-thought、私有工具参数、note body 或 repository 绝对路径。

#### Scenario: 生成 report
- **WHEN** 用户生成六周或任意时间窗 report
- **THEN** output MUST 只包含枚举、计数、比例、时间、opaque ID/digest 和 bounded warning codes
- **AND** report MUST 列出 single-operator、single-repository、self-selection 与 missing-feedback bias。

### Requirement: Continuity 主线 MUST 遵守六周 scope freeze
在 2026-08-29 至 2026-10-10 的首轮窗口内，Pinax MUST 只接受 continuity UX、binding、source quality、review efficiency、install/diagnose、recovery、compatibility、blocking bug 和真实 vault safety 工作。其他命令/platform/provider 扩张 MUST 等待唯一 Go/Iterate/Stop receipt。

#### Scenario: 冻结期提出无关新能力
- **WHEN** 有人提出新的顶层 note/platform command、团队协作、Web/desktop client、通信 provider 或公共 SaaS surface
- **THEN** 当前 change MUST 拒绝夹带该能力，并保留到独立后续 owner decision
- **AND** 不得以 continuity dogfood 的工程完成或命令调用量解除冻结。

#### Scenario: Action-capture canary 到达固定结束日
- **WHEN** 现有 2026-08-23 至 2026-09-05 action-capture observation window 结束
- **THEN** 其 owner MUST 生成 Go/Iterate/Stop receipt 并完成 archive 或显式 closeout
- **AND** 当前 change MUST NOT 新增 Feishu/task integration 或稳定 `intent=action_capture` 来替代缺失证据。
