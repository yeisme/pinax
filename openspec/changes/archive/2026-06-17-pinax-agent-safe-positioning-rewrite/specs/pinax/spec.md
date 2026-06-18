# Spec Delta: Pinax 定位

## ADDED Requirements



### Requirement: 产品定位

Pinax 的对外定位 SHALL 是 **agent-safe knowledge control plane for Markdown vault**，不是通用笔记 App 或云笔记平台。

#### Scenario: README 第一屏传达核心价值

- **WHEN** 用户打开 README.md
- **THEN** 第一屏在一句话内传达"让 AI 安全操作你的私人知识库"
- **AND** 展示 proof loop aha moment 代码块
- **AND** 不以功能列表作为第一印象

#### Scenario: 竞品关系互补定位

- **WHEN** 文档描述与 Obsidian/Logseq/Notion/Reflect 的关系
- **THEN** 明确表达互补关系（complements Obsidian/Logseq）
- **AND** 明确避开主战场（不打 Notion 团队协作、不比个人笔记体验）
- **AND** 不做正面功能清单对比

### Requirement: 三个可复述概念

产品复杂度 MUST 压缩为三个可被用户复述的概念。

#### Scenario: 用户可以复述核心概念

- **GIVEN** 用户阅读过 README 第一屏
- **WHEN** 被问 Pinax 是什么
- **THEN** 能复述：Local Vault 是真源 / Proof Loop 保护 agent 写入 / Cloud Sync 只协调密文
