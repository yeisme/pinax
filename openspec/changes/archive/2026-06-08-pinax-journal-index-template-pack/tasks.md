## 1. 规格和测试底座

- [x] 1.1 确认本 change 的 spec delta 与现有 `notebook-workflows`、`notebook-index-search`、`pinax` 主规格不冲突。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: none
  - Scope: `openspec/changes/pinax-journal-index-template-pack/specs/**/spec.md`
  - Acceptance: 运行 `openspec validate pinax-journal-index-template-pack`，预期该 change 校验通过；失败后先修 spec 格式再进入代码。
  - Docs/comments: 规格文本保持中文；命令示例必须是真实用户命令。

- [x] 1.2 增加 templateengine 单元测试，锁定 path pattern、managed block inspection 和非法 block 错误。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.1
  - Scope: `internal/templateengine/metadata_test.go`、`internal/templateengine/engine_test.go` 或新增 `internal/templateengine/managed_block_test.go`
  - Acceptance: 运行 `go test ./internal/templateengine -run 'PathPattern|ManagedBlock|IndexTemplate|JournalTemplate' -count=1`，预期新增测试先失败，随后实现后通过。
  - Docs/comments: managed block 状态机和错误映射实现时必须加中文注释，解释为何缺失/重复/未闭合 block 不能写文件。

- [x] 1.3 增加 app service 单元测试，锁定 journal 模板创建和 index page 刷新行为。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.1
  - Scope: `internal/app/service_test.go` 或新增 `internal/app/journal_templates_test.go`、`internal/app/index_pages_test.go`
  - Acceptance: 运行 `go test ./internal/app -run 'JournalTemplate|IndexPage|ManagedBlockRefresh|RootContentLayout' -count=1`，预期新增测试先失败，随后实现后通过。
  - Docs/comments: 文件写入边界、旧 `notes/` daily note 兼容、根目录内容布局和只 patch managed block 的测试 fixture 需要中文注释说明风险。

- [x] 1.5 增加默认根目录笔记布局测试，锁定普通 note、journal 和 index page 的新默认路径。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.1
  - Scope: `internal/app/service_test.go`、`internal/cli/note_cmd.go`、`internal/app/service.go` 中路径 resolver 相关测试
  - Acceptance: 运行 `go test ./internal/app ./internal/cli -run 'RootContentLayout|DefaultNoteRoot|LegacyNotesCompat' -count=1`，预期新增测试先失败，随后实现后通过；`pinax note add "demo" --vault ./my-notes --json` 的计划路径为 `demo.md`，`pinax journal daily open --vault ./my-notes --json` 的路径为 `daily/YYYY-MM-DD.md`，`pinax index page create home --vault ./my-notes --json` 的路径为 `index/home.md`。
  - Docs/comments: 路径 resolver 必须用中文注释说明保留目录和 legacy `notes/` 兼容规则。

- [x] 1.4 增加 CLI contract tests，锁定用户命令、错误码、facts 和 next action。
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 1.1
  - Scope: `internal/cli/root_test.go`、`internal/cli/config_cmd_test.go` 或新增 `internal/cli/journal_index_template_test.go`
  - Acceptance: 运行 `go test ./internal/cli -run 'JournalTemplate|IndexPage|TemplateInspect' -count=1`，预期新增测试先失败，随后实现后通过。
  - Docs/comments: CLI 输出测试必须验证 stdout/stderr 分离，中文 human summary 不进入 `--json` envelope 外层。

## 2. 模板 metadata 和内置模板包

- [x] 2.1 扩展 `internal/templateengine.Metadata`，支持 `kind`、`name`、`title`、`output.path_pattern`、`queries`、`variables` 和 `defaults` 的 journal/index 模板读取。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.2
  - Scope: `internal/templateengine/metadata.go`、`internal/templateengine/metadata_test.go`
  - Acceptance: `go test ./internal/templateengine -run 'Metadata|PathPattern|TemplateKind' -count=1` 通过；非法 `path_pattern` 返回稳定错误码 `template_output_path_invalid`。
  - Docs/comments: path pattern 渲染必须说明只允许 vault-relative content path，禁止绝对路径、`..` 和 `.pinax/`、`.git/`、`attachments/` 等保留目录；不再要求 `notes/` 前缀。

