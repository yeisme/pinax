# cli-tree-ux Specification

## Purpose
TBD - created by archiving change pinax-cli-tree-refactor. Update Purpose after archive.
## Requirements
### Requirement: Pinax exposes a scannable primary command tree

Pinax SHALL organize commands around user workflows and operational domains rather than exposing every internal module as an equally prominent root command, and SHALL present user-facing help chrome in Chinese by default.

#### Scenario: Root help emphasizes primary groups

- **WHEN** a user runs `pinax --help`
- **THEN** the help output SHALL include Chinese primary groups for local vault management, notes, journal, inbox, search, saved views, organization, templates, config, storage/backend, index, sync, Git protection, MCP, briefing, cloud, and planning workflows
- **AND** it SHALL keep compatibility-only aliases out of the primary command list when Cobra supports hiding them.

#### Scenario: Commands use Chinese for humans

- **WHEN** a user reads help, usage, examples, flag descriptions, or argument errors
- **THEN** human-facing CLI chrome SHALL be Chinese
- **AND** command names, flag names, JSON fields, agent keys, event types, schema keys, and protocol identifiers SHALL remain stable English or existing stable identifiers.

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
- **WHEN** a user runs `pinax note add`, `pinax note create`, `pinax note new`, `pinax note show`, `pinax note read`, `pinax note edit`, or `pinax note open`
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

#### Scenario: High-value object completion covers local workflow objects

- **GIVEN** a local vault contains projects, project subprojects, folders, backend profiles, prompt assets, plugins, and sync conflict files
- **WHEN** the shell requests completion for matching `project`, `folder`, `backend`, `prompt`, `plugin`, `collection`, `graph`, or `sync conflicts` commands
- **THEN** Pinax SHALL return matching local candidates with short non-sensitive descriptions
- **AND** profile completion SHALL NOT expose endpoints, raw tokens, Authorization headers, cookies, or secret values.

#### Scenario: Path-like flags keep file completion

- **WHEN** the shell requests completion for path-like flags such as `--from`, `--to`, `--api-token-file`, `--root`, or `sync conflicts resolve --merged`
- **THEN** Pinax SHALL leave shell file completion enabled unless the command has an explicit safe object registry for that argument.

#### Scenario: Rendering and enum flags complete statically

- **WHEN** the shell requests completion for bounded enum flags such as `--color`, `--theme`, `--markdown-style`, lifecycle, collection export format, graph node kind, or project board display fields
- **THEN** Pinax SHALL return the documented enum values with `ShellCompDirectiveNoFileComp`.

### Requirement: CLI tree exposes version and asset primary paths

Pinax SHALL expose version control and asset management as primary command groups with Chinese help text and stable machine command names.

#### Scenario: Root help shows version and asset groups

- **WHEN** a user runs `pinax --help`
- **THEN** root help SHALL include `version` and `asset` in appropriate command groups
- **AND** it SHALL NOT show `git` as a primary command.

#### Scenario: Version help recommends safe workflows

- **WHEN** a user runs `pinax version --help`
- **THEN** help SHALL show status, snapshot, history, diff, show, changed, restore, and backends commands with Chinese descriptions
- **AND** examples SHALL use `--plan`, `--yes`, and snapshot protection where writes are possible.

#### Scenario: Asset help avoids direct metadata editing

- **WHEN** a user runs `pinax asset --help`
- **THEN** help SHALL recommend `asset add/list/show/preview/link/backlinks/orphans/missing/move/remove/verify/repair` with Chinese descriptions
- **AND** it SHALL NOT instruct users or agents to hand-edit `.pinax/assets/*.json`.

#### Scenario: Note attach help exposes attachment placement options

- **WHEN** a user runs `pinax note attach --help`
- **THEN** help SHALL describe `--placement`, `--link-style`, `--embed`, `--mode`, and `--rename` in Chinese
- **AND** examples SHALL include `pinax note attach "Auth design" ./diagram.png --placement note-folder --embed --vault ./my-notes --json`.

#### Scenario: Note show help exposes attachment preview options

- **WHEN** a user runs `pinax note show --help`
- **THEN** help SHALL describe rendered preview flags such as `--embed-attachments`, `--max-embed-depth`, `--max-embed-bytes`, and `--max-preview-bytes` in Chinese
- **AND** examples SHALL include `pinax note show "Auth design" --view rendered --embed-attachments markdown --vault ./my-notes`.

#### Scenario: Note preview is a readonly alias

- **WHEN** a user runs `pinax note preview --help`
- **THEN** help SHALL present it as a readonly rendered note preview command in Chinese
- **AND** examples SHALL include `pinax note preview "Auth design" --embed-attachments markdown --vault ./my-notes`.

### Requirement: Existing actions migrate from git to version wording
Pinax SHALL update user-facing next actions, errors, examples, and docs to use `version` terminology for snapshot and history workflows.

