## 1. 回归测试先行

- [x] 1.1 在 `internal/app` 和 `cmd/pinax` 增加 tag YAML 注入回归测试，覆盖 `note new --tags` 和 `note tag add/set` 拒绝换行、方括号、冒号、逗号、控制字符等 unsafe tag，并验证不写 Markdown、ledger、index 或 event。（证据：`go test ./internal/templateengine ./internal/app ./cmd/pinax -run 'Template|Tag|NoteCommandUX|OutputContract' -count=1` 通过；`task check` 通过）
- [x] 1.2 增加设计稿模板执行阻断测试，覆盖 `template preview/render` 和 `note new --template` 对 `pinax.template_design.v1` 返回 `template_design_not_executable`。（证据：focused gate 与 `task check` 通过）
- [x] 1.3 增加 query-backed `template preview` 只读测试，删除 `.pinax/index.sqlite` 后运行 preview，验证不会创建 index 或其它 `.pinax` structured assets。（证据：focused gate 与 `task check` 通过）
- [x] 1.4 增加 template `example` 上下文测试，验证 preview 使用 example title/project/tags/vars，显式 `--title`、`--project`、`--tags`、`--var` 覆盖 example。（证据：focused gate 与 `task check` 通过）
- [x] 1.5 增加 built-in note template metadata 测试，覆盖 `inbox.capture`、`meeting.notes`、`decision.record` 的 `output.path_pattern`、`defaults.kind`、`defaults.status` 生效，且显式 CLI 字段优先。（证据：focused gate、`go test ./tests/e2e -run 'JournalIndexTemplate' -count=1` 与 `task check` 通过）
- [x] 1.6 增加 `note tag` record/index/output contract 测试，覆盖成功 mutation 的 `record_event`、`ledger_seq`、`index_updated` 或 `index_status` facts。（证据：`go test ./internal/records ./internal/templateengine ./internal/app ./internal/output ./cmd/pinax -run 'MetadataUpdate|Template|Tag|NoteCommandUX|OutputContract|NoteTagRecordFactsRender' -count=1` 通过）

## 2. Tag 和 frontmatter 安全实现

- [x] 2.1 新增统一 tag 规范化/校验 helper，并让 `CreateNote`、`TagNote`、import defaults、repair/organize tag patch 和相关 schema values 入口复用。（证据：focused gate 与 `task check` 通过）
- [x] 2.2 将 unsafe tag 错误映射为稳定 `invalid_tag` command error，错误 hint 给出允许字符和可运行修正示例。（证据：focused gate 与 `task check` 通过）
- [x] 2.3 确保 frontmatter 写入只接收已校验 tags，并保留未知用户 metadata 和常见注释的现有行为。（证据：focused gate 与 `task check` 通过）

## 3. 模板执行边界实现

- [x] 3.1 在模板执行路径加入 executable guard：`pinax.template_design.v1` 允许 inspect/validate，但 preview/render/note-create 必须 fail closed。（证据：focused gate 与 `task check` 通过）
- [x] 3.2 调整 `renderTemplateBody` 的 request/example/default 合并顺序，先应用 example，再构造 `templateengine.Context`。（证据：focused gate 与 `task check` 通过）
- [x] 3.3 补齐安全白名单函数或调整 spec 对函数集合的要求；如果实现函数，覆盖 `slug`、`date`、`yaml`、`json`、`quote` 的纯函数测试。（证据：focused gate 与 `task check` 通过）
- [x] 3.4 为缺失变量错误补齐安全 action，确保不泄漏 secret-like 原始值、raw prompt、provider payload 或隐藏指令。（证据：`go test ./internal/app -run 'TemplateMissingVariableIncludesSafeAction' -count=1` 与 `task check` 通过）

## 4. Preview 只读和 query-backed 模板

- [x] 4.1 给 template render 内部增加 read-only query execution 选项，`PreviewTemplate` 禁止 lazy index rebuild，`RenderTemplate` 保持 bounded query 能力。（证据：focused gate 与 `task check` 通过）
- [x] 4.2 当 preview 遇到缺失或 stale index 时返回 partial/failed projection，包含稳定 error/warning 和 `pinax index rebuild --vault <vault>` action。（证据：focused gate 与 `task check` 通过）
- [x] 4.3 确认 `template inspect` 只做 query explain，不执行 query、不写 `.pinax`、不写 event。（证据：focused gate 与 `task check` 通过）

## 5. Note template metadata 应用

- [x] 5.1 在 `CreateNote` 中解析 v2 note template metadata，构造 effective create request，并保证显式 CLI 参数覆盖 template defaults。（证据：focused gate、`go test ./tests/e2e -run 'JournalIndexTemplate' -count=1` 与 `task check` 通过）
- [x] 5.2 实现 `output.path_pattern` 到安全 root-relative note path 的转换，复用 template output path validator 和 note path validator。（证据：focused gate、`go test ./tests/e2e -run 'JournalIndexTemplate' -count=1` 与 `task check` 通过）
- [x] 5.3 在 projection facts/data 中报告 template、effective path、kind、status、defaults source 和 override 情况。（证据：focused gate 与 `task check` 通过）
- [x] 5.4 更新 inspect/recommend actions，starter note template 至少提供 preview 和 create-note 两个 action。（证据：focused gate 与 `task check` 通过）

## 6. Record ledger、index 和输出合同

- [x] 6.1 为 tag/metadata mutation 增加 record event kind 或复用 metadata event，并更新 replay/registry materialization。（证据：`go test ./internal/records ./internal/templateengine ./internal/app ./internal/output ./cmd/pinax -run 'MetadataUpdate|Template|Tag|NoteCommandUX|OutputContract|NoteTagRecordFactsRender' -count=1` 通过）
- [x] 6.2 `note tag` 成功后执行 incremental index refresh；如果刷新失败或被跳过，返回 partial/stale facts 和 next action。（证据：focused output/records gate 与 `task check` 通过）
- [x] 6.3 更新 `internal/output` contract tests，验证 `--json`、`--agent`、default human 对新增 facts/actions 的渲染稳定且 stdout/stderr 分离。（证据：focused output/records gate 与 `task check` 通过）
- [x] 6.4 确保所有新增 error projection 在 machine modes 中保持单一 envelope，不输出 ANSI、人类段落或调试信息。（证据：focused gate 与 `task check` 通过）

## 7. 文档和验证

- [x] 7.1 更新 `README.md` 或 `docs/commands/template.md`、`docs/commands/note.md` 中关于 tag 字符、设计稿模板、preview 只读和 starter metadata 的用户说明。（证据：已更新 `docs/commands/template.md`、`docs/commands/note.md`）
- [x] 7.2 运行 focused gate：`go test ./internal/templateengine ./internal/app ./cmd/pinax -run 'Template|Tag|NoteCommandUX|OutputContract' -count=1`。（证据：命令通过）
- [x] 7.3 运行完整门禁：`task check`。（证据：命令通过，含 lint、fmt、Go tests、build、OpenSpec validate 和 e2e）
- [x] 7.4 记录验证证据到本 change 的 tasks.md 对应完成项，确保后续 archive 前可追溯。（证据：本文件已记录 focused gate、output/records gate、e2e 修正验证和 `task check`）
