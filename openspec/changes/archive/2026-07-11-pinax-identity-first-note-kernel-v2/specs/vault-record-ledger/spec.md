## ADDED Requirements

### Requirement: Record Ledger allocates and owns canonical object identity
The Record Ledger SHALL be the machine source of truth for canonical object identity and SHALL allocate or adopt an object ID exactly once. Application services SHALL receive the allocated ID from the ledger workflow and SHALL NOT reconstruct identity from path after allocation.

#### Scenario: Lifecycle services reuse the ledger identity
- **WHEN** note creation writes frontmatter, a record event, an index projection, a project item and an output projection
- **THEN** every write SHALL use the same ledger-issued object ID.

#### Scenario: Existing frontmatter disagrees with ledger
- **WHEN** frontmatter `note_id` differs from the active ledger object ID
- **THEN** Pinax SHALL keep the ledger identity authoritative, report a reviewable mismatch and SHALL NOT silently replace either value outside an approved migration or repair plan.

### Requirement: Record events preserve identity across lifecycle transitions
Rename, move, archive, trash, restore, classification and Tag events SHALL reference the canonical object ID and SHALL include before and after locator facts where applicable.

#### Scenario: Rename event preserves identity
- **WHEN** a note is renamed through Pinax
- **THEN** the appended event SHALL contain the same object ID, the old path, the new path and revision evidence, and ledger replay SHALL materialize one active record rather than a deleted record plus a created record.

### Requirement: Record replay detects identity corruption
Record replay SHALL detect duplicate canonical IDs, one object with multiple active paths, multiple objects with one active path, missing creation events and non-idempotent allocation attempts.

#### Scenario: Duplicate active identity is replayed
- **WHEN** ledger input contains two active records that claim the same canonical object ID with incompatible histories
- **THEN** replay SHALL stop the unsafe materialization, report a stable corruption code and provide a repair-plan next action without choosing a winner automatically.
