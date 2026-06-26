# pinax-cloud-sync Specification

## Purpose

Pinax Cloud Sync defines the local-first, distributed sync protocol for Pinax Markdown vaults. It keeps each device's vault usable offline, stores only encrypted sync artifacts outside the device, and gates `remote_write=true` on a durable revision commit rather than on plan generation or blob upload.
## Requirements
### Requirement: Local-only 模式完整可用

所有普通笔记、vault 和索引命令 SHALL 在无后端时正常工作；Cloud 命令只在用户配置后端时执行同步相关检查或写入。

#### Scenario: 无后端正常使用

- **WHEN** 用户未配置云端后端
- **THEN** 所有本地笔记、vault 和索引命令 SHALL 正常执行
- **AND** cloud/sync cloud 命令 SHALL 提示用户先配置后端
- **AND** SHALL NOT 改写本地 Markdown 文件、远端对象或 sync-state revision

### Requirement: Local API 与 Cloud Sync 语义分离

Pinax SHALL distinguish centralized Local API access from distributed Cloud Sync.

#### Scenario: Local API 访问一个中心化 vault

- **WHEN** 用户运行 `pinax api serve` 并通过 `pinax --api-url http://127.0.0.1:8787 ...` 调用命令
- **THEN** Pinax SHALL 将该请求视为访问运行中进程拥有的单一 vault
- **AND** SHALL NOT 将该工作流描述为 Cloud Sync transport

#### Scenario: Cloud Sync 每台设备拥有本地 vault

- **WHEN** 用户运行 `pinax sync push --target cloud` 或 `pinax sync pull --target cloud`
- **THEN** Pinax SHALL 使用已配置 Cloud Sync transport 交换加密 revision、manifest 和 blob
- **AND** 每台设备 SHALL 保留可离线使用的本地 Markdown vault

#### Scenario: 未忽略普通文件纳入 manifest

- **GIVEN** a vault contains `notes/a.md`, `scripts/build.sh`, and `assets/logo.png`
- **AND** `.pinaxignore` does not exclude those paths
- **WHEN** the user runs `pinax sync push --target cloud --dry-run --vault <vault> --json`
- **THEN** the sync plan SHALL include those files in the local content manifest
- **AND** protected output SHALL report counts and hashes, not file payload bytes.

#### Scenario: `.pinaxignore` 排除内容文件

- **GIVEN** `.pinaxignore` excludes `.env*` and `dist/`
- **WHEN** Pinax builds a Cloud Sync manifest
- **THEN** matching files SHALL NOT be uploaded, pulled, or recorded as content entries
- **AND** `.gitignore` SHALL NOT be used as an implicit Pinax content rule source.

#### Scenario: hard deny 路径永不同步

- **GIVEN** a vault contains `.git/`, `.pinax/index.sqlite`, `.pinax/cloud/blob-cache/`, or symlinks
- **WHEN** Pinax builds a Cloud Sync manifest
- **THEN** those paths SHALL be skipped even if `.pinaxignore` tries to re-include them.

### Requirement: Cloud Sync transport 抽象

Cloud Sync SHALL expose a transport-independent protocol so server, S3 direct, rclone direct, and embedded/local API paths share one push/pull/conflict engine.

#### Scenario: Server transport MLP contract

- **WHEN** cloud backend kind is `server` and the configured endpoint exposes the Pinax Cloud Sync MLP API
- **THEN** Pinax SHALL use the HTTP cloud client transport for auth/session facts, vault create/link, changes cursor, blob batch-check/upload planning and revision commit
- **AND** successful push apply SHALL use the shared sync engine
- **AND** successful push apply SHALL report `remote_write=true` only after durable server CAS commit plus local sync-state evidence.

#### Scenario: Server transport failure does not fallback

- **WHEN** server transport returns unauthenticated, forbidden, backend unavailable, blob missing, validation failed or revision conflict
- **THEN** Pinax SHALL return a structured failed or partial projection
- **AND** it SHALL report `remote_write=false`
- **AND** it SHALL NOT silently fallback to local-only execution, direct transport or dummy success.

### Requirement: 端侧加密保护明文

Manifest 和 blob SHALL 使用 client-side encryption；明文 SHALL NOT 离开本地设备的 explicit local content flows.

