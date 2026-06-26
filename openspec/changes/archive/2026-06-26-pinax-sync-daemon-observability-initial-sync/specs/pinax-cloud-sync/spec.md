# pinax-cloud-sync Delta Spec

## MODIFIED Requirements

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
