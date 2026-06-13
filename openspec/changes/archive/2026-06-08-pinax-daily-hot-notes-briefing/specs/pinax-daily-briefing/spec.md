## ADDED Requirements

### Requirement: Briefing recipe 由 CLI service 管理

Briefing recipe SHALL 由 CLI service 创建和修改；agent 不直接手写 recipe metadata。

#### Scenario: 创建 briefing recipe

- **WHEN** 用户运行 `pinax briefing recipe init`
- **THEN** CLI service SHALL 创建默认 recipe 结构化资产
- **AND** recipe SHALL 包含 source 配置、评分权重和输出格式

#### Scenario: 查看 briefing recipe

- **WHEN** 用户运行 `pinax briefing recipe show`
- **THEN** CLI SHALL 输出当前 recipe 配置摘要

### Requirement: Hermes 作为外部服务配置

Research adapter SHALL 通过外部服务配置与 Hermes 交互；本地开发使用 fake harness fixture。

#### Scenario: Hermes 不可用

- **WHEN** Hermes endpoint 不可达或未配置
- **THEN** research adapter SHALL 使用 fake harness fixture 进行本地开发
- **AND** 不阻塞 briefing 流程其它阶段

### Requirement: 飞书 delivery 使用 webhook adapter

飞书 delivery MVP SHALL 优先使用 webhook adapter，不引入原生 SDK。

#### Scenario: 发送 briefing 到飞书

- **WHEN** 用户配置了飞书 webhook URL
- **THEN** delivery adapter SHALL 通过 HTTP POST 发送 briefing 消息
- **AND** delivery receipt SHALL 由 CLI service 写入

#### Scenario: 飞书 webhook secret 脱敏

- **WHEN** delivery adapter 写入 delivery receipt 或 event
- **THEN** receipt 和 event MUST 脱敏 webhook URL 和 token
- **AND** 不得在 stdout、event 或日志中暴露 secret

### Requirement: Feedback loop 回写 Pinax

Feedback action SHALL 回写 Pinax event 和 feedback 结构化资产。

#### Scenario: 用户反馈

- **WHEN** 用户执行 accept/archive/dismiss/follow_up/less_like_this 操作
- **THEN** feedback SHALL 回写 Pinax event 和 feedback 结构化资产
- **AND** feedback 权重 SHALL 影响后续评分
