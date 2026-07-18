## 1. 文件级同步事件

- [x] 1.1 为最终 sync receipt 的脱敏 operations 追加 `sync.file` 时间线事件
- [x] 1.2 让 sync event reader 返回 run/file 类型、稳定序号和安全字段
- [x] 1.3 覆盖 default/hash/omitted 路径策略与敏感内容禁止泄漏

## 2. 实时日志跟随

- [x] 2.1 为应用层增加可取消的同步事件 follow 能力
- [x] 2.2 为 `sync logs tail` 增加 `--follow` 和 JSON/explain 模式保护
- [x] 2.3 为 summary、agent、events 实现逐事件流式渲染

## 3. 输出与文档

- [x] 3.1 更新同步日志 human/agent 渲染以显示操作类型和文件路径
- [x] 3.2 更新 `docs/commands/sync.md` 的日志跟随示例与脱敏说明

## 4. 验证

- [x] 4.1 运行聚焦 app、output 和 CLI contract tests
- [x] 4.2 运行 `task check`
- [x] 4.3 记录验证证据并执行 `openspec validate --all`

## 验证证据

- 聚焦 TDD：`go test ./internal/app ./internal/output ./internal/cli ./cmd/pinax -run 'TestSync|TestSyncLogFollowEmitterRendersSafeStreamingModes' -count=1` 通过。
- Race：`go test -race ./internal/app -run 'TestSyncLogsListAndTailIncludeLimitFacts|TestSyncRunTimelineIncludesSanitizedFileOperations|TestSyncLogsFollowEmitsInitialAndAppendedEvents' -count=1` 通过。
- 输出合同：同步日志 summary、agent、JSON、events、follow guard 与 path policy 聚焦测试通过。
- 结构检查：`codegraph build`、`codegraph diff-impact`、`codegraph check` 完成；无 graph/boundary failure，仓库已有复杂度 warning 保持不变。
- 规范：`openspec validate --all` 为 59 passed、0 failed。
- 全门禁：`task check` PASS（exit 0，60 passed / 0 failed）。先前阻断的 E2E `/no-home` 配置失败已修复——`EnsureStoredSecret` 写入 `$HOME/.config/pinax`，而 testscript 默认 `HOME=/no-home` 不可写；按仓库既有模式（`config_rendering_test.go`）为 TestCloud、TestSyncDaemon、TestSyncOfflineAndRedaction 注入 per-script 可写 `XDG_CONFIG_HOME=$WorkDir/xdg`，使生产自动生成加密密钥路径在测试环境正常工作。fmt、lint、build、OpenSpec、sidecar protocol 和 renderer checks 全部通过。
