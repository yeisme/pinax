## 1. 合同与测试夹具基线

- [x] 1.1 定义 Pinax Cloud HTTP contract fixture，覆盖 auth/device、current revision、blob batch-check、blob upload/download、revision commit 成功、revision conflict、unauthorized 和 backend unavailable。证据：`internal/cloudclient/testdata/cloud_contract.json`；`go test ./internal/cloudclient -count=1` 和 `task check` 通过。
- [x] 1.2 在 `internal/cloudclient` 增加 RED tests，验证 request headers、request id/idempotency key、错误码解码和 token/header 脱敏。证据：`internal/cloudclient/client_test.go`、`transport_test.go`；`task check` 通过。
- [x] 1.3 在 backend owner 仓库建立相同 contract fixture，并验证 Cloud handler 返回的成功/失败 shape 与 CLI fake server 一致。证据：`backend-server/pinax-cloud/internal/httpapi/testdata/cloud_contract.json` 与 CLI fixture 对齐；`go test ./internal/httpapi -count=1`、`task check` 通过。
- [x] 1.4 记录合同验证命令证据。证据：Pinax `task check` 通过；backend owner `task check` 通过。

验证记录：2026-06-12 Pinax 运行 `task check`，结果 OpenSpec 36 passed、lint 0 issues、`go test ./...` 和 build 通过；backend owner `backend-server/pinax-cloud` 运行 `task check`，结果 deps/mod-check/fmt-check/lint/test/build 通过。

## 2. 协议核心与 transport 抽象

- [x] 2.1 新增 `internal/cloudsync` 协议包，定义 `Envelope`、`Manifest`、`Head`、`Revision`、`CommitRequest`、`CommitResult`、`Conflict` 和稳定错误码。证据：`internal/cloudsync/protocol.go`、`protocol_test.go`；`go test ./internal/cloudsync -count=1` 与 `task check` 通过。
- [x] 2.2 定义 `cloudsync.Transport` interface：`CurrentHead`、`BatchCheck`、`PutBlob`、`GetBlob`、`PutManifest`、`GetManifest`、`CommitRevision`。证据：`TestMemoryTransportCommitAndConflict`、`TestObjectStoreTransportCommitsHeadWithCAS`、`task check` 通过。
- [x] 2.3 把 `internal/cloudclient` 适配成 server transport 实现。证据：`internal/cloudclient/transport.go`、`TestServerTransportMapsCloudsyncOperations`；`task check` 通过。
- [x] 2.4 设计并测试 object key layout helper：`protocol.json`、`head.json`、`revisions/`、`manifests/sha256/`、`blobs/sha256/`、`locks/commit.lock`。证据：`TestObjectKeysNeverContainPlaintextPath` 通过。

## 3. S3 direct backend

- [x] 3.1 新增 `pinax cloud backend set s3` 命令，写入 CLI-authored cloud state，保存 bucket/region/prefix/endpoint/profile/secret-ref，不保存 access key 或 secret key。证据：`TestCloudBackendSetS3CLI`、`TestCloudStateWritesStructuredYAMLS3Config`、`task check` 通过。
- [x] 3.2 实现 S3 direct transport，复用 `internal/remote.S3Backend` 和 `cloudsync.ObjectStoreTransport`。证据：`TestObjectStoreTransportCommitsHeadWithCAS`、`task check` 通过；不依赖真实公网。
- [x] 3.3 完成 S3/direct head CAS 与 lock fallback 验收。证据：`internal/cloudsync/object_store_test.go` 覆盖首次建头、同 base revision 并发更新、无 conditional write 的 lock fallback、lock held/recovery；`go test ./internal/cloudsync -count=1` 与 `task check` 通过。
- [x] 3.4 更新 `cloud doctor`，明确 direct S3 的权限边界是 provider credential，不具备 Pinax Cloud Server auth/audit/multi-tenant policy。证据：`TestCloudBackendSetS3CLI` 验证 `auth_boundary=provider_credentials`、`server_audit=false`。

## 4. Rclone / OneDrive direct backend

- [x] 4.1 新增 rclone transport adapter，支持 `rclone://<remote>/<prefix>` endpoint。证据：`internal/remote/rclone_backend.go`、`rclone_backend_test.go` 覆盖 `cat`、`copyto`、`lsf`、missing object、command failure、timeout/cancellation 和 stderr 脱敏；`task check` 通过。
- [x] 4.2 新增 `pinax cloud backend set rclone --remote onedrive:PinaxSync` 命令。证据：`TestCloudBackendSetRcloneCLI` 通过，输出 `backend_kind=rclone-direct` 且不泄漏 `refresh_token`/`Authorization`/`client_secret`。
- [x] 4.3 用 lock object 实现 rclone/OneDrive commit protection。证据：`ObjectStoreTransport` 在无 conditional write capability 时使用 TTL `locks/commit.lock`；lock expiry/recovery tests 通过，uncertain write state 不输出 `remote_write=true`。
- [x] 4.4 文档明确 native OneDrive Microsoft Graph adapter 不属于 MVP。证据：`docs/commands/cloud.md`、`docs/commands/sync.md`、`docs/architecture/cloud-sync-design.md` 只展示 rclone OneDrive 示例；`task check` 通过。

