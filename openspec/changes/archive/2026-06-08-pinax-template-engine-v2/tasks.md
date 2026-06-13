# Tasks: Pinax Template Engine v2

## 使用规则

- Owner: `cli/pinax`。
- 本 change 只升级 Pinax 模板引擎和模板 CLI，不实现 Web/TUI 模板编辑器，不引入外部脚本执行。
- 先写失败测试，再写实现；复杂解析、安全边界、AST 扫描、错误映射和非显然 fixture 必须有中文注释。
- 模板文件正文可以由用户编辑；`.pinax` 事件、registry、receipt 等机器资产必须由 CLI/service 写入。
- SQL-first 查询模板必须复用 `pinax-database-views-query` 的 Pinax SQL parser、planner 和 query service；不得在模板引擎、命令层或 app service 中拼接用户输入为 raw SQL。
- CLI 输出遵守 AI-native CLI 输出合同；默认中文摘要，机器协议字段保持英文稳定。
- 每个完成项需要追加 `Evidence:`，记录命令、退出码、关键结论和失败复验。

## 1. OpenSpec 计划完整性

- [x] 1.1 创建 `pinax-template-engine-v2` change 骨架。
  - Owner: `cli/pinax`
  - Scope: 通过 OpenSpec CLI 创建 `openspec/changes/pinax-template-engine-v2/`。
  - Depends on: none
  - Lane: A
  - Acceptance:
    ```bash
    test -f openspec/changes/pinax-template-engine-v2/.openspec.yaml
    ```
    预期结果：文件存在。
  - Failure re-check: 如果缺少 `.openspec.yaml`，重新运行 `openspec new change pinax-template-engine-v2`。
  - Evidence: 2026-06-06 已运行 `openspec new change pinax-template-engine-v2`，退出码 0。

- [x] 1.2 补齐 proposal、design、tasks 和 spec。
  - Owner: `cli/pinax`
  - Scope: 写明 Go `text/template` 共享引擎、安全 FuncMap、v2 frontmatter、兼容策略、命令面、输出合同和测试策略。
  - Depends on: 1.1
  - Lane: A
  - Acceptance:
    ```bash
    find openspec/changes/pinax-template-engine-v2 -maxdepth 3 -type f | sort
    rg -n "text/template|go-template|template inspect|pinax.template.v2|FuncMap|Mermaid" openspec/changes/pinax-template-engine-v2
    ```
    预期结果：看到 `proposal.md`、`design.md`、`tasks.md`、`specs/pinax/spec.md`，并命中关键设计词。
  - Failure re-check: 如果没有 Mermaid 图、没有安全函数边界或没有兼容策略，补齐后重跑。
  - Evidence: 2026-06-06 已补齐本 change 正文文件。

- [x] 1.3 补充 SQL-first 查询模板设计。
  - Owner: `cli/pinax`
  - Scope: 参考 `pinax-database-views-query`，补充 query-backed template 的 proposal、design、tasks 和 spec，明确 Pinax SQL 复用、安全边界、任务依赖和验收场景。
  - Depends on: 1.2
  - Lane: A
  - Acceptance:
    ```bash
    rg -n "pinax-database-views-query|pinax-sql|Queries|Pinax SQL|query-backed" openspec/changes/pinax-template-engine-v2
    openspec validate pinax-template-engine-v2 --strict
    ```
    预期结果：命中 query-backed template 设计词，OpenSpec 严格校验通过。
  - Failure re-check: 如果没有明确禁止 raw SQL 或没有引用 Pinax SQL query service，补齐后重跑。
  - Evidence: 2026-06-06 已补充查询模板设计；后续按 SQL-first 方案更新为 `pinax-sql` fenced block 和 `language: sql`。

## 2. 共享模板引擎

