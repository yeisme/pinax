# pinax-dataview-database-query 提案

## 问题

Pinax 已经支持本地 SQLite/GORM index、`pinax search`、`pinax query run/explain`、`database view`、frontmatter property、query-backed template 和 managed block，但当前能力仍偏“窄 SQL 查询”：

- 只能覆盖简单 `SELECT ... FROM notes WHERE ... ORDER BY ... LIMIT ...`，表达力不足以替代 Obsidian Dataview 的常见用法。
- `database view` 能保存查询，但视图类型、列配置、分组、聚合、任务源、日历/看板等数据库体验还不完整。
- 内嵌查询目前分散在 template 和 managed block 场景，缺少统一的 Dataview 风格入口、解释、验证和刷新路径。
- 用户需要从真实 Markdown vault 直接得到“我有哪些活跃项目、过期任务、近期更新、断链高风险笔记、按属性分组的知识库表格”等数据库视图，而不是只能全文搜索。

## 目标用户

- 用 Markdown/Obsidian/Logseq 管理长期知识库的个人用户。
- 需要让 agent 安全读取结构化笔记上下文、但不想暴露全文正文或让 agent 直接改 `.pinax` 元数据的 Pinax 用户。
- 需要从 frontmatter、inline field、tags、links/backlinks、tasks、attachments 和 project metadata 中构建本地数据库视图的高级用户。

## MVP 范围

本变更把 Pinax 查询层升级为“SQL-first + Dataview-compatible”的本地数据库能力：

1. 增强 Pinax SQL v2：比较运算、`IN`、`EXISTS`、`IS EMPTY`、日期/布尔/数字类型比较、`GROUP BY`、`COUNT`、基础聚合和稳定分页。
2. 新增只读 `pinax dataview` 命令族：支持 Dataview 风格 `TABLE`、`LIST`、`TASK` 查询，并编译到同一安全 AST，不执行 JavaScript。
3. 新增 query source：`notes`、`tasks`、`links`、`backlinks`、`assets`，其中 `tasks` 来自 Markdown task list，不要求外部 Todo provider。
4. 增强 `database view`：保存 `table|list|task|calendar|board` 视图定义，支持列、分组、排序、限制、渲染偏好和 `--from-query`。
5. 统一内嵌查询块：`pinax-sql` 和 `pinax-dataview` fenced blocks 共用查询服务，`note refresh --rendered` 可安全刷新 managed blocks。
6. 保持现有 `query.run`、`query.explain`、`database.view.*` envelope 顶层字段、命令名和基础 facts 向后兼容，只新增可选 `data` 字段和 facts。

## 非目标

- 不实现 DataviewJS，不执行 JavaScript、shell、网络请求、环境变量读取或任意 SQLite SQL。
- 不把 SQLite 数据库变成笔记真源；Markdown vault 仍是真源，index 是可重建投影。
- 不做完整 Obsidian 插件兼容，不支持所有 Dataview 语法边角。
- 不做 GUI 数据库编辑器；CLI/API/MCP 投影先行。
- 不让 agent 自动重写正文。内嵌查询刷新必须走 managed block 和显式命令。

## 兼容性

本变更为 additive change：新增命令、source、可选字段、可选 facts 和新 managed block 类型。不得删除、重命名或重定义现有 `query.run`、`query.explain`、`database.view.save/list/show/delete` 输出字段。若实现过程中发现必须改动既有字段，必须先扩展本 OpenSpec 的 migration/deprecation/rollback 段并等待 review。

## 验收摘要

- 用户可以运行 SQL v2 查询：`pinax query run 'SELECT status, COUNT(*) AS count FROM notes WHERE tags CONTAINS "project" GROUP BY status LIMIT 20' --vault ./my-notes --json`。
- 用户可以运行 Dataview 查询：`pinax dataview run 'TABLE title, status, due FROM #project WHERE status != "done" SORT due ASC LIMIT 20' --vault ./my-notes --json`。
- 用户可以查询任务：`pinax dataview run 'TASK FROM #project WHERE !completed SORT due ASC LIMIT 20' --vault ./my-notes --json`。
- 用户可以保存和显示数据库视图：`pinax database view save project-dashboard --from-query active-projects --kind table --group-by status --vault ./my-notes --json`。
- 用户可以在笔记里放 `pinax-dataview` fenced block，然后运行 `pinax note refresh Dashboard --rendered --vault ./my-notes --json` 刷新 managed table。
- 所有机器输出继续满足 AI-native CLI output contract；运行证据不包含 note body、raw prompts、provider payload、Authorization header、secret 或 full chain-of-thought。
