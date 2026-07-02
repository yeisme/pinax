# 任务

- [x] 1. Owner: `cli/pinax`; Lane: spec; Depends on: none; Scope: OpenSpec proposal/design/tasks/spec delta。Acceptance: `openspec validate pinax-workbench-activity-logs --strict` 通过；失败时先检查 spec scenario 和 Mermaid 语法。
  - Evidence: 2026-06-27 运行 `openspec validate pinax-workbench-activity-logs --strict`，退出码 0。
- [x] 2. Owner: `cli/pinax`; Lane: app; Depends on: 1; Scope: 新增 Activity service 和归一化 reader。Acceptance: service tests 覆盖五个来源、过滤、排序、partial warning、redaction、show not found；`go test ./internal/app -run 'Activity' -count=1` 通过。
  - Evidence: 2026-06-27 运行 `go test ./internal/app ./internal/api ./internal/cli ./cmd/pinax -run 'Activity|LocalRESTRoutesMatchRegistry|LocalRPCWorkbenchActivity|LocalRPCRoutesMatchRegistry|Remote|Route' -count=1`，退出码 0。
- [x] 3. Owner: `cli/pinax`; Lane: cli; Depends on: 2; Scope: 新增 `pinax activity sources|list|show|tail|manage`。Acceptance: CLI contract tests 覆盖 default、`--json`、`--agent`、`--events`、`--explain`；`go test ./cmd/pinax -run 'Activity|OutputContract' -count=1` 通过。
  - Evidence: 2026-06-27 运行 `go test ./internal/app ./internal/api ./internal/cli ./cmd/pinax -run 'Activity|LocalRESTRoutesMatchRegistry|LocalRPCWorkbenchActivity|LocalRPCRoutesMatchRegistry|Remote|Route' -count=1`，退出码 0。
- [x] 4. Owner: `cli/pinax`; Lane: api; Depends on: 2; Scope: 新增 REST/RPC/capability 和 remote mapper。Acceptance: API/RPC tests 覆盖 list/show/capability/routes；`go test ./internal/api ./internal/app ./cmd/pinax -run 'Activity|Remote|Route' -count=1` 通过。
  - Evidence: 2026-06-27 运行 `go test ./internal/app ./internal/api ./internal/cli ./cmd/pinax -run 'Activity|LocalRESTRoutesMatchRegistry|LocalRPCWorkbenchActivity|LocalRPCRoutesMatchRegistry|Remote|Route' -count=1`，退出码 0。
- [x] 5. Owner: `cli/pinax`; Lane: verify; Depends on: 2,3,4; Scope: 全量验证和收口。Acceptance: `task check` 通过；若环境缺少 `task`，运行 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all --strict` 并记录结果。
  - Evidence: 2026-06-27 运行 `task check`，退出码 0，覆盖 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、sidecar protocol tests 和 `openspec validate --all`。