- [x] 2.2 增加 managed block parser/patcher，提供 inspect 和 replace API。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 2.1
  - Scope: 新增 `internal/templateengine/managed_block.go` 和测试
  - Acceptance: `go test ./internal/templateengine -run 'ManagedBlock' -count=1` 通过；缺失 block 返回 `managed_block_missing`，重复 block 返回 `managed_block_ambiguous`，未闭合 block 返回 `managed_block_unclosed`。
  - Docs/comments: patcher 必须有中文注释说明“只替换托管区块，不重写整篇 Markdown”的安全边界。

- [x] 2.3 将内置模板包从 `builtInTemplates()` 简单 map 拆到专门文件，新增 `journal.*` 和 `index.*` 模板正文。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.1
  - Scope: 新增 `internal/app/builtin_templates.go`，修改 `internal/app/service.go`
  - Acceptance: `go test ./internal/app -run 'BuiltInTemplate|TemplateInspect' -count=1` 通过；`daily` legacy 模板仍可加载，`journal.daily` 推荐模板可 inspect。
  - Docs/comments: 内置模板正文旁边保留中文注释，说明 legacy 名称兼容和推荐名称的差异。

- [x] 2.4 扩展内置 note 模板目录，覆盖 starter 和 focused workflow。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.1, 2.3
  - Scope: `internal/app/builtin_templates.go`、`internal/templateengine/metadata.go`、`internal/app/service_test.go`
  - Acceptance: `go test ./internal/app ./internal/templateengine -run 'BuiltInNoteTemplates|StarterTemplateMetadata|FocusedTemplateMetadata' -count=1` 通过；内置模板至少包含 `note.quick`、`inbox.capture`、`meeting.notes`、`decision.record`、`project.brief`、`learning.video`、`learning.book`、`research.topic`、`person.profile`；每个模板都有 `use_cases`、`aliases`、`difficulty`、`starter`、`output.path_pattern` 和默认 metadata。
  - Docs/comments: 模板正文旁边用中文注释说明每类模板的“最小可用结构”，避免后续把模板扩成过重表单。

- [x] 2.5 扩展内置 index 模板目录，覆盖 decisions、learning、meetings 和 research。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.1, 2.3
  - Scope: `internal/app/builtin_templates.go`、`internal/app/index_pages.go`、`internal/app/service_test.go`
  - Acceptance: `go test ./internal/app -run 'BuiltInIndexTemplates|IndexDecisions|IndexLearning|IndexMeetings|IndexResearch' -count=1` 通过；内置模板至少包含 `index.decisions`、`index.learning`、`index.meetings`、`index.research`，且每个模板有 managed block 和 bounded query metadata。
  - Docs/comments: index query 注释必须说明只通过 Pinax SQL service 执行，不在模板函数或 command 层拼 raw SQLite。

## 3. Journal 模板化创建

- [x] 3.1 让 `ensureJournalNote` 支持模板渲染上下文，不再硬编码 `# Daily YYYY-MM-DD` 正文。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.3
  - Scope: `internal/app/service.go` 或新增 `internal/app/journal_templates.go`
  - Acceptance: `go test ./internal/app -run 'JournalTemplateCreatesDaily|ExistingDailyNotRewritten|LegacyNotesDailyCompat' -count=1` 通过；新建 daily note 写入 `daily/YYYY-MM-DD.md`；已有 daily note 再 open 不改变文件 hash；旧 `notes/daily/YYYY-MM-DD.md` 可识别为 legacy 路径但不自动迁移。
  - Docs/comments: 已存在文件不重写和 legacy `notes/` 不自动迁移的判断需要中文注释，避免未来把 open 命令变成隐式迁移。

- [x] 3.2 扩展 journal CLI flags，支持 `--template <name>` 并保持默认模板行为。
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 3.1
  - Scope: `internal/cli/journal_cmd.go`、相关 command context
  - Acceptance: `go test ./internal/cli -run 'JournalDailyTemplateFlag|JournalWeeklyTemplateFlag' -count=1` 通过；`pinax journal daily open --template journal.daily --vault ./my-notes --json` 输出 facts 包含 `template=journal.daily`。
  - Docs/comments: CLI help 示例使用 `pinax journal daily open --template journal.daily --vault ./my-notes`。