#### Scenario: 加密 manifest 和 blob

- **WHEN** 客户端上传 manifest 或 blob
- **THEN** 数据 SHALL 使用端侧加密 envelope
- **AND** 后端或 direct object store SHALL 只看到 encrypted payload 和非敏感 revision metadata

#### Scenario: 对象 key 和 metadata 不暴露路径

- **WHEN** direct transport 写入对象 key、metadata、revision 或 head
- **THEN** object key SHALL use protocol/layout ids such as `head.json`, `revisions/`, `manifests/sha256/`, and `blobs/sha256/`
- **AND** SHALL NOT contain plaintext note path, plaintext note body, raw token, Authorization header, Cookie, raw secret ref, provider stderr, or provider payload

#### Scenario: 脱敏验证

- **WHEN** 测试、stdout、stderr、event、receipt、fixture、backend log 或 object metadata 写入 Cloud Sync 相关数据
- **THEN** 不得暴露 plaintext note body、plaintext path in protected surfaces, raw token, Authorization header, Cookie, raw secret ref, provider stderr, or provider payload

### Requirement: Revision CAS 提交准入

Cloud push SHALL only report a remote write after a compare-and-swap revision commit succeeds and local sync-state evidence is written.

#### Scenario: durable commit 后 remote_write=true

- **WHEN** missing encrypted blobs and encrypted manifest have been uploaded
- **AND** transport atomically commits a new revision against the observed base revision
- **THEN** CLI MAY output `remote_write=true`
- **AND** local sync-state/run evidence SHALL include backend kind, device/workspace ids, revision id, manifest id, status, and timestamp without leaking secrets

#### Scenario: plan/blob upload 不算 remote write

- **WHEN** CLI only generated a plan, performed dry-run, uploaded blobs, uploaded a manifest, or hit an unsupported transport path
- **THEN** CLI SHALL output `remote_write=false`
- **AND** SHALL NOT create a dummy revision or claim sync success

#### Scenario: Base revision 失配时拒绝推送

- **GIVEN** 客户端基于 base revision `rev_a` 提交 commit
- **AND** transport current revision is `rev_b`
- **WHEN** transport receives the commit request
- **THEN** it SHALL reject the commit with stable `revision_conflict`
- **AND** SHALL NOT accept partial manifest or partial revision write
- **AND** CLI SHALL prompt the user to pull and resolve conflicts before retrying push

### Requirement: Sync plan 支持 dry-run 和冲突

Sync planner SHALL support dry-run mode and SHALL preserve concurrent local edits as conflict copies instead of silently overwriting user data.

#### Scenario: dry-run sync

- **WHEN** 用户运行 `pinax sync diff --target cloud --dry-run` or `pinax sync push --target cloud --dry-run`
- **THEN** 输出 SHALL 显示 sync plan
- **AND** SHALL NOT write remote objects, update sync-state revision, or modify Markdown

#### Scenario: 冲突检测与原目录副本保留

- **WHEN** local and remote both changed the same path after the base revision
- **THEN** pull SHALL preserve the local version as `<filename>.<timestamp>.conflict.md` in the same directory
- **AND** the remote trunk version SHALL be written to the original file path
- **AND** output SHALL include next actions for conflict inspection when available

### Requirement: 冲突辅助处理 CLI

CLI SHALL provide commands to inspect and resolve local conflict copies through app-service/projection owned behavior.

#### Scenario: 机器可读的冲突列举（面向 AI）

- **WHEN** 用户运行 `pinax sync conflicts list --json`
- **THEN** CLI SHALL output a JSON envelope containing each conflict file and corresponding trunk file path
- **AND** machine output SHALL include stable next actions where possible

#### Scenario: 差异对比

- **WHEN** 用户运行 `pinax sync conflicts diff <conflict-file>`
- **THEN** CLI SHALL find its corresponding trunk file
- **AND** SHALL output a diff view without writing vault state

#### Scenario: 机器友好的内容导出（面向 AI）

- **WHEN** 用户或 AI 运行 `pinax sync conflicts show <conflict-file> --json`
- **THEN** CLI SHALL output JSON
- **AND** MAY include local `original_content` and `conflict_content` because the user explicitly requested local conflict content
- **AND** this content SHALL NOT be copied into Cloud transport logs, receipts, fixtures, or backend evidence

