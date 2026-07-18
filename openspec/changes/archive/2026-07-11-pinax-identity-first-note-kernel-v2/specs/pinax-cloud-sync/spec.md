## ADDED Requirements

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
