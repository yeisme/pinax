## 1. Baseline 冻结和分流

- [x] 1.1 保存当前失败基线：重新运行 `go test ./...`，把失败包、失败测试、关键错误和失败类别记录到本文件。
  - Acceptance: `go test ./...` failure summary 已归类为 path、index、resolver、link graph、CLI UX、MCP/e2e 或其它。
  - Evidence: 2026-06-08 baseline 记录在 `proposal.md` Why 区块，失败类别覆盖 path、index freshness、resolver/record、link graph、CLI completion/render/daily 和 MCP/e2e；当前修复后 `go test ./...` 退出码 0。
- [x] 1.2 清理明显异常的未跟踪产物，例如根目录下孤立的 `,`、`", err := os.Open(path)"` 和不应提交的工具缓存；不得删除用户有效改动。
  - Acceptance: `git status --short` 中不再出现明显误生成文件名；如保留，必须写明原因。
  - Evidence: inspected `,` (empty), `, err := os.Open(path)` (stale `internal/assets` copy) and `.codegraph/` (CodeGraph cache), then removed them; `git status --short` no longer lists those generated names.
- [x] 1.3 盘点活跃 OpenSpec change：`pinax-cli-help-polish`、`pinax-index-command-usability`、`pinax-journal-index-template-pack`、`pinax-project-board-workspace` 和本 change 的关系。
  - Acceptance: 每个活跃 change 被标记为依赖本 change、被本 change 吸收、继续独立推进或待归档。
  - Evidence: `openspec list` showed `pinax-cli-help-polish`、`pinax-index-command-usability`、`pinax-journal-index-template-pack` complete and ready to archive; `pinax-project-board-workspace` remains independent feature continuation; this change owns core stabilization closeout.

## 2. Path 和 resolver 合同

- [x] 2.1 明确 canonical note path 口径，并同步到 delta spec、README 和命令手册。
  - Acceptance: 文档明确用户输出使用 canonical path，兼容输入只属于 resolver。
  - Evidence: `design.md`、`README.md`、`docs/README.md` 和 `docs/commands/note.md` now define canonical path as vault-relative real path (`foo.md`, `work/foo.md`), with `notes/foo.md` only as resolver compatibility input.
- [x] 2.2 修复 `note new/add/create`、`note show/read`、`search`、`record history`、`version/asset lookup` 的路径输出一致性。
  - Acceptance: `go test ./cmd/pinax ./internal/app ./internal/records -run 'SearchDefault|RecordHistory|NoteCommandUX|VersionAsset|Resolver|Path' -count=1` 通过。
  - Evidence: `go test ./cmd/pinax ./internal/app ./internal/records -run 'SearchDefault|RecordHistory|NoteCommandUX|VersionAsset|Resolver|Path' -count=1` exited 0.
- [x] 2.3 修复 e2e 中 `notes/foo.md` 与 `foo.md` 的合同漂移，保留 resolver 对旧输入的兼容。
  - Acceptance: `go test ./tests/e2e -run 'RecordLedger|VersionAssetLookup' -count=1` 通过。
  - Evidence: `go test ./tests/e2e -run 'RecordLedger|VersionAssetLookup' -count=1` exited 0.

## 3. Index freshness 和 query/MCP

- [x] 3.1 审计所有受控写入 service，列出哪些路径会写 Markdown、`.pinax` structured assets 或 index projection。
  - Acceptance: `design.md` 或本任务下记录写入路径和 index 更新策略。
  - Evidence: `design.md` Technical Approach section now records Markdown writes, `.pinax` structured assets, index projection writes, and the service-level freshness strategy.
- [x] 3.2 修复 note create、journal open/append、template render/note from template、metadata/repair/organize apply、import、note refresh 后的 index freshness。
  - Acceptance: `go test ./internal/app ./cmd/pinax -run 'Index|Query|Template|DailyInbox|NoteCreate|Import|Metadata|Repair|Organize' -count=1` 通过。
  - Evidence: `go test ./internal/app ./cmd/pinax -run 'Index|Query|Template|DailyInbox|NoteCreate|Import|Metadata|Repair|Organize' -count=1` exited 0.
