## Phase 1: Cloud CLI State and Fake Server

- [ ] P1.1 Owner: `cli/pinax`; Lane: A; Depends on: none; Scope: cloud state。实现 `internal/cloud` 包：cloud config、device session、secret ref、`pinax cloud login/status/logout/doctor`；Acceptance: `go test ./internal/cloud ./cmd/pinax -run CloudState -count=1` 通过。
- [ ] P1.2 Owner: `cli/pinax`; Lane: A; Depends on: P1.1; Scope: fake HTTP server。实现 fake pinax-cloud backend server 用于本地开发和测试；Acceptance: `go test ./internal/cloud -run FakeServer -count=1` 通过。

## Phase 2: Manifest and Client Crypto

- [ ] P2.1 Owner: `cli/pinax`; Lane: B; Depends on: P1.2; Scope: manifest builder。实现 manifest schema、path hash、local blob cache；Acceptance: `go test ./internal/cloud -run Manifest -count=1` 通过。
- [ ] P2.2 Owner: `cli/pinax`; Lane: B; Depends on: P2.1; Scope: client crypto。实现 encrypted blob envelope、redaction；Acceptance: `go test ./internal/cloud ./internal/redaction -run Crypto -count=1` 通过。

## Phase 3: Sync Planner and Commands

- [ ] P3.1 Owner: `cli/pinax`; Lane: C; Depends on: P2.2; Scope: sync planner。实现 `sync diff/pull/push` plan、base revision、dry-run/yes、conflict queue；Acceptance: `go test ./internal/sync ./internal/cloud -count=1` 通过。
- [ ] P3.2 Owner: `cli/pinax`; Lane: C; Depends on: P3.1; Scope: CLI commands。实现 `pinax sync diff/pull/push` 命令，支持 `--dry-run`、`--yes`、`--json`；Acceptance: `go test ./cmd/pinax -run Sync -count=1` 通过。

## Phase 4: Output Contract and E2E

- [ ] P4.1 Owner: `cli/pinax`; Lane: D; Depends on: P3.2; Scope: output contract。默认中文摘要、`--agent`、`--json`、`--events`、`--explain` 同源 projection；Acceptance: `go test ./internal/output ./cmd/pinax -run Cloud -count=1` 通过。
- [ ] P4.2 Owner: `cli/pinax`; Lane: sequential; Depends on: P4.1; Scope: e2e。fake server + temp vault + testscript，覆盖 dry-run/yes/json/conflict；Acceptance: `go test ./tests/e2e -run Cloud -count=1` 通过。