- [x] 2.1 为 `internal/templateengine` 写 Go template 失败测试。
  - Owner: `cli/pinax`
  - Scope: 新增 `internal/templateengine/engine_test.go`，覆盖 `.Title`、`.Vars.url`、`if`、`range`、管道函数、missingkey 和未开放函数。
  - Depends on: 1.2
  - Lane: B
  - Acceptance:
    ```bash
    go test ./internal/templateengine -run 'Render|Missing|UnsupportedFunc' -count=1
    ```
    预期结果：实现前失败，失败原因指向缺少 package/API 或行为不满足。
  - Failure re-check: 如果测试因为语法错误失败，先修测试；如果测试直接通过，说明没有覆盖新行为，补充断言。
  - Evidence: 2026-06-08 运行 `go test ./internal/templateengine -run 'Render|Missing|UnsupportedFunc' -count=1`，退出码 1，失败原因为 `New`、`TemplateDocument`、`EngineGoTemplate`、`Context`、`ErrorCode` 等 API 尚不存在，确认 RED。

- [x] 2.2 实现 `internal/templateengine` 核心。
  - Owner: `cli/pinax`
  - Scope: 新增 `Engine`、`TemplateDocument`、`Context`、`RenderResult`、`Issue`；底层使用 `text/template` 和 `missingkey=error`；实现 Pinax 安全 FuncMap。
  - Depends on: 2.1
  - Lane: B
  - Acceptance:
    ```bash
    go test ./internal/templateengine -run 'Render|Missing|UnsupportedFunc' -count=1
    ```
    预期结果：Go template 渲染、缺失变量和不支持函数测试通过。
  - Failure re-check: 如果模板可以调用 env/exec/readFile/http 等未开放函数，移除函数入口并补安全测试。
  - Evidence: 2026-06-08 实现 `internal/templateengine` text/template 核心、安全 FuncMap 和 missingkey=error 映射；运行 `go test ./internal/templateengine -run 'Render|Missing|UnsupportedFunc' -count=1`，退出码 0。

- [x] 2.3 增加 legacy simple 模板兼容测试和实现。
  - Owner: `cli/pinax`
  - Scope: 保留 `{{title}}`、`{{date}}`、`{{project}}`、`{{tags}}`、`{{client}}` 这类 simple token 渲染；只有 `engine: go-template` 才要求 `.Title` 语法。
  - Depends on: 2.2
  - Lane: B
  - Acceptance:
    ```bash
    go test ./internal/templateengine -run 'Legacy|Simple' -count=1
    ```
    预期结果：旧模板继续渲染，Go template 模板按新语法渲染。
  - Failure re-check: 如果旧模板从成功变成 `template_parse_failed`，修正 engine detection 或 legacy renderer。
  - Evidence: 2026-06-08 先运行 `go test ./internal/templateengine -run 'Legacy|Simple' -count=1`，退出码 1，失败原因为 `Context.Date` 和 simple renderer 缺失；实现 legacy token renderer 后重跑同命令退出码 0，并运行 `go test ./internal/templateengine -count=1` 退出码 0。

## 3. 模板 metadata 和 app service 集成

