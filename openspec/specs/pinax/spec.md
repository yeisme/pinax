# pinax Specification

## Purpose

Pinax 是本地优先统一笔记 Agent CLI。当前 spec 记录子项目底座和后续实现的稳定边界，具体能力通过 `openspec/changes/pinax-*` 增量落地。
## Requirements
### Requirement: Pinax owns a local-first note CLI subproject

Pinax SHALL be implemented under `cli/pinax` as an independent Go CLI subproject and SHALL keep root repository OpenSpec limited to design handoff and governance.

#### Scenario: validating the development base
- **GIVEN** a developer enters `cli/pinax`
- **WHEN** they run `go test ./...` and `openspec validate --all`
- **THEN** the Go development base and OpenSpec workflow SHALL validate without requiring external provider credentials or a user vault

### Requirement: Machine-readable assets are CLI-authored

Pinax SHALL create and update machine-readable vault assets through commands or application services rather than requiring agents to hand-write JSON, YAML, or JSONL metadata.

#### Scenario: adding structured asset behavior
- **GIVEN** an implementation change adds config, provider profile, mapping, sync-state, event, briefing receipt, delivery receipt, feedback, or MCP evidence
- **WHEN** tasks are written
- **THEN** they SHALL include a command or service path that authors the asset
- **AND** tests SHALL validate schema version, redaction, path boundaries, and stable machine-readable errors

### Requirement: Pinax has a working development base

Pinax SHALL provide a minimal Go/Cobra development base before non-trivial product implementation starts.

#### Scenario: running bootstrap checks
- **GIVEN** the developer is in `cli/pinax`
- **WHEN** they run `task check`
- **THEN** tests, build, and OpenSpec validation SHALL pass without external provider credentials

#### Scenario: running checks without Taskfile
- **GIVEN** the developer is in `cli/pinax` and Taskfile is not installed
- **WHEN** they run `gofmt -w cmd internal`, `go test ./...`, `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`, and `openspec validate --all`
- **THEN** the same local quality gate SHALL pass without external provider credentials

### Requirement: Pinax implementation work is OpenSpec-gated

Business capability implementation SHALL be tracked by Pinax subproject OpenSpec changes.

#### Scenario: adding a business feature
- **GIVEN** a developer wants to implement vault, provider, sync, briefing, MCP, delivery, or feedback behavior
- **WHEN** code changes are planned
- **THEN** a `pinax-*` OpenSpec change SHALL describe proposal, design, tasks, validation commands, and failure re-checks before implementation proceeds

### Requirement: Pinax exposes a Go development task surface

Pinax SHALL provide a Taskfile-based development task surface that maps to direct Go and OpenSpec commands.

#### Scenario: building through Taskfile
- **GIVEN** the developer is in `cli/pinax`
- **WHEN** they run `task build`
- **THEN** Pinax SHALL produce `dist/pinax`
- **AND** the task SHALL verify Go formatting before building

#### Scenario: listing local tasks
- **GIVEN** the developer is in `cli/pinax`
- **WHEN** they run `task --list`
- **THEN** the output SHALL include at least `build`, `test`, `fmt`, `fmt-check`, `openspec`, `check`, and `clean`

### Requirement: Pinax initializes and validates a local Markdown vault

Pinax SHALL initialize and validate local Markdown vaults without requiring provider credentials, remote services, or agent-written metadata files.

#### Scenario: initializing a vault
- **GIVEN** a user runs `pinax init ./my-notes --title "我的知识库"`
- **WHEN** the vault is initialized
- **THEN** Pinax SHALL create `notes/`, `.pinax/config.yaml`, and `.pinax/events.jsonl` through CLI services
- **AND** it SHALL NOT overwrite existing Markdown note bodies