- [x] 3.3 将 daily capture index 写入策略迁移到 `daily-captures` managed block。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 2.2, 3.1
  - Scope: `internal/app/service.go` 或新增 `internal/app/daily_capture_index.go`
  - Acceptance: `go test ./internal/app -run 'DailyCaptureManagedBlock|DailyCaptureLegacyMissingBlock' -count=1` 通过；缺 block 不写文件，返回 action 建议创建或升级 daily 模板。
  - Docs/comments: 兼容旧 daily note 的失败路径必须有中文注释，说明为什么不自动猜测插入位置。

## 4. Index page 命令和刷新

- [x] 4.1 增加 app service 的 `CreateIndexPage`、`PreviewIndexPage`、`RefreshIndexPage` 请求和 projection。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.2, 2.3
  - Scope: 新增 `internal/app/index_pages.go`，修改 `internal/app/service.go` 的共享 helper
  - Acceptance: `go test ./internal/app -run 'IndexPageCreate|IndexPagePreview|IndexPageRefresh' -count=1` 通过；preview 不写文件，refresh 只 patch managed blocks。
  - Docs/comments: refresh 写路径必须通过 `safeJoin`，并注释 `index/*.md` 的 vault 边界和 `.pinax/`、`.git/`、`attachments/` 等保留目录限制。

- [x] 4.2 增加 `pinax index page create|preview|refresh` CLI 子命令。
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 4.1
  - Scope: `internal/cli/index_cmd.go`、`cmd/pinax/main_test.go` 或 `internal/cli/*_test.go`
  - Acceptance: `go test ./cmd/pinax ./internal/cli -run 'IndexPage|OutputContract' -count=1` 通过；`pinax index page refresh home --vault ./my-notes --json` 输出 command `index.page.refresh`。
  - Docs/comments: help 文案中文，机器字段英文稳定。

- [x] 4.3 确保 index page system notes 默认不进入 ordinary orphan/search/stat 结果。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 4.1
  - Scope: `internal/index/*`、`internal/app/query.go`、`internal/app/linkgraph.go` 中已有 system note 过滤点
  - Acceptance: `go test ./internal/app ./internal/index -run 'SystemIndexNote|IndexPageExcluded|Orphans' -count=1` 通过；`index/home.md` 不被普通 orphan 检测计入；旧 `notes/index/home.md` 如果存在也按 system index legacy path 分类。
  - Docs/comments: 过滤条件需要中文注释说明 index page 是系统导航页，不是普通用户知识卡片。

## 5. Template inspect/preview 输出

- [x] 5.1 扩展 `template inspect` projection facts，展示模板 kind、path pattern、managed blocks、queries 和 refreshable 状态。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 2.1, 2.2
  - Scope: `internal/app/service.go`、`internal/domain/types.go`、`internal/output/render.go`
  - Acceptance: `go test ./internal/app ./internal/output -run 'TemplateInspect|ManagedBlocks|TemplateUseCases|OutputContract' -count=1` 通过；`--agent` 输出稳定 key=value，不输出中文 prose；`meeting.notes` inspect facts 包含 `use_cases`、`aliases`、`difficulty`、`starter` 和 `after_create_action_count`。
  - Docs/comments: 输出字段变更需要测试覆盖 human/json/agent 至少两种模式。

- [x] 5.2 扩展 `template preview`，对 journal/index 模板使用 example vars 和 bounded queries。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 5.1
  - Scope: `internal/app/service.go`、`internal/templateengine/engine.go`
  - Acceptance: `go test ./internal/app -run 'TemplatePreviewJournal|TemplatePreviewIndexQuery' -count=1` 通过；query 失败时返回 `template_query_execute_failed`，并建议 `pinax query explain ...` 或 `pinax index sync --vault <vault>`。
  - Docs/comments: query 结果注入必须注释只使用 Pinax SQL service，不拼 raw SQLite。

