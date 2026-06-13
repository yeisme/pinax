## 1. 行为测试

- [x] 1.1 为默认 `template preview` 和 `note preview` 增加 CLI 测试，覆盖正文展示和标签事实展示。
- [x] 1.2 为默认 `note tags` / 维度列表增加 renderer 测试，覆盖数量、占比、热度条和 agent 输出边界。

## 2. 实现

- [x] 2.1 在 application service 的 preview projection 中补充标签 facts/data，保持命令层只做参数接线。
- [x] 2.2 在 summary renderer 中让 `template.preview` 和 `note.preview` 复用 Markdown body 渲染路径。
- [x] 2.3 在维度列表 summary renderer 中增加 `占比` 和纯文本 `热度` 列，并保持 JSON/agent/events 不混入人类可视化文本。

## 3. 验证

- [x] 3.1 `go test ./internal/output -run 'TestSummaryDimensionListRendersVisualShare' -count=1` 通过。
- [x] 3.2 `go test ./cmd/pinax -run 'TestPreviewSummaryShowsBodyAndTags|TestNoteDimensionPrimaryPaths' -count=1` 通过。
- [x] 3.3 `go test ./internal/app ./internal/output ./cmd/pinax -count=1` 通过。
- [x] 3.4 `task check` 通过，覆盖 `fmt-check`、`lint`、`go test ./...`、`build` 和 `openspec validate --all`。
