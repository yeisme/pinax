## ADDED Requirements

### Requirement: Server sync client SHALL wait for local proof-loop safety gates

Pinax server sync client implementation SHALL not become the first writer that lacks local restore, shared redaction and proof-run evidence gates.

#### Scenario: Local safety gate precedes server mutation

- **GIVEN** server sync client work mutates local vault state based on remote revision state
- **WHEN** implementation begins
- **THEN** `pinax-proof-loop-operational-hardening` SHALL be complete or explicitly waived with reviewed evidence
- **AND** server sync output SHALL use the same projection redaction gate as local proof-loop commands.

## MODIFIED Requirements

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