- [x] 5.3 补齐 template 名称补全，覆盖 inspect/show/validate/preview/render/delete 和 journal/index/note 的 `--template` 场景。
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 2.3, 5.1
  - Scope: `internal/cli/template_cmd.go`、`internal/cli/journal_cmd.go`、`internal/cli/note_cmd.go`、`internal/cli/index_cmd.go`、`internal/cli/root.go` 中 completion helper
  - Acceptance: `go test ./cmd/pinax ./internal/cli -run 'TemplateCompletion|JournalTemplateCompletion|IndexTemplateCompletion|NoteTemplateCompletion' -count=1` 通过；`pinax __complete template inspect --vault <vault> ""` 返回 `journal.daily\tbuiltin journal_template`、`index.home\tbuiltin index_template` 和 `ShellCompDirectiveNoFileComp`；`template delete` 只补 local/override 模板。
  - Docs/comments: completion helper 需要中文注释说明“补全只读，不执行模板、不执行 SQL、不写资产”的边界。

- [x] 5.4 补齐 template flag 上下文补全，覆盖 `--engine`、`--var`、`--run` 和 `--template` filter。
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 5.3
  - Scope: `internal/cli/template_cmd.go`、`internal/cli/note_cmd.go`、`internal/cli/journal_cmd.go`、`internal/cli/index_cmd.go`、`cmd/pinax/main_test.go`
  - Acceptance: `go test ./cmd/pinax -run 'TemplateFlagCompletion|TemplateVarCompletion|TemplateRunCompletion' -count=1` 通过；`pinax __complete template create demo --engine ""` 返回 `simple\tengine` 和 `go-template\tengine`；`pinax __complete template render video-study --var ""` 返回模板 schema 中的 `url=\trequired string`；render run 补全描述包含 alias、created 和 freshness。
  - Docs/comments: `--var` 补全不得填入 secret-like 变量值，只补 `key=` 形式和描述。

- [x] 5.5 将 template 命令下一步 action 纳入 projection 合同。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 5.1, 5.2
  - Scope: `internal/app/service.go` 或新增 `internal/app/template_actions.go`、`internal/domain/types.go`、`internal/output/render.go`、`internal/output/render_test.go`
  - Acceptance: `go test ./internal/app ./internal/output -run 'TemplateNextAction|ProjectionActions|AgentActions|JSONActions' -count=1` 通过；`template inspect journal.daily` 的 default human 输出包含一条真实可运行推荐命令，`--json` 的 `actions[0].command` 可直接复制运行，`--agent` 输出 `action.primary=...`，失败路径包含修复/验证命令。
  - Docs/comments: action 生成逻辑需要中文注释说明如何避免把 secret-like `--var` 原值写进 action。

- [x] 5.6 增加 `template list --pack|--use-case` 和 `template recommend --intent` 的本地推荐能力。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 2.4, 5.1, 5.5
  - Scope: `internal/app/service.go` 或新增 `internal/app/template_recommend.go`、`internal/cli/template_cmd.go`、`internal/output/render.go`
  - Acceptance: `go test ./internal/app ./internal/cli ./internal/output -run 'TemplateListPack|TemplateListUseCase|TemplateRecommend|TemplateRecommendFallback' -count=1` 通过；`pinax template list --pack starter --vault ./my-notes --json` 返回 starter 模板；`pinax template recommend --intent meeting --vault ./my-notes --json` primary 为 `meeting.notes`；unknown intent fallback 到 `note.quick` 或 `inbox.capture`；推荐流程不执行模板、不执行 SQL、不写文件、不联网。
  - Docs/comments: 推荐匹配逻辑必须用中文注释说明这是 metadata-only 本地匹配，不是 LLM 推理或联网搜索。

## 6. E2E、文档和验证

