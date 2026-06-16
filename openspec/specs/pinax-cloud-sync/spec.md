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

