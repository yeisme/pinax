## ADDED Requirements

### Requirement: Note list supports notebook organization filters
Pinax SHALL allow note listing to filter by notebook organization dimensions used by the core workflows.

#### Scenario: List notes by folder and kind
- **WHEN** a user runs `pinax note list --group work --folder inbox --kind reference --status active --vault ./my-notes --json`
- **THEN** Pinax SHALL return only notes matching the selected group, folder, kind, and status
- **AND** JSON facts SHALL include each selected filter using stable keys.

#### Scenario: List notes by date range
- **WHEN** a user runs `pinax note list --created-after 2026-01-01 --updated-before 2026-02-01 --vault ./my-notes --json`
- **THEN** Pinax SHALL filter notes by frontmatter or filesystem timestamps when frontmatter is missing
- **AND** invalid date values SHALL fail with stable error code `invalid_date_filter`.

### Requirement: Note commands expose link and attachment subcommands
Pinax SHALL expose link, backlink, orphan, and attachment inspection from the note command surface.

#### Scenario: Note help includes relationship commands
- **WHEN** a user runs `pinax note --help`
- **THEN** help output SHALL include links, backlinks, orphans, attach, and attachments commands
- **AND** help text SHALL describe local Markdown note relationships.

#### Scenario: Note relationship commands follow output contract
- **WHEN** a user runs a note relationship command with `--agent` or `--json`
- **THEN** stdout SHALL contain only the selected machine format
- **AND** diagnostics SHALL go to stderr.

### Requirement: Note maintenance supports inbox triage semantics
Pinax SHALL let inbox triage reuse safe note move and metadata patch behavior.

#### Scenario: Move note while updating folder and kind
- **WHEN** a user runs `pinax note move note_123 work/ideas --kind reference --status active --vault ./my-notes --json`
- **THEN** Pinax SHALL move the note inside the vault and update selected frontmatter fields
- **AND** it SHALL preserve unknown frontmatter fields where practical.
