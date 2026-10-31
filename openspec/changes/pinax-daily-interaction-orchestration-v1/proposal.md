# Pinax 日常交互闭环编排

## Why

Pinax 已经分别具备 capture、search、inbox review、memory review、planning、proof loop 和 continuity 能力，但这些命令的状态和下一步动作没有一个共同的交互投影。用户和 Operator 需要自己判断当前处于记录、找回、审阅、计划、应用还是交接阶段，容易把只读预览误认为已完成写入，也难以从一个命令安全地进入下一个命令。

本变更把现有命令组合为一个无状态的日常交互投影，保持 Markdown vault、现有应用服务、proposal、snapshot、revision、receipt 和 operation recovery 为唯一事实来源。

## What Changes

- 为现有 `domain.Projection` 增加可选 `pinax.daily_interaction.v1` 投影。
- 根据现有 command、status、facts、warnings 和 owner actions 派生 `phase`、`state`、读写/远端影响、source/freshness 状态、一个推荐动作和最多三个恢复动作。
- 在统一 CLI renderer 边界装配 projection，使 `--json`、`--agent`、`--events`、`--explain` 与人类摘要共用同一事实。
- capture、search、review、plan、organize、continue 和 checkpoint 继续复用原有命令与门禁，不新增顶层命令、持久 session、数据库表或 flow id。
- 新增根仓 `pinax-daily-interaction-operator` Skill，负责 Codex/Claude Code 的自然语言编排，不读取 Pinax 私有存储，不猜 vault/scope，不自行确认写入。

## Boundaries

- Pinax 仍拥有 canonical Markdown、权限、revision、proposal、snapshot、receipt 和 recovery。
- DriveBridge 只消费已有公开合同，本变更不改桌面 UI 或人读 API。
- 默认流程不调用 provider、不执行远端写入、不自动确认 memory、不自动 apply/sync/publish。
- 两周真实使用观察是独立 evidence gate；本地测试通过不代表 adoption 或 continuity dogfood 完成。

## Rollback

移除 additive interaction decoration、Skill profile assignment 和 generated runtime copies即可回退。既有 CLI envelope、facts、commands、vault、receipts、memory 和 continuity 数据不删除、不迁移。
