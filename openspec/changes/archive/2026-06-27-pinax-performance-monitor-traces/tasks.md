# 任务

- [x] 新增 monitor run/event 存储、资源采样、list/show/summary/manage app service。
- [x] 为 search/index/query/dataview/database view 接入步骤级 recorder。
- [x] 新增 `pinax monitor runs|show|tail|summary|manage` CLI。
- [x] 将 monitor runs 聚合为 activity source `monitor_runs`。
- [x] 补齐 `monitor show <run-id>` 和 `activity show <event-id>` 动态补全。
- [x] 新增 readonly REST/RPC/capability 和 remote CLI 映射。
- [x] 补 app、CLI、REST、RPC 测试覆盖。
- [x] 运行验证：`go test ./cmd/pinax -run TestMonitorAndActivityShowCompletionCLI -count=1`
- [x] 运行验证：`go test ./internal/app ./internal/api ./internal/cli ./cmd/pinax -run 'Monitor|Activity|Index|Search|Query|Dataview|Route|RPC' -count=1`
- [x] 运行验证：`openspec validate pinax-performance-monitor-traces --strict`
- [x] 运行验证：`task check`

## 失败复查

- 如果 monitor 测试失败，先检查 `.pinax/monitor/runs/**` run JSON 是否写入，以及 events JSONL 是否可解析。
- 如果 API/RPC registry 测试失败，检查 `RemoteCapabilities`、`RemoteRoutes`、HTTP handler、middleware route map 和 test fixture 是否同步。
- 如果 redaction 测试失败，检查是否把 raw query、note body 或敏感 fact 写入了 monitor facts/steps。
- 如果补全测试失败，先用 `pinax __complete monitor show --vault <vault> ""` 和 `pinax __complete activity show --vault <vault> ""` 复查是否读取了正确 vault，以及 completion 描述是否只包含安全字段。