#### Scenario: validating a vault
- **GIVEN** a vault contains Markdown notes
- **WHEN** the user runs `pinax validate --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with validation facts, issues, and next actions
- **AND** path and metadata errors SHALL use stable machine-readable error codes

### Requirement: Pinax plans and applies metadata safely

Pinax SHALL plan metadata normalization before writing frontmatter and SHALL require explicit approval for writes.

#### Scenario: planning metadata changes
- **GIVEN** Markdown notes are missing Pinax metadata
- **WHEN** the user runs `pinax metadata plan --vault ./my-notes --json`
- **THEN** Pinax SHALL report planned frontmatter additions without modifying files

#### Scenario: applying metadata changes
- **GIVEN** a metadata plan exists for local notes
- **WHEN** the user runs `pinax metadata apply --vault ./my-notes --yes`
- **THEN** Pinax SHALL update only Markdown files inside the vault
- **AND** it SHALL append redacted event evidence through the event service

### Requirement: Pinax organizes note files with Git protection

Pinax SHALL generate an organize plan before moving or renaming notes, and SHALL require explicit Git snapshot protection before true apply.

#### Scenario: previewing organize changes
- **GIVEN** notes have titles that imply normalized paths
- **WHEN** the user runs `pinax organize plan --vault ./my-notes --json`
- **THEN** Pinax SHALL report planned moves, skips, and conflicts without modifying files

#### Scenario: refusing unprotected organize apply
- **GIVEN** no recent Pinax Git snapshot evidence exists
- **WHEN** the user runs `pinax organize apply --vault ./my-notes --yes`
- **THEN** Pinax SHALL refuse the write with a stable error code and a runnable `pinax git snapshot` next action

#### Scenario: applying organize changes after snapshot
- **GIVEN** the user has run `pinax git snapshot --vault ./my-notes --message "整理前快照"`
- **WHEN** the user runs `pinax organize apply --vault ./my-notes --yes`
- **THEN** Pinax SHALL move only files within the vault boundary
- **AND** it SHALL record redacted event evidence for each applied move

### Requirement: Pinax local commands follow the AI-native CLI output contract

Pinax SHALL render human, agent, JSON, events, and explain outputs from one command projection, and note commands SHALL expose stable machine-readable facts for editor execution, mutation outcomes, trash paths, sorting semantics, and ambiguous candidates.

#### Scenario: rendering machine output
- **GIVEN** a local vault command supports `--json` or `--agent`
- **WHEN** that output mode is selected
- **THEN** stdout SHALL contain only the selected machine format
- **AND** diagnostics SHALL go to stderr

#### Scenario: rendering note hardening facts
- **GIVEN** a note command executes an editor, mutates a note, moves a note to trash, lists recent notes, or returns ambiguous candidates
- **WHEN** `--json` or `--agent` is selected
- **THEN** stdout SHALL include stable fields for the relevant path, note id, editor executable or args, trash path, sort facts, mutation outcome, or candidate path/title/note id
- **AND** stdout SHALL NOT include raw provider credentials, shell-expanded secrets, or unredacted trace payloads.

### Requirement: Pinax prioritizes local notebook core before external extensions
Pinax SHALL provide a complete local-first notebook core before relying on external provider, cloud sync, or AI automation capabilities.

#### Scenario: Notebook core commands require no external credentials
- **WHEN** a user runs daily, inbox, organization view, link, backlink, attachment, saved view, import, or export commands against a valid local vault
- **THEN** Pinax SHALL NOT require firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, cookies, or external network access.

#### Scenario: Notebook core writes stay inside CLI-owned boundaries
- **WHEN** a notebook core command writes notes, attachments, saved views, import receipts, export receipts, or index projections
- **THEN** the write SHALL happen through Cobra command dispatch into `internal/app` services
- **AND** the command layer SHALL NOT hand-write `.pinax` JSON/YAML/JSONL assets.

#### Scenario: Notebook core keeps Markdown portable
- **WHEN** a user opens the vault in a normal Markdown editor
- **THEN** created notes, daily notes, inbox notes, wiki links, Markdown links, and attachment references SHALL remain readable without Pinax running.

### Requirement: Pinax note command is ergonomic and backwards compatible
Pinax SHALL expose an ergonomic note command surface while preserving existing `note new`, `note list`, and `note show` behavior.

#### Scenario: Note help shows daily workflow commands
- **WHEN** a user runs `pinax note --help`
- **THEN** help output SHALL include create/new, list, show/read, open/edit, rename, move, archive, delete, and tag commands
- **AND** help text SHALL describe local Markdown note management.

#### Scenario: Note new help shows information architecture flags
- **WHEN** a user runs `pinax note new --help`
- **THEN** help output SHALL include notebook information architecture flags such as `--group`, `--folder`, `--kind`, `--tags`, `--project`, `--dir`, and `--status`.

#### Scenario: Existing note commands remain valid
- **WHEN** a user runs existing commands `pinax note new`, `pinax note list`, or `pinax note show`
- **THEN** Pinax SHALL preserve backwards-compatible behavior and output contract unless the user selects new flags.

#### Scenario: Note commands require no provider credentials
- **WHEN** a user runs note creation, listing, reading, editing, tagging, archiving, or deletion commands against a valid local vault
- **THEN** Pinax SHALL NOT require firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, or external network access.

### Requirement: Pinax exposes a readonly local MCP server

Pinax SHALL expose a local stdio MCP server for agent read workflows while routing through the same application services as CLI commands.

#### Scenario: starting MCP serve
- **GIVEN** a user has a local Pinax vault
- **WHEN** they run `pinax mcp serve --vault ./my-notes`
- **THEN** Pinax SHALL accept MCP JSON-RPC requests over stdio
- **AND** it SHALL advertise only read-only resources and tools in MVP

#### Scenario: listing readonly resources
- **GIVEN** an MCP client calls `resources/list`
- **WHEN** Pinax responds
- **THEN** it SHALL include compact resources such as `pinax://vault/current`, `pinax://note/{note_id}`, `pinax://search/{query}`, and `pinax://organize/plan`
- **AND** it SHALL NOT return every note body by default

