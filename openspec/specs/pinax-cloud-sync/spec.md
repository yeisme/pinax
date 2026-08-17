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

- **WHEN** 用户运行 `pinax sync push --target capsa` 或 `pinax sync pull --target capsa`
- **THEN** Pinax SHALL 使用已配置 Cloud Sync transport 交换加密 revision、manifest 和 blob
- **AND** 每台设备 SHALL 保留可离线使用的本地 Markdown vault

#### Scenario: 未忽略普通文件纳入 manifest

- **GIVEN** a vault contains `notes/a.md`, `scripts/build.sh`, and `assets/logo.png`
- **AND** `.pinaxignore` does not exclude those paths
- **WHEN** the user runs `pinax sync push --target capsa --dry-run --vault <vault> --json`
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

### Requirement: direct backup mirror、Pinax Cloud 与 realtime daemon 边界分离

Pinax SHALL preserve the local-first boundary between CLI-side backup mirror transports, Pinax Cloud server transport, and realtime daemon/conflict behavior.

#### Scenario: S3 direct backup mirror 不获得 Pinax Cloud server 能力

- **GIVEN** 用户配置 S3 direct 或 rclone direct object-store transport
- **WHEN** Pinax builds a sync or backup mirror plan
- **THEN** Pinax SHALL treat the direct transport as a CLI-side provider-credential backup mirror for encrypted Cloud Sync objects
- **AND** it SHALL NOT claim Pinax Cloud server-side auth, server audit, object lifecycle policy, multi-tenant controls, or rate limiting for direct transport writes
- **AND** Pinax Cloud server transport SHALL remain the path that owns server auth, audit, object lifecycle, and cloud sync semantics.

#### Scenario: backup mirror 不包含 daemon 和 conflict policy 变更

- **WHEN** docs, plans, or command help describe backup mirror behavior
- **THEN** Pinax SHALL NOT imply realtime daemon convergence, automatic merge, conflict resolution, or push notification behavior
- **AND** daemon lifecycle, automatic conflict handling, conflict resolution semantics, and transport-specific realtime behavior SHALL require separate OpenSpec coverage before implementation.

### Requirement: 端侧加密保护明文

Manifest 和 blob SHALL 使用 client-side encryption；明文 SHALL NOT 离开本地设备。加密密钥 SHALL 通过 Capsa SDK 从 secret reference 解析出真实密钥值，SHALL NOT 使用引用字符串本身作为密钥材料。Capsa SDK Go module source SHALL 指向版本化 submodule `backend-server/capsa/sdk`，SHALL NOT 依赖 plain directory copy 作为生产 SDK source。

#### Scenario: 加密 manifest 和 blob

- **WHEN** 客户端上传 manifest 或 blob
- **THEN** 数据 SHALL 使用端侧加密 envelope
- **AND** 后端或 direct object store SHALL 只看到 encrypted payload 和非敏感 revision metadata

#### Scenario: profile:// 引用解析为真实密钥

- **GIVEN** `secret_ref` 配置为 `profile://tencent-cos-pinax`
- **WHEN** Pinax 执行 Cloud Sync 加密操作
- **THEN** Pinax SHALL 调用 `github.com/yeisme/capsa` SDK crypto API 解析为真实的 `aws_secret_access_key`
- **AND** SHALL NOT 使用字面字符串 `profile://tencent-cos-pinax` 作为 PBKDF2 输入
- **AND** 如果 profile 不存在或无法解析，SHALL 返回错误

#### Scenario: SDK salt migration requires re-push

- **GIVEN** 远端 COS/S3 对象由旧 Pinax-local crypto path 写入
- **WHEN** Pinax 升级到 Capsa SDK crypto path
- **THEN** key derivation SHALL use `capsa-sync-salt-v1`
- **AND** 旧 `pinax-cloud-sync-salt-v1` 加密对象 SHALL require a fresh `pinax sync push --target capsa --yes` from a complete local vault
- **AND** docs SHALL describe the migration rather than promising transparent remote decryption

#### Scenario: 弱加密密钥警告

- **GIVEN** 用户运行 `pinax capsa backend set s3` 未指定 `--encryption-secret-ref`
- **AND** `secret_ref` 不是 `env://` 或 `keychain://` scheme
- **WHEN** Pinax 配置 Capsa Sync backend
- **THEN** projection SHALL 包含 `weak_encryption_key` 警告

#### Scenario: SDK source 指向版本化 submodule

