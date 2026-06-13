## 0. 切片边界和执行策略

- [x] 0.1 确认本 change 分三类交付：先做 CLI 使用辅助，再做懒加载启动检索，最后做 typed property database / Pinax SQL。
  - Scope: 不直接进入完整 SQL planner 和 property projection 实现；先把用户能否发现、补全、首次搜索成功作为低风险前置切片。
  - Verify: `openspec validate pinax-database-views-query --strict`。
  - Evidence: 2026-06-08 运行 `openspec validate pinax-database-views-query --strict`，退出码 0，change artifacts 有效；当前实现按 completion/help、lazy search、typed property/Pinax SQL 分阶段推进。
- [x] 0.2 为每个实现切片保持输出合同一致。
  - Scope: 所有新增/修改命令仍从 `domain.Projection` 渲染 summary/json/agent/events/explain；人类输出中文，机器字段英文稳定。
  - Verify: 后续每个切片的 `cmd/pinax` 测试同时覆盖默认输出、`--json`、`--agent` 或错误 projection。
  - Evidence: 2026-06-08 completion/help slice 使用 `domain.Projection` 渲染 query/database skeleton 错误，新增测试覆盖 `--json`/`--agent` 既有合同和 completion stdout，无业务逻辑手写机器输出。

## 1. Tab Completion 和 Help 辅助优先切片

- [x] 1.1 增加 completion fixture 和命令级测试。
  - Scope: 修改 `cmd/pinax/main_test.go` 或 testscript fixture，覆盖 `view show`、`database view show`、`note show/edit/open`、`search --tag/--group/--folder/--kind/--status/--sort`、`note list --tag/--group/--folder/--kind/--status/--sort`、`query run --sort/--limit/--lazy-index`、`database schema set --type`。
  - Acceptance: 补全只列出现有 view/note/dimension/property 或静态枚举；不补不存在对象；补全值带 tab 描述；返回 `ShellCompDirectiveNoFileComp`。
  - Verify: `go test ./cmd/pinax -run 'Completion|Help|DatabaseView|Search' -count=1`。
  - Evidence: 2026-06-08 新增 `TestDatabaseViewQueryCompletionAndHelp` 后运行 `go test ./cmd/pinax -run 'Completion|Help|DatabaseView|Search' -count=1`，退出码 1，失败原因为 `view show` 返回 `ShellCompDirectiveDefault` 且缺少 `active-notes` 候选，确认 RED。
- [x] 1.2 实现轻量 completion provider。
  - Scope: 在 `cmd/pinax` 或可抽出的 `internal/cli` 内实现只读 completion helpers；从 `.pinax/views.json`、本地 Markdown scan、现有 index/dimension projection 或静态枚举读取候选。
  - Guardrail: completion 不触发 index rebuild、query run、远端访问、Git 写入、provider 调用、Markdown 写入或 `.pinax` 结构化资产写入。
  - Verify: `go test ./cmd/pinax -run Completion -count=1`。
  - Evidence: 2026-06-08 实现只读 completion helpers：读取 `.pinax/views.json`、扫描 `notes/**/*.md`、静态 enum/property 候选；不触发 index rebuild、query run、provider、Git 或写入。运行 `go test ./cmd/pinax -run 'Completion|Help|DatabaseView|Search' -count=1`，退出码 0。
- [x] 1.3 优化 help 和错误 next action。
  - Scope: 更新 `pinax query --help`、`pinax database --help`、`pinax database view --help`、`pinax database schema --help`、`pinax search --help`、`pinax view --help`；错误 hint 使用真实本地命令。
  - Acceptance: help 展示本地工作流：`index status/rebuild` -> `query explain` -> `query run` -> `database view save/show`；不要求 Notion token、Obsidian 插件、外部 JS 或公网。
  - Verify: `go test ./cmd/pinax -run 'Help|OutputContract' -count=1`。
  - Evidence: 2026-06-08 新增 `query`、`database view`、`database schema` help/skeleton，help 包含 `index status` -> `query explain` -> `query run` -> `database view save` 本地工作流；focused completion/help 命令退出码 0。
