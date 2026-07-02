# pinax-dataview-database-query 任务

## 0. 任务约束

- Owner: `cli/pinax`。
- 兼容性：只做 additive change；不得移除、重命名、重定义现有 CLI envelope 顶层字段、命令名、`status` 枚举和已存在 `--agent` key。
- 注释：新增或修改复杂 parser、AST lowering、managed block 写入、错误边界和非显然 fixture 时必须写中文注释说明不变量。
- 质量门禁：每个实现批次至少运行相关 focused tests；完成前运行 `task check`。涉及集成入口时运行 `task test:integration` 并保留脱敏 evidence。

## 1. SQL v2 parser 和 AST 扩展

- [x] Owner: `cli/pinax`; Lane: A; Depends on: none
- Scope: `internal/domain/database.go`、`internal/app/searchops/query.go`、`internal/app/searchops/query_test.go`
- Work: 扩展 AST 支持 comparison operators、`IN`、`EXISTS`、`IS EMPTY`、aggregate select、`GROUP BY` 和 source kind。
- Acceptance: `go test ./internal/app/searchops ./internal/domain -run 'Query|SQL|Aggregate|Group' -count=1` 通过；现有 `TestQueryRunOutputContract` 继续通过。
- Evidence: `go test ./internal/app/searchops ./internal/domain -run 'Query|SQL|Aggregate|Group' -count=1` 通过；`go test ./cmd/pinax -run TestQueryRunOutputContract -count=1` 通过；补充运行 `go test ./internal/app/searchops ./internal/domain ./internal/app -run 'Query|SQL|Aggregate|Group' -count=1` 覆盖 parser/app 单测。
- Failure re-check: 若现有 SQL 报错码变化，补兼容 shim 或恢复旧错误码。

## 2. 查询执行器支持类型比较、聚合、分组和稳定分页

- [x] Owner: `cli/pinax`; Lane: A; Depends on: 1
- Scope: `internal/app/searchops/query.go`、`internal/index` 必要投影读取、`internal/domain/database.go`
- Work: 对 frontmatter/inline properties 做 number/date/bool/list 规范比较；实现 `COUNT/MIN/MAX` 和 group rows；保留默认 limit 50 与 cursor 分页。
- Acceptance: `go test ./internal/app/searchops ./internal/index -run 'Query|Property|Group|Page' -count=1` 通过；查询输出不包含 note body。
- Evidence: `go test ./internal/app/searchops ./internal/index -run 'Query|Property|Group|Page' -count=1` 通过；`go test ./internal/app ./cmd/pinax -run 'Query|SQL|Aggregate|Group|Page' -count=1` 通过；新增 searchops 单测覆盖 `COUNT/MIN/MAX`、`GROUP BY`、类型比较、分页和 body 清空。
- Failure re-check: 对超过 limit、空字段、混合类型、非法 cursor 加测试并返回稳定错误或空结果。

## 3. Dataview parser 和 `pinax dataview` 命令

- [x] Owner: `cli/pinax`; Lane: B; Depends on: 1
- Scope: `internal/app/searchops/dataview.go`、`internal/cli/search_database_cmd.go`、`cmd/pinax/search_database_command_test.go`
- Work: 新增 `dataview run` 和 `dataview explain`，支持 `TABLE/LIST/TASK FROM ... WHERE ... SORT ... GROUP BY ... LIMIT ...` 子集并 lowering 到 `QueryAST`。
- Acceptance: `go test ./cmd/pinax ./internal/app/searchops -run 'Dataview|Query' -count=1` 通过；`pinax dataview run 'TABLE title FROM #project LIMIT 5' --vault <vault> --json` 输出 `command=dataview.run`。
- Evidence: `go test ./cmd/pinax ./internal/app/searchops -run 'Dataview|Query' -count=1` 通过；新增 `ParseDataview` lowering 单测和 `dataview run/explain` CLI 输出合同测试，`dataview run` JSON envelope 输出 `command=dataview.run`。
- Review fix evidence: 2026-06-20 新增 `TestParseDataviewAcceptsMultilineClauses` 覆盖 `TABLE` 和 `TASK` 查询中 `FROM/WHERE/SORT/GROUP BY/LIMIT` 按换行分隔的常见 Dataview fenced block 形态；修复 parser 的 clause 切分从单空格匹配改为 whitespace-boundary 扫描，并避免匹配引号内 clause。运行 `go test ./internal/app/searchops -run TestParseDataviewAcceptsMultilineClauses -count=1` 通过；运行 `go test ./internal/plugin ./internal/app/searchops ./cmd/pinax -count=1` 通过。
- Failure re-check: 对 DataviewJS、unsupported function、网络/文件/环境访问语法返回 `dataview_unsupported_clause` 或 `dataview_forbidden_function`。

