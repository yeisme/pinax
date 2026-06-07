## 1. RED Tests

- [x] 1.1 新增 CLI 用户流程测试，覆盖 `note new --group --folder --kind --tags`。
- [x] 1.2 测试创建后 note 文件包含 project/folder/kind/tags frontmatter。
- [x] 1.3 测试 daily index 包含新 note 路径、tags、group、folder、kind。
- [x] 1.4 测试创建后 `stats --json` 报告 `index_status=fresh`。

## 2. Implementation

- [x] 2.1 扩展 `CreateNoteRequest` 和 `domain.Note`，增加 folder/kind。
- [x] 2.2 新增 Cobra flags：`--group`、`--folder`、`--kind`。
- [x] 2.3 调整 note path 计算：group/project prefix 下可叠加 folder。
- [x] 2.4 创建 note 后由 service 写入 daily index note。
- [x] 2.5 创建 note 后刷新 GORM SQLite index。
- [x] 2.6 SQLite note projection 记录 frontmatter project/folder/kind。

## 3. Verification

- [x] 3.1 运行聚焦测试：`go test ./cmd/pinax -run TestNoteCreateBuildsNotebookInformationArchitecture -count=1`。
- [x] 3.2 运行聚焦包测试：`go test ./internal/app ./cmd/pinax ./internal/index -count=1`。
- [x] 3.3 运行全量门禁：`task check`。

## Verification Evidence

- RED confirmed: `go test ./cmd/pinax -run TestNoteCreateBuildsNotebookInformationArchitecture -count=1` failed with `unknown flag: --group` before implementation.
- GREEN confirmed: `go test ./cmd/pinax -run TestNoteCreateBuildsNotebookInformationArchitecture -count=1` exited 0 after implementation.
- Package verification confirmed: `go test ./internal/app ./cmd/pinax ./internal/index -count=1` exited 0.
- Full gate confirmed: `task check` exited 0 after OpenSpec spec sync.