- **GIVEN** Pinax `go.mod` 声明 `require github.com/yeisme/capsa v0.0.0`
- **WHEN** 开发者编译 sync 或 crypto 相关代码
- **THEN** `go.mod` replace SHALL 指向 `../../backend-server/capsa/sdk`
- **AND** SHALL NOT 指向 `../../shared/capsa` plain directory
- **AND** public module path SHALL 保持 `github.com/yeisme/capsa` 不变
- **AND** 加密 salt 与 envelope schema SHALL 不因 source 迁移改变

### Requirement: Agent Brain 投影不作为 Cloud Sync 明文状态

Cloud Sync SHALL preserve the distinction between encrypted source content, service-owned evidence, and rebuildable Agent Brain projections.

#### Scenario: Brain projections are rebuilt locally

- **WHEN** Cloud Sync scans a vault with local `.pinax/index.sqlite`, external RAG export/cache material, `.pinax/graph/`, answer cache, or provider cache files
- **THEN** those files SHALL be treated as local rebuildable projections or cache state, and external RAG material SHALL remain outside Pinax ownership
- **AND** they SHALL NOT be uploaded as plaintext Cloud Sync content
- **AND** after pull/import, users MAY rebuild projections with commands such as `pinax index refresh --vault ./my-notes --json`, `pinax export markdown ./temp/rag-export --vault ./my-notes --json`, or `pinax graph rebuild --vault ./my-notes --json`.

#### Scenario: Memory and maintenance evidence require explicit encrypted contract

- **WHEN** memory ledger records, maintenance plans, proof receipts, or answer caches are considered for cross-device behavior
- **THEN** Pinax SHALL classify them as service-owned evidence or rebuildable cache rather than raw note content
- **AND** raw prompts, hidden system prompts, provider payloads, plaintext vectors, full note bodies, Authorization headers, cookies, tokens, and private tool arguments SHALL NOT be written to Cloud transport evidence
- **AND** any future cross-device memory or answer-cache sync SHALL require a dedicated encrypted contract.

### Requirement: Revision CAS 提交准入

Cloud push SHALL only report a remote write after a compare-and-swap revision commit succeeds. 当 `--yes` 已确认且 commit 因 `revision_conflict` 失败时，push SHALL 自动拉取远端修订、重建计划并重试一次。

#### Scenario: durable commit 后 remote_write=true

- **WHEN** transport atomically commits a new revision against the observed base revision
- **THEN** CLI MAY output `remote_write=true`

#### Scenario: Base revision 失配时自动 rebase

- **GIVEN** 用户已通过 `--yes` 确认同步写入
- **WHEN** commit 返回 `revision_conflict`
- **THEN** CLI SHALL 自动拉取远端修订，重建 push plan，并重试一次 commit
- **AND** 如果重建后的 plan 无冲突且 commit 成功，SHALL 输出 `remote_write=true`
- **AND** 如果重建后的 plan 有冲突，SHALL 返回 `conflict_required`

#### Scenario: 无 --yes 时不自动 rebase

- **GIVEN** 用户运行 `pinax sync push` 未带 `--yes`
- **WHEN** commit 返回 `revision_conflict`
- **THEN** CLI SHALL 直接返回 `revision_conflict` 错误

### Requirement: Sync plan 支持 dry-run 和冲突

Sync planner SHALL support dry-run mode and SHALL preserve concurrent local edits as conflict copies instead of silently overwriting user data.

#### Scenario: dry-run sync

- **WHEN** 用户运行 `pinax sync diff --target capsa --dry-run` or `pinax sync push --target capsa --dry-run`
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

- **WHEN** 用户运行 `pinax sync init --target capsa --vault <vault>`
- **THEN** CLI SHALL reuse existing `.pinax/cloud/config.yaml` when present
- **AND** SHALL report configured backend kind, endpoint, workspace, and device in redacted output

#### Scenario: 状态健康检测

- **WHEN** 用户运行 `pinax sync status` or `pinax cloud doctor`
- **THEN** CLI SHALL distinguish configured, missing config, provider credential boundary, server audit availability, and recommended next actions
- **AND** SHALL NOT resolve or print raw secrets

### Requirement: 单向与双向同步拆分

同步逻辑 SHALL support separate push, pull, and future bidirectional consistency workflows.

#### Scenario: 仅拉取 (Pull Only)

- **WHEN** 用户运行 `pinax sync pull --target capsa --yes`
- **THEN** CLI SHALL download and decrypt the committed remote revision for local application
- **AND** SHALL NOT push local unsynced changes to the transport during that pull

#### Scenario: 仅推送 (Push Only)

- **WHEN** 用户运行 `pinax sync push --target capsa --yes`
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

