## MODIFIED Requirements

### Requirement: Cloud Sync transport 抽象

Cloud Sync SHALL expose a transport-independent protocol so server, S3 direct, rclone direct, and embedded/local API paths share one push/pull/conflict engine.

#### Scenario: Server transport is wired through cloudclient

- **WHEN** cloud backend kind is `server` or the endpoint is an HTTP Pinax Cloud Server endpoint
- **THEN** Pinax SHALL use `internal/cloudclient.Transport` for current revision, blob batch-check/upload/download, and revision commit
- **AND** Pinax Cloud Server SHALL own auth/device scope, idempotency, revision CAS, audit, and readiness
- **AND** a successful server revision commit MAY be the durable commit evidence required before `remote_write=true`.

#### Scenario: S3 direct transport uses shared object-store transport

- **WHEN** cloud backend kind is `s3-direct` and S3-compatible fields are configured
- **THEN** Pinax SHALL use provider credentials locally through `internal/cloudsync.ObjectStoreTransport` to read/write encrypted Cloud Sync objects directly
- **AND** it SHALL NOT require a running Pinax Cloud Server
- **AND** `cloud doctor` output SHALL identify provider credentials as the auth boundary and `server_audit=false`.

#### Scenario: Rclone direct transport uses shared object-store transport

- **WHEN** cloud backend kind is `rclone-direct` with a remote such as `onedrive:PinaxSync`
- **THEN** Pinax SHALL treat rclone as the provider boundary and SHALL NOT save OAuth tokens
- **AND** it SHALL use `internal/cloudsync.ObjectStoreTransport` and the shared lock-object commit fallback for durable head updates
- **AND** the MVP SHALL use rclone examples for OneDrive instead of native Microsoft Graph.

#### Scenario: Embedded Go API and local RPC

- **WHEN** a local agent, desktop app, MCP bridge, or local RPC method invokes Cloud Sync
- **THEN** it SHALL call the same application service and sync engine as the CLI
- **AND** it SHALL NOT bypass approval, dry-run, snapshot, conflict, event, or redaction rules.

### Requirement: Revision CAS 提交准入

Cloud push SHALL only report a remote write after a compare-and-swap revision commit succeeds and local sync-state evidence is written.

#### Scenario: durable commit 后 remote_write=true

- **WHEN** missing encrypted blobs and encrypted manifest have been uploaded
- **AND** transport atomically commits a new revision against the observed base revision
- **AND** local sync-state/run evidence is written with backend kind, device/workspace ids, revision id, manifest id, status, and timestamp
- **THEN** CLI MAY output `remote_write=true`
- **AND** the evidence SHALL NOT leak secrets, plaintext note body, plaintext protected paths, raw token, Authorization header, Cookie, raw secret ref, provider stderr, or provider payload.

#### Scenario: plan/blob upload/pull 不算 remote write

- **WHEN** CLI only generated a plan, performed dry-run, pulled a remote revision, uploaded blobs, uploaded a manifest, hit an unsupported transport path, or encountered uncertain direct commit state
- **THEN** CLI SHALL output `remote_write=false`
- **AND** SHALL NOT create a dummy revision or claim sync success.

#### Scenario: Base revision 失配时拒绝推送

- **GIVEN** 客户端基于 base revision `rev_a` 提交 commit
- **AND** transport current revision is `rev_b`
- **WHEN** transport receives the commit request
- **THEN** it SHALL reject the commit with stable `revision_conflict`
- **AND** SHALL NOT accept partial manifest or partial revision write
- **AND** CLI SHALL prompt the user to pull and resolve conflicts before retrying push.

### Requirement: 多种存储后端支持与并发锁

Direct sync SHALL use an abstract remote object store where possible and SHALL provide optimistic concurrency or lock protection for head updates.

#### Scenario: S3 或兼容对象存储 (S3 API)

- **WHEN** 用户配置后端为 S3-compatible direct transport
- **THEN** system SHALL use provider conditional-write semantics such as `If-Match`, ETag, or equivalent store revision when available
- **AND** unsupported conditional writes SHALL require a lock-object fallback before claiming success.