## 4. Task source 和 block/task 投影

- [x] Owner: `cli/pinax`; Lane: B; Depends on: 2,3
- Scope: `internal/index/model/records.go`、`internal/index/gormgen/main.go`、`internal/index/query/*`、`internal/app/searchops/query.go`
- Work: 为 Markdown task list 建立可重建投影，字段包括 note path、line、text、completed、due、scheduled、priority、tags、block id。
- Acceptance: 先运行 `task gen:index`，再运行 `go test ./internal/index ./internal/app/searchops ./cmd/pinax -run 'Task|Dataview|Query' -count=1` 通过。
- Evidence: `task gen:index` 通过并生成 `internal/index/query/task_records.gen.go`；`go test ./internal/index ./internal/app/searchops ./cmd/pinax -run 'Task|Dataview|Query' -count=1` 通过；`FROM tasks` 查询覆盖 text/completed/due/priority/tags/block_id，结果不泄露 note body。
- Failure re-check: 确认 GORM Gen guard 不允许业务层 direct SQL；任务正文投影不得泄露整篇 note body。

## 5. Links/backlinks/assets source 查询

- [x] Owner: `cli/pinax`; Lane: C; Depends on: 2
- Scope: `internal/app/searchops/query.go`、`internal/index`、`cmd/pinax/search_database_command_test.go`
- Work: 让 `FROM links|backlinks|assets` 可查询 source path、target、status、kind、linked_notes、media_type、missing/orphan 状态。
- Acceptance: `go test ./internal/app/searchops ./cmd/pinax -run 'Link|Backlink|Asset|Query|Dataview' -count=1` 通过。
- Evidence: `go test ./internal/app/searchops ./cmd/pinax -run 'Link|Backlink|Asset|Query|Dataview' -count=1` 通过；新增 `FROM links|backlinks|assets` source row 测试，覆盖 source path、target、status、kind、linked_notes、media_type。
- Failure re-check: 断链和歧义链接必须返回事实字段，不自动修复或选择候选。

## 6. Database view v3 和视图渲染增强

- [x] Owner: `cli/pinax`; Lane: C; Depends on: 2,3
- Scope: `internal/domain/database.go`、`internal/app` view registry、`internal/cli/search_database_cmd.go`、`cmd/pinax/search_database_command_test.go`
- Work: `database view save` 支持 `--language sql|dataview`、`--kind table|list|task|calendar|board`、`--group-by`、`--calendar-field`、`--board-column`、`database view render`。
- Acceptance: `go test ./cmd/pinax -run 'DatabaseView|Dataview|Query' -count=1` 通过；旧 v1/v2 view fixture 可读；写回为 v3。
- Evidence: `go test ./cmd/pinax -run 'DatabaseView|Dataview|Query' -count=1` 通过；`go test ./internal/app -run 'SavedView|DatabaseView|Query' -count=1` 通过；`database view save` 写 `pinax.views.v3`，支持 `--language dataview`、`--group-by`、`--calendar-field`、`--board-column` 和 `database view render`。
- Failure re-check: 缺少 `--yes` 的 delete 仍返回 `approval_required`；保存视图只通过 CLI/service 写 `.pinax/views.json`。

## 7. 内嵌 `pinax-dataview` managed block

