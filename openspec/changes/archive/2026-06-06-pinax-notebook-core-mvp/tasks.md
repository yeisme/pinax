## 1. RED 用户流程测试

- [x] 1.1 新增 daily workflow CLI 测试：`daily open/show/append` 创建、读取、追加 daily note，并验证 editor 边界和 index refresh。
- [x] 1.2 新增 inbox workflow CLI 测试：`inbox capture/list/triage` 覆盖 inbox frontmatter、daily index、移动和冲突。
- [x] 1.3 新增组织视图 CLI 测试：`tag list`、`folder list`、`kind list`、`group list` 返回 counts 和机器输出 facts。
- [x] 1.4 新增链接关系 CLI 测试：`note links/backlinks/orphans` 覆盖 resolved、broken、orphan 和系统 index note 排除。
- [x] 1.5 新增附件 CLI 测试：`note attach/attachments` 覆盖复制、Markdown 引用、缺失源文件和 vault 边界。
- [x] 1.6 新增 saved view CLI 测试：`view save/list/show/delete` 覆盖 `.pinax/views.json` service 写入和当前结果重算。
- [x] 1.7 新增 import/export CLI 测试：dry-run、conflict rename、receipt、Markdown bundle 和附件导出。
- [x] 1.8 新增 index/search CLI 测试：`index init/status/rebuild`、fresh/stale/missing、indexed search 和 fallback engine。
- [x] 1.9 新增 agent organize CLI 测试：`organize suggest --save --json/--agent`、plan schema、risk 分类、stale plan 拒绝和 snapshot-protected apply。

## 2. 数据模型和索引投影

- [x] 2.1 扩展 `domain.Note`、projection 数据结构和输出 facts，支持 group/folder/kind/status/date/link/attachment/view/import/export。
- [x] 2.2 扩展 GORM index records：note timestamps、resolved links、broken links、attachments 和组织维度 counts。
- [x] 2.3 增加 index schema metadata、note text、search token、index run/source facts records。
- [x] 2.4 实现 index repository 边界，所有数据库读写通过 GORM，不在 service/command 层硬编码 SQL。
- [x] 2.5 实现 scan fallback，确保 index 缺失时 organization/link/attachment/search 命令仍能读 vault。
- [x] 2.6 确认 `kind=index` 的 daily system index 不污染普通 note 统计、orphans、search 和 saved view 结果。

## 3. Index 和 Search

- [x] 3.1 实现 `pinax index init`，创建 `.pinax/index.sqlite` schema metadata 并返回稳定 facts。
- [x] 3.2 增强 `pinax index rebuild`，在事务边界内重建 note/text/tag/token/link/attachment projection。
- [x] 3.3 实现 `pinax index status`，报告 schema version、fresh/stale/missing/unreadable 和 stale evidence。
- [x] 3.4 实现 index-first search，支持 relevance/updated/created/title/path 排序和 limit。
- [x] 3.5 增强 search filters：tag/group/folder/kind/status/date/link-target/has-attachment。
- [x] 3.6 保留 rg/scan fallback，并在 `--json`/`--agent` facts 中稳定报告 engine 和 index_status。

## 4. Daily 和 Inbox 工作流

- [x] 4.1 在 `internal/app` 实现 daily note create/show/open/append 用例。
- [x] 4.2 在 `cmd/pinax` 接入 `daily open/show/append`，复用 editor 和输出合同。
- [x] 4.3 在 `internal/app` 实现 inbox capture/list/triage，用 service 复用 note create/move/frontmatter patch。
- [x] 4.4 在 `cmd/pinax` 接入 `inbox capture/list/triage`，支持 `--group`、`--folder`、`--kind`、`--status`。

## 5. 组织浏览和保存视图

- [x] 5.1 增强 `note list` 支持 `--group`、`--folder`、`--kind`、`--created-after`、`--updated-before`。
- [x] 5.2 实现 `tag list`、`folder list`、`kind list`、`group list` 的 app service 和 Cobra 命令。
- [x] 5.3 实现 `.pinax/views.json` 读写 service，禁止命令层直接写结构化资产。
- [x] 5.4 实现 `view save/list/show/delete`，其中 `show` 每次根据当前 vault 重新查询。

## 6. 链接、反链和附件

- [x] 6.1 实现 Markdown wiki link、Markdown link/image、相对路径附件引用解析。
- [x] 6.2 实现 `note links`、`note backlinks`、`note orphans` service 和命令。
- [x] 6.3 实现 `note attach` 文件复制、命名冲突处理和 Markdown 引用生成。
- [x] 6.4 实现 `note attachments` 和 doctor 可复用的缺失附件诊断数据。

## 7. Agent 自动整理计划

