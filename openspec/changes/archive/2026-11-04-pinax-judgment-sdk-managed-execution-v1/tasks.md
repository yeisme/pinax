# 实施与验收任务

由 scripts/openspec-tasks.py 维护状态。

- [x] 1.1 按本 owner 边界实现 internal/inboxjudgment 的公共合同接线：普通构建引用真实 SDK、宿主 config 到 gate、GORM 持久回执与 CLI/REST/RPC 入口。 | evidence: 实现于 internal/inboxjudgment/judgment_sdk_bridge.go（ReceiptStore + NewManagedJudgmentSDKBridge）、internal/app/inbox_review.go（gate/profile/入口）、internal/cli/inbox_review_cmd.go、internal/api/{http,rpc,middleware}.go 与 internal/app/remote.go；go build ./... 与 go vet ./... 通过。
- [x] 1.2 验证默认关闭、scope/digest、原操作恢复与兼容行为 | evidence: go test -count=1 ./internal/inboxjudgment ./internal/app ./internal/api ./internal/cli ./cmd/pinax 通过：默认 off 零装配零凭据、source/owner scope 复核、未知结果重启后仍粘滞不重提、dry-run 零 provider 调用、OpenAPI 加性 golden 可回退到原 digest。
- [x] 1.3 记录 owner 集成证据、同步文档并完成受影响规格验证 | evidence: owner 集成证据 temp/integration-test-runs/20261104T164727Z-1356741（profile judgment-managed：31 个用例，含 CLI/REST/RPC 入口与回执持久化；fixture only，provider_calls=0）；docs/inbox-judgment.md 与 docs/commands/inbox.md 已同步；task check 退出 0（OpenSpec 114/114、lint 0 issues、Go 全量、build）；未运行 race。
