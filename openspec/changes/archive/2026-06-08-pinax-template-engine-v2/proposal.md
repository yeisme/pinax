# pinax-template-engine-v2

## 背景

Pinax 现在的模板能力已经能创建、校验、渲染和删除 `.pinax/templates/*.md`，也能让 `note new --template` 消费模板。但当前渲染核心仍是手写 `{{name}}` 文本替换，只能处理最基础变量，不能表达真实笔记模板常见需求：条件段落、列表循环、默认值、日期格式化、标签处理、frontmatter 生成、模板上下文预览、变量发现和错误定位。

用户已经明确希望模板设计继续完善，并倾向使用 Go 标准库 template 能力。这个 change 将模板系统升级为 v2：以 Go `text/template` 作为统一渲染核心，在 Pinax 内封装一个共享模板引擎，供 `template render`、`template validate`、`note new --template`、daily/inbox/project 等后续命令复用。

## 目标

- 用 Go `text/template` 替代手写变量替换，支持条件、循环、管道和安全函数。
- 建立 Pinax 共享模板引擎模块，避免每个命令自己解析模板。
- 为模板文件定义 `pinax.template.v2` frontmatter，记录 engine、kind、变量 schema、默认值和示例上下文。
- 支持 SQL-first 查询模板：模板可以声明 Pinax SQL 查询，把当前 vault 的笔记、属性、任务或保存视图结果注入 `.Queries.<name>`。
- 查询模板必须复用 `pinax-database-views-query` 中的 Pinax SQL parser、planner 和 query service，不允许模板把用户输入直接拼接成 SQLite SQL。
- 继续支持现有 v1 简单模板，按兼容路径渲染，不让用户已有模板失效。
- 增加模板设计工作流：`template create` 创建设计稿，`template inspect` 查看变量和 schema，`template validate` 定位错误，`template render`/`template preview` 预览输出。
- 增加 render run 版本化：正式渲染可以在 `.pinax/renders/<note-path>/` 或 `.pinax/renders/templates/<template>/` 下生成带时间戳、参数、hash、query facts 和 rendered artifact 的脱敏版本记录；后续可通过 `--run` 复用长参数，或通过 `--snapshot` 查看/写回历史渲染结果。
- 保持安全边界：模板不得读取环境变量、执行 shell、访问网络、读取任意文件、调用 provider 或输出 secret。
- 所有 CLI 输出继续遵守 AI-native CLI 输出合同。

## 非目标

- 不引入 Lua、JavaScript、Starlark、Sprig 全量函数或任意插件执行。
- 不实现 Obsidian/Notion 模板语法兼容层；只支持 Pinax 自己声明的 Go template 方言。
- 不在模板引擎里实现第二套 SQL parser；查询能力以 `pinax-database-views-query` 的 Pinax SQL AST 为准。
- 不支持任意 SQL、join、subquery、window function、跨 vault 查询、shell/network/provider 函数或动态读取文件。
- 不把模板注册状态落 SQLite；模板文件和 `.pinax/events.jsonl` 仍是真源和证据。
- 不让 agent 直接手写 `.pinax` 机器资产；模板正文可编辑，结构化模板 metadata 由 CLI/service 生成或规范化。
- 不在本 change 中实现 Web/TUI 模板编辑器。

## 用户体验

创建一篇模板设计稿：

```bash
pinax template create "视频学习" --vault ./my-notes
```

从正文创建 Go template：

```bash
pinax template create video-study --engine go-template --body '# {{ .Title }}

{{ if .Vars.url }}链接：{{ .Vars.url }}{{ end }}' --vault ./my-notes
```

查看模板变量、函数和示例上下文：

```bash
pinax template inspect video-study --vault ./my-notes --json
```

渲染预览：

```bash
pinax template render video-study --title "Go 模板学习" --var url=https://go.dev --vault ./my-notes --json
```

用模板生成笔记：

```bash
pinax note new "Go 模板学习" --template video-study --var url=https://go.dev --tags learning,golang --vault ./my-notes
```

创建 SQL 查询模板：

```bash
pinax template create project-dashboard --engine go-template --from ./project-dashboard.md --vault ./my-notes
```

模板 frontmatter 声明查询，正文消费 `.Queries.active.Rows`：