- [x] 7.1 设计并实现 `domain.OrganizePlan`、`OrganizeOperation`、source facts 和 operation id。
- [x] 7.2 实现 `organize suggest`，基于 index/search/facts 生成 move、tag_patch、kind_patch、status_patch、link_resolution、attachment_repair、manual_review 操作。
- [x] 7.3 实现 `.pinax/organize-plans/<plan_id>.json` service 写入，禁止命令层手写 JSON。
- [x] 7.4 增强 `organize apply --plan`，要求 `--yes` 和 Git snapshot，跳过 manual_review 操作并刷新 index。
- [x] 7.5 为 `--agent` 输出补稳定 facts：plan_id、operations、automatic、manual_review、risk.low、risk.medium、risk.review、saved_path。
- [x] 7.6 实现 organize plan 读取和 source facts stale 检查。
- [x] 7.7 实现 organize plan 列表命令。

## 8. 本地导入导出

- [x] 8.1 实现 `import markdown` plan/dry-run，支持文件和目录、group/folder/kind/status/tags 默认值。
- [x] 8.2 实现 import apply，支持 `skip`、`rename`、显式 `overwrite --yes` 冲突策略。
- [x] 8.3 实现 `export markdown`，按 note filters 导出 Markdown、附件和 bundle manifest。
- [x] 8.4 通过 app service 写入 redacted import/export receipts，禁止记录 provider token、cookie 或 raw external payload。

## 9. 输出合同、文档和验证

- [x] 9.1 为新增命令补齐默认中文输出、`--json`、`--agent` 和错误 code contract tests。
- [x] 9.2 更新 `note --help`、`search --help`、`index --help`、`organize --help`、root help 和必要 README/docs 示例，示例只使用本地 vault。
- [x] 9.3 运行聚焦测试：`go test ./cmd/pinax ./internal/app ./internal/index ./internal/output -count=1`。
- [x] 9.4 运行完整门禁：`task check`。
- [x] 9.5 同步 delta specs 到主 specs，`openspec validate --all` 通过后归档 change。

## Verification Evidence

