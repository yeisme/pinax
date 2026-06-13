## Context

这个 change 的目标是让 Pinax 直接支持用户和 agent 都熟悉的 SQL 形态查询本地 Markdown vault。产品上借鉴 database view 的 typed properties、filter、sort、pagination 和只返回所需 property，但不实现外部查询语法兼容层，也不把用户输入作为 raw SQLite SQL 执行。

Pinax 已经有 saved views、note list/search、SQLite/GORM index projection 和本地 Markdown vault 边界，但 saved views 只是简单过滤器，不能表达表格列、类型化属性、SQL 查询、分组、聚合、分页、任务视图或数据库式表单。这个变更把这些能力统一为本地 database view/query 层。

## Goals / Non-Goals

**Goals:**

- 从 Markdown frontmatter、inline fields、tags、links、系统字段和可选 task 行抽取 typed properties。
- 提供 Pinax SQL：支持 `SELECT ... FROM notes|tasks ... WHERE ... ORDER BY ... GROUP BY ... LIMIT ...` 这类 SQL-first 查询，但不把用户输入直接传给 SQLite。
- 支持数据库视图：table、list、cards、task，保存 columns、filters、sorts、group、limit、visible properties、query text 和 display options。
- 支持 Notion 风格属性类型：title、text、number、checkbox、select、multi_select、date、url、email、phone、relation、file、tags、created_time、updated_time、formula-lite。
- 支持高性能查询：property projection、typed value index、FTS/search candidate narrowing、cursor pagination 和 selected property loading。
- 所有结构化视图资产通过 CLI/service 写入 `.pinax/views.json` 或后续 `.pinax/databases/*.json`，不让 agent 手写。
- 输出遵守 AI-native CLI 合同，默认中文摘要，机器字段英文稳定。

**Non-Goals:**

- 不实现外部查询语法兼容，不支持 `TABLE/LIST/TASK` 查询入口。
- 不执行用户 JavaScript。
- 不兼容 Obsidian Bases 的私有文件格式或 UI 交互细节。
- 不把 Notion API 或 Notion 云数据库作为 Pinax 真源。
- 不实现完整 SQL 引擎、任意 join、子查询、窗口函数或跨 vault 查询。
- 不在 MVP 自动编辑大量 note frontmatter；批量修复 schema 只生成 reviewable plan。
- 不实现长期 daemon；增量索引由 CLI 写路径、显式 index 命令和后续 watcher change 承接。

## Decisions

### 1. 用 typed property projection 表达数据库行

每篇 note 是默认 database row。未来 task、block 或 attachment 可以成为其他 row source，但 MVP 先做 note row。

```text
DatabaseRow
  row_id             // note:<note_id>
  source_kind        // note|task|block
  note_id
  path
  title
  is_system
  created_at
  updated_at

PropertyDefinition
  property_id
  name
  normalized_name
  type               // title|text|number|checkbox|select|multi_select|date|url|email|phone|relation|file|tags|computed
  source             // frontmatter|inline|system|derived|view_schema
  multi              // bool

PropertyValue
  row_id
  property_id
  value_text
  value_number
  value_bool
  value_time
  value_json
  value_norm
```

理由：Notion 的 properties 和 Obsidian 的 file properties 本质都是 row + typed property。用 EAV 风格表可以支持用户自定义字段，同时通过类型列和索引避免所有值都变成字符串。

备选方案是给每个属性动态建列。它对固定 schema 快，但 Markdown vault 的属性高度动态，迁移和查询规划复杂，不适合作为 MVP 默认。

### 2. 属性来源和类型推断要保守

属性来源优先级：

1. system fields：`file.path`、`file.name`、`file.folder`、`file.tags`、`file.created`、`file.updated`、`note.id`。
2. frontmatter：YAML key/value。
3. inline fields：Pinax 支持的 `key:: value` 行或 `[key:: value]` 片段。
4. derived fields：links、backlinks、attachments、word_count、status 等 index projection。
5. view schema override：用户通过 CLI 声明某字段类型。

类型推断规则：boolean、number、date、list、link、string。冲突时保留原始值，type 标记为 `mixed` 或按 view schema 校验失败返回 warning，不静默丢弃。

### 3. Pinax SQL 是安全 DSL，不是 raw SQLite SQL

Pinax SQL 首版只支持 SQL 形态入口：

```text
SELECT title, status, due
FROM notes
WHERE tags CONTAINS "project" AND status = "active"
ORDER BY due ASC
LIMIT 20
```

内部统一解析成 AST：

```text
QueryAST
  result_kind: table|list|task|cards
  source: notes|tasks + folder/tag/link filters
  columns[]
  filters: expression tree
  sorts[]
  group_by[]
  limit
  cursor
  selected_properties[]
```

查询执行路径：

```text
Pinax SQL string / view definition
  -> lexer/parser
  -> semantic validation
  -> query planner
  -> repository query
  -> table/list/task projection
  -> output renderer
```

安全边界：不允许任意 SQL、join、子查询、窗口函数、文件路径函数、shell、网络、JS、正则灾难表达式或无限函数递归。表达式函数和操作符使用白名单，如 `CONTAINS`、`LIKE`、`IN`、`exists`、`date`、`today`、`length`、`lower`、`upper`、`link_count`。