## 5. Pinax Cloud server backend

- [x] 5.1 实现 auth/device session：bootstrap/login、device id、workspace scope、token/session 校验、稳定 `unauthorized` / `insufficient_scope` 错误。证据：backend `internal/httpapi/server_sync_test.go`；backend `task check` 通过。
- [x] 5.2 实现 encrypted blob batch-check、upload、download；禁止服务端日志、审计、fixture 输出 plaintext note body、raw path、Authorization、Cookie 或 token。证据：backend HTTP tests 与 redaction assertions；backend `task check` 通过。
- [x] 5.3 实现 vault current revision read，返回 revision id、encrypted manifest reference 和必要的同步元数据。证据：backend `current_revision` handler tests；backend `task check` 通过。
- [x] 5.4 实现 revision CAS commit：base revision 匹配时原子创建新 revision，失配时返回稳定 revision conflict，重复 request id/idempotency key 不重复提交。证据：backend commit/idempotency/conflict tests；backend `task check` 通过。
- [x] 5.5 实现 backend audit/health/readiness：记录 workspace、device、operation、revision、status、duration、error_code，保持 payload 脱敏。证据：backend audit/health tests；backend `task check` 通过。
- [x] 5.6 增加 backend 并发测试：两个设备同时提交同一 base revision 时只允许一个成功，另一个得到 revision conflict。证据：`go test -race ./internal/httpapi -run TestTwoDeviceConcurrentCommitRejectsOneWriter -count=1` 通过；backend `task check` 通过。

## 6. CLI sync engine 与本地状态

- [x] 6.1 将 cloud state/session 与 sync-state receipt 连接：保存 backend kind、last synced revision、device id、workspace、vault id、manifest reference 和 updated_at。证据：`TestSyncCloudPlannerCLI`、`TestSyncRunReceiptsLogsStatusAndRedactionCLI`；`task check` 通过。
- [x] 6.2 在 RED test 中证明 `sync push --target cloud --yes` 只有 transport commit 成功后才能输出 `remote_write=true`。证据：server fake、embedded/file、rclone tests 均验证 durable commit gate；`task check` 通过。
- [x] 6.3 实现 push orchestration：scan local vault、build manifest、encrypt missing blobs、batch-check、upload missing blobs、CAS commit revision、write sync-state receipt、append redacted event。证据：server、embedded/file、S3-compatible object-store、rclone fake tests；`task check` 通过。
- [x] 6.4 实现 pull orchestration：读取 current head、下载 encrypted manifest、计算本地/远端差异、下载 missing blobs、本地解密应用、写 sync-state receipt。证据：`TestDirectCloudPushPullCLI`、`TestSyncConflictNextActionsAppearInSyncJSONAndAgentOutputsCLI`、`TestCloud` e2e；`task check` 通过。
- [x] 6.5 实现 backend unavailable、unauthorized、blob upload failure、revision conflict、lock timeout 的结构化错误和 next action，确保失败不改写本地 Markdown。证据：`commandErrorFromError` 映射、backend HTTP error contract tests、CLI sync failure tests；`task check` 通过。

## 7. Pull 与冲突处理路径

- [x] 7.1 在 RED test 中构造远端 trunk 更新、本地未变更路径，证明 pull 会下载、解密并更新本地 note 与 sync-state revision。证据：`TestDirectCloudPushPullCLI`、`TestCloud/cloud_direct_two_device` 通过。
- [x] 7.2 在 RED test 中构造同一路径本地和远端都变化，证明 pull 会保留同目录 `.conflict.md` 副本并写入远端 trunk。证据：`TestDirectCloudPushPullCLI`、`TestSyncConflictNextActionsAppearInSyncJSONAndAgentOutputsCLI` 通过。
- [x] 7.3 复用并补强 `pinax sync conflicts list/diff/show/resolve`，确保冲突 next action 可被人和 agent 消费。证据：`internal/app/sync_conflicts.go`、`internal/cli/sync_conflicts_cmd.go`、`cmd/pinax/main_test.go`；`task check` 通过。
- [x] 7.4 验证 pull/conflict tests。证据：`go test ./cmd/pinax ./internal/app ./internal/cloudsync ./internal/cloudclient -run 'Cloud|Sync|Transport|ObjectStore|Direct|Conflict' -count=1` 通过；`task check` 通过。

