## ADDED Requirements

### Requirement: Ledger records vault object tombstones
Pinax SHALL extend ledger tombstone evidence beyond notes so project, subproject, registry asset, view, template, and future structured object deletions are replayable and restorable.

#### Scenario: Append project tombstone event
- **WHEN** a project delete operation succeeds
- **THEN** Pinax SHALL append a redacted lifecycle event for object id `project/<slug>`
- **AND** materialized tombstone state SHALL include object kind, object id, previous registry facts, trash path, deleted time, source command, and version evidence.

#### Scenario: Replay hides deleted objects
- **GIVEN** ledger replay sees a project tombstone event for `project/history`
- **WHEN** Pinax materializes project registry projections
- **THEN** active registry projections SHALL exclude `history`
- **AND** trash projections SHALL include the tombstone until restore or purge.

#### Scenario: Restore appends lifecycle event
- **WHEN** a user restores `project/history` from trash
- **THEN** Pinax SHALL append a restore lifecycle event
- **AND** replay SHALL remove the active tombstone and restore the project registry entry when no conflict exists.