#### Scenario: calling readonly tools
- **GIVEN** an MCP client calls `pinax.search`, `pinax.note.read`, `pinax.organize.plan`, or `pinax.git.snapshot_plan`
- **WHEN** the tool is handled
- **THEN** Pinax SHALL route through `internal/app` services
- **AND** it SHALL NOT modify Markdown files, `.pinax/` state, Git state, or provider state

#### Scenario: rejecting write tools
- **GIVEN** an MCP client asks for write-capable behavior
- **WHEN** approval metadata or explicit local write flags are missing
- **THEN** Pinax SHALL return an approval-required or method-not-found error
- **AND** it MAY include a human-runnable CLI command for the user to apply manually

### Requirement: Pinax manages multiple projects inside one vault

Pinax SHALL allow a local vault to contain multiple named projects through CLI-authored structured metadata.

#### Scenario: creating a project
- **GIVEN** a Pinax vault exists
- **WHEN** the user runs `pinax project create research --name "研究" --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update `.pinax/projects.json` through the application service
- **AND** stdout SHALL contain one JSON envelope with `command=project.create`, `status=success`, project facts, and a runnable next action

#### Scenario: listing projects
- **GIVEN** a vault has project metadata
- **WHEN** the user runs `pinax project list --vault ./my-notes --json`
- **THEN** stdout SHALL contain project records and the current project without reading note bodies

#### Scenario: switching current project
- **GIVEN** a vault has a project with slug `research`
- **WHEN** the user runs `pinax project switch research --vault ./my-notes`
- **THEN** Pinax SHALL update only the current project pointer in `.pinax/projects.json`
- **AND** it SHALL append redacted event evidence

### Requirement: Pinax stores backend configuration for local and S3 storage

Pinax SHALL configure storage backend metadata through CLI commands without requiring real network access or persisted provider secrets.

#### Scenario: configuring S3 backend
- **GIVEN** a Pinax vault exists
- **WHEN** the user runs `pinax storage set-s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault ./my-notes --json`
- **THEN** Pinax SHALL write `.pinax/storage.json` through the application service
- **AND** stdout SHALL contain a JSON envelope with backend facts and no credentials

#### Scenario: diagnosing S3 backend configuration
- **GIVEN** a vault has S3 backend metadata
- **WHEN** the user runs `pinax storage doctor --vault ./my-notes --json`
- **THEN** Pinax SHALL validate required fields without connecting to S3
- **AND** it SHALL report expected credential source without printing secret values

### Requirement: Core note creation

Pinax SHALL create Markdown notes from the CLI while preserving Markdown files as the source of truth.

#### Scenario: Create a note with frontmatter

- **WHEN** a user runs `pinax note new "研究日志" --tags research,pinax --vault <vault>`
- **THEN** Pinax creates a Markdown file under the vault
- **AND** the file contains YAML frontmatter with `schema_version`, `note_id`, `title`, `tags`, `created_at`, and `updated_at`
- **AND** the command returns a structured projection containing the created note path.

### Requirement: Template rendering

Pinax SHALL manage editable Markdown templates and render them without executing code.

#### Scenario: Initialize and render built-in templates

- **WHEN** a user runs `pinax template init --vault <vault>` and `pinax template render mermaid --title "架构" --vault <vault>`
- **THEN** Pinax creates built-in templates under `.pinax/templates/`
- **AND** rendering replaces safe variables such as `{{title}}`, `{{date}}`, `{{datetime}}`, `{{project}}`, and `{{tags}}`
- **AND** the mermaid template contains a Markdown Mermaid code fence.

### Requirement: Hybrid search and local index

Pinax SHALL combine fast full-text search with a local SQLite/GORM index projection.

#### Scenario: Rebuild index and search backlinks

- **WHEN** a vault contains notes using `[[Wiki Link]]` and `#tag`
- **AND** a user runs `pinax index rebuild --vault <vault>`
- **THEN** Pinax stores note, tag, and link projections in `.pinax/index.sqlite` through GORM
- **AND** `pinax search <query> --vault <vault>` reports whether it used `rg` or scan fallback.