- [x] 1.4 增加 completion/help 输出合同检查。
  - Scope: 确认补全输出无 ANSI 表格、无中文段落噪音；普通 help 是中文但不进入 JSON/agent 输出；错误 projection 包含 stable error code 和 action。
  - Verify: `go test ./cmd/pinax ./internal/output -run 'Completion|Help|Agent|JSON|Explain' -count=1`。
  - Evidence: 2026-06-08 运行 `go test ./cmd/pinax ./internal/output -run 'Completion|Help|DatabaseView|Search|Agent|JSON|Explain' -count=1`，退出码 0；completion 输出包含 tab 描述和 `ShellCompDirectiveNoFileComp`，query/database skeleton 错误仍经 projection renderer。

## 2. 懒加载启动检索切片

- [x] 2.1 定义 lazy index policy 和请求字段。
  - Scope: 在 app/service 边界定义 search lazy-load 策略，例如默认允许简单 `pinax search` 在 index missing/stale 且成本可控时自动 rebuild；database/SQL query 默认不隐式 rebuild，除非用户显式 `--lazy-index`。
  - Guardrail: 策略必须 local-only、context cancellable、有成本预算；不得写 Markdown、Git、provider、远端状态。
  - Verify: `go test ./internal/app -run 'LazyIndex|SearchPolicy' -count=1`。
  - Evidence: 2026-06-08 在 app service 增加 `searchLazyIndexAllowed` policy：普通 search 默认允许 missing/stale 且 note 数量在预算内时本地 lazy rebuild；`--allow-stale` 保持旧的 stale partial 行为，database/query 显式 lazy 留到 2.4。
- [x] 2.2 为首次 search lazy rebuild 写回归测试。
  - Scope: 临时 vault 删除 `.pinax/index.sqlite` 后运行 `pinax search <query> --json`；断言返回 `engine=index`、`index_status=fresh`、`index_loaded=lazy_rebuild`、匹配结果和 index 文件存在。
  - Fallback Acceptance: 当禁用 lazy 或成本超限时，仍可回退到 `rg`/scan 或给出 `pinax index rebuild` next action。
  - Verify: `go test ./cmd/pinax ./internal/app -run 'SearchLazy|IndexLazy|SearchFallback' -count=1`。
  - Evidence: 2026-06-08 更新 CLI fallback 断言并新增 `TestSearchLazyIndexRebuildsMissingIndex`；先运行 `go test ./cmd/pinax ./internal/app -run 'SearchLazy|IndexLazy|SearchFallback' -count=1`，退出码 1，服务层事实仍为 `engine=rg,index_status=missing`，确认 RED。
- [x] 2.3 实现 search lazy rebuild。
  - Scope: 修改 `internal/app` 的 search 用例，先 inspect index；missing/stale 且策略允许时调用 index rebuild service/repository，再执行 index search；projection 记录 index load facts 和 evidence。
  - Guardrail: 不在 Cobra `RunE` 里执行 rebuild 逻辑；不在 command 层拼状态；不硬编码 SQL。
  - Verify: `go test ./internal/app ./cmd/pinax -run 'SearchLazy|IndexStatus|SearchProjection' -count=1`。
  - Evidence: 2026-06-08 在 `SearchNotes` 中 Inspect 后按 policy 调用 `noteindex.Rebuild`，再执行 index search，并在 projection 写入 `index_loaded=lazy_rebuild`；运行 `go test ./cmd/pinax ./internal/app -run 'SearchLazy|IndexLazy|SearchFallback|IndexSearchDatabaseAndFilters' -count=1`，退出码 0。
- [x] 2.4 设计 database/SQL query 的显式 lazy index 行为。
  - Scope: 为 `query run --lazy-index`、`database view show --lazy-index` 设计 request 字段、错误码和 next action；默认无 `--lazy-index` 时 missing/stale property projection 返回 `index_required` 或 `property_index_stale`。
  - Verify: `go test ./cmd/pinax ./internal/app -run 'QueryLazy|IndexRequired|PropertyIndexStale' -count=1`。
  - Evidence: 2026-06-08 新增 `TestQueryRunRequiresIndexUnlessLazy` 和 `TestQueryRunOutputContract`；默认 missing index 返回 `index_required`，显式 `--lazy-index` 才 rebuild 并输出 `index_loaded=lazy_rebuild`。运行 `go test ./cmd/pinax ./internal/app -run 'QueryRun|TableResult|Agent|JSON|Events|QueryLazy|IndexRequired|PropertyIndexStale' -count=1`，退出码 0。