- [x] Owner: `cli/pinax`; Lane: D; Depends on: 3,6
- Scope: `internal/app/templateops`、`internal/app/noteops`、`cmd/pinax/note_record_command_test.go`、`tests/e2e/testdata` 如需要
- Work: 支持 fenced block `pinax-dataview`，`note preview` 可只读渲染，`note refresh --rendered` 只更新 `pinax:managed` 块。
- Acceptance: `go test ./internal/app ./cmd/pinax -run 'Dataview|Managed|Refresh|Preview' -count=1` 通过；未标记 managed 的 block 不被写入。
- Evidence: `go test ./internal/app ./cmd/pinax -run 'Dataview|Managed|Refresh|Preview' -count=1` 通过；`note preview` 只读渲染 `pinax-dataview` fence；`note refresh --rendered --yes` 只替换匹配的 `pinax:managed` block，并保留用户正文。
- Failure re-check: 写入前必须有版本/record/index evidence，失败不得留下半更新正文。

## 8. 输出合同、帮助、补全和文档

- [x] Owner: `cli/pinax`; Lane: E; Depends on: 3,6,7
- Scope: `cmd/pinax/cli_output_contract_test.go`、`README.md`、`README.zh-CN.md`、`docs/operations/local-development.md`、`docs/commands/README.md`
- Work: 更新 help 示例、shell completion、JSON/agent/events/explain contract tests；文档展示真实可运行命令，不展示内部 wrapper。
- Acceptance: `go test ./cmd/pinax -run 'CLIOutput|Help|Completion|Dataview|Database|Query' -count=1` 通过。
- Evidence: `go test ./cmd/pinax -run 'CLIOutput|Help|Completion|Dataview|Database|Query' -count=1` 通过；新增 `docs/commands/dataview.md`，更新 query/database command docs、command map、README 和 local development docs，示例均为真实 `pinax ...` 命令。
- Failure re-check: `--json` stdout 必须是单个 JSON envelope；`--agent` 不含中文 prose、ANSI 或 note body。

## 9. E2E 和集成证据

- [x] Owner: `cli/pinax`; Lane: sequential; Depends on: 1-8
- Scope: `tests/e2e/testdata/dataview_database/scripts/*`、`tests/e2e/*`、`temp/integration-test-runs/<run-id>/`
- Work: 构造真实 vault，覆盖 frontmatter、inline fields、tasks、links/backlinks、assets、saved view、managed dataview block。
- Acceptance: `task test:integration` 通过，并生成脱敏 evidence；`task check` 通过；`openspec validate pinax-dataview-database-query --strict` 和 `openspec validate --all --strict` 通过。
- Evidence: `go test ./tests/e2e -run TestDataviewDatabase -count=1` 通过；`task test:integration` 通过并生成 `temp/integration-test-runs/20260620T170528Z-165852/`；精确 payload 扫描 `Authorization: Bearer|Bearer [A-Za-z0-9._-]+|DATAVIEW_DATABASE_BODY_SENTINEL|raw prompt|provider payload|full chain-of-thought|hidden system prompt|private tool arguments` 无命中；`task check` 通过；`openspec validate pinax-dataview-database-query --strict` 和 `openspec validate --all --strict` 通过。
- Review fix evidence: 2026-06-20 修复 Dataview 多行 clause parsing 后运行 `task check` 通过，覆盖 `golangci-lint fmt --diff`、`openspec validate --all`（45/45 items）、`golangci-lint run`（0 issues）、`go test ./...`、kb sidecar Python tests 和 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
- Review fix evidence: 2026-06-21 refreshed integration evidence with `task test:integration`; current run is `temp/integration-test-runs/20260621T073517Z-965454/`. Redaction scan for `DATAVIEW_DATABASE_BODY_SENTINEL`, Authorization/Bearer, provider payload, raw prompt, hidden prompt and private tool argument patterns had no matches.
- Failure re-check: evidence 扫描不得命中 `Authorization|Bearer|secret|raw prompt|provider payload|full chain-of-thought|note body sentinel`。
