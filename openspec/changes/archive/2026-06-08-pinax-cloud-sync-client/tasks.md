## Phase 1: Cloud CLI State and Fake Server

- [x] P1.1 Owner: `cli/pinax`; Lane: A; Depends on: none; Scope: cloud state。实现 `internal/cloud` 包：cloud config、device session、secret ref、`pinax cloud login/status/logout/doctor`；Acceptance: `go test ./internal/cloud ./cmd/pinax -run CloudState -count=1` 通过。
  - Evidence: 2026-06-08 先新增 `internal/cloud/state_test.go` 和 `cmd/pinax/main_test.go` 的 `TestCloudStateCLI`，运行 `go test ./internal/cloud ./cmd/pinax -run CloudState -count=1`，退出码 1，失败于缺少 `internal/cloud` 的 `Login/Load/Logout/Doctor` 和 CLI helper，确认测试覆盖 cloud state 合同。随后新增 `internal/cloud` state 层（`.pinax/cloud/config.json`、`.pinax/cloud/session.json`，只保存 secret ref 不输出 raw token）、app service `CloudLogin/CloudStatus/CloudLogout/CloudDoctor` 和 `pinax cloud login/status/logout/doctor` 命令；重跑同一验收命令，退出码 0。
- [x] P1.2 Owner: `cli/pinax`; Lane: A; Depends on: P1.1; Scope: fake HTTP server。实现 fake pinax-cloud backend server 用于本地开发和测试；Acceptance: `go test ./internal/cloud -run FakeServer -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/cloud/fake_server_test.go`，先运行 `go test ./internal/cloud -run FakeServer -count=1`，退出码 1，失败于缺少 `NewFakeServer`、manifest/blob response 类型和 conflict error 类型。实现 `internal/cloud/fake_server.go`，提供 `/health`、`/v1/workspaces/<id>/manifest` GET/POST、`/v1/workspaces/<id>/blobs/<id>` PUT/GET，manifest POST 使用 `base_revision` 做 `REVISION_CONFLICT` 检测；重跑同一验收命令，退出码 0。

## Phase 2: Manifest and Client Crypto

- [x] P2.1 Owner: `cli/pinax`; Lane: B; Depends on: P1.2; Scope: manifest builder。实现 manifest schema、path hash、local blob cache；Acceptance: `go test ./internal/cloud -run Manifest -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/cloud/manifest_test.go`，先运行 `go test ./internal/cloud -run Manifest -count=1`，退出码 1，失败于缺少 `BuildManifest`、`ManifestSchemaVersion` 和 `PathHash`。实现 `internal/cloud/manifest.go`：只扫描 vault 内 Markdown、跳过 `.pinax/.git/dist/temp`，manifest 条目使用 `path_` hash 和 `blob_` id，不包含原始路径或正文，并写入 `.pinax/cloud/blob-cache/<blob_id>`；重跑同一验收命令，退出码 0。
- [x] P2.2 Owner: `cli/pinax`; Lane: B; Depends on: P2.1; Scope: client crypto。实现 encrypted blob envelope、redaction；Acceptance: `go test ./internal/cloud ./internal/redaction -run Crypto -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/cloud/crypto_test.go` 和 `internal/redaction/cloud_test.go`，先运行 `go test ./internal/cloud ./internal/redaction -run Crypto -count=1`，退出码 1，失败于缺少 `DeriveKey`、`EncryptBlob`、`DecryptBlob`、`EncryptManifest`、`DecryptManifest` 和 `redaction.Cloud`。实现 AES-256-GCM `EncryptedEnvelope`、manifest/blob 本地加解密和 cloud redaction helper，envelope JSON 不包含正文、路径、token 或 secret ref；重跑同一验收命令，退出码 0。

## Phase 3: Sync Planner and Commands

- [x] P3.1 Owner: `cli/pinax`; Lane: C; Depends on: P2.2; Scope: sync planner。实现 `sync diff/pull/push` plan、base revision、dry-run/yes、conflict queue；Acceptance: `go test ./internal/sync ./internal/cloud -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/sync/planner_test.go`，先运行 `go test ./internal/sync ./internal/cloud -count=1`，退出码 1，失败于缺少 `BuildPlan`、`Request`、`DirectionPush` 和 `ErrRevisionConflict`。实现 `internal/sync/planner.go`（package `syncplan`），支持 diff/pull/push plan、base/remote revision、dry-run/yes、remote_write、requires_approval 和 `REVISION_CONFLICT` conflict queue；重跑同一验收命令，退出码 0。
- [x] P3.2 Owner: `cli/pinax`; Lane: C; Depends on: P3.1; Scope: CLI commands。实现 `pinax sync diff/pull/push` 命令，支持 `--dry-run`、`--yes`、`--json`；Acceptance: `go test ./cmd/pinax -run Sync -count=1` 通过。
  - Evidence: 2026-06-08 新增 `cmd/pinax/main_test.go` 的 `TestSyncCloudPlannerCLI`，先运行 `go test ./cmd/pinax -run Sync -count=1`，退出码 1，失败于 `sync diff` 缺少 `--dry-run` flag。随后扩展 `app.SyncRequest` 和 `SyncDiff/SyncPush/SyncPull` cloud 分支，接入 `internal/cloud.BuildManifest` 与 `internal/sync.BuildPlan`，新增 `--dry-run`、`--base-revision`、`--remote-revision` flags，冲突返回 `REVISION_CONFLICT` error envelope；重跑同一验收命令，退出码 0。

## Phase 4: Output Contract and E2E

- [x] P4.1 Owner: `cli/pinax`; Lane: D; Depends on: P3.2; Scope: output contract。默认中文摘要、`--agent`、`--json`、`--events`、`--explain` 同源 projection；Acceptance: `go test ./internal/output ./cmd/pinax -run Cloud -count=1` 通过。
  - Evidence: 2026-06-08 新增 `cmd/pinax/main_test.go` 的 `TestCloudOutputContractModes`，覆盖 `cloud status --agent`、`cloud doctor --events`、`sync push --dry-run --explain` 和冲突 `sync push --json`，断言 envelope/event/agent key 稳定、中文 explain 含结论/证据、机器输出不泄漏 secret ref、token 或 note body。运行 `go test ./internal/output ./cmd/pinax -run Cloud -count=1`，退出码 0。
- [x] P4.2 Owner: `cli/pinax`; Lane: sequential; Depends on: P4.1; Scope: e2e。fake server + temp vault + testscript，覆盖 dry-run/yes/json/conflict；Acceptance: `go test ./tests/e2e -run Cloud -count=1` 通过。
  - Evidence: 2026-06-08 新增 `tests/e2e/cloud_sync_test.go` 和 `tests/e2e/testdata/cloud/scripts/cloud_sync.txt`。`TestCloud` 构建当前 `cmd/pinax` 到临时 PATH，启动 `internal/cloud.NewFakeServer()` 并通过 `PINAX_FAKE_CLOUD_URL` 注入脚本；脚本覆盖 cloud status 未配置错误、login/status/doctor 输出、sync diff/push/pull dry-run、push --yes、revision conflict，并断言不输出 `cloud-token`、Authorization、note body 或 raw path。运行 `go test ./tests/e2e -run Cloud -count=1`，退出码 0。