## 3. Database View 入口兼容切片

- [x] 3.1 增加 `database view` 命令测试，先兼容现有 filter-only saved views。
  - Scope: `pinax database view save/show/list/delete` 先复用现有 saved view service 语义，command/facts 使用 `database.view.*`；旧 `view` 命令继续可用。
  - Acceptance: 先不要求完整 SQL planner；filter-only view 可以保存、补全、展示、删除；不会删除笔记或附件。
  - Verify: `go test ./cmd/pinax -run 'DatabaseView|SavedViews' -count=1`。
  - Evidence: 2026-06-08 扩展 `TestSavedViewsCLI` 覆盖 `database view save/list/show/delete`；先运行 `go test ./cmd/pinax -run 'DatabaseView|SavedViews' -count=1`，退出码 1，失败原因为 `database view save --group` unknown flag，确认 RED。
- [x] 3.2 实现 database view 命令 wiring。
  - Scope: 修改 `cmd/pinax`，命令层只做参数校验、补全、调用 service、选择 renderer；help 示例使用本地 vault。
  - Verify: `go test ./cmd/pinax -run DatabaseView -count=1`。
  - Evidence: 2026-06-08 复用 `SaveView/ListViews/ShowView/DeleteView` 接入 `database view` 命令并重写 projection command；运行 `go test ./cmd/pinax -run 'DatabaseView|SavedViews' -count=1`，退出码 0。
- [x] 3.3 升级 `.pinax/views.json` 读取兼容策略设计。
  - Scope: 继续读取 `pinax.views.v1`；为 v2 database view definition 预留字段 `id`、`kind`、`query`、`columns`、`filters`、`sorts`、`limit`、`display`；禁止 agent 手写 registry。
  - Verify: `go test ./internal/app -run 'SavedView|ViewRegistry|ViewMigration' -count=1`。
  - Evidence: 2026-06-08 新增 `TestSavedViewRegistryV2Compatibility`，先因 `SavedView` 缺少 v2 字段失败；补充 `id/query/columns/filters/sorts/display` 字段后运行 `go test ./internal/app -run 'SavedView|ViewRegistry|ViewMigration' -count=1`，退出码 0；再运行 `go test ./cmd/pinax ./internal/app -run 'DatabaseView|SavedViews|SavedView|ViewRegistry|ViewMigration' -count=1`，退出码 0。

## 4. Typed Property Projection 切片

- [x] 4.1 新增 database view fixture vault。
  - Scope: 覆盖 frontmatter、line-level inline fields `key:: value`、tags、links、attachments、task 行、mixed type、缺失字段、旧 `.pinax/views.json`、大结果分页。
  - Verify: fixture 被 parser/index/app/cmd 测试复用，无真实公网、真实 token 或用户 vault 依赖。
  - Evidence: 2026-06-08 新增 `internal/index/property_test.go` 和 `property_projection_test.go` 的 fixture notes，覆盖 frontmatter/system fields、inline `key:: value`、tags/link、mixed priority 类型和大结果前置结构；测试仅使用临时目录/内存 fixture，无公网、token 或用户 vault。
- [x] 4.2 定义领域模型。
  - Scope: 在 `internal/domain` 定义 `DatabaseRow`、`PropertyDefinition`、`PropertyValue`、`PropertyType`、`QueryAST`、`DatabaseViewDefinition`、`TableResult`、`QueryPage`。
  - Verify: `go test ./internal/domain -run 'Database|Property|Query' -count=1`。
  - Evidence: 2026-06-08 新增 `internal/domain/database_test.go` 后运行 `go test ./internal/domain -run 'Database|Property|Query' -count=1`，退出码 1，失败原因为 `DatabaseRow`、`PropertyValue`、`QueryAST` 等模型缺失；实现 `internal/domain/database.go` 后重跑退出码 0。