Pinax SHALL provide a managed local sync daemon. 后台 daemon 子进程 SHALL 继承父进程的环境变量。Pinax SHALL 提供 `daemon install`/`uninstall` 命令生成 OS 服务单元。

#### Scenario: 后台 daemon 继承环境变量

- **GIVEN** `PINAX_SYNC_SECRET` 设置在当前 shell 环境中
- **WHEN** the user runs `pinax sync daemon start --target capsa --vault <vault> --yes`
- **THEN** daemon 子进程 SHALL 继承 `PINAX_SYNC_SECRET` 及其他所有环境变量

#### Scenario: Linux systemd 服务安装

- **GIVEN** 用户在 Linux 上运行 `pinax sync daemon install --vault ./my-notes`
- **THEN** SHALL 在 `~/.config/systemd/user/capsa-sync-<slug>.service` 写入服务单元
- **AND** SHALL NOT 自动启动服务
- **AND** projection SHALL 包含 `systemctl --user enable --now capsa-sync-<slug>`

#### Scenario: macOS launchd 服务安装

- **GIVEN** 用户在 macOS 上运行 `pinax sync daemon install --vault ./my-notes`
- **THEN** SHALL 在 `~/Library/LaunchAgents/com.yeisme.capsa-sync.<slug>.plist` 写入 plist
- **AND** SHALL NOT 自动加载服务

#### Scenario: 卸载服务单元

- **WHEN** 用户运行 `pinax sync daemon uninstall --vault ./my-notes`
- **THEN** Pinax SHALL 删除服务单元文件
- **AND** SHALL NOT 停止正在运行的服务

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

- **WHEN** the user runs `pinax sync daemon run --target capsa --vault <vault> --yes`
- **THEN** Pinax SHALL emit concise human-readable progress lines for daemon lifecycle and sync attempts
- **AND** `pinax sync daemon run --target capsa --vault <vault> --yes --events` SHALL emit NDJSON events with stable additive event types
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
- **THEN** the documented command SHALL be `pinax sync daemon run --target capsa --vault <vault> --yes`
- **AND** each device SHALL keep its own local vault while the Cloud Sync transport coordinates only encrypted revisions, encrypted manifests, encrypted blobs, and conflict metadata.

#### Scenario: explicit remote sync RPC does not replace daemon lifecycle

- **WHEN** a client calls a registered `sync.push` or `sync.pull` RPC method
- **THEN** Pinax SHALL treat it as an explicit sync operation
- **AND** realtime watch/poll behavior SHALL remain owned by `pinax sync daemon` rather than the Remote API server.

### Requirement: Pinax SHALL separate direct backup transport, Cloud Sync, and realtime daemon boundaries

Pinax SHALL document and preserve the boundary between CLI-side backup mirror transports, Pinax Cloud server transport, and realtime daemon/conflict behavior.

#### Scenario: S3 direct is not Pinax Cloud storage
- **GIVEN** a user configures S3 direct or rclone direct object-store transport
- **WHEN** Pinax builds a sync or backup mirror plan
- **THEN** Pinax SHALL treat the direct transport as a CLI-side provider-credential backup mirror for encrypted Cloud Sync objects
- **AND** it SHALL NOT claim Pinax Cloud server-side auth, server audit, object lifecycle policy, multi-tenant controls, or rate limiting for direct transport writes
- **AND** Pinax Cloud server transport SHALL remain the server-authenticated path for cloud sync semantics
- **AND** realtime daemon behavior, automatic merge, conflict resolution, and push notification semantics SHALL require separate OpenSpec coverage before implementation

#### Scenario: backup mirror wording does not expand daemon or conflict behavior
- **WHEN** docs, plans, or command help describe backup mirror behavior
- **THEN** Pinax SHALL NOT imply realtime daemon convergence, automatic merge, conflict resolution, or transport-specific push notification behavior
- **AND** `pinax sync daemon` SHALL remain the local realtime automation layer
- **AND** `pinax sync conflicts` SHALL remain the explicit conflict inspection and resolution surface unless a separate OpenSpec change modifies that behavior

### Requirement: Cloud Sync propagates explicit delete tombstones
Cloud Sync SHALL represent deletions as encrypted tombstone/delete marker entries in the committed manifest instead of inferring deletion from missing content entries.

#### Scenario: Push includes delete marker after project delete
- **GIVEN** device A deletes project `history` through `pinax project delete history --yes`
- **WHEN** device A runs `pinax sync push --target capsa --vault ./device-a --yes --json`
- **THEN** the encrypted manifest SHALL include a delete marker for object id `project/history` or its path hash
- **AND** protected stdout, receipts, object keys, and object metadata SHALL NOT expose plaintext note bodies, tokens, Authorization headers, or provider payloads
- **AND** `remote_write=true` SHALL only appear after the transport commits the revision successfully.

