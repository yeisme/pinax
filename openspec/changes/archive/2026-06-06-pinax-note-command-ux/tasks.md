## 1. RED Tests and Fixtures

- [x] 1.1 新增 note UX fixture vault，覆盖 note id、路径、标题唯一匹配、标题歧义、tags、project、status、recent/stale 和 nested directory。
- [x] 1.2 先写 `note list` 过滤/排序/limit 的 JSON 和 human contract tests，并确认测试先失败。
- [x] 1.3 先写 note ref resolver tests，覆盖 id、path、title、唯一标题和 ambiguous candidates，并确认测试先失败。
- [x] 1.4 先写 `note new/create` 的 `--body`、`--from`、`--stdin`、`--dry-run` 和 source conflict tests，并确认测试先失败。
- [x] 1.5 先写 `note edit/open` fake editor tests，覆盖 editor 调用、缺 editor 错误和 stdout/stderr 分离，并确认测试先失败。
- [x] 1.6 先写 `note rename/move/archive/delete/tag` 安全行为 tests，并确认测试先失败。
- [x] 1.7 先写 `pinax note --help` UX tests，验证 daily workflow commands 和 aliases 出现在 help 中，并确认测试先失败。

## 2. Note Reference and Query Model

- [x] 2.1 新增 `NoteRefResolver`，按 note id、路径、`notes/` 前缀容错、标题精确匹配、唯一标题匹配顺序解析。
- [x] 2.2 实现 ambiguous candidates projection，错误码 `note_ref_ambiguous`，JSON/agent 包含候选 path/note_id/title。
- [x] 2.3 新增 `NoteListQuery`，支持 tag、project、status、recent、limit、sort、path-prefix。
- [x] 2.4 扩展 note scan facts，读取 frontmatter project/status/created_at/updated_at 和 file mtime，保持无 frontmatter 时可降级。
- [x] 2.5 为 resolver 和 query 复杂边界加中文注释，说明为什么禁止模糊误匹配。

## 3. Ergonomic Note Creation

- [x] 3.1 扩展 `CreateNoteRequest`，支持 body、source file、stdin body、dir、slug、status、dry-run 和 open-after-create。
- [x] 3.2 实现内容来源互斥校验，冲突返回 `note_source_conflict`。
- [x] 3.3 实现 `--dir` 和 `--slug` 的 vault boundary 校验，拒绝绝对路径、`..` 和 `.pinax`。
- [x] 3.4 实现 dry-run projection，返回 planned path、frontmatter preview 和 body preview，不写文件或事件。
- [x] 3.5 保持旧 `pinax note new <title>` 行为兼容，新增 `note create` alias。

## 4. List, Show, Read, Open, Edit Commands

- [x] 4.1 增强 `pinax note list` flags：`--tag`、`--project`、`--status`、`--recent`、`--limit`、`--sort`、`--path-prefix`。
- [x] 4.2 改善 default human list 输出，展示 path/title/tags/status/recent，保持短且可扫。
- [x] 4.3 确保 `note list --json` 输出单一 envelope，包含 filter facts、total、returned 和 notes。
- [x] 4.4 新增 `note read` alias，复用 `note show` projection 和 resolver。
- [x] 4.5 新增 `note open`/`note edit` 命令，支持 `$EDITOR`、`--editor` 和 fake editor 测试。
- [x] 4.6 缺 editor 返回 `editor_not_configured`，并给出设置 `$EDITOR` 或传 `--editor` 的 next action。

## 5. Single-note Maintenance Commands

- [x] 5.1 新增 `note rename <ref> <title>`，更新 frontmatter title 和安全目标路径，冲突返回 `note_path_conflict`。
- [x] 5.2 新增 `note move <ref> <dir>`，只允许移动到 vault 内非 `.pinax` 路径。
- [x] 5.3 新增 `note archive <ref>`，只写 frontmatter `status: archived`，不移动文件。
- [x] 5.4 新增 `note delete <ref>`，默认要求 `--yes` 后移动到 `.pinax/trash/YYYYMMDD/`，记录 redacted event。
- [x] 5.5 新增 `note delete --hard --yes`，真实删除必须同时具备 `--hard` 和 `--yes`，否则返回 `approval_required`。
- [x] 5.6 新增 `note tag add/remove/set`，通过 service 更新 frontmatter tags，去重并保持稳定排序。
- [x] 5.7 所有写入操作必须 append redacted event evidence，禁止输出 raw payload、secret 或未脱敏 trace。

## 6. CLI Output Contract and Docs

- [x] 6.1 更新 note command projection，覆盖 created/updated/deleted/trashed/tagged/editor/ambiguous candidates 等 data shape。
- [x] 6.2 为 `--agent` 增加稳定 key=value：command、status、fact.path、fact.note_id、fact.count、action.*。
- [x] 6.3 为 `--json` 增加 envelope contract tests，确保 stdout 只有 JSON。
- [x] 6.4 为默认 human 输出增加 golden tests，确保中文摘要可扫且不混入 diagnostics。
- [x] 6.5 更新 README 或 docs 的 note CLI 日常工作流示例，不新增独立执行 checklist。

## 7. Verification

- [x] 7.1 运行 `gofmt -w` 覆盖变更 Go 文件。
- [x] 7.2 运行聚焦测试：`go test ./internal/app ./cmd/pinax ./internal/output`。
- [x] 7.3 运行全量测试：`go test ./...`。
- [x] 7.4 运行构建：`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
- [x] 7.5 运行 OpenSpec 校验：`openspec validate --all`。
- [x] 7.6 如果本机安装 `task`，运行 `task check`；否则记录 fallback 命令结果。

## Verification Evidence

2026-06-06 local verification:

- `gofmt -w internal/app/service.go internal/output/render.go cmd/pinax/main_test.go`
- `go test ./internal/app ./cmd/pinax ./internal/output`
- `go test ./...`
- `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`
- `openspec validate --all`
- `task check`
