# pinax-cloud-sync Delta Spec

## ADDED Requirements

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
