# pinax-cloud-sync Delta Spec

## ADDED Requirements

### Requirement: 本地后台实时同步进程

Pinax SHALL provide an explicitly managed local sync daemon for a configured vault. The daemon SHALL reuse the existing Cloud Sync push/pull/conflict engine and SHALL NOT introduce a separate synchronization protocol or bypass existing approval, receipt, redaction, and `remote_write=true` rules.

#### Scenario: 前台运行 daemon 供 supervisor 托管

- **GIVEN** a vault has a configured Cloud Sync backend
- **WHEN** the user runs `pinax sync daemon run --target cloud --vault <vault> --yes --json`
- **THEN** Pinax SHALL start a long-running local sync loop for that vault
- **AND** it SHALL acquire a per-vault daemon lock before writing daemon state
- **AND** it SHALL emit a `sync.daemon.run` projection without plaintext note bodies, raw secrets, provider payloads, or private tool arguments.

#### Scenario: 后台启动和状态查询

- **GIVEN** no daemon is running for the vault
- **WHEN** the user runs `pinax sync daemon start --target cloud --vault <vault> --yes --json`
- **THEN** Pinax SHALL start a background runner or report a platform-specific partial status with a next action that uses `pinax sync daemon run --target cloud --vault <vault> --yes`
- **AND** `pinax sync daemon status --vault <vault> --json` SHALL report running state, pid when available, target, backend kind, detection mode, sync state, last success, last error code, and conflict count.

#### Scenario: 单 vault 只允许一个 daemon 写同步

- **GIVEN** a daemon is already running for a vault
- **WHEN** another process runs `pinax sync daemon start --target cloud --vault <vault> --yes --json`
- **THEN** Pinax SHALL refuse the second daemon with a stable lock error or partial status
- **AND** it SHALL NOT run two concurrent Cloud Sync writers against the same vault.

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

#### Scenario: status 输出 bounded facts

- **WHEN** the user runs `pinax sync daemon status --vault <vault> --json`
- **THEN** the output SHALL include bounded facts such as running state, target, backend kind, detection mode, sync state, last success, last error code, local pending state, remote revision id, and conflict count
- **AND** the output SHALL NOT include plaintext note body, raw secret ref value, Authorization header, cookie, provider stderr payload, raw prompt, hidden system prompt, or private tool argument.

#### Scenario: daemon events use stable additive types

- **WHEN** the daemon starts, detects local changes, detects remote changes, starts sync, succeeds, fails, pauses on conflict, or stops
- **THEN** Pinax SHALL emit redacted event types under the `sync.daemon.*` namespace
- **AND** older consumers SHALL be able to ignore these new event types without changing existing `pinax sync push|pull` behavior.

