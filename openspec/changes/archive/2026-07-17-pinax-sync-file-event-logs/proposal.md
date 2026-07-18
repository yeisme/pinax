## Why

当前同步日志只记录一次 run 级摘要，用户无法从 `sync logs tail` 直接看到本次同步涉及哪些文件，也不能持续跟随新事件。需要在不暴露正文、凭据或受保护路径的前提下，提供可实时观察的文件级同步时间线。

## What Changes

- 为同步 run 增加安全的文件操作事件，记录操作类型、脱敏路径、方向、状态和 run 关联信息。
- 让 `pinax sync logs tail` 同时展示 run 事件和文件事件。
- 为 `pinax sync logs tail` 增加 `--follow`，持续输出后续追加的安全同步事件，直到上下文取消。
- 保持 `--json`、`--agent`、`--events` 和默认 human 输出可解析、stdout/stderr 分离及统一脱敏。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `pinax-cloud-sync`: 扩展同步日志时间线，使其包含脱敏的文件级操作，并支持持续跟随新事件。

## Impact

- 应用层：`internal/app/sync_runs.go` 的事件持久化与读取。
- CLI 层：`internal/cli/sync_cmd.go` 的日志跟随参数和流式接线。
- 输出层：`internal/output` 的同步文件事件 projection 渲染。
- 测试与文档：同步日志 contract tests、命令文档和 OpenSpec 验证证据。
