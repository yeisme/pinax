## 1. 行为测试

- [x] 1.1 为自定义 frontmatter 属性进入 strict property 查询增加 service 测试。
- [x] 1.2 为 `note property set/remove` 增加 CLI 流程测试，覆盖 JSON、agent、文件内容和索引刷新事实。
- [x] 1.3 为 `note tags rename/delete` 增加 CLI 流程测试，覆盖 `--dry-run`、`--yes`、文件内容和输出 facts。

## 2. 实现

- [x] 2.1 在 note 解析和 property index 中保留并投影任意非空 frontmatter 属性。
- [x] 2.2 在 application service 中实现 note property set/remove，统一处理保留字段校验、frontmatter patch、record event 和索引刷新。
- [x] 2.3 在 application service 中实现 tag taxonomy rename/delete，支持 dry-run、显式确认、批量写入、record event 和单次索引刷新。
- [x] 2.4 在 Cobra note 命令树接入 `property set/remove` 和 `tags rename/delete`，命令层只做参数接线。
- [x] 2.5 补充 summary fact 中文标签，保持 JSON/agent/events 输出合同稳定。

## 3. 验证

- [x] 3.1 `go test ./internal/app -run 'TestFrontmatterPropertiesAreSelectable|TestNoteListPropertyStrictProperties|TestTagNoteWritesRecordAndRefreshesIndexFacts' -count=1` 通过。
- [x] 3.2 `go test ./cmd/pinax -run 'TestNoteCommandUXCLI|TestNoteTagBulkManagementCLI|TestNoteListPropertyOutputContract' -count=1` 通过。
- [x] 3.3 `go test ./internal/app ./internal/index ./internal/output ./cmd/pinax -count=1` 通过。
- [x] 3.4 `task check` 通过，覆盖 `fmt-check`、`lint`、`go test ./...`、`build` 和 `openspec validate --all`。
