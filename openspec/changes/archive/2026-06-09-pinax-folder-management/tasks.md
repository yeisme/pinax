## 1. 行为测试

- [x] 1.1 为 `note folders rename` 增加 CLI 流程测试，覆盖 `--dry-run` 无写入、缺少 `--yes` 报错、`--yes` 后移动文件和更新 frontmatter。

## 2. 实现

- [x] 2.1 在 application service 中实现 folder rename 计划和写入流程，包含 folder 校验、目标路径计算、冲突预检、record event 和索引刷新。
- [x] 2.2 在 Cobra note folders 命令下接入 `rename <old> <new>`，保持命令层只做参数和 flag 接线。
- [x] 2.3 补充 summary fact 中文标签，保持 JSON/agent/events 输出合同稳定。

## 3. 验证

- [x] 3.1 `go test ./cmd/pinax -run TestNoteFolderBulkManagementCLI -count=1` 红灯确认缺少命令。
- [x] 3.2 `go test ./cmd/pinax -run TestNoteFolderBulkManagementCLI -count=1` 通过。
- [x] 3.3 `go test ./cmd/pinax ./internal/app ./internal/output -count=1` 通过。
- [x] 3.4 `task check` 通过，覆盖 `fmt-check`、`lint`、`go test ./...`、`build` 和 `openspec validate --all`。