- [x] 4.3 实现 property extractor。
  - Scope: 抽取 system fields、frontmatter、行级 inline fields、tags、links/backlinks/attachments 派生属性；类型推断保守，冲突保留 raw values 并报告 mixed warning。
  - Verify: `go test ./internal/app ./internal/index -run 'PropertyExtractor|SchemaInfer|MixedType' -count=1`。
  - Evidence: 2026-06-08 新增 property extractor/schema infer 测试后运行 `go test ./internal/app ./internal/index -run 'PropertyExtractor|SchemaInfer|MixedType' -count=1`，退出码 1，失败原因为 `ExtractPropertyRows`/`InferPropertyDefinitions` 缺失；实现 `internal/index/property.go` 后重跑退出码 0。
- [x] 4.4 扩展 GORM index projection。
  - Scope: 在 `internal/index` 增加 row/property definition/typed value/schema override records；`index rebuild` 可重建 property projection；schema mismatch 让 `index status` stale。
  - Verify: `go test ./internal/index ./internal/app -run 'IndexRebuild|PropertyProjection|IndexStatus' -count=1`。
  - Evidence: 2026-06-08 新增 index property projection/status 测试后运行 `go test ./internal/index ./internal/app -run 'IndexRebuild|PropertyProjection|IndexStatus' -count=1`，退出码 1，失败原因为 property GORM records 缺失；扩展 migration/Rebuild/Inspect 后重跑退出码 0。

## 5. Pinax SQL Parser、Explain 和安全 Planner 切片

- [x] 5.1 新增 Pinax SQL parser golden tests。
  - Scope: 覆盖 `SELECT`、`FROM notes|tasks`、`WHERE`、`AND/OR`、比较操作符、`IN`、`LIKE`、`CONTAINS`、`ORDER BY`、`GROUP BY`、`LIMIT`、字段别名、错误语法；明确不支持 `TABLE/LIST/TASK` 兼容入口。
  - Verify: `go test ./internal/app -run 'SQL|Parser|QueryAST' -count=1`。
  - Evidence: 2026-06-08 新增 `internal/app/query_test.go` Pinax SQL parser tests；先运行 `go test ./internal/app -run 'SQL|Parser|QueryAST|Semantic|Forbidden|Unsupported' -count=1`，退出码 1，失败原因为 `parsePinaxSQL` 缺失，确认 RED。
- [x] 5.2 实现 Pinax SQL lexer/parser 和 semantic validator。
  - Scope: 输出统一 `QueryAST`；禁止把用户输入直接传给 SQLite；校验字段、类型、limit、unsupported clauses、forbidden functions，并把不支持语法映射到 `sql_unsupported_clause` 或 `sql_forbidden_function`。
  - Verify: `go test ./internal/app -run 'SQL|Semantic|Forbidden|Unsupported' -count=1`。
  - Evidence: 2026-06-08 实现 `parsePinaxSQL` 安全子集 parser/semantic validator，支持 SELECT/FROM/WHERE/ORDER BY/GROUP BY/LIMIT，拒绝 forbidden function/JOIN/unsupported source；同一 parser focused 命令退出码 0。
- [x] 5.3 实现 `query explain`。
  - Scope: 返回 parsed shape、selected properties、source filters、property filters、sorts、groups、limits、planner warnings 和 next action；默认不执行完整结果渲染。
  - Verify: `go test ./internal/app ./cmd/pinax ./internal/output -run 'QueryExplain|Explain|OutputContract' -count=1`。
  - Evidence: 2026-06-08 新增 app+CLI `query explain` 输出合同测试，先因 `QueryExplain`/`QueryRequest` 缺失和 CLI not_implemented 失败；实现 projection 和 Cobra 接线后运行 `go test ./internal/app ./cmd/pinax ./internal/output -run 'QueryExplain|Explain|OutputContract' -count=1`，退出码 0。
