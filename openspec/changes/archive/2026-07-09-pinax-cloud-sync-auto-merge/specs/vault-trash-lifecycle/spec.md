## MODIFIED Requirements

### Requirement: Vault objects use a recoverable trash lifecycle

Pinax SHALL include notes in Cloud Sync delete marker handling so note soft deletes converge across devices without hard-deleting user content silently.

#### Scenario: Note soft delete produces a cloud delete marker

- **WHEN** a user runs `pinax note delete "Draft" --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL move the note to `.pinax/trash/`
- **AND** the next Cloud Sync manifest SHALL include a `note` delete marker for the old path and a trash backup blob.

#### Scenario: Remote note delete marker enters local trash

- **GIVEN** device A soft-deletes a note and pushes Cloud Sync
- **WHEN** device B runs `pinax sync pull --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL remove the active note path on device B
- **AND** it SHALL preserve the note body under local trash with a note tombstone.
