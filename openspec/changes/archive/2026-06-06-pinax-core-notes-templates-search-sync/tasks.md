# Tasks

## 1. 计划和规格

- [x] 1.1 补齐 proposal/design/tasks/spec，并通过 `openspec validate --all`。
- [x] 1.2 在任务完成时补充验证命令、输出摘要和残余风险。

## 2. 笔记和模板

- [x] 2.1 为 `note new`、`template init/list/show/render` 写 service 和 CLI 测试。
- [x] 2.2 实现带 YAML frontmatter 的笔记创建，支持标题、项目、标签、模板和安全路径。
- [x] 2.3 实现内置 Markdown/YAML/Mermaid 模板和保守变量替换。

## 3. 检索和索引

- [x] 3.1 为 `rg` 回退、tag/backlink 投影和 `index rebuild` 写测试。
- [x] 3.2 增加 `internal/search`，优先调用 `rg`，失败回退扫描。
- [x] 3.3 增加 `internal/index`，通过 GORM/SQLite 写入 note/tag/link 投影。

## 4. 同步计划

- [x] 4.1 为 `sync diff/push/pull` 的 `git/s3/cloud` 目标和 `--yes` 门禁写测试。
- [x] 4.2 实现同步 plan/state 资产创建，确保不保存 secret，不执行真实远端写入。
- [x] 4.3 输出 Pinax Cloud 后端 handoff，说明独立子模块接入条件。

## 5. 验证

- [x] 5.1 运行 `gofmt -w <changed-go-files>`。
- [x] 5.2 运行 `go test ./...`。
- [x] 5.3 运行 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
- [x] 5.4 运行 `openspec validate --all`，并归档已完成 change。

## Verification Evidence

- 2026-06-06: `go test ./internal/app -run 'CoreNoteTemplateIndexAndSyncMVP' -count=1` passed.
- 2026-06-06: `go test ./cmd/pinax -run 'CoreMVPCLIJSON' -count=1` passed.
- 2026-06-06: `go test ./...` passed.
- 2026-06-06: `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` passed.
- 2026-06-06: `task check` passed; validates both active OpenSpec changes, checks gofmt, runs full tests, and rebuilds `dist/pinax`.
- Residual risk: `sync push/pull` records local sync state only; real S3 object IO and Pinax Cloud backend require follow-up adapter/backend implementation.
- Residual blocker: `backend-server/pinax-cloud` cannot be created correctly until an independent remote repository URL is provided for the required submodule.
