# Pinax TaskBridge 每日 Markdown TodoList

## Why

TaskBridge 已经能输出每日任务控制面的稳定机器结果，Pinax 已经能生成 daily note 和 planning snapshot。当前缺口是用户每天开始工作时，不能把 TaskBridge 的当天任务事实自然落到 Pinax 的每日 Markdown 笔记里，导致“今天做什么”和“为什么这么安排”分散在两个工具中。

## What Changes

- 扩展 `pinax plan daily --taskbridge`，通过 TaskBridge CLI 读取当天任务事实。
- `--dry-run` 只返回计划预览和目标 daily note，不写 Markdown 或 `.pinax` 资产。
- `--yes` 创建或更新 `daily/YYYY-MM-DD.md`，只写 `planning-daily` managed block。
- `--save --yes` 继续保存 planning snapshot，记录 TaskBridge 来源、`captured_at`、任务计数、风险和证据引用。
- 保留 Pinax 为 Markdown vault 写入方，TaskBridge 不直接写 Pinax vault。

## 不做什么

- 不新增 `taskbridge today --format markdown --output ...`，TaskBridge Markdown 导出后续单独设计。
- 不让 Pinax 直接读取 `~/.taskbridge` store、Provider token 或 Provider API。
- 不从 Markdown todolist 反解析为 TaskBridge action；action draft 仍从 planning snapshot/decision 生成。
- 不执行 `taskbridge agent execute --confirm`。

## 影响范围

- Owner：`cli/pinax`。
- 主要命令：`pinax plan daily --taskbridge`、`pinax plan actions --from daily --taskbridge`。
- 主要规格：`planning-workflows`、`notebook-workflows`。
