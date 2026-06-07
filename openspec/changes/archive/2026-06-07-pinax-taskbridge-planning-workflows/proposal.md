# Proposal: Pinax TaskBridge Planning Workflows

## 背景

Pinax 当前已经具备本地 Markdown vault、daily/inbox、note 管理、SQLite/GORM 索引、搜索、项目 metadata、dashboard、organize/repair 计划和只读 MCP surface。TaskBridge 已经具备多 Todo Provider、任务同步、项目拆分、治理、`today/next/review` 路线和 Agent JSON 契约。

下一步不应让 Pinax 重新实现 Todo provider，也不应让 TaskBridge 变成知识库。更合理的方向是：Pinax 升级为个人计划和知识操作系统，负责目标、计划、复盘、决策依据和知识链接；TaskBridge 保持任务执行控制面，负责任务事实、Provider 写回、安全 action file 和确认门禁。

## 目标

本 change 建立 Pinax 与 TaskBridge 的计划协作工作流：

- Pinax 通过 CLI-backed adapter 只读获取 TaskBridge 的任务事实和项目状态。
- Pinax 将任务事实、vault 中的目标/项目/日周月笔记、搜索索引和链接关系组合成每日、每周、每月计划。
- Pinax 把计划写入 Markdown vault，保留可搜索、可回滚、可审计的计划资产。
- Pinax 可以生成 TaskBridge action file 草稿，但真实任务写回必须交给 TaskBridge dry-run 和 confirm。
- Agent 只能消费稳定 `--json` / `--agent` 输出，不直接读写 `.pinax` 或 `.taskbridge` 结构化资产。

一句话产品目标：

> Pinax 是个人计划和知识操作系统；TaskBridge 是任务执行控制面。

## 非目标

- 不在 Pinax 中实现 Todo Provider 同步。
- 不让 Pinax 直接写 Microsoft Todo、Todoist、飞书任务、TickTick、Google Tasks 等远端 Provider。
- 不引入长期 daemon、自动排程引擎或日历排班系统作为 MVP 必需能力。
- 不把 TaskBridge 本地 store 当成 Pinax 直接读取的数据源。
- 不让 Agent 手写 planning snapshot、action file、receipt、event JSONL 或 `.pinax` metadata。
- 不把所有长期目标强制拆成远期 Todo；长期目标优先保存在 Pinax Markdown notes 中。

## 用户工作流

### 每日计划

```bash
taskbridge agent today
pinax plan daily --vault ./my-notes --taskbridge --dry-run --json
pinax plan daily --vault ./my-notes --taskbridge --yes
pinax daily open --vault ./my-notes --editor "$EDITOR"
```

输出结果：Pinax 在当天 daily note 中写入一个受管理的计划区块，包含今日必须做、推荐下一步、风险任务、项目推进、容量判断、决策依据和晚间复盘占位。

### 每周计划与复盘

```bash
pinax plan weekly --vault ./my-notes --taskbridge --dry-run --json
pinax plan weekly --vault ./my-notes --taskbridge --yes
```

输出结果：Pinax 写入或更新 weekly note 的计划区块，包含本周承诺、项目风险、上周继承项、需要冻结/放弃/调整的项目，以及建议生成的 TaskBridge action file 草稿。

### 每月和长期目标

```bash
pinax plan monthly --vault ./my-notes --taskbridge --dry-run --json
pinax note create "2026 Q3 目标" --kind goal --tags goal,quarterly --vault ./my-notes
pinax view save active-goals --kind goal --status active --vault ./my-notes
```

输出结果：Pinax 用 Markdown goal/project notes 保存长期方向，用 monthly/weekly/daily 计划逐层降解为可执行承诺，TaskBridge 只接近期可执行任务。

### 受控任务写回

```bash
pinax plan actions --vault ./my-notes --from daily --dry-run --json
taskbridge agent execute --action-file actions.json --dry-run
taskbridge agent execute --action-file actions.json --confirm
```

Pinax 只生成 action file 草稿和解释，不直接执行远端写入。

## 价值判断

- 对用户：每天先看 Pinax 计划，知道为什么做；执行任务时由 TaskBridge 安全写回。
- 对 Agent：Pinax 提供低 token 的计划上下文，TaskBridge 提供安全 action 执行边界。
- 对系统：Markdown vault 是计划和知识真源，Todo provider 是执行表面，二者边界清晰。

## Owner 和范围

- Owner: `cli/pinax`
- 主要实现路径：`cmd/pinax`、`internal/app`、`internal/domain`、`internal/output`、`internal/redaction`、`internal/testkit`
- 可能新增路径：`internal/taskbridge`、`internal/planning`
- 如 TaskBridge 缺少稳定 JSON 字段，应在 `cli/taskbridge/openspec/changes/` 单独创建小 change 补 contract，不在本 change 中修改 TaskBridge 行为。