### Requirement: Sync planning boundary

Pinax SHALL expose sync plans for Git, S3, and Pinax Cloud without pretending that unimplemented remote writes succeeded.

#### Scenario: Cloud sync reports backend requirement

- **WHEN** a user runs `pinax sync diff --target cloud --vault <vault>`
- **THEN** Pinax returns a plan with `backend_required=true`
- **AND** the projection includes the minimum Pinax Cloud API handoff
- **AND** `pinax sync push --target cloud --vault <vault>` without `--yes` fails with an approval-required error.

### Requirement: Template authoring from the CLI

Pinax SHALL let users create editable Markdown templates through CLI commands while keeping templates as local text files.

#### Scenario: Create a template from a file

- **WHEN** a user runs `pinax template create meeting --from ./meeting.md --vault ./my-notes --json`
- **THEN** Pinax writes `.pinax/templates/meeting.md` through the application service
- **AND** stdout contains one JSON projection for `template.create`
- **AND** the projection contains the template name and path
- **AND** no cloud backend, provider credential, or network connection is required.

#### Scenario: Create a template from inline body

- **WHEN** a user runs `pinax template create daily-review --body "# {{date}}" --vault ./my-notes --json`
- **THEN** Pinax writes `.pinax/templates/daily-review.md`
- **AND** the body is stored as plain Markdown text
- **AND** no shell, script, environment variable, or network interpolation is executed.

#### Scenario: Reject unsafe template names

- **WHEN** a user runs `pinax template create ../bad --body "x" --vault ./my-notes --json`
- **THEN** Pinax fails with stable error code `invalid_template_name`
- **AND** no file outside `.pinax/templates/` is created.

### Requirement: Template variables are safe and explicit

Pinax SHALL render templates using explicit text variables without executing code.

#### Scenario: Render custom variables

- **GIVEN** `.pinax/templates/meeting.md` contains `# {{title}}\n客户: {{client}}`
- **WHEN** a user runs `pinax template render meeting --title "客户会议" --var client=Acme --vault ./my-notes --json`
- **THEN** the rendered body contains `# 客户会议`
- **AND** the rendered body contains `客户: Acme`
- **AND** the command does not execute scripts, shell commands, environment lookups, or network calls.

#### Scenario: Missing variables fail clearly

