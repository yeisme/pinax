## 任务

- [x] 1. 写红灯测试：全局输出 flag、project/subproject、folder、profile、backend、prompt、plugin、collection、sync conflict 和常见枚举补全。
- [x] 2. 实现共享 completion helper 和候选读取函数，保证只读、脱敏、前缀过滤和正确 shell directive。
- [x] 3. 把 helper 接到对应 Cobra 命令和 flag，保留路径类参数默认文件补全。
- [x] 4. 更新 Pinax 文档，说明补全安装入口、覆盖矩阵和边界。
- [x] 5. 运行验证并记录结果：`go test ./cmd/pinax ./internal/cli -count=1`、targeted completion smoke、`golangci-lint run ./cmd/pinax ./internal/cli`、`openspec validate --all`、`task check`。

## 验证记录

- `go test ./cmd/pinax -run 'TestHighValueCompletionCoverageCLI|TestPathLikeCompletionKeepsFileCompletionCLI' -count=1`：通过。
- `go test ./cmd/pinax ./internal/cli -count=1`：通过。
- `golangci-lint run ./cmd/pinax ./internal/cli`：通过，0 issues。
- `openspec validate --all`：通过，50 passed, 0 failed。
- `go run ./cmd/pinax __complete --color ""`：通过，返回 `auto`、`always`、`never` 且 `ShellCompDirectiveNoFileComp`。
- `go run ./cmd/pinax __complete project board show stock-trading --vault /workspaces/yeisme-agent/data/yeisme-notes --subproject ""`：通过，返回炒股 vault 的 9 个子项目候选。
- `go run ./cmd/pinax __complete --theme h`：通过，返回 `high-contrast`。
- `go run ./cmd/pinax __complete --markdown-style d`：通过，返回 `dark`。
- `task check`：已运行但未通过；失败点为既有无关 full lint 问题 `internal/index/store.go` 中 `notePathByTitle` 和 `notePathByTitleRecords` 未使用。本次补全改动的聚焦 lint/test/OpenSpec/smoke 均已通过。
