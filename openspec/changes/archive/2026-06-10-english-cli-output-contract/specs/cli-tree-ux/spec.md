# cli-tree-ux Delta Spec

## MODIFIED Requirements

### Requirement: Pinax exposes a scannable primary command tree

Pinax SHALL organize commands around user workflows and operational domains rather than exposing every internal module as an equally prominent root command, and SHALL present user-facing help chrome in English by default.

#### Scenario: Root help emphasizes primary groups

- **WHEN** a user runs `pinax --help`
- **THEN** the help output SHALL include English primary groups for local vault management, notes, journal, inbox, search, saved views, organization, templates, config, storage/backend, index, sync, Git protection, MCP, briefing, cloud, and planning workflows
- **AND** it SHALL keep compatibility-only aliases out of the primary command list when Cobra supports hiding them.

#### Scenario: Commands use English for humans

- **WHEN** a user reads help, usage, examples, flag descriptions, or argument errors
- **THEN** human-facing CLI chrome SHALL be English
- **AND** command names, flag names, JSON fields, agent keys, event types, schema keys, and protocol identifiers SHALL remain stable English or existing stable identifiers.

### Requirement: Root help uses workflow groups

Pinax SHALL render root help as a scannable workflow map with grouped primary commands instead of a flat list of every executable command.

#### Scenario: Root help displays grouped primary commands

- **WHEN** a user runs `pinax --help`
- **THEN** stdout SHALL include English group headings for vault, notes, organization, automation, configuration, and integration workflows
- **AND** each visible command SHALL appear under exactly one group
- **AND** compatibility-only aliases SHALL NOT appear in the root primary command list.

#### Scenario: Root help remains plain terminal text

- **WHEN** a user runs `pinax --help` in a plain terminal
- **THEN** stdout SHALL remain readable without ANSI color or box drawing characters
- **AND** command names, flag names, JSON fields, and protocol names SHALL remain stable English.

### Requirement: Index help presents a maintenance workflow

Pinax SHALL present `pinax index` help as a small maintenance workflow with primary commands ordered by user decision flow rather than by implementation history.

#### Scenario: Index help orders commands by workflow

- **WHEN** a user runs `pinax index --help`
- **THEN** stdout SHALL show English descriptions for the primary flow `status`, `refresh`, `doctor`, `rebuild`, `sync`, and `repair`
- **AND** the help text SHALL explain that `refresh` is the ordinary low-cost maintenance action and `rebuild` is the full reset action.

#### Scenario: Index help includes safe examples

- **WHEN** a user reads `pinax index --help`
- **THEN** examples SHALL include `pinax index`, `pinax index refresh --vault ./my-notes`, `pinax index doctor --vault ./my-notes`, and `pinax index rebuild --vault ./my-notes`
- **AND** examples SHALL NOT require real provider credentials, external network access, or a real user vault.

### Requirement: CLI tree exposes version and asset primary paths

Pinax SHALL expose version control and asset management as primary command groups with English help text and stable machine command names.

#### Scenario: Root help shows version and asset groups

- **WHEN** a user runs `pinax --help`
- **THEN** root help SHALL include `version` and `asset` in appropriate command groups
- **AND** it SHALL NOT show `git` as a primary command.

#### Scenario: Version help recommends safe workflows

- **WHEN** a user runs `pinax version --help`
- **THEN** help SHALL show status, snapshot, history, diff, show, changed, restore, and backends commands with English descriptions
- **AND** examples SHALL use `--plan`, `--yes`, and snapshot protection where writes are possible.

#### Scenario: Asset help avoids direct metadata editing

- **WHEN** a user runs `pinax asset --help`
- **THEN** help SHALL recommend `asset add/list/show/preview/link/backlinks/orphans/missing/move/remove/verify/repair` with English descriptions
- **AND** it SHALL NOT instruct users or agents to hand-edit `.pinax/assets/*.json`.

#### Scenario: Note attach help exposes attachment placement options

- **WHEN** a user runs `pinax note attach --help`
- **THEN** help SHALL describe `--placement`, `--link-style`, `--embed`, `--mode`, and `--rename` in English
- **AND** examples SHALL include `pinax note attach "Auth design" ./diagram.png --placement note-folder --embed --vault ./my-notes --json`.

#### Scenario: Note show help exposes attachment preview options

- **WHEN** a user runs `pinax note show --help`
- **THEN** help SHALL describe rendered preview flags such as `--embed-attachments`, `--max-embed-depth`, `--max-embed-bytes`, and `--max-preview-bytes` in English
- **AND** examples SHALL include `pinax note show "Auth design" --view rendered --embed-attachments markdown --vault ./my-notes`.

#### Scenario: Note preview is a readonly alias

- **WHEN** a user runs `pinax note preview --help`
- **THEN** help SHALL present it as a readonly rendered note preview command in English
- **AND** examples SHALL include `pinax note preview "Auth design" --embed-attachments markdown --vault ./my-notes`.