## 8. Local RPC / Go API entrypoints

- [x] 8.1 增加 Go API entrypoint，使本地 agent/desktop/MCP bridge 可调用同一个 cloud sync service。证据：`app.Service` 暴露 `SyncPush`/`SyncPull`/`SyncStatus`/`SyncLogs*`/`SyncConflicts*`，docs 中记录 API 入口；`task check` 通过。
- [x] 8.2 增加 local RPC method，例如 `pinax.sync.push` / `pinax.sync.pull`，复用 CLI app service。证据：`TestLocalRPCSyncPushPullUsesWriteGateAndService`、`TestLocalRPCRoutesMatchRegistry` 通过。
- [x] 8.3 确认 `pinax api serve` 的中心化 vault 访问不被文档描述为 Cloud Sync transport。证据：`docs/commands/cloud.md`、`docs/architecture/cloud-sync-design.md` 明确区分 Local API 与 Cloud Sync Protocol；`task check` 通过。

## 9. 双设备 E2E 与回归门禁

- [x] 9.1 增加 testscript 或 Go e2e：两个临时 vault、两个 device id、一个 fake/local transport，执行 A push -> B pull 后两端收敛。证据：`TestDirectCloudPushPullCLI`、`TestCloud/cloud_direct_two_device` 通过。
- [x] 9.2 增加 S3 direct E2E：fake/minimal S3 或 local object adapter 验证 A push -> B pull -> concurrent conflict。证据：同一 object-store transport 通过 `FileBackend`/S3 `BlobStore` 接口复用，`ObjectStoreTransport` CAS/concurrency tests 与 direct e2e 通过。
- [x] 9.3 增加 rclone/OneDrive fake E2E：fake rclone executable 验证 lock-object conflict 和 recovery。证据：`internal/remote/rclone_backend_test.go`、`internal/cloudsync/object_store_test.go`、`TestCloud` rclone failure path；`task check` 通过。
- [x] 9.4 增加 server transport E2E：fake/local Pinax Cloud backend 验证 HTTP contract 与 CLI sync engine 收敛。证据：`internal/remote.NewFakeServer` 支持 Cloud HTTP contract；`TestCloud/cloud_sync` 与 `TestSyncCloudPlannerCLI` 通过。
- [x] 9.5 增加红线扫描：stdout、stderr、events、audit、fixtures、backend logs、object metadata 中不得出现 plaintext note body、raw token、Authorization、Cookie、secret-ref 原文或 provider payload。证据：`TestSyncRunReceiptsLogsStatusAndRedactionCLI`、`TestSyncOfflineAndRedaction`、backend audit tests；`task check` 通过。
- [x] 9.6 运行 Cloud sync focused e2e。证据：`go test ./cmd/pinax ./internal/app ./internal/output ./internal/remote ./internal/cloudsync ./internal/cloudclient ./tests/e2e -run 'Cloud|Sync|TwoDevice|Conflict|Direct|Rclone|Transport|ObjectStore|Agent|Render' -count=1` 通过；`task check` 通过。

## 10. 文档和发布准入

- [x] 10.1 更新 `docs/commands/cloud.md`、`docs/commands/sync.md` 和 `docs/architecture/cloud-sync-design.md`，说明 server、s3-direct、rclone-direct 和 embedded transport 的适用边界。证据：相关文档已更新；`task check` 通过。
- [x] 10.2 更新 `README.md` 和 command examples，只在真实 transport revision commit 成功后展示 `remote_write=true` 示例。证据：`README.md` 已更新；`task check` 通过。
- [x] 10.3 更新 `openspec/specs/pinax-cloud-sync/spec.md` 并归档本 change。证据：spec 与 change spec 已更新；归档前验证 `openspec validate pinax-cloud-distributed-sync` 与 `task check` 通过。
- [x] 10.4 最终检查 no generated dist/coverage/local vault/provider cache/secrets，并记录证据。证据：`task check` 生成的 `dist/pinax` 为本地构建产物且不作为提交对象；红线扫描测试通过；未新增 provider cache/secrets fixtures。

## 11. Review 后新增发布阻断任务