#### Scenario: Pull applies delete marker to local registry and index
- **GIVEN** device B has project `history` active locally
- **AND** the remote committed revision contains a delete marker for `project/history`
- **WHEN** device B runs `pinax sync pull --target capsa --vault ./device-b --yes --json`
- **THEN** Pinax SHALL move or mark the local project as trashed through the trash service
- **AND** `pinax project list --vault ./device-b --json` SHALL exclude `history`
- **AND** `pinax trash list --vault ./device-b --json` SHALL include the tombstone.

#### Scenario: Pull preserves local conflicting edit before applying delete
- **GIVEN** device B has unsynced local changes under a subproject workspace
- **AND** the remote revision deletes that same subproject
- **WHEN** device B pulls the remote revision
- **THEN** Pinax SHALL preserve the local changed files as conflict copies or a conflict trash backup
- **AND** it SHALL report conflict next actions without silently discarding the local content.

### Requirement: Cloud Sync transfers encrypted trash backups
Cloud Sync SHALL transfer recoverable trash backup blobs when a deletion is synchronized, subject to encryption and redaction rules.

#### Scenario: Trash backup is uploaded as encrypted content
- **GIVEN** deleting `subproject/history-learning/history-info` created a trash backup
- **WHEN** the next Cloud Sync push commits a revision
- **THEN** the trash backup SHALL be uploaded as encrypted blob content referenced by the delete marker
- **AND** the remote transport SHALL NOT receive plaintext paths inside object keys or plaintext file bodies in metadata.