- [x] 6.1 增加 testscript e2e，覆盖 journal 模板创建 daily、starter note 模板、index page 创建/刷新、template 推荐、template 补全、下一步 action 和 managed block 安全失败。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 3.2, 4.2, 5.2, 5.3, 5.4, 5.5, 5.6
  - Scope: `tests/e2e/*_test.go`、`tests/e2e/testdata/journal_index_templates/scripts/*.txt`
  - Acceptance: `go test ./tests/e2e -run 'JournalIndexTemplate|StarterTemplates|IndexPageRefresh|TemplateRecommend|TemplateCompletion|TemplateNextAction' -count=1` 通过；每次 e2e 运行写入 `temp/integration-test-runs/<run-id>/summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json` 和 `artifacts/`。
  - Evidence: 2026-06-08 新增 `tests/e2e/journal_index_template_test.go` 和 `tests/e2e/testdata/journal_index_template/scripts/journal_index_template.txt`；运行 `go test ./tests/e2e -run 'JournalIndexTemplate|StarterTemplates|IndexPageRefresh|TemplateRecommend|TemplateCompletion|TemplateNextAction' -count=1` 通过。扩展 `internal/testkit/integrationevidence` 后运行 `task test:integration` 通过，生成 `temp/integration-test-runs/20260608T145731Z-2245567/{summary.json,command.txt,stdout.log,stderr.log,env.json,artifacts/README.txt}`。
  - Docs/comments: 非显然 fixture 需要中文注释说明 managed block 和 system index note 的意图。

- [x] 6.2 更新 Pinax README 和本地开发文档中的模板示例，明确模板推荐、starter pack、Tab 补全和下一步 action 的主路径。
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 4.2, 5.1
  - Scope: `README.md`、`docs/operations/local-development.md`
  - Acceptance: 文档示例包含 `pinax template list --pack starter --vault ./my-notes`、`pinax template recommend --intent meeting --vault ./my-notes --json`、`pinax template inspect journal.daily --vault ./my-notes --json`、`pinax journal daily open --template journal.daily --vault ./my-notes`、`pinax note add "客户同步" --template meeting.notes --vault ./my-notes --json`、`pinax index page refresh home --vault ./my-notes --json`；说明 `pinax template inspect <TAB>`、`pinax template render <template> --var <TAB>` 和 default 输出的“推荐下一步”是主入口；不推荐 legacy `daily` 作为新模板入口。
  - Evidence: 2026-06-08 更新 `README.md` 和 `docs/operations/local-development.md`，加入 `template list --pack starter`、`template recommend --intent`、`journal.daily`、`index.home`、`meeting.notes`、Tab completion 和下一步 action 主路径说明。
  - Docs/comments: 文档正文中文，命令字段英文保持原样。

- [x] 6.3 运行聚焦验证和 OpenSpec 校验。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 6.1, 6.2
  - Scope: 全项目验证
  - Acceptance: 依次运行 `go test ./internal/templateengine ./internal/app ./internal/cli ./tests/e2e -run 'JournalTemplate|StarterTemplates|IndexPage|ManagedBlock|TemplateInspect|TemplateRecommend|TemplateCompletion|TemplateNextAction' -count=1`、`openspec validate pinax-journal-index-template-pack`、`openspec validate --all`，预期全部通过。
  - Evidence: 2026-06-08 运行 `go test ./internal/templateengine ./internal/app ./internal/cli ./tests/e2e -run 'JournalTemplate|StarterTemplates|IndexPage|ManagedBlock|TemplateInspect|TemplateRecommend|TemplateCompletion|TemplateNextAction' -count=1` 通过；`openspec validate pinax-journal-index-template-pack` 通过；`openspec validate --all` 通过。
  - Docs/comments: 若失败，记录失败命令、错误摘要、已确认是否与本 change 相关，并复验修复后的同一命令。