#### Scenario: 快速解决冲突

- **WHEN** 用户运行 `pinax sync conflicts resolve <file> --keep-local --yes`
- **THEN** CLI SHALL copy the conflict file content to the trunk file and remove the conflict file
- **AND** `--keep-remote --yes` SHALL remove only the conflict file
- **AND** `--merged <merged-file> --yes` SHALL copy the merged file to the trunk file and remove the conflict file

### Requirement: 同步连接管理与状态 CLI

CLI SHALL provide commands for initializing, reusing, diagnosing, and viewing Cloud Sync configuration and state.

#### Scenario: 初始化或复用同步配置

- **WHEN** 用户运行 `pinax sync init --target cloud --vault <vault>`
- **THEN** CLI SHALL reuse existing `.pinax/cloud/config.yaml` when present
- **AND** SHALL report configured backend kind, endpoint, workspace, and device in redacted output

#### Scenario: 状态健康检测

- **WHEN** 用户运行 `pinax sync status` or `pinax cloud doctor`
- **THEN** CLI SHALL distinguish configured, missing config, provider credential boundary, server audit availability, and recommended next actions
- **AND** SHALL NOT resolve or print raw secrets

### Requirement: 单向与双向同步拆分

同步逻辑 SHALL support separate push, pull, and future bidirectional consistency workflows.

#### Scenario: 仅拉取 (Pull Only)

- **WHEN** 用户运行 `pinax sync pull --target cloud --yes`
- **THEN** CLI SHALL download and decrypt the committed remote revision for local application
- **AND** SHALL NOT push local unsynced changes to the transport during that pull

#### Scenario: 仅推送 (Push Only)

- **WHEN** 用户运行 `pinax sync push --target cloud --yes`
- **THEN** CLI SHALL attempt to push local changes through the configured Cloud Sync transport
- **AND** if base revision mismatch occurs, CLI SHALL refuse the push and require pull/conflict handling before retry

### Requirement: 多种存储后端支持与并发锁

Direct sync SHALL use an abstract remote object store where possible and SHALL provide optimistic concurrency or lock protection for head updates.

#### Scenario: S3 或兼容对象存储 (S3 API)

- **WHEN** 用户配置后端为 S3-compatible direct transport
- **THEN** system SHALL use provider conditional-write semantics such as `If-Match`, ETag, or equivalent store revision when available
- **AND** unsupported conditional writes SHALL require a lock-object fallback before claiming success

#### Scenario: 本地/网络挂载文件系统

- **WHEN** 用户配置后端为 `file://` 等 local object-store transport
- **THEN** system SHALL use atomic file/object update semantics for head CAS
- **AND** concurrent first commits or same-base updates SHALL allow at most one successful `remote_write=true`

#### Scenario: Rclone provider lock fallback

- **WHEN** rclone direct transport lacks reliable conditional writes
- **THEN** it SHALL use `locks/commit.lock` with device id, request id, and expiry
- **AND** uncertain write state SHALL return retryable/diagnosable error with `remote_write=false`

#### Scenario: 动态 URI Scheme 注册路由

- **WHEN** system loads storage media
- **THEN** it SHALL use registered scheme factories rather than hardcoding transport behavior in config parsing
- **AND** unsupported schemes SHALL return stable unsupported errors instead of writable no-op stores

### Requirement: 双设备 E2E 与发布准入

Cloud Sync release readiness SHALL be proven by focused tests and OpenSpec validation before archive.

#### Scenario: 两台设备通过 Pinax Cloud Server 顺序同步后收敛

- **GIVEN** device A and device B each have independent local vaults linked to the same Pinax Cloud MLP vault
- **WHEN** device A creates a note and successfully pushes through server transport
- **AND** device B pulls the committed revision through server transport
- **THEN** device B SHALL contain the decrypted note locally
- **AND** protected Cloud surfaces SHALL NOT contain plaintext note body, Authorization header, token or provider payload.

#### Scenario: 两台设备通过 Pinax Cloud Server 并发编辑后保留冲突

- **GIVEN** device A and device B both start from revision `rev_a`
- **WHEN** both edit the same note and one device commits `rev_b` through server transport
- **AND** the other device pushes or pulls from the stale base
- **THEN** stale push SHALL receive `REVISION_CONFLICT` or pull SHALL preserve the local edit as a conflict copy
- **AND** output SHALL include next actions for conflict list, diff, show and resolve.

