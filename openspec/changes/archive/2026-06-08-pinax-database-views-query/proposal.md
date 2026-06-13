## Why

Pinax 已经有 saved views、索引和搜索，但还不能把本地 Markdown vault 当成可筛选、排序、分组和表格展示的知识数据库。现在需要把“笔记列表过滤”升级为 typed property projection + SQL-first 安全查询语言 + 可保存表格视图，让用户和 agent 能用熟悉的 SQL 形态稳定查询项目、任务、阅读清单、研究资料和自定义属性。

## What Changes

- 新增本地数据库视图能力：从 frontmatter、inline fields、系统字段和索引维度提取 typed properties，写入 SQLite/GORM 可重建 projection。
- 新增 Pinax SQL：提供受限 SQL 子集，例如 `SELECT title, status FROM notes WHERE tags CONTAINS "project" ORDER BY updated DESC LIMIT 20`，支持安全表达式、字段选择、别名、聚合和分页。
- 增强 saved views：从简单过滤器升级为 CLI-authored database view definition，支持 table/list/cards/task 视图、columns、filters、sorts、group、limit、visible properties 和 query text。
- 新增表格查询命令：`pinax query run`、`pinax query explain`、`pinax database view save/show/list/delete` 或等价命令，用同一 projection 输出中文摘要、`--json`、`--agent`、`--events` 和 `--explain`。
- 支持 Notion/Obsidian 风格属性类型：title、text、number、checkbox、select、multi_select、date、url、email、phone、relation、tags、file/path、created/updated、computed/formula-lite。
- 支持高性能过滤：用 property value 表、维度索引、FTS5/搜索索引和 cursor pagination 限制返回字段，避免大 vault 查询返回全量正文。
- 保持本地优先：不依赖 Notion API、Obsidian 插件、外部查询语法兼容、真实公网、provider token 或云数据库。

## Capabilities

### New Capabilities
- `database-views-query`: 定义本地 typed property database、Pinax SQL 查询语言、表格视图、筛选排序分组和分页输出。

### Modified Capabilities
- `notebook-workflows`: 将 saved views 从简单过滤器扩展为数据库视图，支持 table/list/cards/task 视图和 CLI-authored view definition。
- `notebook-index-search`: 扩展 SQLite/GORM index projection，增加 typed property、query planner、property indexes、cursor pagination 和查询 explain。
- `note-command-ux`: 让 note list/search/query 输出在机器模式下暴露稳定 properties、columns、filters、sorts 和 pagination facts。

## Impact

- 影响领域模型：`internal/domain` 需要新增 database、property schema、query AST、view definition、table result 和 cursor projection。
- 影响索引：`internal/index` 需要新增 property projection、property value index、query repository、可能的 FTS5 受控 raw SQL 例外。
- 影响应用层：`internal/app` 需要新增 query service、view service、safe expression evaluator、query explain 和 saved view lifecycle。
- 影响 CLI：`cmd/pinax` 新增或增强 query/database/view 命令；所有输出必须复用 `internal/output` projection。
- 影响结构化资产：`.pinax/views.json` 需要版本化升级为 database view registry；只能由 CLI/service 创建和修改。
- 影响测试：需要 parser tests、query planner tests、SQLite/GORM integration tests、contract tests、testscript e2e 和性能 benchmark。
- 不实现云端 Notion 数据库同步、Obsidian Bases 文件格式兼容、外部查询语法兼容、用户 JavaScript 执行、复杂公式引擎、多人协作或长期 daemon。