- [x] 6.4 运行项目门禁。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 6.3
  - Scope: 全项目质量门禁
  - Acceptance: 运行 `task check`，预期通过；若本地缺少 `task`，按 AGENTS.md 备用命令运行 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`。
  - Evidence: 2026-06-08 运行 `task check` 通过，覆盖 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`openspec validate --all` 和 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
  - Docs/comments: closeout 记录最终验证证据，不提交 `dist/`、coverage、temp evidence 或本地 vault。

## Verification Evidence

- 2026-06-08: `openspec validate pinax-journal-index-template-pack` 通过；1.1 确认本 change 的 delta specs 在 OpenSpec 校验层与现有 `notebook-workflows`、`notebook-index-search`、`pinax` 主规格不冲突，可进入代码测试底座。
- 2026-06-08: `go test ./internal/templateengine -run 'PathPattern|ManagedBlock|IndexTemplate|JournalTemplate' -count=1` 和 `go test ./internal/templateengine -count=1` 通过；1.2 增加 templateengine 红绿测试并实现 metadata `output.path_pattern` 校验、`template_output_path_invalid` 稳定错误、managed block inspect/replace API，以及 `managed_block_missing`、`managed_block_ambiguous`、`managed_block_unclosed` fail-closed 错误。
- 2026-06-08: `go test ./internal/app -run 'JournalTemplate|IndexPage|ManagedBlockRefresh|RootContentLayout' -count=1` 和 `go test ./internal/app -count=1` 通过；1.3 增加 app service 红绿测试并实现 journal 模板创建、已有 daily note 不重写、index page preview 不写文件、create 写入 `index/home.md` facts，以及 refresh 只 patch managed block 并保留用户正文。
- 2026-06-08: `go test ./internal/app ./internal/cli -run 'RootContentLayout|DefaultNoteRoot|LegacyNotesCompat' -count=1` 通过；1.5 增加 app 和 CLI 红绿测试并实现普通 note 默认根目录 `demo.md`、journal 默认 `daily/YYYY-MM-DD.md`、index page 默认 `index/home.md`，同时保留真正 legacy `notes/daily/*.md` note 不自动迁移并跳过旧 daily system index。
- 2026-06-08: `go test ./internal/cli -run 'JournalTemplate|IndexPage|TemplateInspect' -count=1` 通过；1.4 增加 CLI JSON contract tests 并实现 `journal daily show --template`、`index page preview|create|refresh --template`，验证 stdout JSON envelope、stderr 分离、stable command/facts，以及 `template inspect index.home` 输出 `kind`、`path_pattern`、`managed_blocks` facts。
- 2026-06-08: `go test ./internal/templateengine -run 'Metadata|PathPattern|TemplateKind' -count=1` 通过；2.1 实现 template metadata `kind`、`name`、`title`、`output.path_pattern`、`queries`、`variables`、`defaults` 读取，并用 `template_output_path_invalid` 拒绝非 vault-relative 或保留目录输出路径。
- 2026-06-08: `go test ./internal/templateengine -run 'ManagedBlock' -count=1` 通过；2.2 实现 managed block inspect/replace API，缺失、重复、未闭合分别返回稳定 fail-closed 错误码。
- 2026-06-08: `go test ./internal/app -run 'BuiltInTemplate|TemplateInspect' -count=1` 通过；2.3 将 `builtInTemplates()` 拆到 `internal/app/builtin_templates.go`，补充 legacy `daily` 与推荐 `journal.daily`/`index.home` inspect 测试，并在内置模板正文旁用中文注释说明兼容关系和 managed block 边界。
- 2026-06-08: `go test ./internal/app ./internal/templateengine -run 'BuiltInNoteTemplates|StarterTemplateMetadata|FocusedTemplateMetadata' -count=1` 通过；2.4 扩展 metadata `use_cases`、`aliases`、`difficulty`、`starter`，并新增 `note.quick`、`inbox.capture`、`meeting.notes`、`decision.record`、`project.brief`、`learning.video`、`learning.book`、`research.topic`、`person.profile` 九个内置 note 模板，每个模板包含默认 metadata 和 `output.path_pattern`。
- 2026-06-08: `go test ./internal/app -run 'BuiltInIndexTemplates|IndexDecisions|IndexLearning|IndexMeetings|IndexResearch' -count=1` 通过；2.5 新增 `index.decisions`、`index.learning`、`index.meetings`、`index.research`，每个模板包含 managed block 和 bounded SQL query metadata，preview 通过 app service 执行 query 且不写文件。
- 2026-06-08: `go test ./internal/app -run 'JournalTemplateCreatesDaily|ExistingDailyNotRewritten|LegacyNotesDailyCompat' -count=1` 通过；3.1 让 `ensureJournalNote` 使用模板渲染上下文创建 `daily/YYYY-MM-DD.md`，已有 daily note 不重写，真正 legacy `notes/daily/*.md` 复用但不自动迁移。
- 2026-06-08: `go test ./internal/cli -run 'JournalDailyTemplateFlag|JournalWeeklyTemplateFlag' -count=1` 通过；3.2 为 journal CLI 增加 `--template` flag，daily/weekly JSON 输出 facts 包含 `template` 和新默认路径，并补齐 `journal.weekly`/`journal.monthly` 内置模板。
- 2026-06-08: `go test ./internal/app -run 'DailyCaptureManagedBlock|DailyCaptureLegacyMissingBlock' -count=1` 通过；3.3 将 note 创建后的 daily capture 写入迁移到 `daily-captures` managed block，不再创建 legacy `notes/daily` index；缺 block 时不重写 daily，projection 返回 partial、`daily_index_status=managed_block_missing` 和升级 action。
- 2026-06-08: `go test ./internal/app -run 'IndexPageCreate|IndexPagePreview|IndexPageRefresh' -count=1` 通过；4.1 实现 app service `CreateIndexPage`、`PreviewIndexPage`、`RefreshIndexPage`，preview 不写文件，refresh 只 patch managed blocks。
- 2026-06-08: `go test ./cmd/pinax ./internal/cli -run 'IndexPage|OutputContract' -count=1` 通过；4.2 增加 `pinax index page preview|create|refresh` CLI 子命令，JSON 输出 command/facts 稳定，并修正 daily capture 使其不污染普通 OutputContract 查询和 resolver。
- 2026-06-08: `go test ./internal/app ./internal/index -run 'SystemIndexNote|IndexPageExcluded|Orphans' -count=1` 通过；4.3 将 `index/*.md`、legacy `notes/index/*.md` 和旧 `notes/daily/*.md` index 识别为 system index note，排除出普通 registered note lookup 和 orphan 结果。
- 2026-06-08: `go test ./internal/app ./internal/output -run 'TemplateInspect|ManagedBlocks|TemplateUseCases|OutputContract' -count=1` 通过；5.1 扩展 `template inspect` facts，输出 kind、path pattern、managed blocks、queries、refreshable、use_cases、aliases、difficulty、starter、after_create_action_count，并验证 agent 输出为稳定 key=value 且不含中文 prose。
- 2026-06-08: `go test ./internal/app -run 'TemplatePreviewJournal|TemplatePreviewIndexQuery' -count=1` 通过；5.2 让 preview/render 合并模板 example vars、暴露 `query_count`，index preview 使用 bounded queries，经 Pinax SQL service 执行；required query 失败返回 `template_query_execute_failed` 并提示 `query explain` 或 `index sync`。
- 2026-06-08: `go test ./cmd/pinax ./internal/cli -run 'TemplateCompletion|JournalTemplateCompletion|IndexTemplateCompletion|NoteTemplateCompletion' -count=1` 通过；5.3 增加只读模板名称补全，覆盖 template inspect/validate/preview/render/delete 以及 journal/index/note `--template`，builtin/local 描述包含模板 kind，delete 只补 local 模板。
- 2026-06-08: `go test ./cmd/pinax -run 'TemplateFlagCompletion|TemplateVarCompletion|TemplateRunCompletion' -count=1` 通过；5.4 增加 `--engine`、`--var`、`--run` 上下文补全，`--var` 只补 `key=` 和 required/optional 描述，不填 secret-like 变量值。
- 2026-06-08: `go test ./internal/app ./internal/output -run 'TemplateNextAction|ProjectionActions|AgentActions|JSONActions' -count=1` 通过；5.5 将 template inspect 下一步 action 纳入 projection，summary/json/agent 都从同一 actions 字段渲染，agent 输出稳定 `action.<name>=...`。
- 2026-06-08: `go test ./internal/app ./internal/cli ./internal/output -run 'TemplateListPack|TemplateListUseCase|TemplateRecommend|TemplateRecommendFallback' -count=1` 和 `go test ./cmd/pinax -run 'TemplateListPack|TemplateListUseCase|TemplateRecommend|TemplateRecommendFallback' -count=1` 通过；5.6 增加 metadata-only 本地 `template list --pack|--use-case` 与 `template recommend --intent`，meeting intent 推荐 `meeting.notes`，未知 intent fallback 到 starter capture 模板，不执行模板、不执行 SQL、不联网。