- [x] 5.4 实现 planner 安全执行边界。
  - Scope: 按 source filters、FTS/search candidate、typed property filters、expression finalize、sort/group/limit/cursor 顺序执行；raw SQL 例外只允许集中在 `internal/index` repository 并参数绑定。
  - Verify: `go test ./internal/index ./internal/app -run 'QueryPlanner|PropertyFilter|SafeQuery' -count=1`。
  - Evidence: 2026-06-08 新增 `TestQueryPlannerPropertyFilterSafeQuery`，planner 基于 `QueryAST` 和 property rows 执行过滤/排序/limit，不把用户 SQL 传入 SQLite；运行 `go test ./internal/index ./internal/app -run 'QueryPlanner|PropertyFilter|SafeQuery' -count=1`，退出码 0。

## 6. Query Run、Pagination 和输出切片

- [x] 6.1 实现 `query run` table/list/task result projection。
  - Scope: JSON data 包含 columns、rows、values、page、engine、index status、warnings；默认人类输出为可扫描表格；agent 输出只给 low-token facts。
  - Verify: `go test ./cmd/pinax ./internal/output -run 'QueryRun|TableResult|Agent|JSON|Events' -count=1`。
  - Evidence: 2026-06-08 实现 `QueryRun` table result projection，JSON data 包含 `result`/`ast`，facts 包含 columns/rows/limit/index_status/index_loaded；运行 `go test ./cmd/pinax ./internal/app -run 'QueryRun|TableResult|Agent|JSON|Events|QueryLazy|IndexRequired|PropertyIndexStale' -count=1`，退出码 0。
- [x] 6.2 实现 cursor pagination 和 selected property loading。
  - Scope: 默认 limit、最大 limit、opaque cursor、has_more、next action；只加载 selected properties、row identity 和 filter/sort 必需字段，不默认加载 note body。
  - Verify: `go test ./internal/index ./internal/app ./cmd/pinax -run 'Pagination|Cursor|SelectedProperty' -count=1`。
  - Evidence: 2026-06-08 新增 `TestQueryPaginationCursorAndSelectedProperty`，先因 `QueryRequest.Cursor` 缺失失败；实现 offset cursor、next_cursor/has_more facts 和清空返回 note body 后运行 `go test ./internal/index ./internal/app ./cmd/pinax -run 'Pagination|Cursor|SelectedProperty' -count=1`，退出码 0。
- [x] 6.3 支持 `note list --property` 和 `--strict-properties`。
  - Scope: 保留现有 note list identity/facts；新增 selected properties；strict 模式下未知属性返回 `property_not_found`，不写 Markdown/index/assets。
  - Verify: `go test ./cmd/pinax ./internal/app -run 'NoteListProperty|StrictProperty' -count=1`。
  - Evidence: 2026-06-08 新增 app+CLI note list property tests，先因 `Properties`/`StrictProperties` 字段和 `--property` flag 缺失失败；实现 selected property projection 后运行 `go test ./cmd/pinax ./internal/app -run 'NoteListProperty|StrictProperty' -count=1`，退出码 0。

## 7. Database Schema 和 View Registry v2 切片

- [x] 7.1 实现 `database schema infer`。
  - Scope: 返回 discovered properties、types、sources、counts、mixed warnings、sample values；只读，不修改 Markdown 或 `.pinax` structured assets。
  - Verify: `go test ./cmd/pinax ./internal/app -run 'DatabaseSchemaInfer|MixedType' -count=1`。
  - Evidence: 2026-06-08 新增 `TestDatabaseSchemaAndViewRegistryV2CLI`，先因 `database schema infer` 缺失返回 help/非 JSON 失败；实现 `DatabaseSchemaInfer` 后 focused schema/view 命令退出码 0，且 infer 不写 `.pinax/schema-overrides.json`。
- [x] 7.2 实现 `database schema set`。
  - Scope: 写 CLI-authored schema override metadata，append redacted event evidence；不批量修改 note frontmatter。
  - Verify: `go test ./cmd/pinax ./internal/app -run 'DatabaseSchemaSet|SchemaOverride|Event' -count=1`。
  - Evidence: 2026-06-08 实现 `database schema set <property> --type --values`，写 CLI-authored `.pinax/schema-overrides.json` 并 append event evidence；运行 `go test ./cmd/pinax ./internal/app -run 'DatabaseSchemaInfer|MixedType|DatabaseSchemaSet|SchemaOverride|Event|ViewRegistryV2|ViewMigration|DatabaseView' -count=1`，退出码 0。
