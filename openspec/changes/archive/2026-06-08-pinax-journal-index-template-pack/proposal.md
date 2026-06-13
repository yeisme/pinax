## Why

Pinax 当前模板能力已经支持 `.pinax/templates/*.md`、`template create/inspect/preview/render` 和 `note new --template`，但内置模板仍停留在极薄的 Markdown 片段。`daily` 只有标题和“今日记录”，而 daily index 追加逻辑仍由 application service 硬编码生成，导致用户无法通过模板理解、预览、覆盖或测试日记和索引页结构。

真实笔记工作流里，日记和 index 是两类不同对象：日记是每天/每周/月持续写作的入口，index 是可刷新、可导航、可查询的系统页。把它们继续混在硬编码逻辑里，会让模板功能看起来“能用但不好用”。

## What Changes

- 新增 Pinax 内置模板包：`journal.daily`、`journal.weekly`、`journal.monthly`、`note.quick`、`inbox.capture`、`meeting.notes`、`decision.record`、`project.brief`、`learning.video`、`learning.book`、`research.topic`、`person.profile`、`index.home`、`index.projects`、`index.inbox`、`index.decisions`、`index.learning`、`index.meetings`、`index.research`。
- 将日记模板升级为 `pinax.template.v2` metadata 契约，声明 `kind`、`output.path_pattern`、默认 tags/status、变量和示例上下文。
- 将 index 模板定义为 query-backed template，通过 Pinax SQL 查询本地索引 projection，并只渲染受限结果。
- 引入 Markdown managed block 契约：系统刷新只更新 `<!-- pinax:managed name=... -->` 到 `<!-- /pinax:managed -->` 区域，不重写用户正文。
- 将新 vault 的默认笔记内容根改为 vault 根目录：普通笔记默认直接生成在 `<slug>.md`，日记在 `daily/`，索引页在 `index/`，不再默认塞进 `notes/`。
- 统一 daily note 与 daily index 心智：每天只有一篇 `daily/YYYY-MM-DD.md`；捕获索引作为 daily note 内的 managed block，而不是隐藏创建另一种 daily index 文件。
- 增加 `index page` 命令面，用于创建、预览和刷新 `index/*.md` 这类系统 index 页面。
- `template inspect/preview` 展示模板输出路径、managed blocks、queries、示例变量和刷新风险。
- 增加模板推荐和场景目录心智：模板 metadata 声明 `use_cases`、`aliases`、`difficulty`、`starter`、`after_create_actions`，`template list/inspect` 能告诉用户“这个模板适合什么，下一步做什么”。
- 补齐 `template` 子命令的模板目录心智：区分 built-in、vault-local override、effective resolved template 和 legacy simple template。
- 增加模板定制与差异审查路径：用户可以从内置模板创建 vault-local 副本，并在升级内置模板后比较本地覆盖版本与内置版本。
- 保留 legacy `daily`、`project`、`note` 模板兼容，不删除现有用户模板。

## Capabilities

### New Capabilities

- `notebook-workflows`: journal 模板包驱动 daily/weekly/monthly note 创建。
- `notebook-index-search`: index page 模板包驱动本地索引页生成和 managed block 刷新。
- `pinax`: 模板 inspect/preview 输出补充 path pattern、managed block 和 query facts。
- `pinax`: 模板 list/show/inspect/diff/customize 明确 source resolution、override 状态和安全输出合同。
- `pinax`: 新建普通笔记、journal 和 index page 的默认路径使用 vault 根内容布局，同时兼容读取旧 `notes/` vault。
- `pinax`: 内置模板目录覆盖捕获、会议、决策、项目、学习、研究、联系人和索引导航，并支持按场景筛选和推荐。

### Modified Capabilities

- `notebook-workflows`: daily capture index 从隐藏硬编码文件追加，收敛为 daily note 内 managed block。
- `notebook-index-search`: system index notes 明确位于 `index/*.md` 或 journal managed block，普通 orphan/search 统计默认排除系统 index 页面。

## Impact

- 代码：`internal/app/service.go` 的 journal 创建、daily capture index、template render、note refresh 和 index page 编排；必要时拆出 `internal/app/journal_templates.go`、`internal/app/managed_blocks.go`、`internal/app/index_pages.go`。
- 模板引擎：`internal/templateengine` 增强 metadata、path pattern 渲染、managed block inspection 和 query-backed preview facts。
- CLI：`internal/cli/journal_cmd.go`、`internal/cli/template_cmd.go`、新增或扩展 `internal/cli/index_cmd.go` 的 `index page` 子命令。
- CLI：`internal/cli/journal_cmd.go`、`internal/cli/template_cmd.go`、新增或扩展 `internal/cli/index_cmd.go` 的 `index page` 子命令；`template` 命令需要补目录发现、source 选择、本地定制和 diff 审查入口。
- 测试：Go service tests、templateengine tests、CLI contract tests 和 testscript e2e，覆盖日记创建、index page 刷新、managed block patch、输出合同和索引排除。
- 文档：更新 `README.md`、`docs/operations/local-development.md` 和相关 OpenSpec 主规格。

## Non-Goals

- 不重新设计 Go template v2 引擎，不引入 JS/Lua/Starlark/Sprig 全量函数。
- 不实现 Web/TUI 模板编辑器。
- 不允许模板执行 shell、读取环境变量、访问网络或调用 provider。
- 不把模板注册状态落入 SQLite；模板文件仍是用户可编辑 Markdown 真源。
- 不让 agent 手写 `.pinax` 机器资产；结构化模板 metadata 的创建、规范化和事件证据由 Pinax CLI/application service 负责。
- 不把所有普通 Markdown 文件纳入 index；仍只处理注册过的 Pinax note。
