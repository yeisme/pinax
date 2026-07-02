## ADDED Requirements

### Requirement: Memory commands expose Local REST and RPC parity

Pinax SHALL expose the stable `memory` CLI capabilities through Local REST, RPC, and the remote capability registry.

#### Scenario: Memory list is available through REST and RPC
- **WHEN** a local client requests `GET /v1/memory` or `Pinax.Memory.List`
- **THEN** Pinax returns the same memory projection used by `pinax memory list --json`

#### Scenario: Memory capture supports dry-run preview without persistence
- **WHEN** a client submits `memory.capture` with `dry_run=true`
- **THEN** Pinax validates and returns the preview record without writing to the memory ledger

#### Scenario: Memory capture requires write confirmation for persistence
- **WHEN** a client submits `memory.capture` without `dry_run=true`
- **THEN** Pinax requires API write mode and `yes=true` before writing the record

#### Scenario: Memory recall, context, and stats are read-only routes
- **WHEN** a client calls recall, context, or stats through REST or RPC
- **THEN** Pinax treats the route as read-only and returns the corresponding memory projection

### Requirement: Remote CLI forwards stable memory commands

Pinax SHALL map `memory list`, `memory capture`, `memory recall`, `memory context`, and `memory stats` to Local API RPC methods when `--api-url` is configured.

#### Scenario: Remote memory command preserves CLI request fields
- **WHEN** a user runs `pinax --api-url http://127.0.0.1:8787 memory capture --type fact --subject pinax --predicate memory_capture_usage --object "Use --body or --subject and --object" --dry-run --json`
- **THEN** the remote request includes the same type, triple fields, dry-run flag, and JSON output mode
