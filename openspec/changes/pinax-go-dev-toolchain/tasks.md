## 0. Preflight

- [ ] 0.1 Owner: `cli/pinax`; Lane: sequential; Scope: 只读检查。运行 `git status --short`。
- [ ] 0.2 Owner: `cli/pinax`; Lane: sequential; Scope: 工具可用性。运行 `golangci-lint version`、`task --version`。

## 1. 新增 lint 配置

- [ ] 1.1 Owner: `cli/pinax`; Lane: A; Depends on: 0.2; Scope: 新增 `.golangci.yml`。使用 `version: "2"`，覆盖基线 linters 和 formatters；Acceptance: `golangci-lint run --new-from-rev=HEAD~` 退出码 0。
- [ ] 1.2 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: 全量 lint。运行 `golangci-lint run`。

## 2. Taskfile 补齐

- [ ] 2.1 Owner: `cli/pinax`; Lane: B; Depends on: 0.1; Scope: 补齐任务。新增 `deps`、`mod-check`、`fmt`（golangci-lint fmt）、`fmt-check`（golangci-lint fmt --diff）、`lint`、`run`；Acceptance: `task --list` 显示新任务。
- [ ] 2.2 Owner: `cli/pinax`; Lane: B; Depends on: 2.1; Scope: 热加载不适用说明。在 `AGENTS.md` 或 design 中说明 Pinax 是 CLI-only 项目，不新增 `.air.toml`。

## 3. 文档更新

- [ ] 3.1 Owner: `cli/pinax`; Lane: C; Depends on: 2.2; Scope: `AGENTS.md`。更新质量门禁命令。

## 4. 验证

- [ ] 4.1 Owner: `cli/pinax`; Lane: D; Depends on: 3.1; Scope: 全量验证。运行 `task fmt-check`、`task lint`、`task test`、`task build`。
