## ADDED Requirements

### Requirement: Root help uses workflow groups
Pinax SHALL render root help as a scannable workflow map with grouped primary commands instead of a flat list of every executable command.

#### Scenario: Root help displays grouped primary commands
- **WHEN** a user runs `pinax --help`
- **THEN** stdout SHALL include Chinese group headings for vault, notes, organization, automation, configuration, and integration workflows
- **AND** each visible command SHALL appear under exactly one group
- **AND** compatibility-only aliases SHALL NOT appear in the root primary command list.

#### Scenario: Root help remains plain terminal text
- **WHEN** a user runs `pinax --help` in a plain terminal
- **THEN** stdout SHALL remain readable without ANSI color or box drawing characters
- **AND** command names, flag names, JSON fields, and protocol names SHALL remain stable English.

### Requirement: Compatibility aliases stay executable but hidden from primary help
Pinax SHALL keep compatibility aliases executable while hiding them from primary help when Cobra supports it.

#### Scenario: Vault root aliases are hidden but compatible
- **WHEN** a user runs `pinax --help`
- **THEN** root aliases `stats`, `validate`, `doctor`, and `dashboard` SHALL NOT appear as primary root commands
- **AND** `pinax stats --json`, `pinax validate --json`, and `pinax doctor --json` SHALL still execute with the same machine command names and facts as their `pinax vault ...` primary paths.

#### Scenario: Dimension root aliases are hidden but compatible
- **WHEN** a user runs `pinax --help`
- **THEN** root aliases `tag`, `folder`, `kind`, and `group` SHALL NOT appear as primary root commands
- **AND** `pinax tag list --json`, `pinax folder list --json`, `pinax kind list --json`, and `pinax group list --json` SHALL remain compatible with the corresponding `pinax note tags|folders|kinds|groups --json` paths.

#### Scenario: Storage direct set aliases are hidden but compatible
- **WHEN** a user runs `pinax storage --help`
- **THEN** compatibility commands `set-local` and `set-s3` SHALL NOT appear as primary storage commands
- **AND** `pinax storage set-local --json` and `pinax storage set-s3 --json` SHALL remain executable and preserve machine output compatibility.

#### Scenario: Organize suggest alias is hidden but compatible
- **WHEN** a user runs `pinax organize --help`
- **THEN** compatibility command `suggest` SHALL NOT appear as a primary organize command
- **AND** `pinax organize suggest --json` SHALL remain executable and preserve machine output compatibility with the reviewable organize plan workflow.

### Requirement: Help examples prefer primary paths
Pinax SHALL prefer primary command paths in help examples, error next actions, and user-facing docs.

#### Scenario: Help and errors recommend primary paths
- **WHEN** a user reads help examples or receives a user-facing next action for vault, organization dimensions, storage backend configuration, journal, or organize workflows
- **THEN** the recommended command SHALL use `pinax vault ...`, `pinax note ...`, `pinax storage set <backend>`, `pinax journal ...`, or `pinax organize plan --save`
- **AND** compatibility paths SHALL only appear in explicit migration or compatibility documentation.

## MODIFIED Requirements

### Requirement: Pinax exposes a scannable primary command tree
Pinax SHALL organize commands around user workflows and operational domains rather than exposing every internal module as an equally prominent root command.

#### Scenario: Root help emphasizes primary groups
- **WHEN** a user runs `pinax --help`
- **THEN** the help output SHALL include primary groups for local vault management, notes, journal, inbox, search, saved views, organization, templates, config, storage/backend, index, sync, Git protection, MCP, briefing, cloud, and planning workflows
- **AND** it SHALL keep compatibility-only aliases out of the primary command list when Cobra supports hiding them.

#### Scenario: Commands remain Chinese for humans
- **WHEN** a user reads help, usage, examples, flag descriptions, or argument errors
- **THEN** human-facing text SHALL remain Chinese
- **AND** command names, flag names, JSON fields, agent keys, and protocol identifiers SHALL remain stable English.
