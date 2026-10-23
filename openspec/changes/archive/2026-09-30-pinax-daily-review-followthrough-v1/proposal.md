# 日常审阅后续闭环

## Why

日常五条 review 无法限定后续 organize 的写入范围；note proposal 固定最新 50 条导致较旧待审项不可续阅。补齐两条本地路径，不新增产品域。

## What Changes

- `organize plan|suggest --note` 与 MCP `note_refs` 提供限定对象的只读规划；保存为独立 scoped schema，应用整批预检并留下失败证据。
- proposal list 增加 status/kind/limit/cursor，保留旧数组和默认 50 条；单 ID 读取不被无关坏投影阻塞。
- CLI 动作绑定 vault；MCP 提供 server-bound 工具参数，不泄露 owner 路径，不提供审批。
- 新能力 experimental，旧全库计划、默认工具及 memory proposal 合同不变。

## Capabilities

### New Capabilities
- `daily-review-followthrough`: 选择集整理、脱敏提案续阅和失败恢复。

### Modified Capabilities

无既有要求删除或重定义，新增要求覆盖 additive 路径。

## Impact

Owner fit: `cli/pinax`。影响 application、CLI/MCP、组织计划与 receipt 可选字段、命令文档和 Pinax 源技能说明。没有 DB migration、REST/RPC 新能力、客户端、付费模型或真实外部写入；11 项真实验收门保留。本次不运行 race、不提交或发布。
