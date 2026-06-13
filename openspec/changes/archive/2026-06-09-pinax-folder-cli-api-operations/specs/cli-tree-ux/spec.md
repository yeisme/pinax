## ADDED Requirements

### Requirement: Folder operations have a primary root command group
Pinax SHALL expose `pinax folder` as the primary command group for vault directory lifecycle operations while preserving `pinax note folders` as a note dimension browsing command.

#### Scenario: Root help includes folder operations
- **WHEN** a user runs `pinax --help`
- **THEN** root help SHALL include `folder` in the vault or organization workflow group
- **AND** help SHALL describe it as directory management rather than a note dimension alias.

#### Scenario: Note folder dimensions remain available
- **WHEN** a user runs `pinax note folders --vault ./my-notes --json`
- **THEN** Pinax SHALL continue to list note folder dimensions
- **AND** it SHALL NOT require users to switch to `pinax folder list` for ordinary note dimension browsing.

#### Scenario: Folder help discourages raw filesystem mutation
- **WHEN** a user runs `pinax folder --help`
- **THEN** help examples SHALL prefer commands such as `pinax folder create`, `pinax folder rename --dry-run`, and `pinax folder delete --empty-only --yes`
- **AND** help SHALL NOT instruct users or agents to run raw `mkdir`, `mv`, `rm`, or directly edit `.pinax` metadata.