- RED confirmed: `go test ./cmd/pinax -run TestIndexSearchDatabaseAndFiltersCLI -count=1` failed on missing `index init` command before implementation.
- RED confirmed: the same focused test later failed on missing `--allow-stale` before stale-index search support was implemented.
- RED confirmed: the same focused test later failed on missing `--sort` before search sorting support was implemented.
- GREEN confirmed: `go test ./cmd/pinax -run TestIndexSearchDatabaseAndFiltersCLI -count=1` exited 0 after index/search implementation.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0.
- Full gate confirmed: `task check` exited 0 after index/search implementation.
- RED confirmed: `go test ./cmd/pinax -run TestOrganizeSuggestCreatesReviewableAgentPlan -count=1` failed on missing `organize suggest` command before implementation.
- GREEN confirmed: `go test ./cmd/pinax -run TestOrganizeSuggestCreatesReviewableAgentPlan -count=1` exited 0 after organize suggest implementation.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after organize suggest implementation.
- Full gate confirmed: `task check` exited 0 after organize suggest implementation.
- RED confirmed: `go test ./cmd/pinax -run TestOrganizeApplySavedPlanRejectsStaleAndMoves -count=1` failed on missing `--plan` flag before saved organize plan apply was implemented.
- GREEN confirmed: `go test ./cmd/pinax -run TestOrganizeApplySavedPlanRejectsStaleAndMoves -count=1` exited 0 after saved organize plan apply and stale check implementation.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after organize apply --plan implementation.
- Full gate confirmed: `task check` exited 0 after organize apply --plan implementation.
- RED confirmed: `go test ./cmd/pinax -run TestDailyInboxWorkflowCLI -count=1` failed on missing `daily` command before daily workflow implementation.
- RED confirmed: the same focused test later failed on missing `inbox` command before inbox workflow implementation.
- GREEN confirmed: `go test ./cmd/pinax -run TestDailyInboxWorkflowCLI -count=1` exited 0 after daily/inbox workflow implementation.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after daily/inbox workflow implementation.
- Full gate confirmed: `task check` exited 0 after daily/inbox workflow implementation.
- RED confirmed: `go test ./cmd/pinax -run TestNotebookOrganizationViewsCLI -count=1` failed on missing `--group` note list flag before organization filters were implemented.
- RED confirmed: the same focused test later failed on missing root `tag` command before organization dimension views were implemented.
- GREEN confirmed: `go test ./cmd/pinax -run TestNotebookOrganizationViewsCLI -count=1` exited 0 after organization views implementation.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after organization views implementation.
- Full gate confirmed: `task check` exited 0 after organization views implementation.
- RED confirmed: `go test ./cmd/pinax -run TestSavedViewsCLI -count=1` failed on missing root `view` command before saved view CLI implementation.
- GREEN confirmed: `go test ./cmd/pinax -run TestSavedViewsCLI -count=1` exited 0 after saved view service and CLI implementation.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after saved view implementation.
- Full gate confirmed: `task check` exited 0 after saved view implementation.
- RED confirmed: `go test ./cmd/pinax -run TestNoteLinkGraphCLI -count=1` failed on missing `note links` command before link graph CLI implementation.
- GREEN confirmed: `go test ./cmd/pinax -run TestNoteLinkGraphCLI -count=1` exited 0 after note links/backlinks/orphans implementation.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after link graph implementation.
- Full gate confirmed: `task check` exited 0 after link graph implementation.
- RED confirmed: `go test ./cmd/pinax -run TestNoteAttachmentCLI -count=1` failed on missing `note attach` command before attachment CLI implementation.
- GREEN confirmed: `go test ./cmd/pinax -run TestNoteAttachmentCLI -count=1` exited 0 after note attach/attachments implementation.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after attachment implementation.
- Full gate confirmed: `task check` exited 0 after attachment implementation.
- RED confirmed: `go test ./cmd/pinax -run TestImportExportMarkdownCLI -count=1` failed on missing root `import` command before import/export implementation.
- GREEN confirmed: `go test ./cmd/pinax -run TestImportExportMarkdownCLI -count=1` exited 0 after import markdown and export markdown implementation.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after import/export implementation.
- Full gate confirmed: `task check` exited 0 after import/export implementation.
- RED confirmed: `go test ./cmd/pinax -run TestOrganizeSuggestCreatesReviewableAgentPlan -count=1` failed on missing `tag_patch` operation kind before expanded organize suggestions.
- GREEN confirmed: `go test ./cmd/pinax -run TestOrganizeSuggestCreatesReviewableAgentPlan -count=1` exited 0 after expanded organize operations and organize list implementation.
- Regression confirmed: `go test ./cmd/pinax -run TestOrganizeApplySavedPlanRejectsStaleAndMoves -count=1` exited 0 after expanded organize operations, confirming non-move operations are not applied automatically.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after organize enhancement.
- Full gate confirmed: `task check` exited 0 after organize enhancement.
- RED confirmed: `go test ./cmd/pinax -run TestImportExportMarkdownCLI -count=1` failed with `invalid_import_conflict` before overwrite import support.
- GREEN confirmed: `go test ./cmd/pinax -run TestImportExportMarkdownCLI -count=1` exited 0 after overwrite import support.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/search -count=1` exited 0 after overwrite import support.
- Full gate confirmed: `task check` exited 0 after overwrite import support.
- RED confirmed: `go test ./cmd/pinax -run 'TestNoteLinkGraphCLI|TestNotebookCoreOutputContractAndHelp' -count=1` failed on missing `note.backlinks` `unresolved` fact before output contract hardening.
- GREEN confirmed: `go test ./cmd/pinax -run 'TestNoteLinkGraphCLI|TestNotebookCoreOutputContractAndHelp' -count=1` exited 0 after output contract/help hardening.
- RED confirmed: `go test ./cmd/pinax -run TestIndexSearchDatabaseAndFiltersCLI -count=1` failed on missing `dimensions` index rebuild fact before GORM dimension count records.
- GREEN confirmed: `go test ./cmd/pinax -run TestIndexSearchDatabaseAndFiltersCLI -count=1` exited 0 after adding `DimensionCountRecord` and dimension rebuild facts.
- RED confirmed: `go test ./cmd/pinax -run TestNoteCreateBuildsNotebookInformationArchitecture -count=1` failed because `kind=index` daily index notes polluted `vault.stats` note count.
- GREEN confirmed: `go test ./cmd/pinax -run 'TestNoteCreateBuildsNotebookInformationArchitecture|TestIndexSearchDatabaseAndFiltersCLI' -count=1` exited 0 after excluding system index notes from ordinary note facts.
- Package verification confirmed: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/output -count=1` exited 0 after output/model/index hardening.
- Docs synced: `README.md`, `docs/README.md`, `docs/product/mvp-scope.md`, and `docs/interfaces/cli-output-contract.md` updated with notebook core commands and output facts.
- Main specs synced: `openspec/specs/pinax/spec.md`, `openspec/specs/note-command-ux/spec.md`, `openspec/specs/notebook-index-search/spec.md`, and `openspec/specs/notebook-workflows/spec.md` updated from delta specs.
- OpenSpec verification confirmed: `openspec validate --all` exited 0 after spec sync.
- Full gate confirmed: `task check` exited 0 after docs/spec sync and final implementation hardening.