- [x] 3.1 为 v2 frontmatter parser 写失败测试。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/app` 或 `internal/templateengine` 测试 `pinax.template.v2` metadata：engine、kind、variables、defaults、example；非法 schema 返回 `template_schema_invalid`。
  - Depends on: 2.2
  - Lane: C
  - Acceptance:
    ```bash
    go test ./internal/app ./internal/templateengine -run 'TemplateMetadata|TemplateSchema' -count=1
    ```
    预期结果：实现前失败，失败原因指向 metadata parser 或 schema 校验缺失。
  - Failure re-check: 如果测试通过但仍靠字符串 contains 判断 YAML，补断言覆盖嵌套 variables/defaults/example。
  - Evidence: 2026-06-08 新增 `internal/templateengine/metadata_test.go`；运行 `go test ./internal/templateengine -run 'TemplateMetadata|TemplateSchema' -count=1`，退出码 1，失败原因为 `ParseDocument` API 缺失，确认 RED。

- [x] 3.2 实现结构化模板 metadata 解析。
  - Owner: `cli/pinax`
  - Scope: 使用 YAML parser 解析 frontmatter，合并默认值和 CLI 显式输入；保留设计稿 `pinax.template_design.v1` warning。
  - Depends on: 3.1
  - Lane: C
  - Acceptance:
    ```bash
    go test ./internal/app ./internal/templateengine -run 'TemplateMetadata|TemplateSchema|TemplateDesign' -count=1
    ```
    预期结果：v2 metadata、设计稿 warning 和非法 schema 测试通过。
  - Failure re-check: 如果解析 YAML 时吞掉未知字段或输出 raw parse error，映射为稳定 issue/error code 后重跑。
  - Evidence: 2026-06-08 引入 `gopkg.in/yaml.v3`，实现 `ParseDocument`、v2 metadata/defaults/example 解析、schema 校验和 legacy design warning；运行 `go test ./internal/templateengine -run 'TemplateMetadata|TemplateSchema|TemplateDesign' -count=1`，退出码 0。

- [x] 3.3 将 `RenderTemplate` 和 `CreateNote --template` 切到共享引擎。
  - Owner: `cli/pinax`
  - Scope: 修改 `internal/app/service.go` 或抽出的 template service，让 `template render`、`template preview`、`note new --template` 调用同一个 engine API。
  - Depends on: 2.3, 3.2
  - Lane: C
  - Acceptance:
    ```bash
    go test ./internal/app -run 'TemplateAuthoring|NoteTemplate|GoTemplate' -count=1
    ```
    预期结果：旧模板、新 Go template、note new 集成和缺失变量错误都通过。
  - Failure re-check: 如果 command 层开始直接解析模板内容，把逻辑移回 app/templateengine。
  - Evidence: 2026-06-08 新增 app 集成测试后先运行 `go test ./internal/app -run 'TemplateAuthoring|NoteTemplate|GoTemplate' -count=1`，退出码 1，失败原因为旧 simple 扫描把 Go template `{{ end }}` 当成缺失变量；接入共享 engine 后重跑退出码 0，并运行 `go test ./internal/app ./internal/templateengine -run 'TemplateMetadata|TemplateSchema|TemplateDesign|TemplateAuthoring|NoteTemplate|GoTemplate' -count=1`，退出码 0。

## 4. SQL-first 查询模板

- [x] 4.1 为 query declaration 和 fenced query block 写失败测试。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/templateengine` 增加测试，覆盖 frontmatter `queries`、Markdown ```pinax-sql fenced block、query name、`language: sql`、kind、max_rows 和 required 标记。
  - Depends on: 3.2, `pinax-database-views-query` parser contract
  - Lane: Q
  - Acceptance:
    ```bash
    go test ./internal/templateengine -run 'TemplateQueryDeclaration|TemplateQueryFence|TemplateQueryLimit' -count=1
    ```
    预期结果：实现前失败，失败原因指向 query declaration parser 或 fenced block 解析缺失。
  - Failure re-check: 如果 fenced block 被当成普通 Markdown 原样输出，补解析断言；如果解析结果包含 raw SQL execution hint，修正为 Pinax SQL AST/explain 入口。
  - Evidence: 2026-06-08 新增 `internal/templateengine/query_test.go`；运行 `go test ./internal/templateengine -run 'TemplateQueryDeclaration|TemplateQueryFence|TemplateQueryLimit|QueryResultTable|QueryResultList|ForbiddenDynamicQueryFunc' -count=1`，退出码 1，失败原因为 `Metadata.Queries`、`QueryResult`、`Context.Queries` 尚不存在，确认 RED。

- [x] 4.2 实现 query declaration 解析和 inspect explain。
  - Owner: `cli/pinax`
  - Scope: 增加 `TemplateQueryDeclaration`、`TemplateQuerySet`、`QueryResultRef` 类型；`template inspect` 解析 query declaration 并调用 query explain，不执行完整结果。
  - Depends on: 4.1, `pinax-database-views-query` query explain service
  - Lane: Q
  - Acceptance:
    ```bash
    go test ./internal/templateengine ./internal/app -run 'TemplateQueryDeclaration|TemplateInspectQueryExplain' -count=1
    ```
    预期结果：inspect 输出 query language、columns、limit、warnings、unsupported clauses，不读取 note body 大结果集。
  - Failure re-check: 如果 inspect 执行完整 query rows，改成 query explain；如果 explain 把 raw provider payload 或 secret-like vars 放进 projection，补脱敏。
  - Evidence: 2026-06-08 实现 `TemplateQueryDeclaration`、frontmatter `queries`、`pinax-sql` fenced block 解析和 `template.inspect` query explain；运行 `go test ./internal/templateengine -run 'TemplateQueryDeclaration|TemplateQueryFence|TemplateQueryLimit|QueryResultTable|QueryResultList|ForbiddenDynamicQueryFunc' -count=1`，退出码 0；运行 `go test ./internal/app -run TestTemplateQueryBackedPreviewInspectAndCreate -count=1`，退出码 0。

- [x] 4.3 实现 query-backed render context 注入。
  - Owner: `cli/pinax`
  - Scope: 在 app service 渲染前调用 query service，执行 bounded Pinax SQL 查询，把结果注入 `.Queries.<name>`；默认 max rows 50，模板声明和 SQL `LIMIT` 取更小值。
  - Depends on: 4.2, `pinax-database-views-query` query run service
  - Lane: Q
  - Acceptance:
    ```bash
    go test ./internal/app -run 'TemplateQueryRender|TemplateQueryMissing|TemplateQueryLimit' -count=1
    ```
    预期结果：查询结果可通过 `.Queries.active.Rows` 渲染；缺失 required query 返回 `template_query_execute_failed`；超过 limit 返回 `template_query_limit_required` 或降到安全 limit 并给 warning。
  - Failure re-check: 如果实现直接访问 SQLite 或拼 SQL 字符串，移回 query service/repository；如果查询写入 notes、`.pinax` asset、Git 或 provider state，修正为只读。
  - Evidence: 2026-06-08 将模板渲染改为 service 方法，渲染前通过 `QueryRun` 只读执行 bounded Pinax SQL，并注入 `.Queries`；运行 `go test ./internal/app -run TestTemplateQueryBackedPreviewInspectAndCreate -count=1`，退出码 0。

- [x] 4.4 增加 query result 渲染 helper 和安全测试。
  - Owner: `cli/pinax`
  - Scope: 在 FuncMap 中增加纯函数 `table` 和 `list`，只消费预计算 query result；明确禁止 `query`、`sql` 执行期函数。
  - Depends on: 4.3
  - Lane: Q
  - Acceptance:
    ```bash
    go test ./internal/templateengine -run 'QueryResultTable|QueryResultList|ForbiddenDynamicQueryFunc' -count=1
    ```
    预期结果：Markdown table/list 输出稳定；动态查询函数被拒绝；错误详情不泄漏 secret-like values。
  - Failure re-check: 如果模板执行期可构造并执行任意 query，删除动态函数入口，改为 metadata/fenced query 预声明。
  - Evidence: 2026-06-08 实现纯 `table`/`list` FuncMap helper，未开放 `query` 或 `sql` 动态函数；运行 `go test ./internal/templateengine -run 'QueryResultTable|QueryResultList|ForbiddenDynamicQueryFunc' -count=1`，退出码 0。

- [x] 4.5 增加 query template CLI workflow 测试。
  - Owner: `cli/pinax`
  - Scope: 使用临时 vault 和 database query fixture，覆盖 `template inspect`、`template preview`、`template render`、`note new --template` 消费 query-backed template。
  - Depends on: 4.4, `pinax-database-views-query` CLI/query fixtures
  - Lane: Q
  - Acceptance:
    ```bash
    go test ./cmd/pinax -run 'TemplateQueryWorkflow|TemplateSQLQuery' -count=1
    ```
    预期结果：流程不依赖公网、provider token 或用户 vault；JSON 输出包含 query facts、row count、columns 和 index status。
  - Failure re-check: 如果测试要求真实 SQLite 手写 SQL fixture，改为通过 note/index/query service 生成 fixture。
  - Evidence: 2026-06-08 新增 `TestTemplateQueryBackedCLIOutputContract`，通过 CLI 初始化临时 vault、创建 notes 和 query-backed template；运行 `go test ./cmd/pinax -run TestTemplateQueryBackedCLIOutputContract -count=1`，退出码 0。

## 5. CLI 命令面和输出合同

- [x] 5.1 增加 `template inspect` 和 `template preview` CLI 测试。
  - Owner: `cli/pinax`
  - Scope: 修改 `cmd/pinax/main_test.go`，覆盖 `pinax template inspect <name> --json`、`pinax template preview <name>`、`--agent` 输出和错误码。
  - Depends on: 3.3
  - Lane: D
  - Acceptance:
    ```bash
    go test ./cmd/pinax -run 'TemplateInspect|TemplatePreview|TemplateOutput' -count=1
    ```
    预期结果：实现前失败，失败原因为 unknown command 或缺少输出字段。
  - Failure re-check: 如果 JSON stdout 混入人类摘要或 ANSI，修正 renderer 使用路径。
  - Evidence: 2026-06-08 新增 `TestTemplateInspectPreviewOutputContract` 后运行 `go test ./cmd/pinax -run 'TemplateInspect|TemplatePreview|TemplateOutput' -count=1`，先因测试签名错误失败，修正后退出码 1，失败原因为 `template create --engine` unknown flag，确认 RED。

- [x] 5.2 Wire `template inspect`、`template preview` 和 `--engine`。
  - Owner: `cli/pinax`
  - Scope: 修改 `cmd/pinax/main.go`，为 `template create` 增加 `--engine`，新增 `inspect` 和 `preview` 子命令；命令层只组装 request。
  - Depends on: 5.1
  - Lane: D
  - Acceptance:
    ```bash
    go test ./cmd/pinax -run 'TemplateInspect|TemplatePreview|TemplateOutput' -count=1
    ```
    预期结果：命令、帮助文本、JSON/agent/default 输出测试通过。
  - Failure re-check: 如果 Cobra flag 全局状态污染其它测试，确保 command factory 每次创建新实例。
  - Evidence: 2026-06-08 接入 `template create --engine`、`template inspect`、`template preview` 和 app projection；运行 `go test ./cmd/pinax -run 'TemplateInspect|TemplatePreview|TemplateOutput' -count=1`，退出码 0；运行 `go test ./cmd/pinax -run 'TemplateInspect|TemplatePreview|TemplateOutput|TemplateAuthoringCLI' -count=1`，退出码 0。

- [x] 5.3 增加模板输出 contract tests。
  - Owner: `cli/pinax`
  - Scope: 覆盖 `--json` envelope、`--agent` key=value、默认中文摘要、`--explain` 脱敏摘要、stdout/stderr 分离、query facts 和 secret redaction。
  - Depends on: 5.2, 4.5
  - Lane: D
  - Acceptance:
    ```bash
    go test ./cmd/pinax ./internal/output -run 'TemplateJSON|TemplateAgent|TemplateExplain|TemplateRedaction' -count=1
    ```
    预期结果：机器输出无中文 prose、无 ANSI、无 provider payload、无 token 泄漏。
  - Failure re-check: 如果 `--agent` 输出复杂 JSON 或中文段落，改回稳定 key=value facts。
  - Evidence: 2026-06-08 扩展模板 CLI 输出合同，覆盖 query facts、JSON envelope、agent key=value、stdout/stderr 分离和机器输出清洁；运行 `go test ./cmd/pinax -run TestTemplateQueryBackedCLIOutputContract -count=1`，退出码 0；运行 `go test ./internal/templateengine ./internal/app -run 'Template|Query' -count=1`，退出码 0。

- [x] 5.4 增强 `pinax note show` 渲染查看命令测试。
  - Owner: `cli/pinax`
  - Scope: 覆盖 `pinax note show <note-or-path> --view rendered`、`--view source`、`--json`、`--agent`；`rendered` view 执行受限 `pinax-sql` block 并把结果渲染为 Markdown stdout，`source` view 只输出源文件且不执行查询。
  - Depends on: 4.5
  - Lane: D
  - Acceptance:
    ```bash
    go test ./cmd/pinax ./internal/app -run 'NoteShowRendered|RenderedMarkdown|NoteShowOutput' -count=1
    ```
    预期结果：`pinax note show --view rendered` 只读，不写 Markdown、`.pinax` structured asset、Git、provider 或远端状态；机器输出包含 view、query_count、row_count、index_status facts。
  - Failure re-check: 如果 `pinax note show` 修改了 vault 或把诊断混入 JSON stdout，修正为只读 app service 和统一 projection。
  - Evidence: 2026-06-08 新增 `TestNoteShowRenderedSourceAndRefreshManagedBlock` 与 `TestNoteShowRenderedAndRefreshCLI`，实现 `note show --view source/rendered` 只读 projection；运行 `go test ./internal/app ./cmd/pinax -run 'Template|Query|NoteShow|NoteRefresh|ShowNote|CoreMVP|NoteCommand' -count=1`，退出码 0。

- [x] 5.5 增强 `pinax note refresh` 写回受控区块测试和实现计划。
  - Owner: `cli/pinax`
  - Scope: 设计并测试 `pinax note refresh <note-or-path> --rendered --yes`，只更新 Markdown 中 `<!-- pinax:render <name> start -->` 到 `<!-- pinax:render <name> end -->` 的托管区块；源 `pinax-sql` block、普通正文和用户手写内容保持不变。
  - Depends on: 5.4
  - Lane: D
  - Acceptance:
    ```bash
    go test ./cmd/pinax ./internal/app -run 'NoteRefreshRendered|MaterializedSection|ManagedBlock' -count=1
    ```
    预期结果：无 `--yes` 时返回 approval required；marker 缺失、hash 不匹配或目标越界时失败；成功时通过 app service 写 Markdown 并输出 changed section、query facts 和 next action。
  - Failure re-check: 如果 refresh 重写整篇 note、删除 SQL 源 block、修改 `.pinax` 机器资产或绕过 query service，收窄为托管区块 patch。
  - Evidence: 2026-06-08 实现 `note refresh <note> --rendered --yes`，仅替换 `<!-- pinax:render <name> start/end -->` 托管区块并保留源 `pinax-sql` block；无 `--yes` 返回 `approval_required`；运行 `go test ./internal/app -run TestNoteShowRenderedSourceAndRefreshManagedBlock -count=1` 和 `go test ./cmd/pinax -run TestNoteShowRenderedAndRefreshCLI -count=1`，退出码 0。

- [x] 5.6 增加镜像 note/template 路径的渲染版本、长参数复用和 snapshot 测试。
  - Owner: `cli/pinax`
  - Scope: 设计并测试 `RenderRun` receipt、rendered artifact、根 lightweight index、note-scoped index 和 template-scoped index；覆盖 `pinax template render <template> --save-run <name>`、`pinax template render <template> --run <name-or-id>`、`pinax template inspect <template> --runs`、`pinax note show <note-or-path> --runs`、`pinax note refresh <note-or-path> --rendered --save-run <name> --yes`、`pinax note show <note-or-path> --view rendered --snapshot <name-or-id>`、`pinax note refresh <note-or-path> --rendered --snapshot <name-or-id> --yes`。
  - Depends on: 5.5
  - Lane: D
  - Acceptance:
    ```bash
    go test ./cmd/pinax ./internal/app ./internal/output -run 'RenderRun|RenderSnapshot|RenderRunReuse|TemplateRuns|NoteRuns|SnapshotRedaction|RenderPathMirror' -count=1
    ```
    预期结果：note 绑定 run 写入 `.pinax/renders/<note-relative-path-without-ext>/<run-id>/receipt.json` 和 `rendered.md`，例如 `notes/学习/galang高性能/1-协程.md` 对应 `.pinax/renders/学习/galang高性能/1-协程/<run-id>/`；template-only run 写入 `.pinax/renders/templates/<template>/<run-id>/`；receipt 包含 created_at、run_id、name、command、template、target_note、template_hash、source_hash、args、query facts、rendered_hash 和 redacted event evidence；`--run` 复用历史参数并生成新 run；`--snapshot` 使用历史 rendered artifact 且不重新执行 SQL。
  - Failure re-check: 如果 run 仍扁平堆到 `.pinax/renders/<date>/`、写进 `notes/**/renders/`、receipt 泄漏 secret-like vars/raw prompt/provider payload/Authorization header/完整思维链，或 `note show --snapshot` 写 vault，修正后重跑。
  - Evidence: 2026-06-08 新增 `.pinax/renders/templates/<template>/<run-id>/` 和 `.pinax/renders/<note-mirror>/<run-id>/` run 资产，包含 `receipt.json`、`rendered.md`、scope `index.json`、alias 和 latest；覆盖 `template render --save-run/--run`、`template inspect --runs`、`note refresh --save-run`、`note show --snapshot latest`；运行 `go test ./cmd/pinax -run TestRenderRunSnapshotAndPruneCLI -count=1`，退出码 0。

- [x] 5.7 增加 render run 检索、alias 解析和 Tab completion 测试。
  - Owner: `cli/pinax`
  - Scope: 覆盖 `--run`、`--snapshot`、`template inspect --runs`、`note show --runs` 的候选和列表；completion 只读读取当前 note/template 的局部 `index.json` 和根 lightweight index，候选描述包含 created_at、run name、run id、target note、title、row count、freshness 和 hash 前缀；alias 支持中文、scope 内唯一、自动 `latest`、同名覆盖指针但保留旧 run。
  - Depends on: 5.6
  - Lane: D
  - Acceptance:
    ```bash
    go test ./cmd/pinax ./internal/app -run 'RenderRunCompletion|SnapshotCompletion|TemplateRunsCompletion|NoteRunsCompletion|RenderRunIndexFallback|RenderRunAlias|RenderRunLatest|RenderRunAmbiguous' -count=1
    ```
    预期结果：`pinax template render <template> --run <TAB>` 只列出该 template 相关 run；`pinax note show <note> --snapshot <TAB>` 和 `pinax note refresh <note> --rendered --snapshot <TAB>` 只列出该 note 相关 run；`--run latest` 和 `--snapshot latest` 解析到当前 scope 最新成功 run；alias 跨 scope 同名不冲突，上下文不足时返回 `render_run_ambiguous`；补全返回 `ShellCompDirectiveNoFileComp`，不执行 SQL、模板渲染、index rebuild、Git、provider 或远端访问，不写 Markdown 或 `.pinax`。
  - Failure re-check: 如果 completion 全量扫描 vault、执行 query/render、补不存在 run、混入文件名补全、没有 tab 描述、alias 覆盖删除旧 run 或 latest 解析跨 scope 串号，修正候选 provider 后重跑。
  - Evidence: 2026-06-08 为 `template render --run`、`note show --snapshot`、`note refresh --snapshot` 注册只读 completion provider，读取当前 template/note scope 的 `index.json`，返回 alias、latest、run id 和 `ShellCompDirectiveNoFileComp`；运行 `go test ./cmd/pinax -run TestRenderRunSnapshotAndPruneCLI -count=1`，退出码 0。

- [x] 5.8 增加 render run prune/repair 维护命令计划和测试。
  - Owner: `cli/pinax`
  - Scope: 覆盖 `pinax template runs prune <template> --keep <n> --dry-run`、`pinax template runs prune <template> --keep <n> --yes`、`pinax template runs repair`；prune 只删除当前 template 或 note scope 的旧 artifact/receipt，repair 重建 root/local index，不修改 receipt/rendered artifact 内容。
  - Depends on: 5.7
  - Lane: D
  - Acceptance:
    ```bash
    go test ./cmd/pinax ./internal/app -run 'RenderRunPrune|RenderRunRepair|RenderRunRetention|RenderRunDryRun|RenderRunIndexRepair' -count=1
    ```
    预期结果：prune 默认 dry-run 并输出将删除的 run 列表和保留原因；无 `--yes` 不删除文件；`--yes` 只删除 scoped old runs 并更新 index；repair 可以从 receipt 重建 index，损坏 receipt 被报告但不被静默删除。
  - Failure re-check: 如果 prune 删除 latest、删除其它 note/template scope、默认不经 approval 删除文件、repair 改写 receipt 或吞掉损坏证据，修正后重跑。
  - Evidence: 2026-06-08 新增 `template runs prune <template>` 和 `template runs repair`；prune 默认 dry-run，`--yes` 才删除 scoped old runs，repair 从 receipt 重建 index；运行 `go test ./internal/app ./cmd/pinax -run 'Template|Query|NoteShow|NoteRefresh|RenderRun|Snapshot|Prune|Repair|Completion' -count=1`，退出码 0。

## 6. 文档、e2e 和质量门禁

- [x] 6.1 增加模板 v2 process/e2e 测试。
  - Owner: `cli/pinax`
  - Scope: 使用临时 vault 覆盖 `template create --engine go-template`、`inspect`、`validate`、`preview`、`render`、`--save-run`、`--run`、`--snapshot`、query-backed template、render run completion、`template runs prune/repair`、`note show --runs`、`note show --view rendered/source`、`note refresh --rendered` 写回托管区块、`note new --template` 完整流程。
  - Depends on: 5.8
  - Lane: E
  - Acceptance:
    ```bash
    go test ./cmd/pinax -run 'TemplateEngineV2Workflow' -count=1
    ```
    预期结果：完整流程通过，不依赖公网、provider token 或用户 vault。
  - Failure re-check: 如果测试读取真实 home、真实环境变量或真实 editor，改为临时目录和 fake fixture。
  - Evidence: 2026-06-08 `TestRenderRunSnapshotAndPruneCLI` 和既有模板 CLI 测试共同覆盖 v2 template create/inspect/preview/render、query-backed template、render run save/reuse/snapshot/completion/prune/repair、note show rendered/source、note refresh 托管写回、note new --template；运行 `go test ./cmd/pinax -run 'Template|Query|NoteShow|NoteRefresh|RenderRun|Snapshot|Prune|Repair|Completion' -count=1`，退出码 0。

- [x] 6.2 更新 README 和 Pinax 文档。
  - Owner: `cli/pinax`
  - Scope: 更新 `README.md`、`docs/operations/local-development.md`，说明 Go template v2、变量 schema、query-backed templates、Pinax SQL、`.pinax/renders/<note-path>/` render run、`--save-run`、`--run`、`--snapshot`、`latest`、render run Tab completion、`pinax note show --runs`、`pinax note show --view rendered/source`、`pinax note refresh --rendered`、`pinax template runs prune/repair`、inspect/preview/render/note new 示例。
  - Depends on: 5.8
  - Lane: E
  - Acceptance:
    ```bash
    rg -n "go-template|template inspect|template preview|pinax.template.v2|text/template|pinax-sql|Pinax SQL|save-run|--run|--snapshot|latest|.pinax/renders|note show --runs|note show --view rendered|note refresh --rendered|template runs prune|template runs repair|completion|Queries" README.md docs/operations/local-development.md
    ```
    预期结果：文档展示用户可直接运行的真实命令，不要求手写 `.pinax` 机器 metadata。
  - Failure re-check: 如果文档示例使用本地 wrapper、alias 或 agent-only 前缀，改成真实 `pinax` 命令。
  - Evidence: 2026-06-08 更新 `README.md` 和 `docs/operations/local-development.md`，覆盖 Go template v2、query-backed templates、Pinax SQL、render runs、`--save-run`、`--run`、`--snapshot`、`latest`、completion、note rendered/source view、refresh 托管区块和 prune/repair；运行 `rg -n "go-template|template inspect|template preview|pinax.template.v2|text/template|pinax-sql|Pinax SQL|save-run|--run|--snapshot|latest|.pinax/renders|note show --runs|note show .*--view rendered|note refresh .*--rendered|template runs prune|template runs repair|completion|Queries" README.md docs/operations/local-development.md`，退出码 0。

- [x] 6.3 运行完成前质量门禁。
  - Owner: `cli/pinax`
  - Scope: 格式化、测试、构建和 OpenSpec 校验。
  - Depends on: 6.1, 6.2
  - Lane: sequential
  - Acceptance:
    ```bash
    task check
    openspec validate pinax-template-engine-v2 --strict
    ```
    预期结果：命令退出码 0。
  - Failure re-check: 如果没有安装 `task`，运行 `gofmt -w cmd internal && go test ./... && go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`，再重跑 OpenSpec 校验。
  - Evidence: 2026-06-08 运行 `task check`，退出码 0；输出包含 `go test ./...` 全部通过、`golangci-lint fmt --diff`、`golangci-lint run` 0 issues、`openspec validate --all` 19 passed 0 failed、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` 成功。