#### Scenario: Server integration evidence is explicit

- **WHEN** maintainers run `task test:integration` for Cloud server sync
- **THEN** evidence SHALL be written under `temp/integration-test-runs/<run-id>/`
- **AND** evidence SHALL include server-backed convergence, conflict and redaction checks.

### Requirement: Server sync client SHALL wait for local proof-loop safety gates

Pinax server sync client implementation SHALL not become the first writer that lacks local restore, shared redaction and proof-run evidence gates.

#### Scenario: Local safety gate precedes server mutation

- **GIVEN** server sync client work mutates local vault state based on remote revision state
- **WHEN** implementation begins
- **THEN** `pinax-proof-loop-operational-hardening` SHALL be complete or explicitly waived with reviewed evidence
- **AND** server sync output SHALL use the same projection redaction gate as local proof-loop commands.

### Requirement: 本地后台实时同步进程

Pinax SHALL provide an explicitly managed local sync daemon for a configured vault. The daemon SHALL reuse the existing Cloud Sync push/pull/conflict engine and SHALL NOT introduce a separate synchronization protocol or bypass existing approval, receipt, redaction, and `remote_write=true` rules.

#### Scenario: 前台运行 daemon 启动后立即同步

- **GIVEN** a vault has a configured Cloud Sync backend
- **WHEN** the user runs `pinax sync daemon run --target cloud --vault <vault> --yes`
- **THEN** Pinax SHALL start a local daemon runner for that vault
- **AND** it SHALL immediately execute one startup sync cycle before waiting for the next poll interval
- **AND** that cycle SHALL pull a newer remote revision before pushing local dirty content
- **AND** it SHALL persist redacted daemon events under `.pinax/sync-daemon/events.jsonl`.

#### Scenario: 机器输出保持稳定

- **WHEN** the user runs `pinax sync daemon run --target cloud --vault <vault> --yes --json`
- **THEN** stdout SHALL remain one final JSON envelope for `sync.daemon.run`
- **AND** intermediate progress SHALL NOT be mixed into JSON stdout.

### Requirement: 本地变更触发同步

The sync daemon SHALL detect local content changes in Pinax-managed vault paths and schedule a debounced sync attempt. Runtime paths, ignored paths, and hard-denied paths SHALL NOT trigger content sync.

#### Scenario: 本地 Markdown 修改触发 push

- **GIVEN** the daemon is running for a vault with Cloud Sync configured
- **AND** `notes/alpha.md` is selected by `.pinaxignore`
- **WHEN** the user modifies `notes/alpha.md`
- **THEN** Pinax SHALL coalesce local file events using the configured debounce window
- **AND** it SHALL schedule a sync attempt that can push the changed encrypted manifest and blobs through the configured Cloud Sync transport
- **AND** any `remote_write=true` output SHALL still require a durable revision commit and local sync-state receipt.

#### Scenario: 运行态路径不触发内容同步

- **GIVEN** the daemon writes `.pinax/sync-daemon/daemon.json` or `.pinax/sync-daemon/events.jsonl`
- **WHEN** the watcher observes those writes
- **THEN** Pinax SHALL ignore the events for content sync purposes
- **AND** `.pinax/**` SHALL remain excluded from Cloud Sync content manifests.

#### Scenario: watcher 降级到扫描

- **GIVEN** filesystem watching is unavailable, over limit, or returns an unrecoverable error
- **WHEN** the daemon continues running
- **THEN** Pinax SHALL switch to periodic manifest scan fallback or report `watch_degraded`
- **AND** `pinax sync daemon status --vault <vault> --json` SHALL expose the detection mode and a next action.

### Requirement: 远端变化轮询同步

The sync daemon SHALL poll the configured Cloud Sync transport for remote head changes. First release remote detection SHALL be polling-based unless a future transport-specific change adds push notification semantics.

#### Scenario: 远端 revision 更新触发 pull

- **GIVEN** the daemon last observed remote revision `rev_a`
- **AND** the configured transport reports current head `rev_b`
- **WHEN** the daemon poll loop detects `rev_b` is newer than the local base revision
- **THEN** Pinax SHALL schedule a pull before any local push attempt
- **AND** the pull SHALL preserve conflicting local edits as conflict copies using existing Cloud Sync conflict rules.