### 4. Query planner 分阶段缩小候选集

性能路径按从便宜到昂贵执行：

1. source filter：folder、tag、kind、status、link target 等已有维度索引。
2. FTS/search query：如 query text 存在，用 FTS5 或现有 search index 缩小候选。
3. typed property filter：通过 `(property_id, typed_value)` 索引过滤。
4. in-memory expression finalize：只对候选 rows 执行函数、mixed type fallback、display formatting。
5. sort/group/limit/cursor。
6. selected property loading：只加载 table columns 需要的 property，避免 Notion 文档提到的大属性响应拖慢问题。

SQLite/GORM 是默认 repository。FTS5 virtual table 和复杂 planner 查询可以作为 `internal/index` 的受控 raw SQL 例外，必须集中封装、参数绑定、测试覆盖；命令层和 app service 不硬编码 SQL。

### 5. 数据库视图是 CLI-authored structured asset

`.pinax/views.json` 升级为版本化 registry，或后续拆分到 `.pinax/databases/<view_id>.json`。首版建议保留一个 registry，降低迁移成本：

```json
{
  "schema_version": "pinax.views.v2",
  "views": [
    {
      "id": "view_active_projects",
      "name": "active-projects",
      "kind": "table",
      "query": "SELECT title, status, due FROM notes WHERE tags CONTAINS \"project\" AND status = \"active\" ORDER BY due ASC LIMIT 50",
      "columns": ["title", "status", "due"],
      "filters": [],
      "sorts": [],
      "limit": 50,
      "updated_at": "..."
    }
  ]
}
```

用户和 agent 通过 `pinax database view save/show/list/delete` 或 `pinax view save --query` 管理，不手写 JSON。view 保存查询和显示配置，不保存结果快照。

### 6. 命令面保持小而可组合

建议命令：

```text
pinax query run 'SELECT title, status, due FROM notes WHERE tags CONTAINS "project" AND status = "active" ORDER BY due ASC LIMIT 20' --vault ./my-notes --json
pinax query explain 'SELECT title FROM notes WHERE status = "active" LIMIT 20' --vault ./my-notes
pinax database view save active-projects --query 'SELECT title, status, due FROM notes WHERE tags CONTAINS "project" AND status = "active" ORDER BY due ASC LIMIT 50' --kind table --vault ./my-notes --json
pinax database view show active-projects --vault ./my-notes --json
pinax database view list --vault ./my-notes
pinax database schema infer --vault ./my-notes --json
pinax database schema set status --type select --values active,done,paused --vault ./my-notes --json
```

`database schema set` 写的是 CLI-authored view/schema metadata，不直接批量改 note。若需要规范化 frontmatter，走 `metadata plan/apply` 或 repair/organize plan。

### 7. 输出和表格结果要可被 agent 稳定消费

JSON shape：

```json
{
  "command": "query.run",
  "facts": {
    "rows": "20",
    "columns": "3",
    "engine": "index",
    "index_status": "fresh",
    "has_more": "true"
  },
  "data": {
    "columns": [{"name":"status","type":"select"}],
    "rows": [{"note_id":"note_123","path":"notes/a.md","values":{"status":"active"}}],
    "page": {"cursor":"...","has_more":true}
  }
}
```

`--agent` 只输出 low-token facts 和 next action；不要输出完整表格。大结果集必须分页。

## Risks / Trade-offs

- 查询语言过大导致实现失控 -> Pinax SQL MVP 限定 `SELECT`、`FROM notes|tasks`、`WHERE`、`ORDER BY`、`GROUP BY`、`LIMIT`、基本函数；join、子查询和窗口函数延期。
- 用户以为 Pinax 支持完整 SQL -> help 和错误码明确 `sql_unsupported_clause`，`query explain` 展示受支持语法。
- 属性类型冲突导致筛选结果不稳定 -> property schema override + mixed type warning + explain evidence。
- EAV 查询慢 -> 建 `(property_id, value_norm)`、`(property_id, value_number)`、`(property_id, value_time)`、`row_id` 索引；planner 先缩小候选。
- 结构化 view asset 被 agent 手写 -> 只允许 CLI/service 创建修改，测试覆盖 schema version 和 redaction。
- 用户 SQL 被注入到底层 SQLite -> Pinax SQL parser 产出 AST，repository 参数绑定，不拼接用户表达式为 SQL。

## Migration Plan

1. 保持现有 `.pinax/views.json` v1 兼容；读取旧 view 时转换为 v2 的 filter-only table view projection。
2. `index status` 对缺少 property schema 的旧 index 返回 stale，并建议 `pinax index rebuild`。
3. 先实现 `database schema infer` 和 `query explain`，再实现 `query run` 写入路径。
4. 所有 view 保存/删除都通过 app service append redacted event evidence。
5. 回滚时删除新增 property projection 并重建 index，不影响 Markdown note 真源。

## Open Questions

- inline field 语法是否首版支持 `[key:: value]` 内联片段？建议支持行级 `key:: value`，内联片段作为后续增强。
- task row 是否首版支持？建议支持只读 task query，但 table view 默认还是 note row。
- 是否保留 `view` 命令还是引入 `database view` 命令？建议兼容旧 `view`，新增 `database view` 作为更清晰入口。
