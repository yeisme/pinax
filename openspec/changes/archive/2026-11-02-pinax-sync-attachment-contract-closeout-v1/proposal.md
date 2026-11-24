## Why

用户要求将消费者责任迁出底层 CLI 并完成 Pinax 自身验收。原 `pinax-drivebridge-workbench-v1` 的公开 sync、附件和恢复实现已存在；需要独立复验并同步 Pinax canonical specs，不继续等待客户端发布。

## What Changes

- 复用现有 notebook-backend、operation-recovery、daily-knowledge、agent-memory、agent-continuity 和 default evidence profiles，验收 Pinax 已实现的合同。
- 将 sync-control 和 notebook source/attachment 的 Pinax 约束同步至本 owner canonical specs；Electron DTO、renderer 状态机与安装平台不进入 Pinax spec。
- 将真实 Cloud Sync/Obsidian/恢复资格留给 `pinax-release-qualification-v1`；原 daily、memory、continuity、judgment、S3 的真实门保留。

## Capabilities

### New Capabilities

- `sync-control`: 已实现的公共同步状态、执行、冲突与原 operation 恢复合同。
- `notebook-source-attachments`: Pinax 的源码 revision、原子附件和回读合同。

### Modified Capabilities

无运行时接口变化。本 change 验收既有实现并补齐 canonical 规格。

## Impact

Pinax OpenSpec 与 owner 文档；不更改已有脏工作树中的 Go 代码或测试，不变更消费者、凭据、用户数据、发布或远端状态。
