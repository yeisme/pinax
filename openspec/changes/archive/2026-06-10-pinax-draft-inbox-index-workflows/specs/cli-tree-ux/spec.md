## ADDED Requirements

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