```markdown
---
schema_version: pinax.template.v2
engine: go-template
kind: note_template
queries:
  active:
    language: sql
    text: SELECT title, status, due FROM notes WHERE tags CONTAINS "project" AND status = "active" ORDER BY due ASC LIMIT 10
    required: false
---

# {{ .Title }}

## 活跃项目
{{ table .Queries.active }}
```

渲染查询模板时，Pinax 先通过 query service 执行 Pinax SQL，再把结果注入模板上下文：

```bash
pinax template render project-dashboard --title "项目看板" --vault ./my-notes --json
```

查看一篇已有笔记的渲染后 Markdown，不新增分散的顶层 `cat` 命令，统一走 note surface：

```bash
pinax note show projects/dashboard.md --view rendered --vault ./my-notes
pinax note show projects/dashboard.md --view source --vault ./my-notes
```

需要把 SQL 执行结果写回 Markdown 时，使用显式刷新命令，只更新 Pinax 托管区块：

```bash
pinax note refresh projects/dashboard.md --rendered --yes --vault ./my-notes --json
```

保存一次可复用的渲染版本，并在后续少输长参数：

```bash
pinax template render video-study --title "Go 模板学习" --var url=https://go.dev --save-run video-go --vault ./my-notes --json
pinax template render video-study --run video-go --vault ./my-notes --json
pinax template inspect video-study --runs --vault ./my-notes --json
pinax note show notes/学习/galang高性能/1-协程.md --runs --vault ./my-notes --json
```

查看或写回某个历史 snapshot：

```bash
pinax note show projects/dashboard.md --view rendered --snapshot video-go --vault ./my-notes
pinax note refresh projects/dashboard.md --rendered --save-run dashboard-latest --yes --vault ./my-notes --json
pinax note refresh projects/dashboard.md --rendered --snapshot video-go --yes --vault ./my-notes --json
```

## 影响范围

- `internal/app`：模板 service 请求、创建、校验、渲染、note new 集成。
- `internal/templateengine`：新增共享 Go template 引擎、上下文、函数、安全策略和错误映射。
- `internal/templateengine` / `internal/app`：新增 query-backed template 上下文注入，复用 database view/query service。
- `internal/app`：增强 note show/rendered view、note refresh 托管区块写回、按 note/template 镜像路径的 render run receipt 生成、历史参数复用和只读补全索引读取，保持 command 层只组装 request。
- `internal/domain`：模板 metadata、变量 schema、inspect result、render result、RenderRun receipt、RenderArtifact、note-scoped run index、template-scoped run index 和 completion candidate 类型。
- `cmd/pinax/main.go`：扩展 template 命令树和 flags。
- `internal/output`：复用 projection 输出，必要时增加模板、render run、snapshot 和 refresh 相关事实渲染测试。
- `README.md`、`docs/operations/local-development.md`：更新模板 v2 工作流示例。

## 风险

- Go template 语法比原来强，错误定位和变量发现要清楚，否则用户会困惑。
- 模板函数如果开放过多，会变成执行面；必须使用白名单 FuncMap。
- `{{title}}` 与 `{{ .Title }}` 兼容需要设计清楚，不能让旧模板突然失败。
- YAML frontmatter 解析应使用结构化 parser，避免字符串规则误判。
- 查询模板可能放大渲染成本；必须默认分页/limit，`template inspect` 只 explain 不执行完整结果。
- `.pinax/renders/` 如果扁平堆放会失去空间归属；MVP 采用镜像 note/template 路径，并记录 run count、artifact size 和 next action，后续增加 retention/prune policy。
- RenderRun 会保存参数和渲染产物 hash；必须脱敏 secret-like vars，不保存 raw prompt、provider payload、Authorization header 或完整思维链。
- 把 render artifact 默认放进 `notes/**/renders/` 会污染正文树和搜索索引；默认不采用，用户需要正文可见结果时走 `pinax note refresh --rendered --yes`。
- 补全如果全量扫描历史会变慢；必须优先读取当前 note/template 局部 index，损坏时只扫描局部目录。
- 查看命令如果带写入副作用会破坏用户预期；`note show --view rendered` 必须只读，写回只能通过 `note refresh --rendered --yes`。
- 写回 SQL 结果如果重写整篇 Markdown 容易破坏用户正文；refresh 只能 patch 显式 `pinax:render` 托管区块。
- 如果 `pinax-database-views-query` 尚未落地，查询模板实现任务应保持依赖状态，不要临时绕过成 raw SQL。
