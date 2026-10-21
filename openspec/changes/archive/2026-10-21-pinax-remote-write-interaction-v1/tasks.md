# 任务

- [x] 冻结远程写入范围、owner、兼容和 MCP 只读边界。
- [x] 在 transport catalog、legacy remote projection、binding、OpenAPI 和 SDK 中增加统一 interaction metadata。
- [x] 增加 `pkg/pinaxclient.InboxCapture` 与 `pkg/pinaxclient.FolderRename`，保留原 operation identity 和 revision。
- [x] 增加 manifest metadata、typed remote write、OpenAPI parity 测试。
- [x] 增加 `remote-write-parity` integration evidence profile。
- [x] 更新远程 API、MCP、CLI parity、README 中文文档并完成链接审阅。
- [x] 运行最终 `task check`、`openspec validate --all`，记录完整 evidence 和残余风险。`task check`、`openspec validate --all` 与 `go run ./tools/testkit/integrationevidence --profile remote-write-parity` 通过；未运行 race、未部署、未执行真实远程写入。