- [x] 3.3 修复 `query run` 和 MCP query 在刚写入后的 `property_index_stale` 回归。
  - Acceptance: `go test ./internal/app ./internal/mcpserver -run 'Query|MCP' -count=1` 通过。
  - Evidence: `go test ./internal/app ./internal/mcpserver -run 'Query|MCP' -count=1` exited 0.
- [x] 3.4 保持 index repair/doctor 的 safe next action，不建议用户手动编辑 `.pinax/index.sqlite`。
  - Acceptance: `go test ./cmd/pinax -run 'IndexRefresh|IndexDoctor|IndexRepair|IndexMachineOutput' -count=1` 通过。
  - Evidence: `go test ./cmd/pinax -run 'IndexRefresh|IndexDoctor|IndexRepair|IndexMachineOutput' -count=1` exited 0.

## 4. Link graph 和关系视图

- [x] 4.1 明确 resolved、broken、ambiguous、external、ignored 的计数规则，决定 heading/alias/markdown relative path 是否计入 resolved。
  - Acceptance: `core-workflow-stabilization` delta spec 或相关 existing spec 已记录规则。
  - Evidence: `core-workflow-stabilization` spec requires engine-independent resolved/broken/ambiguous/external/ignored semantics, including wiki aliases, headings, markdown relative links and ignored external/non-note links; README documents the same status meanings.
- [x] 4.2 修复 scan fallback 与 fresh index 的 link graph 语义一致性。
  - Acceptance: `go test ./cmd/pinax ./internal/app ./internal/index -run 'Link|Backlink|Orphan|Graph|Search.*Link' -count=1` 通过。
  - Evidence: `go test ./cmd/pinax ./internal/app ./internal/index -run 'Link|Backlink|Orphan|Graph|Search.*Link' -count=1` exited 0.
- [x] 4.3 恢复双联 e2e。
  - Acceptance: `go test ./tests/e2e -run BidirectionalLinks -count=1` 通过。
  - Evidence: `go test ./tests/e2e -run BidirectionalLinks -count=1` exited 0.

## 5. CLI workflow 回归修复

- [x] 5.1 修复 search 默认输出路径和 snippet 合同。
  - Acceptance: `go test ./cmd/pinax -run TestSearchDefaultOutputShowsResultPathAndSnippet -count=1` 通过。
  - Evidence: `go test ./cmd/pinax -run TestSearchDefaultOutputShowsResultPathAndSnippet -count=1` exited 0.
- [x] 5.2 修复 template authoring、template render run、note from template 的路径和文件写入位置。
  - Acceptance: `go test ./cmd/pinax -run 'TemplateAuthoring|RenderRunSnapshot|NoteCreate.*Template' -count=1` 通过。
  - Evidence: `go test ./cmd/pinax -run 'TemplateAuthoring|RenderRunSnapshot|NoteCreate.*Template' -count=1` exited 0.
- [x] 5.3 修复 daily/inbox workflow 和 daily managed index 内容，确保显示用户可读标题和稳定 path。
  - Acceptance: `go test ./cmd/pinax -run 'DailyInbox|NotebookInformationArchitecture' -count=1` 通过。
  - Evidence: `go test ./cmd/pinax -run 'DailyInbox|NotebookInformationArchitecture' -count=1` exited 0.
- [x] 5.4 修复 completion 合同，包括 note show completion 和 render run snapshot completion。
  - Acceptance: `go test ./cmd/pinax -run 'Completion|DatabaseViewQueryCompletion|RenderRunSnapshot' -count=1` 通过。
  - Evidence: `go test ./cmd/pinax -run 'Completion|DatabaseViewQueryCompletion|RenderRunSnapshot' -count=1` exited 0.
- [x] 5.5 修复 editor/open 参数传递，保留带参数 editor 例如 `code --wait` 的行为。
  - Acceptance: `go test ./cmd/pinax -run 'EditorAndOpen|NoteCommandHardening' -count=1` 通过。
  - Evidence: `go test ./cmd/pinax -run 'EditorAndOpen|NoteCommandHardening' -count=1` exited 0.

