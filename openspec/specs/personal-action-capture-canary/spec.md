# personal-action-capture-canary Specification

## Purpose
TBD - created by archiving change pinax-agent-continuity-iteration-adoption-signal. Update Purpose after archive.
## Requirements
### Requirement: Hermes 必须作为个人行动捕获的唯一交互入口
个人 canary MUST 由 Hermes 接收自然语言行动请求并编排后续步骤。Hermes 只读调用 Pinax 的 `pinax.agent.context`、`pinax.agent.memory_recall` 和 `pinax.agent.handoff_read`；Pinax MUST NOT 解析飞书 payload、调用飞书 SDK 或直接创建外部任务。

#### Scenario: 用户请求把当前内容变成任务
- **WHEN** 用户在 Hermes 中表达“把这个变成任务”或等价意图
- **THEN** Hermes 只使用获准的三个 Pinax 只读 MCP 工具补充最小必要上下文，并在任何外部写入前生成预览

#### Scenario: Pinax 不可用
- **WHEN** Pinax MCP 无法连接、超时或没有可解析来源
- **THEN** Hermes 仍可生成普通任务草案，但 MUST 标记 `Pinax context: unavailable` 或 `Source status: ungrounded`，且 MUST NOT 伪造来源、freshness 或 confidence

### Requirement: 任务预览必须可审阅且来源有界
Hermes MUST 在写入前显示一个有界预览，至少包含行动标题、可观察完成结果、项目背景、建议截止时间、约束、来源、缺失信息和写入状态。来源 MUST 能解析回 Pinax 的 bounded reference；无法确认的信息 MUST 显式保持缺失，不得推断负责人、截止时间或项目归属。

#### Scenario: Pinax 返回有来源上下文
- **WHEN** 至少一个 Pinax source reference 可解析并支持当前行动
- **THEN** 预览显示对应上下文、约束和来源，并将写入状态设为 `awaiting confirmation`

#### Scenario: 用户修改预览
- **WHEN** 用户要求修改标题、结果、截止时间、约束或其他字段
- **THEN** Hermes 更新预览并再次等待确认，修改请求 MUST NOT 被视为创建授权

### Requirement: 外部任务写入必须由明确确认触发
任何任务创建、分配、关闭或修改 MUST 在用户明确确认后由 Hermes 已配置网关、任务工具或 `lark-cli` 执行。Pinax MUST NOT 保存 provider-specific task ID、清单、负责人或状态镜像；外部映射只允许由 Hermes/适配层使用 provider-neutral `external_ref` 管理。

#### Scenario: 用户尚未确认
- **WHEN** 预览已显示但用户没有给出明确创建指令
- **THEN** 外部任务创建次数 MUST 为 0，Hermes 只保留可修改预览

#### Scenario: 用户明确确认
- **WHEN** 用户对当前预览给出“确认创建”等明确授权
- **THEN** Hermes 将预览委派给已配置外部任务工具，并只返回该工具提供的 task identifier 或 link

#### Scenario: 外部任务工具失败
- **WHEN** 已确认写入返回错误、超时或不确定结果
- **THEN** Hermes 报告有界错误并保留预览以供复核，MUST NOT 声称成功、静默重试产生重复任务或写入完成记忆

### Requirement: Canary 指标必须由外部适配层脱敏记录
Canary MUST 通过 CLI 或应用服务生成结构化事件与汇总，不得由 Agent 手写 JSON/JSONL。记录仅包含 preview ID、时间、修改次数分档、来源总数与可打开数、上下文状态、预览延迟、确认/创建结果、重复或错误标志、完成重要性、审阅提案结果和活跃日期；MUST NOT 包含标题、正文、raw prompt、完整会话、provider payload、凭据或完整外部 task ID。

#### Scenario: 一次预览完成
- **WHEN** Hermes 已向用户显示任务预览
- **THEN** 外部 canary recorder 追加一个脱敏 preview event，并可记录零次、一次或多次实质修改及从表达行动到预览的耗时

#### Scenario: 已确认任务完成
- **WHEN** 用户报告一个重要任务已完成并决定是否提出长期记忆
- **THEN** recorder 只记录是否生成提案、是否通过审阅和 provider-neutral digest，不记录任务正文或执行日志

#### Scenario: 生成两周汇总
- **WHEN** 用户运行 canary report 命令
- **THEN** 系统输出带明确分母的预览质量、来源可打开率、未确认写入数、重复/错误率、预览延迟中位数、重要完成任务提案率和每周活跃天数

### Requirement: 完成结果必须经 Pinax proposal/review 才能成为长期记忆
Hermes MUST 区分一次性进度与值得长期保留的决定、偏好、经验、约束或重要结果。任何长期记忆写入 MUST 先显示提案并进入现有 Pinax proposal/review 流程；MUST NOT 自动确认、静默晋升或复制完整会话。

#### Scenario: 完成结果具有长期价值
- **WHEN** 完成摘要包含可复用决定、经验、偏好或约束
- **THEN** Hermes 显示有来源的记忆提案，并等待用户通过 Pinax review 明确接受、修改或拒绝

#### Scenario: 完成结果只是执行噪声
- **WHEN** 内容仅为普通进度、原始日志或一次性状态
- **THEN** Hermes 不向 Pinax 提交长期记忆提案

### Requirement: 两周决策必须遵守预先登记门槛
Canary 结束后 MUST 依据有分母的证据作出 Go/Iterate/Stop 决策。继续固化接口要求：至少 70% 预览只需零次或一次实质修改、至少 90% 来源可打开、未确认创建为 0、重复或错误任务不超过 5%、预览延迟中位数低于 30 秒、至少 50% 重要完成任务产生有价值且通过审阅的提案，并且用户每周至少四天主动使用该入口。

#### Scenario: 所有门槛达到
- **WHEN** 两周汇总满足全部质量、安全、沉淀和主动使用门槛
- **THEN** 后续独立 OpenSpec MAY 评估 provider-neutral 的 `intent=action_capture` 实验视图，但 MUST NOT 加入飞书类型

#### Scenario: 有复用但上下文质量不足
- **WHEN** 主动使用存在但预览修改、来源解析或审阅效率未达到门槛
- **THEN** 下一轮只能优化 Pinax retrieval、source resolution 和 review efficiency，不得扩大 provider 或任务同步范围

#### Scenario: 没有持续复用
- **WHEN** 每周主动使用或有效闭环没有形成持续信号
- **THEN** 停止集成扩张，保留 Pinax、Hermes 和外部任务系统各自独立使用

