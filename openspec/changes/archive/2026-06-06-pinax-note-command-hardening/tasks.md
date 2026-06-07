## 1. RED Tests and Fixtures

- [x] 1.1 新增 note hardening fixture vault，覆盖带注释 frontmatter、未知字段、同日 trash 冲突、rename 目标冲突和 nested path。
- [x] 1.2 先写 editor parser unit tests，覆盖 `code --wait`、`vim -n`、quoted arg、空 editor 和 shell metacharacters，并确认测试先失败。
- [x] 1.3 先写 `note edit/open/new --open` fake editor CLI tests，验证 executable/args/path、stdout/stderr 分离和 `--json`/`--agent` facts，并确认测试先失败。
- [x] 1.4 先写 rename 原子性 tests，模拟目标写入或 rename 失败时原 note path/title/body 不变，并确认测试先失败。
- [x] 1.5 先写 trash 唯一路径 tests，覆盖 `.pinax/trash/YYYYMMDD/...` 已存在时生成后缀且不覆盖，并确认测试先失败。
- [x] 1.6 先写 frontmatter patch 保真 tests，覆盖 tag/archive/rename 保留未知字段、常见注释和正文，并确认测试先失败。
- [x] 1.7 先写 `note list --recent` JSON/agent/human contract tests，验证 sort facts、无隐式过滤和 updated 显示，并确认测试先失败。

## 2. Editor Execution Hardening

- [x] 2.1 新增 `EditorCommand` 解析 helper，支持常见 shell-like quoting，但不通过 shell 执行。
- [x] 2.2 新增 editor runner seam，用 fake executable 测试 executable、args 和 note path 传递。
- [x] 2.3 改造 `EditNote` 使用 parsed editor command，并在 projection facts/data 中输出 editor executable 和 args。
- [x] 2.4 改造 `note new/create --open` 路径，复用 editor runner，并保证 create projection 不被 edit projection 吞掉或产生双 JSON。
- [x] 2.5 对 editor parse/exec 错误返回稳定错误码和中文 hint，禁止泄露环境变量或 shell trace。

## 3. Atomic Note Mutation Helpers

- [x] 3.1 新增 note mutation helper，统一读取、patch frontmatter、写临时文件、commit、append event 和 projection。
- [x] 3.2 改造 `note rename` 为 prepare/commit 流程，避免写旧文件后 rename 失败造成半状态。
- [x] 3.3 改造 `note archive` 使用 mutation helper，仅更新 status/updated_at 并记录 redacted event。
- [x] 3.4 改造 `note tag add/remove/set` 使用 mutation helper，保持 tag 去重和稳定排序。
- [x] 3.5 为 mutation helper 的失败路径增加中文注释，说明哪些失败前保证原文件不变、哪些失败需要用户用 Git 恢复。

## 4. Safe Trash Paths

- [x] 4.1 新增 unique trash path helper，基于日期、原相对路径和数字后缀生成不冲突目标。
- [x] 4.2 改造 `note delete --yes` 使用 unique trash path，不覆盖已存在 trash 文件。
- [x] 4.3 在 JSON/agent projection 和 event facts 中记录最终 `trash_path`、原 path 和 note id。
- [x] 4.4 保持 `note delete --hard` 必须同时具备 `--hard --yes`，并补充 hard delete 不走 trash helper 的测试。

## 5. Frontmatter Patch Preservation

- [x] 5.1 新增 `patchFrontmatterFields`，优先局部替换 Pinax 管理字段并保留未知字段、常见注释和字段顺序。
- [x] 5.2 对缺失 frontmatter 或无法安全 patch 的文件降级 canonical render，并在 projection/evidence 中暴露 normalized outcome。
- [x] 5.3 确保 `title`、`tags`、`status`、`updated_at` 更新后仍可被 `scanNotes` 和 resolver 正确读取。
- [x] 5.4 更新或新增 frontmatter parser tests，覆盖 YAML list tags、inline tags、空 tags、未知字段和 body 分隔。

## 6. Recent Semantics and Output Contract

- [x] 6.1 明确 `NoteListQuery.Recent` 等价于 updated-time sort，并在 facts 中输出 `sort=updated` 和 `recent=true`。
- [x] 6.2 对无 `updated_at` 的 note 使用 file mtime 或稳定 fallback，避免 recent 排序不可预测。
- [x] 6.3 更新 human note list 行输出，保持 path/title/tags/status/updated 可扫且不超过 20 行默认截断。
- [x] 6.4 更新 `--agent` 输出合同，覆盖 `fact.sort`、`fact.recent`、editor facts、trash facts 和 mutation outcome。
- [x] 6.5 更新 README 或 docs 中 note edit/delete/list 的边界说明，不新增独立执行 checklist。

## 7. Verification

- [x] 7.1 运行 `gofmt -w` 覆盖变更 Go 文件。
- [x] 7.2 运行聚焦测试：`go test ./internal/app ./cmd/pinax ./internal/output`。
- [x] 7.3 运行全量测试：`go test ./...`。
- [x] 7.4 运行构建：`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
- [x] 7.5 运行 OpenSpec 校验：`openspec validate --all`。
- [x] 7.6 如果本机安装 `task`，运行 `task check`；否则记录 fallback 命令结果。

## Verification Evidence

2026-06-06 local verification:

- RED confirmed with `go test ./internal/app ./cmd/pinax ./internal/output`: missing `parseEditorCommand`/`patchFrontmatterFields` and editor arg execution failure.
- `gofmt -w internal/app/service.go internal/app/service_test.go cmd/pinax/main.go cmd/pinax/main_test.go`
- `go test ./internal/app ./cmd/pinax ./internal/output`
- `go test ./...`
- `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`
- `openspec validate --all`
- `task check`
