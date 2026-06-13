## ADDED Requirements

### Requirement: Index help presents a maintenance workflow
Pinax SHALL present `pinax index` help as a small maintenance workflow with primary commands ordered by user decision flow rather than by implementation history.

#### Scenario: Index help orders commands by workflow
- **WHEN** a user runs `pinax index --help`
- **THEN** stdout SHALL show Chinese descriptions for the primary flow `status`, `refresh`, `doctor`, `rebuild`, `sync`, and `repair`
- **AND** the help text SHALL explain that `refresh` is the ordinary low-cost maintenance action and `rebuild` is the full reset action.

#### Scenario: Index help includes safe examples
- **WHEN** a user reads `pinax index --help`
- **THEN** examples SHALL include `pinax index`, `pinax index refresh --vault ./my-notes`, `pinax index doctor --vault ./my-notes`, and `pinax index rebuild --vault ./my-notes`
- **AND** examples SHALL NOT require real provider credentials, external network access, or a real user vault.

### Requirement: Index commands recommend next actions consistently
Pinax SHALL use primary index command paths in help examples, errors, and command actions.

#### Scenario: Status recommends refresh before rebuild when safe
- **WHEN** `pinax index status` detects missing or stale projection that can be reconciled incrementally
- **THEN** the user-facing next action SHALL recommend `pinax index refresh --vault <vault>`
- **AND** machine output SHALL expose the action as a stable `action.refresh` entry.

#### Scenario: Doctor recommends repair or rebuild for structural problems
- **WHEN** `pinax index doctor` detects unreadable, corrupt, or incompatible projection state
- **THEN** the user-facing next action SHALL recommend `pinax index repair --kind recreate --dry-run` or `pinax index rebuild`
- **AND** it SHALL NOT recommend direct editing or deleting `.pinax/index.sqlite` by hand.

### Requirement: Compatibility behavior remains script-safe
Pinax SHALL preserve existing index command machine contracts while adding the new user-friendly commands.

#### Scenario: Existing index commands keep machine command names
- **WHEN** a script runs `pinax index init --json`, `pinax index status --json`, `pinax index sync --json`, or `pinax index rebuild --json`
- **THEN** JSON `command` values SHALL remain `index.init`, `index.status`, `index.sync`, and `index.rebuild`
- **AND** existing stable facts SHALL remain available unless a major output contract version is introduced.

#### Scenario: New index commands use stable names
- **WHEN** a user runs `pinax index refresh --json`, `pinax index doctor --json`, or `pinax index repair --json`
- **THEN** JSON `command` values SHALL be `index.refresh`, `index.doctor`, and `index.repair`
- **AND** agent keys SHALL be stable English keys suitable for shell glue and automation.