#### Scenario: 本地/网络挂载文件系统

- **WHEN** 用户配置后端为 `file://` 等 local object-store transport
- **THEN** system SHALL use atomic file/object update semantics for head CAS
- **AND** concurrent first commits or same-base updates SHALL allow at most one successful `remote_write=true`.

#### Scenario: Rclone provider lock fallback

- **WHEN** rclone direct transport lacks reliable conditional writes
- **THEN** it SHALL use `locks/commit.lock` with device id, request id, and expiry
- **AND** uncertain write state SHALL return retryable/diagnosable error with `remote_write=false`.

#### Scenario: Conservative fallback rejects unsafe direct writes

- **WHEN** a direct transport cannot prove provider CAS support and cannot acquire or verify the lock-object fallback
- **THEN** sync apply SHALL return a structured retryable/diagnosable error
- **AND** it SHALL preserve local vault state and output `remote_write=false`
- **AND** it SHALL NOT silently no-op, create dummy revisions, or mark sync-state as successfully committed.

#### Scenario: 动态 URI Scheme 注册路由

- **WHEN** system loads storage media
- **THEN** it SHALL use registered scheme factories rather than hardcoding transport behavior in config parsing
- **AND** unsupported schemes SHALL return stable unsupported errors instead of writable no-op stores.

## ADDED Requirements

### Requirement: Server commit metadata is sufficient and redacted

Pinax Cloud Server commits SHALL carry enough non-secret metadata to audit durable revision writes, diagnose retries, and correlate device activity without exposing user content or provider secrets.

#### Scenario: Server commit records blob and device evidence

- **WHEN** server transport commits a revision for a device
- **THEN** the commit request or result SHALL include workspace id, vault id, device id, request/idempotency id, base revision, committed revision id, manifest id, blob count or blob digest-set reference, status, and timestamp
- **AND** audit/log/receipt surfaces SHALL be able to correlate the commit without reading encrypted blob contents.

#### Scenario: Server commit metadata remains redacted

- **WHEN** commit metadata is written to server audit, local receipts, stdout, stderr, events, fixtures, object metadata, or diagnostics
- **THEN** it SHALL NOT contain plaintext note body, plaintext protected path, raw token, Authorization header, Cookie, raw secret ref, provider stderr, or provider payload.

### Requirement: Cloud Sync execution has one live owner

Cloud Sync push, pull, diff, status, and conflict execution SHALL be owned by the application service and `internal/cloudsync` protocol, without a second dead executor path.

#### Scenario: Dead sync executor is removed

- **WHEN** maintainers complete the review fixes
- **THEN** obsolete sync executor code that is no longer called by push/pull/diff SHALL be deleted rather than kept as a compatibility shim
- **AND** remaining tests SHALL cover behavior through the live app-service/protocol path.

#### Scenario: No stale executor callsites remain

- **WHEN** maintainers search sync execution callsites after cleanup
- **THEN** CLI, RPC, embedded Go API, and tests SHALL route through the same app-service/protocol owner
- **AND** no alias, shim, or stale import SHALL keep the removed executor reachable.

### Requirement: Cloud Sync docs and specs match current transport truth

Cloud Sync docs and OpenSpec wording SHALL describe the current wired transports and the durable-write gate without stale no-op/unimplemented language.

#### Scenario: Server and rclone are not described as unimplemented paths

- **WHEN** docs or specs describe Cloud Sync server or rclone direct transports
- **THEN** they SHALL state that server uses `internal/cloudclient.Transport` and rclone/file/S3 direct paths share `internal/cloudsync.ObjectStoreTransport`
- **AND** they SHALL NOT describe these paths as guarded no-ops or inherently forced to `remote_write=false` after a successful durable commit.

#### Scenario: Durable write gate remains explicit

- **WHEN** docs or specs describe dry-run, plan, pull, blob-only upload, unsupported path, or uncertain direct fallback behavior
- **THEN** they SHALL state that these paths remain `remote_write=false`
- **AND** only durable revision commit plus local sync-state evidence MAY produce `remote_write=true`.