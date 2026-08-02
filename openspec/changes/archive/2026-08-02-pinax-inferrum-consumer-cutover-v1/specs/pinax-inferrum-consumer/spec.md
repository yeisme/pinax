## ADDED Requirements

### Requirement: Pinax KB SHALL consume Inferrum
Pinax KB semantic adapters SHALL import `github.com/yeisme/inferrum`, execute `inferrum-lancedb-sidecar`, and use `inferrum.sidecar.v1` without requiring the retired Lance module, executable, protocol, or error namespace.

#### Scenario: KB dependency resolves through Inferrum
- **WHEN** Pinax builds and tests its KB semantic packages
- **THEN** dependency resolution SHALL use `github.com/yeisme/inferrum`
- **AND** no `github.com/yeisme/lance` replace or import SHALL remain

#### Scenario: sidecar failures use the Inferrum namespace
- **WHEN** the sidecar is unavailable, times out, exceeds output bounds, or returns invalid protocol
- **THEN** Pinax SHALL receive/match `inferrum_sidecar_*` owner errors
- **AND** Pinax stable error projection SHALL remain redacted and behaviorally compatible

### Requirement: Pinax KB behavior SHALL survive the dependency cutover
The migration SHALL preserve generation staging, exact model identity, fail-closed permissions, evaluation receipts, activation decisions, safe citations, retrieval behavior, and LanceDB storage layout.

#### Scenario: existing generation and evaluation tests pass
- **WHEN** Pinax runs semantic, app, command, and evidence tests against Inferrum
- **THEN** existing business acceptance SHALL remain unchanged
- **AND** only owner module/package/protocol identity SHALL differ

### Requirement: Active Pinax documentation SHALL identify Inferrum as the owner
Non-archived Pinax docs and OpenSpec SHALL use Inferrum for the shared vector/RAG owner and SHALL reserve LanceDB terms for the third-party database.

#### Scenario: active-document audit passes
- **WHEN** non-archived Pinax files are searched for retired product contracts
- **THEN** old module/path/protocol/executable names SHALL be absent
- **AND** valid LanceDB technology references MAY remain
