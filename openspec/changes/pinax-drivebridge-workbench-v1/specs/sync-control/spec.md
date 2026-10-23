## ADDED Requirements

### Requirement: Sync control contract

Pinax SHALL expose typed `sync.status`, `sync.diff`, `sync.push`, `sync.pull`, `sync.conflicts.list`, `sync.conflicts.diff` and `sync.conflicts.resolve` contracts through the transport catalog, REST/RPC bindings and OpenAPI schema. The contracts SHALL delegate to the existing Capsa sync application service and SHALL preserve target, base revision, remote revision, conflict and delete-marker facts.

#### Scenario: Read sync status and plan

- **WHEN** an authorized consumer requests status or a dry-run diff for a bound workspace
- **THEN** the response contains local/remote revision, recent run, conflicts, up-to-date state and recovery actions without exposing vault paths, tokens or provider payloads

### Requirement: Durable remote write fact

Pinax SHALL report `remote_write=true` only after the remote durable commit succeeds, a remote revision is returned, and a local read-back verifies the committed state. Local projection, DriveBridge upload or hydrate SHALL NOT create this fact.

#### Scenario: Remote commit is uncertain

- **WHEN** a push or pull response is lost, conflicts, or cannot complete read-back
- **THEN** Pinax returns or preserves `reconcile_required`/unknown state and the consumer uses the original operation or sync run identity for recovery instead of creating a new retry key

### Requirement: Human-confirmed sync mutations

Push, pull and conflict resolve SHALL remain behind the existing write gate and explicit confirmation. Conflict resolution SHALL preserve keep-local/keep-remote semantics and SHALL reject missing or ambiguous targets.

#### Scenario: Read-only consumer attempts push

- **WHEN** a session without write admission or explicit confirmation requests a mutation
- **THEN** the request is rejected without changing vault, Capsa revision, delete markers or operation state