#### Scenario: transport 暂不可用进入 backoff

- **GIVEN** the configured Cloud Sync transport returns `transport_unavailable`, provider throttling, network timeout, or another retryable error
- **WHEN** the daemon polls or attempts sync
- **THEN** Pinax SHALL enter retry backoff with a bounded next retry time
- **AND** daemon status SHALL report the stable error code and next action
- **AND** it SHALL NOT claim local or remote convergence while the error persists.

### Requirement: daemon 冲突和写入安全

The sync daemon SHALL keep all automatic writes behind the same safety boundaries as explicit `pinax sync pull` and `pinax sync push`. It SHALL pause automatic write attempts when manual conflict resolution is required.

#### Scenario: pull 产生冲突后暂停自动写入

- **GIVEN** a local file and the remote committed revision both changed the same path
- **WHEN** the daemon pull detects the conflict
- **THEN** Pinax SHALL preserve the local edit as a conflict copy
- **AND** daemon state SHALL become `conflict_required`
- **AND** subsequent daemon sync attempts SHALL NOT auto-resolve, delete, or overwrite the conflict copy
- **AND** status/actions SHALL point to `pinax sync conflicts list --vault <vault> --json` and explicit conflict resolution commands.

#### Scenario: revision conflict 走 pull/retry

- **GIVEN** a daemon push receives `revision_conflict`
- **WHEN** the error is retryable
- **THEN** Pinax SHALL attempt the configured pull/rebase/retry path within the retry budget
- **AND** if the retry budget is exhausted, it SHALL report `degraded` with `last_error_code=revision_conflict`
- **AND** it SHALL NOT emit `remote_write=true` for the failed push.

### Requirement: daemon 输出和事件脱敏

Daemon command output, daemon events, daemon logs, sync receipts, integration evidence, and test fixtures SHALL remain redacted and machine-consumable.

#### Scenario: realtime human output and events stream

- **WHEN** the user runs `pinax sync daemon run --target cloud --vault <vault> --yes`
- **THEN** Pinax SHALL emit concise human-readable progress lines for daemon lifecycle and sync attempts
- **AND** `pinax sync daemon run --target cloud --vault <vault> --yes --events` SHALL emit NDJSON events with stable additive event types
- **AND** neither mode SHALL expose plaintext note bodies, raw secret refs, Authorization headers, cookies, provider payloads, raw prompts, hidden system prompts, or private tool arguments.

#### Scenario: daemon logs expose persisted events

- **WHEN** the user runs `pinax sync daemon logs --vault <vault> --json`
- **THEN** Pinax SHALL return recent redacted daemon events from `.pinax/sync-daemon/events.jsonl`
- **AND** events MAY include optional fields such as `seq`, `cycle_id`, `trigger`, `direction`, `duration_ms`, `local_dirty`, `remote_revision`, `revision_id`, `sync_run_id`, `remote_write`, and `local_write`.

### Requirement: Remote API Mode 与实时 Cloud Sync 边界清晰

Pinax SHALL document and preserve the distinction between Remote API Mode and Cloud Sync daemon behavior.

#### Scenario: Remote API client operates one server-side vault

- **WHEN** a client runs `pinax --api-url http://127.0.0.1:8787 note list --json`
- **THEN** the command SHALL operate through the API server's configured vault
- **AND** it SHALL NOT imply multi-device synchronization.

#### Scenario: sync daemon owns realtime multi-device convergence

- **WHEN** a user wants realtime multi-device sync
- **THEN** the documented command SHALL be `pinax sync daemon run --target cloud --vault <vault> --yes`
- **AND** each device SHALL keep its own local vault while the Cloud Sync transport coordinates only encrypted revisions, encrypted manifests, encrypted blobs, and conflict metadata.

#### Scenario: explicit remote sync RPC does not replace daemon lifecycle

- **WHEN** a client calls a registered `sync.push` or `sync.pull` RPC method
- **THEN** Pinax SHALL treat it as an explicit sync operation
- **AND** realtime watch/poll behavior SHALL remain owned by `pinax sync daemon` rather than the Remote API server.