#### Scenario: Missing trash backup is diagnosable
- **GIVEN** a local tombstone references a trash backup path that no longer exists
- **WHEN** the user runs `pinax sync push --target capsa --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return partial status with stable issue code `trash_backup_missing`
- **AND** it SHALL NOT claim that the deletion is safely recoverable on another device.

### Requirement: Cloud Sync manifest v2 is object-first
Cloud Sync manifest v2 entries SHALL contain canonical `object_id`, `object_kind`, `current_path`, encrypted blob reference, plaintext content hash, size, object revision, update time and producing device ID. Object ID SHALL be the merge identity; path SHALL be a mutable locator.

#### Scenario: Push a renamed note
- **WHEN** device A renames a note and pushes a new manifest revision
- **THEN** the manifest SHALL record the same object ID with a new current path and revision, and SHALL NOT encode the operation only as an unrelated path deletion and creation.

#### Scenario: Device B pulls an object move
- **WHEN** device B has the prior revision of the same object and pulls the rename
- **THEN** Pinax SHALL move the local object to the remote current path, update ledger and index projections, and preserve its local identity history.

### Requirement: Sync distinguishes revision conflicts from path collisions
Sync planning SHALL classify concurrent changes by object identity before applying content or path operations.

#### Scenario: Same object changes on two devices
- **WHEN** two devices modify different revisions of the same object ID from one base revision
- **THEN** Pinax SHALL attempt the allowed three-way merge or report a revision conflict containing object ID and redacted path evidence without silently choosing one body.

#### Scenario: Different objects claim one path
- **WHEN** local and remote manifests contain different object IDs with the same current path
- **THEN** Pinax SHALL report a path collision, preserve both payloads through conflict-safe storage and require an explicit resolution plan.

### Requirement: Deletes propagate by object identity and UUID tombstone
Cloud Sync deletion entries SHALL contain object ID, object kind, UUID tombstone ID, deletion revision and encrypted recovery evidence where available.

#### Scenario: Pull an object tombstone after local move
- **WHEN** a remote tombstone deletes an object that has a different local path but the same object ID
- **THEN** Pinax SHALL match the object by ID, preserve conflicting local changes, apply trash lifecycle semantics and SHALL NOT leave an active duplicate at the moved path.

### Requirement: Manifest v1 migration is explicit and bounded
Pinax SHALL read existing manifest v1 data during a documented compatibility window and SHALL require identity reconciliation before publishing an authoritative manifest v2 revision.

#### Scenario: First v2 sync against v1 state
- **WHEN** a vault with manifest v1 state prepares its first manifest v2 push
- **THEN** Pinax SHALL produce a migration plan that maps paths to canonical object IDs, reports ambiguous or missing identities and performs no remote write until the plan is approved.

### Requirement: 同步日志提供脱敏的文件级时间线

Pinax SHALL 在同步 run 时间线中记录每个计划文件操作的安全元数据，并允许用户持续跟随新追加事件，而不暴露正文、凭据、provider payload 或违反路径策略的路径。

#### Scenario: 查看同步涉及的文件

- **GIVEN** 一次 sync run 包含上传、下载、删除或冲突操作
- **WHEN** 用户运行 `pinax sync logs tail --vault <vault>`
- **THEN** CLI SHALL 展示每个文件操作的 run ID、方向、操作类型、状态和允许公开的路径标识
- **AND** CLI SHALL 同时保留 run 级完成摘要。

#### Scenario: 持续跟随同步文件事件

- **WHEN** 用户运行 `pinax sync logs tail --follow --vault <vault>`
- **THEN** CLI SHALL 先输出当前尾部事件，再持续输出后续追加的同步事件，直到命令上下文取消
- **AND** `--events` SHALL 输出逐行有效 NDJSON，不混入 human prose 或 diagnostics。

#### Scenario: 文件事件遵守路径策略和脱敏

- **GIVEN** sync run 使用 `default`、`hash` 或 `omitted` path policy
- **WHEN** Pinax 持久化或渲染文件级同步事件
- **THEN** 文件路径 SHALL 使用对应策略处理
- **AND** 事件、stdout、stderr、测试 fixture 和收据 SHALL NOT 包含 note body、token、Authorization、Cookie 或 provider payload。

#### Scenario: JSON 不进入无限跟随模式

- **WHEN** 用户组合 `sync logs tail --follow --json`
- **THEN** CLI SHALL 返回稳定的参数错误和可执行提示
- **AND** SHALL NOT 输出不完整或无限增长的 JSON document。

### Requirement: 同步配置来源与运行态分离

Cloud Sync SHALL treat repository declaration as the portable source for transport topology and local `.pinax/cloud/config.yaml` as generated device runtime state.

#### Scenario: 配置驱动同步

- **WHEN** a repository contains a valid `pinax-sync.yaml` and the user runs `pinax sync repo apply --vault ./my-notes --json`
- **THEN** subsequent `pinax sync push`, `pinax sync pull` and daemon commands SHALL resolve the generated Capsa runtime config without requiring repeated backend flags
- **AND** sync SHALL retain existing encrypted revision, manifest, blob and CAS semantics

#### Scenario: 设备状态不进入远程内容清单

- **WHEN** Pinax builds a content manifest after repository bootstrap
- **THEN** generated cloud config, device state, daemon runtime, secrets assets and sync receipts SHALL follow explicit protected-path rules
- **AND** SHALL NOT be uploaded as ordinary plaintext note content

### Requirement: 首次设备 bootstrap 安全

Cloud Sync SHALL require explicit first-device or new-device bootstrap semantics before enabling bidirectional writes.

#### Scenario: 新设备首次只拉取

- **WHEN** a new device bootstraps a workspace with no local sync receipt
- **THEN** Pinax SHALL perform pull-only initialization by default
- **AND** SHALL require explicit approval before uploading local deletions or replacing remote state

#### Scenario: 加密 key identity 不匹配

- **WHEN** unlocked secrets resolve to a different encryption key identity than the remote head
- **THEN** sync SHALL fail with `encryption_key_mismatch`
- **AND** SHALL provide a recovery action without generating a replacement key automatically

### Requirement: env secrets 不进入内容 manifest

Cloud Sync SHALL treat encrypted and plaintext env assets, materialized runtime files and their caches as protected paths.

#### Scenario: manifest 排除 env secrets

- **WHEN** a vault contains `.env`, `.env.local`, `.pinax/pinax-sync.env.age` and `.pinax/runtime/pinax-sync.env`
- **THEN** none of these files SHALL be uploaded as ordinary plaintext content entries
- **AND** sync receipts SHALL report counts and redacted paths only

### Requirement: Sync key derivation SHALL use per-secret salt and current iteration guidance

The sync encryption key SHALL be derived with PBKDF2-SHA256 at no fewer than 600,000 iterations over a salt derived from the shared secret itself, so devices sharing a secret derive the same key while distinct secrets get distinct salts.

#### Scenario: New envelopes use the v2 derivation

- **WHEN** a push encrypts a blob or manifest
- **THEN** the envelope SHALL carry the KeyID of the v2 derivation

#### Scenario: Legacy envelopes remain readable

- **WHEN** a pull reads an envelope whose KeyID belongs to the pre-v2 derivation
- **AND** the same secret is configured
- **THEN** decryption SHALL succeed through the legacy fallback key without any migration step

#### Scenario: Foreign keys fail closed

- **WHEN** an envelope's KeyID matches neither the active nor a legacy key for the configured secret
- **THEN** decryption SHALL fail with a key ID mismatch error

