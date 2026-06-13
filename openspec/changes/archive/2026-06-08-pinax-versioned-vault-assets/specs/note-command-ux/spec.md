## ADDED Requirements

### Requirement: Note reference commands use the shared vault object resolver
Pinax SHALL resolve note references consistently across note read/show/link/backlink/mutation commands using the shared resolver.

#### Scenario: Show note by filename stem
- **WHEN** a user runs `pinax note show yeisme --vault ./my-notes --json`
- **THEN** Pinax SHALL match a unique registered note by note id, path, filename, stem, title, or alias
- **AND** stdout SHALL include resolver facts such as match field and candidate count.

#### Scenario: Ambiguous note mutation is rejected
- **WHEN** a user runs `pinax note rename yeisme "New" --vault ./my-notes --json`
- **AND** multiple registered notes match `yeisme`
- **THEN** Pinax SHALL fail with stable error code `note_ref_ambiguous`
- **AND** stdout SHALL include candidates without modifying note files, record events, index rows, or version state.

### Requirement: Metadata planning accepts optional note query
Pinax SHALL allow metadata planning to target one resolved note or adoptable Markdown candidate while preserving full-vault planning when no query is provided.

#### Scenario: Plan metadata for one file
- **WHEN** a user runs `pinax metadata plan yeisme --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve `yeisme` through registered-or-adoptable scope
- **AND** stdout SHALL contain metadata operations only for that resolved object.

#### Scenario: Metadata plan does not adopt unmanaged files implicitly
- **WHEN** `pinax metadata plan yeisme --vault ./my-notes --json` resolves an unmanaged Markdown file
- **THEN** Pinax SHALL report mirror operations and a next action for `pinax record adopt yeisme --plan`
- **AND** it SHALL NOT create record ledger events unless an explicit record adopt apply command is approved.

