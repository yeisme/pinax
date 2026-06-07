## Why

根 `pinax-daily-hot-notes-briefing` 设计已完成架构和跨项目 handoff。本 change 在 `cli/pinax` 内实现每日热点笔记 briefing 全链路：从 briefing recipe 到 candidate note 生成、可选飞书投递和反馈回写。

Hermes/OpenWebUI 当前无独立 owner，本 change 将 Hermes 记录为外部服务配置，使用 fake harness fixture 进行本地开发和测试。飞书 delivery MVP 路线优先使用 webhook adapter。

## What Changes

- 实现 briefing recipe service（`pinax briefing recipe init/show/set`）。
- 实现 research harness adapter（Hermes 作为外部服务配置 + fake fixture）。
- 实现 evidence ledger 和 scorer（dedupe、来源可信度、vault 相关性、新颖度）。
- 实现 candidate note generation（Markdown briefing_candidate、review queue）。
- 实现飞书 delivery adapter（webhook MVP，fake sender for testing）。
- 实现 feedback loop（accept/archive/dismiss/follow_up/less_like_this）。
- 覆盖 dry-run/yes/json/agent/events 输出合同。

## Capabilities

### New Capabilities

- `pinax-daily-briefing`: 每日热点笔记 briefing 全链路，从 recipe 到 delivery。

## Impact

- 新增 Go 包：`internal/briefing`、`internal/research`、`internal/delivery`。
- 修改 CLI 命令树：新增 `pinax briefing` 命令组。
- 飞书 webhook secret 通过 secret_ref 管理，不硬编码。
- 所有结构化资产由 CLI service 写入。

## Non-Goals

- 不直接实现 Hermes research harness，只提供 adapter 和 fake fixture。
- 不在 MVP 阶段实现原生飞书 SDK，优先 webhook adapter。
- 不改变 Pinax 核心笔记、vault 和索引功能。