- [x] 11.1 移除或收敛 no-op transport：`http`、`https`、`rclone` 不得再通过 `dummyStore` 静默成功。证据：`TestUnimplementedTransportsDoNotReturnNoopStores`、rclone real adapter tests、server transport e2e；`task check` 通过。
- [x] 11.2 将 server transport 接入同一 Cloud Sync engine。证据：`cloudTransportForState` 使用 `internal/cloudclient.Transport`，`TestSyncCloudPlannerCLI` 和 `TestCloud/cloud_sync` 验证 server commit 后 `remote_write=true`；`task check` 通过。
- [x] 11.3 实现 rclone direct adapter。证据：`internal/remote/rclone_backend.go`、`rclone_backend_test.go`；`task check` 通过。
- [x] 11.4 将 `sync conflicts` 重构为 app service + Projection。证据：`internal/app/sync_conflicts.go`、`internal/cli/sync_conflicts_cmd.go`；search 验证 Cobra 层无直接 `os.Rename`/`os.Remove`/`fmt.Println` resolution；`task check` 通过。
- [x] 11.5 在 pull/conflict projection 中输出可执行 next action。证据：`TestSyncConflictNextActionsAppearInSyncJSONAndAgentOutputsCLI` 通过。
- [x] 11.6 增加 Cloud Sync 红线脱敏扫描门禁。证据：`TestSyncRunReceiptsLogsStatusAndRedactionCLI`、`TestSyncOfflineAndRedaction`、backend audit tests；`task check` 通过。
- [x] 11.7 拆分非 Cloud Sync 的 CLI contract drift。证据：Cloud docs/spec 明确将 `note links --broken-only`、`note backlinks --include-broken`、`note orphans --mode`、root help `Other` 分组、`docs/commands/README.md` 命令地图等剔除出本 change 完成定义，并建议后续 owner change `pinax-cli-contract-drift`。

## 12. 推荐执行顺序

- [x] 12.1 P0：先处理 11.1，消除 `dummyStore` 假成功风险。证据：`TestUnimplementedTransportsDoNotReturnNoopStores` 通过。
- [x] 12.2 P0：完成 11.4 和 11.5，保证冲突队列可被人和 agent 安全消费。证据：冲突 projection tests 与 `task check` 通过。
- [x] 12.3 P0：完成 3.3 与 11.6，补齐 CAS 并发和脱敏红线门禁。证据：CAS/lock tests、redaction tests、`task check` 通过。
- [x] 12.4 P1：完成 11.2 server transport E2E。证据：`TestCloud/cloud_sync` 与 `TestSyncCloudPlannerCLI` 通过。
- [x] 12.5 P1：完成 11.3 rclone direct E2E。证据：fake rclone tests 与 rclone e2e guarded failure path 通过。
- [x] 12.6 P1：完成 9.2-9.6、10.1-10.4 后归档本 change。证据：`task check`、backend `task check`、focused Cloud e2e 通过；OpenSpec validate 通过。

## 13. Sync logs / observability

- [x] 13.1 定义 sync run receipt schema `pinax.sync_run.v1`。证据：`internal/app/sync_runs.go`、`TestSyncRunReceiptsLogsStatusAndRedactionCLI`、`TestSyncRunReceiptsCoverPartialFailedApprovalAndPruneCLI`；`task check` 通过。
- [x] 13.2 在 `sync diff/push/pull/all` 中生成 run receipt。证据：success、failed、approval_required、dry-run、conflict 路径 tests 通过；`task check` 通过。
- [x] 13.3 收敛 `.pinax/sync-state.json` 为 current-state 文件。证据：`writeCurrentSyncState` 只保存 current state 与 `last_sync_run_id`；`TestSyncCloudPlannerCLI` 验证不保留历史 `remote_write` 字段；`task check` 通过。
- [x] 13.4 将 `.pinax/events.jsonl` 降为 sync summary timeline。证据：`appendSyncRunEvent` 只写 run summary facts；redaction tests 通过。
- [x] 13.5 新增 `pinax sync logs list/show/tail/prune`。证据：`internal/app/sync_runs.go`、`internal/cli/sync_cmd.go`、CLI output mode tests；`task check` 通过。
- [x] 13.6 实现路径脱敏策略：默认、hash、omitted。证据：`TestSyncRunPathRedactionPoliciesCLI` 通过。
- [x] 13.7 实现数量加时间 retention。证据：`SyncLogsPrune` 和 prune preview/apply tests 通过，`sync-state.json` 不被删除。
- [x] 13.8 更新 `sync status`：读取 sync-state、最近 run receipt 和 remote head，区分 configured、stale、conflicted、last_failed、transport_unavailable 等状态。证据：`internal/app/service_sync_remote.go`、status CLI tests；`task check` 通过。
- [x] 13.9 增加日志红线扫描门禁。证据：sync receipts、events、stdout/stderr、fixtures、object metadata、backend audit/log tests 均覆盖禁止 token/header/body/provider payload 泄漏；`task check` 通过。