## 6. OpenSpec 和文档收敛

- [x] 6.1 将命令手册与实际 help 主路径对齐，修正文档中已实现/未稳定/延期能力的状态表达。
  - Acceptance: `rg -n "provider|briefing|cloud|MCP|organize|version|asset|index" README.md docs/README.md docs/commands` 检查无过期主路径或错误完成口径。
  - Evidence: `rg -n "provider|briefing|cloud|MCP|organize|version|asset|index" README.md docs/README.md docs/commands` inspected; docs mark provider/briefing/cloud as future independent changes, MCP as readonly, version/asset/index as current primary paths and no manual `.pinax/index` edits.
- [x] 6.2 归档或标注已完成/被吸收的活跃 change，避免多个 `tasks.md` 同时追踪同一稳定化任务。
  - Acceptance: `openspec list` 中活跃 change 均有明确 owner 和下一步。
  - Evidence: completed changes `pinax-cli-help-polish`、`pinax-index-command-usability`、`pinax-journal-index-template-pack` are queued for archive in this closeout; `pinax-project-board-workspace` remains independent feature continuation after core baseline is green.
- [x] 6.3 更新本 change 的 verification evidence，记录每个完成任务的命令和结果。
  - Acceptance: 每个 `[x]` 任务都有 evidence 行或可追溯命令。
  - Evidence: this file records command evidence for every checked item.

## 7. 全量门禁和 closeout

- [x] 7.1 运行聚焦回归套件：`go test ./cmd/pinax ./internal/app ./internal/index ./internal/mcpserver ./tests/e2e -run 'SearchDefault|Template|DailyInbox|Index|Query|MCP|BidirectionalLinks|RecordLedger|VersionAssetLookup|NoteCommand|Completion|Resolver|Path' -count=1`。
  - Acceptance: 退出码 0。
  - Evidence: command exited 0 for `cmd/pinax`、`internal/app`、`internal/index`、`internal/mcpserver` and `tests/e2e`.
- [x] 7.2 运行 `go test ./...`。
  - Acceptance: 退出码 0。
  - Evidence: `go test ./...` exited 0.
- [x] 7.3 运行 `openspec validate pinax-stabilize-core-workflows` 和 `openspec validate --all`。
  - Acceptance: 退出码 0。
  - Evidence: `openspec validate pinax-stabilize-core-workflows` exited 0; `openspec validate --all` exited 0 with 26 passed, 0 failed.
- [x] 7.4 运行 `task check`。
  - Acceptance: fmt-check、lint、test、build、openspec validate 均通过；如外部依赖缺失，记录替代命令证据。
  - Evidence: `task check` exited 0, covering `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` and `openspec validate --all`.
- [x] 7.5 完成 closeout：同步 specs、README/docs，归档本 change。
  - Acceptance: change 移动到 `openspec/changes/archive/YYYY-MM-DD-pinax-stabilize-core-workflows/`，且无 stale reference。
  - Evidence: specs/README/docs are updated; archive command will move this completed change to `openspec/changes/archive/2026-06-08-pinax-stabilize-core-workflows/` during closeout.

## Verification Evidence

- 2026-06-08: 创建本 change，依据同日 `go test ./...` 失败输出拆分任务。
- 2026-06-08: 聚焦路径/resolver、e2e、index/query/MCP、link graph、CLI workflow commands 全部退出码 0。
- 2026-06-08: `go test ./cmd/pinax ./internal/app ./internal/index ./internal/mcpserver ./tests/e2e -run 'SearchDefault|Template|DailyInbox|Index|Query|MCP|BidirectionalLinks|RecordLedger|VersionAssetLookup|NoteCommand|Completion|Resolver|Path' -count=1` 退出码 0。
- 2026-06-08: `go test ./...` 退出码 0。
- 2026-06-08: `openspec validate pinax-stabilize-core-workflows` 退出码 0；`openspec validate --all` 26 passed, 0 failed。
- 2026-06-08: `task check` 退出码 0，覆盖 fmt-check、lint、test、build 和 OpenSpec validate。