- **GIVEN** `.pinax/templates/meeting.md` contains `客户: {{client}}`
- **WHEN** a user runs `pinax template render meeting --vault ./my-notes --json`
- **THEN** Pinax fails with stable error code `template_variable_missing`
- **AND** the error names the missing variable without printing secrets or raw provider payload.

### Requirement: Notes can be generated from custom templates

Pinax SHALL let `note new` consume custom templates and variable values.

#### Scenario: Create note from custom template

- **GIVEN** `.pinax/templates/meeting.md` contains `# {{title}}\n客户: {{client}}`
- **WHEN** a user runs `pinax note new "客户会议" --template meeting --var client=Acme --tags meeting,client --vault ./my-notes --json`
- **THEN** Pinax creates a Markdown note under `notes/`
- **AND** the note has Pinax frontmatter with `schema_version`, `note_id`, `title`, `tags`, `created_at`, and `updated_at`
- **AND** the note body contains the rendered custom template content.

### Requirement: Template validation reports actionable results

Pinax SHALL validate templates before generation so malformed templates do not silently create broken notes.

#### Scenario: Validate a valid Mermaid template

- **GIVEN** `.pinax/templates/diagram.md` contains a closed Mermaid code fence
- **WHEN** a user runs `pinax template validate diagram --vault ./my-notes --json`
- **THEN** Pinax returns `status=success`
- **AND** the projection includes facts for template name, variables, and issues count.

#### Scenario: Detect unclosed fences

- **GIVEN** `.pinax/templates/bad.md` contains an unclosed Markdown code fence
- **WHEN** a user runs `pinax template validate bad --vault ./my-notes --json`
- **THEN** Pinax fails or returns `status=partial` with issue code `template_fence_unclosed`
- **AND** `pinax note new --template bad` SHALL NOT create a note unless validation passes.

### Requirement: Template deletion is explicit and safe

Pinax SHALL protect templates from accidental deletion.

#### Scenario: Delete custom template with approval

- **GIVEN** `.pinax/templates/meeting.md` exists
- **WHEN** a user runs `pinax template delete meeting --vault ./my-notes --yes --json`
- **THEN** Pinax deletes only `.pinax/templates/meeting.md`
- **AND** it records a redacted event through the application service.

#### Scenario: Reject delete without approval

- **GIVEN** `.pinax/templates/meeting.md` exists
- **WHEN** a user runs `pinax template delete meeting --vault ./my-notes --json`
- **THEN** Pinax fails with stable error code `approval_required`
- **AND** the template file remains unchanged.


### Requirement: Pinax exposes note vault management analytics
Pinax SHALL treat stats, doctor, and dashboard as first-class local Markdown note CLI capabilities for managing a user's vault.

#### Scenario: Note CLI includes management commands
- **WHEN** a user runs `pinax --help`
- **THEN** the command list SHALL include `stats`, `doctor`, and `dashboard`
- **AND** their help text SHALL describe local Markdown vault management rather than agent platform or provider automation behavior.

#### Scenario: Analytics commands require no provider credentials
- **WHEN** a user runs `pinax stats`, `pinax doctor`, or `pinax dashboard` against a valid local vault
- **THEN** Pinax SHALL NOT require firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, or external network access.

#### Scenario: Analytics commands follow the CLI output contract
- **WHEN** `pinax stats` or `pinax doctor` is run with default human output, `--json`, or `--agent`
- **THEN** Pinax SHALL render all modes from one command projection
- **AND** machine-readable stdout SHALL be stable enough for scripts and agents.

### Requirement: Pinax exposes local vault repair commands
Pinax SHALL expose repair plan and apply commands as local Markdown vault management capabilities.

#### Scenario: Help includes repair commands
- **WHEN** a user runs `pinax repair --help`
- **THEN** the command list SHALL include `plan` and `apply`
- **AND** help text SHALL describe local Markdown vault maintenance rather than provider automation.

#### Scenario: Repair commands follow output contract
- **WHEN** `pinax repair plan` or `pinax repair apply` is run with default human output, `--json`, or `--agent`
- **THEN** Pinax SHALL render all modes from one command projection
- **AND** machine-readable stdout SHALL contain only the selected format.
