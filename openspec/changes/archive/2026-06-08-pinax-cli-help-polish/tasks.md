## 1. Help 合同测试

- [x] 1.1 增加 root help contract test，验证 `pinax --help` 显示工作流分组、主入口和全局参数。
- [x] 1.2 增加 alias 隐藏测试，验证 root help 不显示 `stats/validate/doctor/dashboard/tag/folder/kind/group`。
- [x] 1.3 增加子命令 alias 隐藏测试，验证 `storage --help` 不显示 `set-local/set-s3`，`organize --help` 不显示 `suggest`。
- [x] 1.4 保留兼容行为测试，验证旧 alias 与主路径的 `--json` command/facts 等价。

## 2. Help 分组实现

- [x] 2.1 在 command 层增加 root help 分组 annotation 或等价元数据，不进入 app service 或 output renderer。
- [x] 2.2 更新 root help template，让 `pinax --help` 按工作流分组展示可见主命令。
- [x] 2.3 将 vault root alias、dimension root alias、storage direct set alias、organize suggest 标记为隐藏兼容入口。
- [x] 2.4 更新 help example 和错误 next action，优先推荐 `vault`、`note`、`storage set`、`organize plan --save` 主路径。

## 3. 文档与规格同步

- [x] 3.1 更新 README/docs 中的示例路径，避免继续把兼容 alias 当主路径推荐。
- [x] 3.2 确认本 change 的 delta specs 与实现一致，并记录验证证据。

## 4. 验证

- [x] 4.1 运行聚焦测试：`go test ./cmd/pinax ./internal/cli -run 'Help|CLITree|PrimaryPathAliases|OutputContract|Organize' -count=1`。
- [x] 4.2 运行 `openspec validate pinax-cli-help-polish` 和 `openspec validate --all`。
- [x] 4.3 运行 `task check`；如失败，记录与本变更相关性和阻塞原因。

## Verification Evidence

- 2026-06-08: `go test ./cmd/pinax ./internal/cli -run 'Help|CLITree|PrimaryPathAliases|OutputContract|Organize' -count=1` 通过。
- 2026-06-08: `openspec validate pinax-cli-help-polish` 通过。
- 2026-06-08: `openspec validate --all` 通过，20 items passed。
- 2026-06-08: `task check` 通过，覆盖 fmt-check、lint、test、build 和 openspec validate。
