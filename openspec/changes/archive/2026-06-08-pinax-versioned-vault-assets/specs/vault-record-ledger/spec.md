## ADDED Requirements

### Requirement: Record adoption accepts scoped vault object queries
Pinax SHALL allow record adoption to target a specific unresolved Markdown candidate while preserving full-vault adoption planning when no query is provided.

#### Scenario: Plan adoption for a single Markdown file
- **WHEN** a user runs `pinax record adopt yeisme --plan --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve `yeisme` using adoptable scope
- **AND** stdout SHALL include adoption operations only for the uniquely matched unmanaged Markdown file.
- **AND** no Markdown, record, asset, index, version, provider, or Git state SHALL be modified.

#### Scenario: Full-vault adoption remains available
- **WHEN** a user runs `pinax record adopt --plan --vault ./my-notes --json` without a query
- **THEN** Pinax SHALL scan all adoptable Markdown notes inside the vault boundary and return the full adoption plan.

### Requirement: Record events include version and asset evidence without payload leakage
Pinax SHALL attach version evidence and asset references to record events when available without embedding raw diff text or binary payload bytes.

#### Scenario: Note mutation records version evidence
- **WHEN** a CLI-approved note create, rename, move, archive, delete, restore, metadata, or adoption operation succeeds
- **THEN** the appended record event SHALL include backend type, snapshot or revision id when available, content hash, file size, ledger sequence, and worktree status when available
- **AND** it SHALL NOT depend on a system Git binary.

#### Scenario: Note references asset evidence
- **WHEN** a note operation creates or changes asset references
- **THEN** the record event SHALL include asset ids and content hashes as evidence refs
- **AND** it SHALL NOT write asset bytes into record event JSONL.

