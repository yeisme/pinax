# 日常知识闭环

## Why

2026-09-28 用户批准优先让 Pinax 成为个人日常工具，以 Agent 对话和现有 Markdown 编辑器完成记录、整理、阅读与找回。当前 inbox 缺少 Skill 已承诺的 stdin 输入，待整理内容散落于 inbox 与 notes/index，详情卡动作以标题定位且中文截断可能破坏 UTF-8。先修复这些真实摩擦，再扩展平台能力。

## What Changes

- 增量支持 inbox capture --stdin；保存返回回读证据与稳定打开动作。
- 新增只读 inbox review，默认五条，聚合 inbox 与未归类的 notes/index，提供有界片段、关联及下一步；跳过不写状态。
- 完善搜索详情卡 Unicode 和稳定对象引用，复用 note open/preview 与现有整理、snapshot/restore 路径。
- 同步日常操作指引及路线；保留 continuity 原始样本和验收标准，另行开展两周日常使用观察。

## Capabilities

### New Capabilities

- `daily-knowledge-loop`: Agent 捕获、只读整理批次与编辑器阅读的兼容流程。

### Modified Capabilities

- 无；旧命令及默认列表行为不变，新字段与子命令均增量提供。

## Impact

Owner 为 cli/pinax；根 operator Skills 只编排命令，编辑器持有阅读体验。无数据库迁移、凭据变更、发布、远程写入或付费模型调用。已有 dirty 工作树保持原样。
