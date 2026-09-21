## ADDED Requirements

### Requirement: Explicit advisory evaluation

Pinax MUST 默认关闭判断调用；仅对明确选择且授权的数据执行评估，结果只是建议。不得自动合并或删除笔记、确认长期记忆、修改 vault canon、启动同步或发布。

#### Scenario: Off mode

- **WHEN** 用户未启用判断功能
- **THEN** 原流程、默认配置和 canonical state 保持不变，零模型调用。

#### Scenario: Suggestion only

- **WHEN** 模型给出高置信度回答
- **THEN** 只生成待审建议，不绕过原采纳和授权门。

### Requirement: Versioned evidence and safe failure

Pinax MUST 绑定 owner scope、source revision、question/policy digest 与精确模型，区别拒答、不可用和过期。

#### Scenario: Stale source

- **WHEN** 源版本或权限在评估后变化
- **THEN** 历史建议只读，不可用于新版本采纳；重新评估必须显式发起。

#### Scenario: Uncertain execution

- **WHEN** 提交后超时或结果状态不明
- **THEN** 报告 outcome_unknown，不自动重试或切换付费模型；保留原流程。

#### Scenario: Replay

- **WHEN** 用户查看已保存的判断 evidence
- **THEN** 零网络重放已归一化结果，不重新调用 provider。

### Requirement: Domain ownership and bounded input

Pinax MUST 在调用与缓存读取前执行领域权限和确定性检查，仅发送有界的必要文本。输入范围：明确选中的 inbox 文本、当前 vault 内获授权的候选笔记摘要及 revision；跨 vault 默认不取材。

#### Scenario: Permission denied

- **WHEN** 候选或旧缓存不再授权给当前主体
- **THEN** 不发送、不展示，也不泄露未授权内容存在性。

#### Scenario: Unsupported modality

- **WHEN** 请求结论需要 adapter 未支持的模态或原语
- **THEN** 预检拒绝并指向原领域工作流，不能把文本结果升级为媒体结论。

### Requirement: Domain-specific review boundary

Pinax MUST 保持以下领域限制：不得自动合并或删除笔记、确认长期记忆、修改 vault canon、启动同步或发布。

#### Scenario: inbox-link

- **WHEN** 给出可审阅的关联建议
- **THEN** 用户接受/拒绝；交回原 inbox review，不越权修改 canonical state。

#### Scenario: duplicate-warning

- **WHEN** 解释重复或补充关系但不合并
- **THEN** 无自动删除；交回原 merge/edit 入口，不越权修改 canonical state。

#### Scenario: vault-isolation

- **WHEN** 阻止跨 vault cache/候选泄露
- **THEN** 权限隔离；交回原访问边界，不越权修改 canonical state。