#### Scenario: Snapshot-required error uses version command
- **WHEN** a command fails with `snapshot_required`
- **THEN** stdout SHALL include a next action using `pinax version snapshot --vault <vault> --message <message>`
- **AND** machine facts MAY include `version_backend` but SHALL NOT require users to run `pinax git`.

#### Scenario: Hidden git alias is absent from primary help
- **WHEN** a user runs `pinax git --help` during compatibility period
- **THEN** Pinax MAY show a deprecation message or route to version snapshot help
- **AND** `pinax --help` SHALL keep the git alias hidden from primary navigation.

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

### Requirement: Pinax exposes hidden API schema discovery alias

Pinax SHALL preserve `pinax api schema export` as the primary API schema path while accepting a hidden root `pinax schema export` compatibility path for users who naturally search for schema from the root command tree.

#### Scenario: Root schema alias exports API schema

- **WHEN** a user runs `pinax schema export --format openapi --vault ./my-notes --json`
- **THEN** stdout SHALL contain the same JSON envelope command and facts as `pinax api schema export --format openapi --vault ./my-notes --json`
- **AND** the command SHALL NOT write vault files, `.pinax` metadata, Git state, provider state, or remote systems.

#### Scenario: Root schema alias stays hidden from primary help

- **WHEN** a user runs `pinax --help`
- **THEN** root help SHALL NOT list `schema` as a primary command
- **AND** `pinax schema --help` SHALL show a runnable `pinax schema export` example.

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

### Requirement: Pinax exposes draft as a primary workflow command

Pinax SHALL expose draft management as a primary command group with Chinese human help and stable machine output fields.

#### Scenario: Root help includes draft workflow
- **WHEN** a user runs `pinax --help`
- **THEN** the primary workflow command list SHALL include `draft`
- **AND** the help text SHALL describe it as 草稿箱 or draft review workflow rather than a low-level folder operation.

#### Scenario: Draft command help lists lifecycle actions
- **WHEN** a user runs `pinax draft --help`
- **THEN** stdout SHALL list `create`, `list`, `show`, `promote`, `archive`, `discard`, and `index` or equivalent review index subcommands
- **AND** examples SHALL use user-runnable commands with `--vault ./my-notes`.

#### Scenario: Draft command output follows shared contract
- **WHEN** a user runs `pinax draft list --vault ./my-notes --agent`
- **THEN** stdout SHALL contain stable key=value facts including `spec_version`, `mode=agent`, `command=draft.list`, `status`, `fact.returned`, `fact.total`, and `fact.filter.status=draft`
- **AND** stdout SHALL NOT contain localized prose, ANSI decorations, debug logs, raw prompts, provider payloads, or secrets.

### Requirement: Inbox command exposes review and index aliases

Pinax SHALL keep existing inbox capture/list/triage behavior compatible and add review-oriented actions under the same command group.

#### Scenario: Inbox help lists review actions
- **WHEN** a user runs `pinax inbox --help`
- **THEN** stdout SHALL include `capture`, `list`, `show`, `triage`, `promote`, `discard`, and `index` or equivalent review index actions
- **AND** existing command examples for `capture`, `list`, and `triage` SHALL remain runnable.

#### Scenario: Inbox index alias uses canonical index page projection
- **WHEN** a user runs `pinax inbox index preview --vault ./my-notes --json`
- **THEN** Pinax SHALL render the same canonical projection as `pinax index page preview inbox --template index.inbox --vault ./my-notes --json`
- **AND** machine output SHALL include stable facts for workflow `inbox`, page name, template, writes, target path, and managed block count.

#### Scenario: Draft index alias uses canonical index page projection
- **WHEN** a user runs `pinax draft index preview --vault ./my-notes --json`
- **THEN** Pinax SHALL render the same canonical projection as `pinax index page preview drafts --template index.drafts --vault ./my-notes --json`
- **AND** it SHALL NOT create or modify Markdown, `.pinax` structured assets, Git state, provider state, remote services, or index projection.

### Requirement: Review lifecycle commands use consistent safety flags

Pinax SHALL use consistent `--dry-run`, `--yes`, `--status`, `--folder`, `--kind`, `--group`, and output mode semantics across inbox and draft lifecycle commands.

#### Scenario: Dry-run flag is available for lifecycle mutation preview
- **WHEN** a user runs `pinax draft promote note_123 --status active --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return a plan projection with `writes=false`
- **AND** the plan SHALL include old status, new status, current path, planned path, approval requirement, and index update expectation.

#### Scenario: Approval flag is required for discard
- **WHEN** a user runs `pinax inbox discard note_123 --vault ./my-notes --json` without `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** no Markdown, `.pinax` asset, Git state, provider state, remote service, or index projection SHALL be modified.

#### Scenario: Commands use app service rather than direct filesystem operations
- **WHEN** `pinax inbox promote`, `pinax draft promote`, `pinax draft archive`, or `pinax draft discard` performs an approved write
- **THEN** the command SHALL call the application service that owns note metadata, path, record event, event log, and index refresh behavior
- **AND** the command layer SHALL only parse flags, validate arguments, call the service, and render the projection.

