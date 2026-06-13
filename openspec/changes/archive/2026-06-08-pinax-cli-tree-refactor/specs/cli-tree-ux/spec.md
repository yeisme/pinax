## ADDED Requirements

### Requirement: Pinax exposes a scannable primary command tree
Pinax SHALL organize commands around user workflows and operational domains rather than exposing every internal module as an equally prominent root command.

#### Scenario: Root help emphasizes primary groups
- **WHEN** a user runs `pinax --help`
- **THEN** the help output SHALL include primary groups for local vault management, notes, journal, inbox, search, saved views, organization, templates, config, storage, index, sync, Git protection, and MCP
- **AND** it SHALL keep compatibility-only aliases out of the primary command list when Cobra supports hiding them.

#### Scenario: Commands remain Chinese for humans
- **WHEN** a user reads help, usage, examples, flag descriptions, or argument errors
- **THEN** human-facing text SHALL remain Chinese
- **AND** command names, flag names, JSON fields, agent keys, and protocol identifiers SHALL remain stable English.

### Requirement: Pinax groups vault operations under vault commands
Pinax SHALL provide `pinax vault` as the primary location for vault-wide status, statistics, validation, doctor, and dashboard operations.

#### Scenario: Vault status commands are reachable under vault
- **WHEN** a user runs `pinax vault stats --vault ./my-notes`, `pinax vault validate --vault ./my-notes`, or `pinax vault doctor --vault ./my-notes`
- **THEN** Pinax SHALL execute the same application services as the existing root-level commands
- **AND** all output modes SHALL use the same command projection and renderer contract.

#### Scenario: Existing root vault commands stay compatible
- **WHEN** a user runs existing commands `pinax stats`, `pinax validate`, or `pinax doctor`
- **THEN** Pinax SHALL preserve backwards-compatible behavior and machine output fields
- **AND** it MAY mark those root commands as compatibility aliases in help.

### Requirement: Pinax groups journal operations under journal commands
Pinax SHALL provide `pinax journal daily|weekly|monthly` as the primary location for journal open, show, and append workflows.

#### Scenario: Journal primary paths work
- **WHEN** a user runs `pinax journal daily show --vault ./my-notes`, `pinax journal weekly append --body x --vault ./my-notes`, or `pinax journal monthly open --vault ./my-notes`
- **THEN** Pinax SHALL execute the corresponding daily, weekly, or monthly journal workflow
- **AND** output SHALL follow the same human and machine contracts as the existing commands.

#### Scenario: Existing journal root commands stay compatible
- **WHEN** a user runs existing commands `pinax daily show`, `pinax weekly show`, or `pinax monthly show`
- **THEN** Pinax SHALL preserve backwards-compatible behavior, flags, and output facts
- **AND** it MAY hide those aliases from primary root help after the new journal tree exists.

### Requirement: Pinax keeps note workflows stable while reducing root noise
Pinax SHALL keep note creation, reading, editing, relationships, attachments, tagging, moving, archiving, and deletion under `pinax note` while avoiding additional root-level note-specific commands.

#### Scenario: Note aliases share implementation
- **WHEN** a user runs `pinax note create`, `pinax note new`, `pinax note show`, `pinax note read`, `pinax note edit`, or `pinax note open`
- **THEN** aliases SHALL call the same command implementation and application service as their primary counterpart
- **AND** they SHALL produce equivalent projections for the same request.

#### Scenario: Dimension browsing moves out of root prominence
- **WHEN** a user needs to inspect tags, folders, kinds, or groups
- **THEN** Pinax SHALL expose those dimensions through note or view oriented commands such as `pinax note tags`, `pinax note folders`, `pinax note kinds`, or equivalent saved-view commands
- **AND** old root dimension paths MAY remain as compatibility aliases.

### Requirement: Pinax standardizes planning command semantics
Pinax SHALL use consistent `plan`, `list`, and `apply` command semantics for workflows that preview changes before writing.

#### Scenario: Organize uses plan list apply as primary flow
- **WHEN** a user runs `pinax organize plan`, `pinax organize list`, or `pinax organize apply`
- **THEN** Pinax SHALL support the preview, saved plan listing, and protected apply workflow through those primary verbs
- **AND** existing `pinax organize suggest` SHALL remain compatible or clearly documented as an alias.

#### Scenario: Protected apply commands keep safety gates
- **WHEN** a user runs `pinax metadata apply`, `pinax repair apply`, or `pinax organize apply`
- **THEN** CLI tree refactoring SHALL NOT weaken dry-run, `--yes`, Git snapshot, vault boundary, or event evidence requirements.

### Requirement: Pinax groups storage backend updates under storage set
Pinax SHALL expose storage backend configuration through `pinax storage set <backend>` while preserving existing direct set commands.

#### Scenario: Storage set subcommands work
- **WHEN** a user runs `pinax storage set local --root ./vault-storage --vault ./my-notes` or `pinax storage set s3 --bucket notes --region us-east-1 --vault ./my-notes`
- **THEN** Pinax SHALL configure the selected backend through the same app services as existing storage commands
- **AND** it SHALL NOT persist provider secrets.

#### Scenario: Existing storage commands stay compatible
- **WHEN** a user runs `pinax storage set-local` or `pinax storage set-s3`
- **THEN** Pinax SHALL preserve backwards-compatible behavior and output contract.

### Requirement: Pinax command factories avoid duplicated behavior
Pinax SHALL build command groups through reusable factories so that primary paths and compatibility aliases share flags, validation, service calls, and rendering behavior.

#### Scenario: Root command creation is test-isolated
- **WHEN** tests create multiple root command instances
- **THEN** each instance SHALL have isolated flags, dependencies, output writers, and service references
- **AND** no global Cobra command or global pflag state SHALL leak between tests.

#### Scenario: Alias and primary path produce the same machine output
- **WHEN** a primary command and its compatibility alias are run with the same arguments and `--json`
- **THEN** both stdout payloads SHALL be valid JSON envelopes with equivalent status, facts, data, error, and actions
- **AND** diagnostics SHALL remain on stderr.

### Requirement: Pinax updates command help and completion safely
Pinax SHALL update help and completion behavior to favor the primary command tree without triggering writes or remote operations.

#### Scenario: Completion is lightweight
- **WHEN** shell completion is invoked for Pinax commands
- **THEN** completion handlers SHALL NOT write vault files, `.pinax` metadata, Git state, provider state, or remote systems.

#### Scenario: Help examples use primary paths
- **WHEN** a user reads help examples for vault, journal, storage, or organize workflows
- **THEN** examples SHALL prefer the new primary command paths
- **AND** compatibility paths SHALL be documented only where needed for migration clarity.
