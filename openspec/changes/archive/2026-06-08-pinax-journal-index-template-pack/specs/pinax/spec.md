## ADDED Requirements

### Requirement: Template commands expose stable human and machine output
Pinax SHALL expose template create, inspect, preview, render, and delete workflows through stable CLI output modes, and SHALL include journal/index template metadata without leaking unsafe internals.

#### Scenario: Inspect journal template exposes workflow facts
- **WHEN** a user runs `pinax template inspect journal.daily --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with command `template.inspect`
- **AND** facts SHALL include stable English keys for `template`, `template_kind`, `engine`, `path_pattern`, `managed_block_count`, `query_count`, `refreshable`, and `source`
- **AND** human-readable summaries and explanations SHALL be Chinese by default.

#### Scenario: Inspect index template exposes refresh facts
- **WHEN** a user runs `pinax template inspect index.home --vault ./my-notes --agent`
- **THEN** stdout SHALL contain stable key=value lines for template name, template kind, path pattern, managed block count, query count, and refreshable status
- **AND** stdout SHALL NOT include localized prose, raw prompts, provider payloads, secrets, Authorization headers, or hidden system instructions.

#### Scenario: Inspect starter note template exposes use cases
- **WHEN** a user runs `pinax template inspect meeting.notes --vault ./my-notes --json`
- **THEN** facts SHALL include stable English keys for `use_cases`, `aliases`, `difficulty`, `starter`, `path_pattern`, and `after_create_action_count`
- **AND** actions SHALL include a preview command and a create-note command that use the current vault path.

#### Scenario: Preview journal or index template is read-only
- **WHEN** a user runs `pinax template preview index.home --vault ./my-notes --json`
- **THEN** Pinax SHALL render preview output through the template engine and query service
- **AND** it SHALL NOT write notes, `.pinax` structured assets, render run receipts, Git state, provider state, or remote services.

#### Scenario: Template output path is constrained
- **WHEN** a template declares `output.path_pattern` with an absolute path, `..`, `.pinax`, `.git`, `attachments`, `temp`, `dist`, `node_modules`, `vendor`, or any path outside the vault content boundary
- **THEN** Pinax SHALL reject the template before writing with stable error code `template_output_path_invalid`
- **AND** no note, index, event, render artifact, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Inspect template exposes root-layout path pattern
- **WHEN** a user runs `pinax template inspect journal.daily --vault ./my-notes --json`
- **THEN** facts SHALL report the default path pattern as `daily/{{ .Date }}.md` or an equivalent root-relative daily pattern
- **AND** facts SHALL NOT report `notes/daily/{{ .Date }}.md` as the recommended path for new vaults.

#### Scenario: Template inspect recommends a concrete next command
- **WHEN** a user runs `pinax template inspect journal.daily --vault ./my-notes`
- **THEN** the default human output SHALL include one recommended next command such as `pinax template preview journal.daily --vault ./my-notes` or `pinax journal daily open --template journal.daily --vault ./my-notes`
- **AND** `--json` output SHALL include the same recommendation in the envelope `actions` array using stable English fields `name`, `command`, and `reason`
- **AND** `--agent` output SHALL include `action.primary=...` without localized prose.

#### Scenario: Missing template variables produce actionable rerun command
- **GIVEN** `video-study` requires variable `url`
- **WHEN** a user runs `pinax template render video-study --vault ./my-notes --json` without `--var url=...`
- **THEN** Pinax SHALL fail with stable error code `template_variable_missing`
- **AND** the error projection SHALL include an action command shaped like `pinax template render video-study --var url=... --vault ./my-notes --json`
- **AND** the action SHALL NOT include secret-like original variable values, raw prompts, provider payloads, Authorization headers, or hidden system instructions.

### Requirement: Template completion is contextual and no-file by default
Pinax SHALL make the `template` command discoverable through contextual shell completion for template names, template flags, variables, and render runs.

#### Scenario: Complete template names for inspect
- **WHEN** a user requests shell completion for `pinax template inspect --vault ./my-notes <TAB>`
- **THEN** completion SHALL include built-in templates such as `journal.daily` and `index.home`
- **AND** it SHALL include vault-local templates from `.pinax/templates/*.md` when present
- **AND** descriptions SHALL identify source and kind, for example `builtin journal_template` or `local note_template`
- **AND** completion SHALL return `ShellCompDirectiveNoFileComp` and SHALL NOT execute templates, execute SQL, rebuild indexes, write files, call providers, call Git, or access network.

#### Scenario: Complete overridden template with source description
- **GIVEN** `.pinax/templates/journal.daily.md` exists inside the vault
- **WHEN** a user requests shell completion for `pinax template preview --vault ./my-notes <TAB>`
- **THEN** completion SHALL include `journal.daily` once
- **AND** the description SHALL identify it as a vault-local override of a built-in template.

#### Scenario: Complete executable template names for render
- **WHEN** a user requests shell completion for `pinax template render --vault ./my-notes <TAB>`
- **THEN** completion SHALL include executable templates such as `note.quick`, `inbox.capture`, `meeting.notes`, `decision.record`, `project.brief`, `learning.video`, `research.topic`, `journal.daily`, and `index.home`
- **AND** it SHALL describe legacy simple templates as `legacy`
- **AND** completion SHOULD avoid suggesting template design drafts that cannot be rendered, unless they are marked `invalid` or `draft` in the description.

#### Scenario: Delete completion only lists local deletable templates
- **WHEN** a user requests shell completion for `pinax template delete --vault ./my-notes <TAB>`
- **THEN** completion SHALL list only vault-local templates or vault-local overrides that can be removed from `.pinax/templates/`
- **AND** it SHALL NOT suggest built-in templates that are protected from deletion.

#### Scenario: Complete template engine flag
- **WHEN** a user requests shell completion for `pinax template create demo --engine <TAB>`
- **THEN** completion SHALL include `simple` and `go-template` with descriptions
- **AND** completion SHALL return `ShellCompDirectiveNoFileComp`.

#### Scenario: Complete template variables from schema
- **GIVEN** `video-study` declares variable `url` as a required string
- **WHEN** a user requests shell completion for `pinax template render video-study --vault ./my-notes --var <TAB>`
- **THEN** completion SHALL include `url=` with a description that includes required/optional status and type
- **AND** completion SHALL NOT infer, print, or persist secret-like variable values.

#### Scenario: Journal template flag completion is filtered by workflow kind
- **WHEN** a user requests shell completion for `pinax journal daily open --vault ./my-notes --template <TAB>`
- **THEN** completion SHALL prefer templates with `template_kind=journal_template`
- **AND** descriptions SHALL identify daily-compatible templates where known.

#### Scenario: Index page template flag completion is filtered by workflow kind
- **WHEN** a user requests shell completion for `pinax index page create home --vault ./my-notes --template <TAB>`
- **THEN** completion SHALL prefer templates with `template_kind=index_template`
- **AND** descriptions SHALL identify query-backed and refreshable templates.

### Requirement: CLI help recommends journal and index template workflows
Pinax SHALL present journal and index template workflows as first-class local notebook workflows while preserving legacy template compatibility.

#### Scenario: Template help includes journal and index examples
- **WHEN** a user runs `pinax template --help`
- **THEN** help output SHALL include examples for `pinax template inspect journal.daily --vault ./my-notes --json` and `pinax template preview index.home --vault ./my-notes`
- **AND** it SHALL NOT present legacy `daily` simple template as the recommended new workflow.

#### Scenario: Template help includes recommendation examples
- **WHEN** a user runs `pinax template --help`
- **THEN** help output SHALL include examples for `pinax template list --pack starter --vault ./my-notes` and `pinax template recommend --intent meeting --vault ./my-notes --json`
- **AND** the examples SHALL use real commands a user can run.

#### Scenario: Index help includes page workflow
- **WHEN** a user runs `pinax index --help`
- **THEN** help output SHALL include the `page` workflow or point to `pinax index page --help`
- **AND** examples SHALL use `pinax index page refresh home --vault ./my-notes --json` for refreshing Markdown index pages.

#### Scenario: Note help describes root content layout
- **WHEN** a user runs `pinax note --help`
- **THEN** help output SHALL describe default new note placement as vault-root content paths such as `demo.md` or `inbox/demo.md`
- **AND** it SHALL describe `notes/` as a legacy-compatible folder rather than the default required note root.
