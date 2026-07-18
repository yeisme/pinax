## ADDED Requirements

### Requirement: Cloud Sync propagates explicit delete tombstones
Cloud Sync SHALL represent deletions as encrypted tombstone/delete marker entries in the committed manifest instead of inferring deletion from missing content entries.

#### Scenario: Push includes delete marker after project delete
- **GIVEN** device A deletes project `history` through `pinax project delete history --yes`
- **WHEN** device A runs `pinax sync push --target cloud --vault ./device-a --yes --json`
- **THEN** the encrypted manifest SHALL include a delete marker for object id `project/history` or its path hash
- **AND** protected stdout, receipts, object keys, and object metadata SHALL NOT expose plaintext note bodies, tokens, Authorization headers, or provider payloads
- **AND** `remote_write=true` SHALL only appear after the transport commits the revision successfully.

#### Scenario: Pull applies delete marker to local registry and index
- **GIVEN** device B has project `history` active locally
- **AND** the remote committed revision contains a delete marker for `project/history`
- **WHEN** device B runs `pinax sync pull --target cloud --vault ./device-b --yes --json`
- **THEN** Pinax SHALL move or mark the local project as trashed through the trash service
- **AND** `pinax project list --vault ./device-b --json` SHALL exclude `history`
- **AND** `pinax trash list --vault ./device-b --json` SHALL include the tombstone.

#### Scenario: Pull preserves local conflicting edit before applying delete
- **GIVEN** device B has unsynced local changes under a subproject workspace
- **AND** the remote revision deletes that same subproject
- **WHEN** device B pulls the remote revision
- **THEN** Pinax SHALL preserve the local changed files as conflict copies or a conflict trash backup
- **AND** it SHALL report conflict next actions without silently discarding the local content.

### Requirement: Cloud Sync transfers encrypted trash backups
Cloud Sync SHALL transfer recoverable trash backup blobs when a deletion is synchronized, subject to encryption and redaction rules.

#### Scenario: Trash backup is uploaded as encrypted content
- **GIVEN** deleting `subproject/history-learning/history-info` created a trash backup
- **WHEN** the next Cloud Sync push commits a revision
- **THEN** the trash backup SHALL be uploaded as encrypted blob content referenced by the delete marker
- **AND** the remote transport SHALL NOT receive plaintext paths inside object keys or plaintext file bodies in metadata.

#### Scenario: Missing trash backup is diagnosable
- **GIVEN** a local tombstone references a trash backup path that no longer exists
- **WHEN** the user runs `pinax sync push --target cloud --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return partial status with stable issue code `trash_backup_missing`
- **AND** it SHALL NOT claim that the deletion is safely recoverable on another device.
