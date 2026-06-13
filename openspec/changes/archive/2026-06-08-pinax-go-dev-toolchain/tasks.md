## 0. Preflight

- [x] 0.1 Owner: `cli/pinax`; Lane: sequential; Scope: 只读检查。运行 `git status --short`。
  - Evidence: 2026-06-08 运行 `git status --short`，退出码 0；worktree 含已归档 `pinax-note-bidirectional-links` 的既有改动和本 change 待添加文件，未回滚用户/既有变更。
- [x] 0.2 Owner: `cli/pinax`; Lane: sequential; Scope: 工具可用性。运行 `golangci-lint version`、`task --version`。
  - Evidence: 2026-06-08 运行 `golangci-lint version`，退出码 0，版本 `2.2.0`；运行 `task --version`，退出码 0，版本 `3.49.1`。

## 1. 新增 lint 配置

- [x] 1.1 Owner: `cli/pinax`; Lane: A; Depends on: 0.2; Scope: 新增 `.golangci.yml`。使用 `version: "2"`，覆盖基线 linters 和 formatters；Acceptance: `golangci-lint run --new-from-rev=HEAD~` 退出码 0。
  - Evidence: 2026-06-08 新增 `.golangci.yml`，`version: "2"`，启用 errcheck、govet、ineffassign、staticcheck、unused、misspell、revive 和 gofmt/goimports formatters；运行 `golangci-lint config verify`，退出码 0。首次运行 `golangci-lint run --new-from-rev=HEAD~` 退出码 1，暴露新增 dashboard/testkit/consistency 文件的 errcheck 和 package comment 问题；修复后重跑同一命令，退出码 0，输出 `0 issues.`。
- [x] 1.2 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: 全量 lint。运行 `golangci-lint run`。
  - Evidence: 2026-06-08 首次运行 `golangci-lint run`，退出码 1，暴露历史 revive exported/package-comments 噪音、少量 errcheck/staticcheck/unused。按 staged lint 方案关闭 revive 的 exported/package-comments 规则，修复 fmt/Close/os.Remove 错误处理、staticcheck 建议，并为保留 helper 添加 `nolint:unused` 说明；重跑 `golangci-lint run`，退出码 0，输出 `0 issues.`。

## 2. Taskfile 补齐

- [x] 2.1 Owner: `cli/pinax`; Lane: B; Depends on: 0.1; Scope: 补齐任务。新增 `deps`、`mod-check`、`fmt`（golangci-lint fmt）、`fmt-check`（golangci-lint fmt --diff）、`lint`、`run`；Acceptance: `task --list` 显示新任务。
  - Evidence: 2026-06-08 更新 `Taskfile.yml`，新增 `deps`、`mod-check`、`lint`、`run`、`ci`，并将 `fmt`/`fmt-check` 切到 `golangci-lint fmt`/`golangci-lint fmt --diff`，`check` 依赖加入 `lint`。运行 `task --list`，退出码 0，显示 `deps`、`mod-check`、`fmt`、`fmt-check`、`lint`、`run`、`test`、`build`、`ci`、`check` 等任务。
- [x] 2.2 Owner: `cli/pinax`; Lane: B; Depends on: 2.1; Scope: 热加载不适用说明。在 `AGENTS.md` 或 design 中说明 Pinax 是 CLI-only 项目，不新增 `.air.toml`。
  - Evidence: 2026-06-08 更新 `AGENTS.md` 技术栈，说明 Pinax 是 CLI-only 短生命周期命令项目，不新增 `.air.toml` 或 Air 热加载入口，本地迭代用 `task run ARGS="..."` 或 `go run ./cmd/pinax ...`。运行 `test ! -f .air.toml`，退出码 0。

## 3. 文档更新

- [x] 3.1 Owner: `cli/pinax`; Lane: C; Depends on: 2.2; Scope: `AGENTS.md`。更新质量门禁命令。
  - Evidence: 2026-06-08 更新 `AGENTS.md` 质量门禁：`task check` 覆盖 `task fmt-check`、`task lint`、`task test`、`task build` 和 `openspec validate --all`，提交前/CI 可运行 `task ci`；无 task fallback 改为 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build ...`、`openspec validate --all`。运行 `rg -n "CLI-only|\.air\.toml|task check|task lint|golangci-lint" AGENTS.md`，退出码 0，命中相关说明。

## 4. 验证

- [x] 4.1 Owner: `cli/pinax`; Lane: D; Depends on: 3.1; Scope: 全量验证。运行 `task fmt-check`、`task lint`、`task test`、`task build`。
  - Evidence: 2026-06-08 运行 `task fmt-check`，退出码 0，执行 `golangci-lint fmt --diff`；运行 `task lint`，退出码 0，`golangci-lint run` 输出 `0 issues.`；运行 `task test`，退出码 0，`go test ./...` 全量通过；运行 `task build`，退出码 0，构建 `dist/pinax` 成功。