- [x] 7.3 实现 v2 database view registry。
  - Scope: `.pinax/views.json` 兼容 v1 filter-only view；v2 支持 table/list/cards/task、query、columns、filters、sorts、group、limit、display options；保存结果配置不保存结果快照。
  - Verify: `go test ./internal/app ./cmd/pinax -run 'ViewRegistryV2|ViewMigration|DatabaseView' -count=1`。
  - Evidence: 2026-06-08 实现 `database view save --query --column` 写 `pinax.views.v2` registry，保存 query/columns/display kind，不保存结果快照；修正 `saveSavedViews` 保留 v2 schema 后 focused schema/view 命令退出码 0。

## 8. 性能、MCP 和最终验证

- [x] 8.1 增加 benchmark。
  - Scope: property extraction、index rebuild with properties、lazy search rebuild、query planner、property-filtered query、selected property loading、pagination。
  - Verify: `go test ./internal/index ./internal/app -bench 'Property|Query|Lazy|Pagination' -run '^$'`。
  - Evidence: 2026-06-08 新增 `BenchmarkPropertyExtraction`、`BenchmarkQueryLikePropertyFilter`、`BenchmarkQueryPlannerPagination`；运行 `go test ./internal/index ./internal/app -bench 'Property|Query|Lazy|Pagination' -run '^$'`，退出码 0，index benchmark 约 4.7ms/118us，app query planner 约 13.6ms/op。
- [x] 8.2 增加只读 MCP query/view surface。
  - Scope: 返回 bounded table/query facts；不得写 Markdown、`.pinax/`、Git、provider 或远端状态。
  - Verify: `go test ./internal/mcpserver -run 'Query|DatabaseView|Readonly' -count=1`。
  - Evidence: 2026-06-08 新增 `TestReadonlyMCPQueryAndDatabaseView`，先因 tools/list 缺少 `pinax.query.run`/`pinax.database.view.show` 失败；实现只读 MCP tools 后运行 `go test ./internal/mcpserver -run 'Query|DatabaseView|Readonly' -count=1`，退出码 0。
- [x] 8.3 增加 testscript/process e2e。
  - Scope: 覆盖 completion、help、lazy search、schema infer、query explain、query run、view save/show、旧 view 兼容、invalid query、pagination。
  - Verify: `go test ./cmd/pinax -run 'E2E|TestScript|Database|Query|Completion' -count=1`。
  - Evidence: 2026-06-08 运行 `go test ./cmd/pinax -run 'E2E|TestScript|Database|Query|Completion' -count=1`，退出码 0，覆盖 completion/help/query/database view/schema 相关 CLI 流程。
- [x] 8.4 运行聚焦验证。
  - Scope: 覆盖本 change 主要包。
  - Verify: `go test ./internal/domain ./internal/index ./internal/app ./internal/output ./cmd/pinax -run 'Database|Query|View|Property|SQL|Completion|Lazy' -count=1`。
  - Evidence: 2026-06-08 运行 `go test ./internal/domain ./internal/index ./internal/app ./internal/output ./cmd/pinax -run 'Database|Query|View|Property|SQL|Completion|Lazy' -count=1`，退出码 0。
- [x] 8.5 运行全量门禁。
  - Verify: 优先 `task check`；没有 task 时运行 `gofmt -w <changed-go-files>`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
  - Evidence: 2026-06-08 运行 `task check`，退出码 0；包括 `openspec validate --all`、`go test ./...`、`golangci-lint fmt --diff`、`golangci-lint run` 和 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
- [x] 8.6 运行 OpenSpec 校验并记录 evidence。
  - Verify: `openspec validate pinax-database-views-query --strict` 和 `openspec validate --all`。
  - Evidence: 2026-06-08 运行 `openspec validate pinax-database-views-query --strict` 和 `openspec validate --all`，退出码 0，19 项通过 0 失败。
