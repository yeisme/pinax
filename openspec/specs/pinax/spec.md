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

Pinax SHALL create and update machine-readable vault assets through commands or application services rather than requiring agents to hand-write JSON, YAML, or JSONL metadata. Pinax SHALL treat record ledger assets as CLI-authored machine records that own note identity, lifecycle, schema, tombstone, version evidence, sync evidence, and repair evidence.

#### Scenario: adding structured asset behavior
- **GIVEN** an implementation change adds config, provider profile, mapping, sync-state, event, record ledger, note registry, schema registry, tombstone, version backend config, diff evidence, snapshot receipt, briefing receipt, delivery receipt, feedback, or MCP evidence
- **WHEN** tasks are written
- **THEN** they SHALL include a command or service path that authors the asset
- **AND** tests SHALL validate schema version, redaction, path boundaries, ledger sequence or identity constraints where relevant, and stable machine-readable errors.

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

Pinax SHALL initialize and validate local Markdown vaults without requiring provider credentials, remote services, or agent-written metadata files. Pinax SHALL preserve Markdown as the content source while initializing CLI-authored record assets for machine identity and lifecycle facts.

#### Scenario: initializing a vault
- **GIVEN** a user runs `pinax init ./my-notes --title "我的知识库"`
- **WHEN** the vault is initialized
- **THEN** Pinax SHALL create `notes/`, `.pinax/config.yaml`, `.pinax/events.jsonl`, and `.pinax/records/` assets through CLI services
- **AND** it SHALL NOT overwrite existing Markdown note bodies

#### Scenario: validating a vault
- **GIVEN** a vault contains Markdown notes and Pinax record assets
- **WHEN** the user runs `pinax validate --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with validation facts, record ledger facts, issues, and next actions
- **AND** path, metadata, record sequence, note identity, and schema errors SHALL use stable machine-readable error codes

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

#### Scenario: successful human output omits command status
- **WHEN** a local vault command succeeds in default human output mode
- **THEN** stdout SHALL NOT show a command execution status row such as `状态: 成功` or `status=success`
- **AND** success SHALL be communicated through the result summary, key facts, exit code 0, and next action when useful
- **AND** business status facts MAY still appear when they describe the returned object, such as note `status`, `index_status`, `ledger_status`, provider health, or worktree state.

#### Scenario: non-success human output shows actionable state
- **WHEN** a command returns `partial`, `failed`, dry-run, approval-required, warning, or risk state in default human output mode
- **THEN** stdout SHALL include the actionable state, error/risk summary, and a real next command when available.

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
- **THEN** help output SHALL include add/create/new, list, show/read, open/edit, rename, move, archive, delete, and tag commands
- **AND** help text SHALL describe local Markdown note management.

#### Scenario: Note add is the recommended creation entry
- **WHEN** a user runs `pinax note add "研究日志" --vault ./my-notes`
- **THEN** Pinax SHALL create a registered Markdown note with `schema_version: pinax.note.v1`
- **AND** `pinax note new` and `pinax note create` SHALL remain compatible aliases for existing scripts.

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

Pinax SHALL create Markdown notes from the CLI while preserving Markdown body content as the content source and creating CLI-authored record ledger facts as the machine source for identity and lifecycle.

#### Scenario: Create a note with frontmatter

- **WHEN** a user runs `pinax note new "研究日志" --tags research,pinax --vault <vault>`
- **THEN** Pinax creates a Markdown file under the vault
- **AND** the file contains YAML frontmatter with `schema_version`, `note_id`, `title`, `tags`, `created_at`, and `updated_at`
- **AND** Pinax appends a record ledger event and updates the note registry for the new note id, path, lifecycle state, and content hash.

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
Pinax SHALL treat stats, doctor, validate, and dashboard as first-class local Markdown vault management capabilities under the `vault` command namespace while preserving existing root compatibility aliases.

#### Scenario: Vault namespace includes management commands
- **WHEN** a user runs `pinax vault --help`
- **THEN** the command list SHALL include `stats`, `validate`, `doctor`, and `dashboard`
- **AND** their help text SHALL describe local Markdown vault management rather than agent platform or provider automation behavior.

#### Scenario: Root help hides vault compatibility aliases
- **WHEN** a user runs `pinax --help`
- **THEN** root aliases `stats`, `validate`, `doctor`, and `dashboard` SHALL NOT appear in the primary command list
- **AND** the root help SHALL include `vault` as the primary local vault management entry.

#### Scenario: Analytics commands require no provider credentials
- **WHEN** a user runs `pinax vault stats`, `pinax vault doctor`, or `pinax vault dashboard` against a valid local vault
- **THEN** Pinax SHALL NOT require firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, or external network access.

#### Scenario: Analytics aliases remain compatible
- **WHEN** a user runs existing commands `pinax stats`, `pinax doctor`, or `pinax dashboard` against a valid local vault
- **THEN** Pinax SHALL preserve backwards-compatible behavior and machine output fields
- **AND** root aliases MAY be hidden from primary help.

#### Scenario: Analytics commands follow the CLI output contract
- **WHEN** `pinax vault stats` or `pinax vault doctor` is run with default human output, `--json`, or `--agent`
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

### Requirement: Pinax templates use a shared Go template engine

Pinax SHALL provide a shared template rendering engine based on Go `text/template` for v2 templates, while preserving local-first Markdown vault behavior and avoiding external execution surfaces.

#### Scenario: render a v2 Go template
- **GIVEN** `.pinax/templates/video-study.md` contains frontmatter with `schema_version: pinax.template.v2` and `engine: go-template`
- **AND** the template body contains `# {{ .Title }}` and `{{ if .Vars.url }}链接：{{ .Vars.url }}{{ end }}`
- **WHEN** the user runs `pinax template render video-study --title "Go 模板学习" --var url=https://go.dev --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `command="template.render"` and `status="success"`
- **AND** the rendered body SHALL include `# Go 模板学习` and `链接：https://go.dev`
- **AND** the render path SHALL use the shared Pinax template engine rather than command-layer string replacement.

#### Scenario: note new consumes the shared template engine
- **GIVEN** a valid v2 Go template named `video-study` exists
- **WHEN** the user runs `pinax note new "Go 模板学习" --template video-study --var url=https://go.dev --tags learning,golang --vault ./my-notes`
- **THEN** Pinax SHALL create a Markdown note through the note application service
- **AND** the note body SHALL contain the rendered Go template output
- **AND** explicit CLI fields such as title and tags SHALL override template defaults where both are present.

#### Scenario: design template is rejected before rendering
- **GIVEN** `.pinax/templates/video-study.md` declares `schema_version: pinax.template_design.v1`
- **WHEN** the user runs `pinax template preview video-study --vault ./my-notes --json`, `pinax template render video-study --vault ./my-notes --json`, or `pinax note new "Video" --template video-study --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `template_design_not_executable`
- **AND** no note, event, index, render run, Git state, provider state, or remote service SHALL be modified.

### Requirement: Template metadata and context are inspectable

Pinax SHALL let users and agents inspect template metadata, variables, function references, defaults, and example context without executing the template body.

#### Scenario: inspect template metadata as JSON
- **GIVEN** `.pinax/templates/video-study.md` declares `schema_version: pinax.template.v2`, variables, defaults, and example context
- **WHEN** the user runs `pinax template inspect video-study --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `command="template.inspect"`
- **AND** the envelope data SHALL include template name, kind, engine, variables, defaults, example context, function references, and warnings
- **AND** inspection SHALL NOT execute the template, write notes, call providers, read environment variables, or access network.

#### Scenario: preview template with example context
- **GIVEN** a v2 template has an `example` context in frontmatter
- **WHEN** the user runs `pinax template preview video-study --vault ./my-notes`
- **THEN** Pinax SHALL render a concise human preview using the example context
- **AND** explicit CLI fields and `--var` values SHALL override the example context
- **AND** it SHALL recommend a runnable `pinax template render ...` or `pinax note new ... --template ...` next command when useful.

### Requirement: Template v2 frontmatter is structured and validated

Pinax SHALL parse v2 template frontmatter with a structured YAML parser and report schema issues through stable template errors or validation issues.

#### Scenario: validate a correct v2 schema
- **GIVEN** `.pinax/templates/video-study.md` declares `schema_version: pinax.template.v2`, `engine: go-template`, and a valid `variables` map
- **WHEN** the user runs `pinax template validate video-study --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `command="template.validate"` and `status="success"`
- **AND** the projection SHALL include facts for engine, variable count, issue count, and template kind.

#### Scenario: reject invalid template schema
- **GIVEN** `.pinax/templates/bad.md` declares `schema_version: pinax.template.v2` but uses an unsupported `engine: shell`
- **WHEN** the user runs `pinax template validate bad --vault ./my-notes --json`
- **THEN** Pinax SHALL return `status="partial"` or `status="failed"`
- **AND** the issue or error code SHALL be `template_engine_unsupported` or `template_schema_invalid`
- **AND** Pinax SHALL NOT execute any template body.

### Requirement: Legacy simple templates remain compatible

Pinax SHALL preserve existing simple templates that use legacy `{{name}}` variables unless a template explicitly opts into `engine: go-template`.

#### Scenario: render legacy simple template
- **GIVEN** `.pinax/templates/meeting.md` contains `# {{title}} - {{client}}` without v2 frontmatter
- **WHEN** the user runs `pinax template render meeting --title "客户会议" --var client=Acme --vault ./my-notes --json`
- **THEN** Pinax SHALL render `# 客户会议 - Acme`
- **AND** it SHALL NOT require the user to rewrite the template as `{{ .Title }}`.

#### Scenario: design draft is not silently treated as executable
- **GIVEN** `.pinax/templates/视频学习.md` is a `pinax.template_design.v1` design draft
- **WHEN** the user runs `pinax template validate "视频学习" --vault ./my-notes --json`
- **THEN** Pinax SHALL report that the file is a template design draft
- **AND** it SHALL provide a safe next action to publish or convert it before using it as an executable template
- **AND** `pinax note new --template "视频学习"` SHALL NOT silently execute ambiguous design prose as a production template.

### Requirement: Template functions are safe and deterministic

Pinax SHALL expose only a small whitelisted function set to Go templates and SHALL reject unsupported or unsafe function references.

#### Scenario: allowed pure functions render successfully
- **GIVEN** a v2 Go template uses allowed functions such as `default`, `join`, `lower`, `upper`, `slug`, `date`, `yaml`, `json`, and `quote`
- **WHEN** the user runs `pinax template render video-study --title "Go 模板学习" --vault ./my-notes --json`
- **THEN** Pinax SHALL render the template successfully
- **AND** the functions SHALL NOT read files, environment variables, network resources, provider state, or shell commands.

#### Scenario: unsupported function is rejected
- **GIVEN** a v2 Go template contains `{{ env "HOME" }}` or `{{ exec "date" }}`
- **WHEN** the user runs `pinax template validate bad --vault ./my-notes --json`
- **THEN** Pinax SHALL report `template_function_unsupported` or `template_parse_failed`
- **AND** it SHALL NOT expose environment variable values, shell output, provider payloads, secrets, or local file contents.

### Requirement: Templates can consume safe Pinax SQL query results

Pinax SHALL let v2 templates declare safe Pinax SQL query blocks and consume bounded query results without executing raw SQLite SQL or implementing a second query language inside the template engine.

#### Scenario: render template with declared SQL query
- **GIVEN** `.pinax/templates/project-dashboard.md` declares `schema_version: pinax.template.v2` and `engine: go-template`
- **AND** its frontmatter contains a named query `active` with `language: sql` and `text: SELECT title, status, due FROM notes WHERE tags CONTAINS "project" AND status = "active" ORDER BY due ASC LIMIT 10`
- **AND** the template body contains `{{ table .Queries.active }}`
- **WHEN** the user runs `pinax template render project-dashboard --title "项目看板" --vault ./my-notes --json`
- **THEN** Pinax SHALL execute the query through the `pinax-database-views-query` query service
- **AND** stdout SHALL contain one JSON envelope with rendered body, query facts, row count, columns, and index status
- **AND** Pinax SHALL NOT pass the raw query string directly to SQLite.

#### Scenario: render template with fenced SQL query block
- **GIVEN** a v2 template body contains a fenced block named `pinax-sql` with `SELECT title, status FROM notes WHERE tags CONTAINS "project" LIMIT 5`
- **WHEN** the user runs `pinax template preview project-dashboard --vault ./my-notes`
- **THEN** Pinax SHALL parse the fenced block as a query declaration
- **AND** it SHALL render a bounded Markdown preview of the query result when the required local query projection is already available
- **AND** it SHALL NOT treat the fenced block as JavaScript, shell, raw SQLite SQL, or arbitrary Markdown to execute.

#### Scenario: inspect query-backed template without full execution
- **GIVEN** a v2 template declares one or more queries
- **WHEN** the user runs `pinax template inspect project-dashboard --vault ./my-notes --json`
- **THEN** Pinax SHALL parse query declarations and return explain-style metadata such as language, selected columns, limit, warnings, and unsupported clauses
- **AND** it SHALL NOT render full query rows, write notes, update `.pinax` assets, call providers, or access network.

#### Scenario: preview query-backed template remains read-only when index is missing
- **GIVEN** a v2 template declares one or more queries
- **AND** `.pinax/index.sqlite` is missing or stale
- **WHEN** the user runs `pinax template preview project-dashboard --vault ./my-notes --json`
- **THEN** Pinax SHALL fail or return partial output with a stable index-required error or warning
- **AND** it SHALL include a next action such as `pinax index rebuild --vault ./my-notes`
- **AND** it SHALL NOT create or modify `.pinax/index.sqlite`, events, render runs, notes, Git state, provider state, or remote services.

#### Scenario: reject unsupported query clauses in template
- **GIVEN** a v2 template declares a query with unsupported SQL clauses such as `JOIN`, subqueries, shell calls, network calls, or JavaScript execution
- **WHEN** the user runs `pinax template validate project-dashboard --vault ./my-notes --json`
- **THEN** Pinax SHALL report a stable template query validation issue
- **AND** it SHALL NOT execute the query or write vault state.

### Requirement: Notes expose rendered and source views through note commands

Pinax SHALL keep rendered Markdown viewing and rendered-result writeback under the `note` command surface rather than introducing a separate top-level file viewing command.

#### Scenario: show rendered note view
- **GIVEN** `projects/dashboard.md` contains a `pinax-sql` fenced block with `SELECT title, status FROM notes WHERE tags CONTAINS "project" LIMIT 5`
- **WHEN** the user runs `pinax note show projects/dashboard.md --view rendered --vault ./my-notes`
- **THEN** stdout SHALL contain Markdown with the query result rendered into the note view
- **AND** Pinax SHALL NOT modify Markdown files, `.pinax` structured assets, Git state, provider state, or remote services.

#### Scenario: show source note view without executing query
- **GIVEN** `projects/dashboard.md` contains one or more `pinax-sql` fenced blocks
- **WHEN** the user runs `pinax note show projects/dashboard.md --view source --vault ./my-notes`
- **THEN** stdout SHALL contain the original Markdown source
- **AND** Pinax SHALL NOT execute SQL queries, trigger lazy index rebuild, or write any asset.

#### Scenario: refresh rendered query result into managed block
- **GIVEN** `projects/dashboard.md` contains a `pinax-sql` fenced block named `active-projects`
- **AND** the note contains matching markers `<!-- pinax:render active-projects start -->` and `<!-- pinax:render active-projects end -->`
- **WHEN** the user runs `pinax note refresh projects/dashboard.md --rendered --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the managed render block with the current bounded query result
- **AND** stdout SHALL contain one JSON envelope with changed section, query facts, row count, index status, and event evidence reference
- **AND** the source `pinax-sql` block, ordinary prose, and unrelated Markdown SHALL remain unchanged.

#### Scenario: refresh requires explicit approval
- **GIVEN** `projects/dashboard.md` contains a refreshable managed render block
- **WHEN** the user runs `pinax note refresh projects/dashboard.md --rendered --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with `note_refresh_approval_required`
- **AND** no Markdown file or structured asset SHALL be modified.

#### Scenario: invalid managed block is rejected
- **GIVEN** a note has missing, nested, mismatched, or hash-conflicting `pinax:render` markers
- **WHEN** the user runs `pinax note refresh projects/dashboard.md --rendered --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with `note_render_block_invalid`
- **AND** it SHALL NOT partially rewrite the note.

### Requirement: Render runs are versioned structured assets

Pinax SHALL let formal template and rendered-note operations create reusable, timestamped render run records under note/template-scoped paths in `.pinax/renders/` through CLI or application services.

#### Scenario: save a named render run
- **GIVEN** a valid v2 template named `video-study` exists
- **WHEN** the user runs `pinax template render video-study --title "Go 模板学习" --var url=https://go.dev --save-run video-go --vault ./my-notes --json`
- **THEN** Pinax SHALL render the template and create a CLI-authored render run under the related note/template mirror path in `.pinax/renders/`
- **AND** the render run receipt SHALL include `schema_version`, `run_id`, `name`, `created_at`, `command`, `template`, source/template hashes, redacted args, query facts when present, rendered artifact hash, and event evidence reference
- **AND** the receipt SHALL NOT include secret-like variable values, raw prompts, provider payloads, Authorization headers, hidden prompts, or full chain-of-thought.

#### Scenario: store note render run under mirrored note path
- **GIVEN** the source note path is `notes/学习/galang高性能/1-协程.md`
- **WHEN** the user runs `pinax note refresh notes/学习/galang高性能/1-协程.md --rendered --save-run 协程快照 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL create the render run under `.pinax/renders/学习/galang高性能/1-协程/<run-id>/`
- **AND** it SHALL NOT create generated render artifacts under `notes/学习/galang高性能/renders/` by default
- **AND** the receipt SHALL preserve the original target note path for audit and lookup.

#### Scenario: store template-only render under template path
- **GIVEN** a render operation has no target note path
- **WHEN** the user runs `pinax template render video-study --title "Go 模板学习" --save-run video-go --vault ./my-notes --json`
- **THEN** Pinax SHALL create the render run under `.pinax/renders/templates/video-study/<run-id>/`
- **AND** template render runs SHALL remain discoverable through `pinax template inspect video-study --runs --vault ./my-notes --json`.

#### Scenario: reuse a render run to avoid long arguments
- **GIVEN** a previous render run named `video-go` exists
- **WHEN** the user runs `pinax template render video-study --run video-go --vault ./my-notes --json`
- **THEN** Pinax SHALL load the stored render args and context from the run receipt
- **AND** it SHALL execute the current template against the current local vault/index state
- **AND** it SHALL create a new render run rather than mutating the old run.

#### Scenario: explicit flags override reused run args
- **GIVEN** a previous render run named `video-go` stores `title="Go 模板学习"`
- **WHEN** the user runs `pinax template render video-study --run video-go --title "Go SQL 模板" --vault ./my-notes --json`
- **THEN** the explicit `--title` flag SHALL override the stored title for the new render
- **AND** the previous render run receipt SHALL remain unchanged.

#### Scenario: repeated save-run moves alias without deleting history
- **GIVEN** a render run named `协程快照` already exists for `notes/学习/galang高性能/1-协程.md`
- **WHEN** the user runs `pinax note refresh notes/学习/galang高性能/1-协程.md --rendered --save-run 协程快照 --yes --vault ./my-notes --json` again
- **THEN** Pinax SHALL create a new immutable run id and move the scoped alias `协程快照` to that new run
- **AND** the previous run SHALL remain accessible by run id.

#### Scenario: latest alias resolves within current scope
- **GIVEN** multiple successful render runs exist for `notes/学习/galang高性能/1-协程.md`
- **WHEN** the user runs `pinax note show notes/学习/galang高性能/1-协程.md --view rendered --snapshot latest --vault ./my-notes`
- **THEN** Pinax SHALL resolve `latest` to the newest successful run for that note only
- **AND** it SHALL NOT use a run from another note or template.

#### Scenario: ambiguous render run alias is rejected
- **GIVEN** the alias `日报` exists under more than one render run scope
- **WHEN** the user runs `pinax template render --run 日报 --vault ./my-notes --json` without a template or note context
- **THEN** Pinax SHALL fail with `render_run_ambiguous`
- **AND** the next action SHALL recommend using a run id, template name, or note path.

#### Scenario: invalid render run alias is rejected
- **GIVEN** a user tries to save a run alias containing a path separator or `..`
- **WHEN** the user runs `pinax template render video-study --title "Go 模板学习" --save-run ../bad --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with `render_run_alias_invalid`
- **AND** no render run, receipt, artifact, or index SHALL be written.

#### Scenario: inspect template render runs
- **GIVEN** one or more render runs reference the `video-study` template
- **WHEN** the user runs `pinax template inspect video-study --runs --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope listing run ids, names, timestamps, command names, template hashes, rendered hashes, row counts, and freshness warnings
- **AND** stdout SHALL NOT include rendered bodies, secret-like args, provider payloads, or raw SQL execution plans.

#### Scenario: show historical rendered snapshot without executing SQL
- **GIVEN** a render run named `video-go` has a stored `rendered.md` artifact
- **WHEN** the user runs `pinax note show projects/dashboard.md --view rendered --snapshot video-go --vault ./my-notes`
- **THEN** stdout SHALL contain the stored rendered Markdown snapshot
- **AND** Pinax SHALL NOT execute SQL queries, rebuild indexes, write Markdown files, update `.pinax` assets, call providers, or access network.

#### Scenario: refresh managed block from historical snapshot
- **GIVEN** `projects/dashboard.md` contains a matching managed render block
- **AND** a render run named `video-go` has a stored rendered artifact whose hash matches its receipt
- **WHEN** the user runs `pinax note refresh projects/dashboard.md --rendered --snapshot video-go --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL write only the managed render block from the stored snapshot
- **AND** it SHALL NOT re-execute SQL or change the source `pinax-sql` block.

#### Scenario: reject corrupted rendered snapshot
- **GIVEN** a render run receipt points to a `rendered.md` artifact whose hash does not match the receipt
- **WHEN** the user runs `pinax note show projects/dashboard.md --view rendered --snapshot video-go --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with `render_snapshot_hash_mismatch`
- **AND** no Markdown file or structured asset SHALL be modified.

#### Scenario: complete reusable render runs for template render
- **GIVEN** render runs exist for the `video-study` template
- **WHEN** the user requests shell completion for `pinax template render video-study --run <TAB>`
- **THEN** completion SHALL list only matching run names and run ids from `.pinax/renders/templates/video-study/index.json` or the lightweight root index
- **AND** completion descriptions SHALL include timestamp, target note when present, title or run name, row count when present, and freshness warning when available
- **AND** completion SHALL return `ShellCompDirectiveNoFileComp` and SHALL NOT execute SQL, render templates, rebuild indexes, write files, call providers, or access network.

#### Scenario: complete snapshots for a note
- **GIVEN** render runs exist for `notes/学习/galang高性能/1-协程.md`
- **WHEN** the user requests shell completion for `pinax note show notes/学习/galang高性能/1-协程.md --snapshot <TAB>`
- **THEN** completion SHALL list only runs under `.pinax/renders/学习/galang高性能/1-协程/` and matching aliases from the lightweight root index
- **AND** it SHALL NOT suggest snapshots for unrelated notes.

#### Scenario: list note render runs
- **GIVEN** render runs exist for `notes/学习/galang高性能/1-协程.md`
- **WHEN** the user runs `pinax note show notes/学习/galang高性能/1-协程.md --runs --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope listing run names, run ids, timestamps, artifact hashes, row counts, source hash, template hash, and freshness warnings for that note
- **AND** stdout SHALL NOT include full rendered bodies or secret-like args.

#### Scenario: completion falls back to local scoped scan
- **GIVEN** the note-scoped render run `index.json` is missing or unreadable
- **WHEN** the user requests completion for `pinax note refresh notes/学习/galang高性能/1-协程.md --rendered --snapshot <TAB>`
- **THEN** Pinax MAY scan only `.pinax/renders/学习/galang高性能/1-协程/` for receipts
- **AND** it SHALL NOT scan every render run in the vault.

#### Scenario: prune old template render runs in dry-run mode
- **GIVEN** the `video-study` template has more than 20 render runs
- **WHEN** the user runs `pinax template runs prune video-study --keep 20 --dry-run --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope listing runs that would be deleted and runs that would be retained
- **AND** no receipt, rendered artifact, index, Markdown file, Git state, provider state, or remote service SHALL be modified.

#### Scenario: prune old template render runs with approval
- **GIVEN** the `video-study` template has more than 20 render runs
- **WHEN** the user runs `pinax template runs prune video-study --keep 20 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL delete only old render runs in `.pinax/renders/templates/video-study/` according to the retention plan
- **AND** it SHALL keep the latest alias target and update related run indexes through the application service.

#### Scenario: repair render run indexes
- **GIVEN** root or scoped render run `index.json` files are missing or unreadable
- **WHEN** the user runs `pinax template runs repair --vault ./my-notes --json`
- **THEN** Pinax SHALL rebuild root and scoped render run indexes from existing receipts
- **AND** it SHALL NOT modify existing receipt contents, rendered artifacts, Markdown files, provider state, or remote services.

### Requirement: Template commands follow the AI-native CLI output contract

Template v2 commands SHALL render human, JSON, agent, events, and explain outputs from one projection and SHALL keep machine stdout stable and secret-free.

#### Scenario: JSON template output is machine-only
- **GIVEN** a user runs `pinax template inspect video-study --vault ./my-notes --json`
- **WHEN** the command completes or fails
- **THEN** stdout SHALL contain exactly one valid JSON object
- **AND** the JSON object SHALL include `spec_version`, `mode="json"`, `command`, `status`, and template-specific data
- **AND** progress, diagnostics, logs, ANSI tables, and localized prose SHALL not be written to JSON stdout.

#### Scenario: agent template output is stable key-value facts
- **GIVEN** a user runs `pinax template validate video-study --vault ./my-notes --agent`
- **WHEN** validation completes
- **THEN** stdout SHALL include `spec_version`, `mode=agent`, `command=template.validate`, `status`, `fact.template`, `fact.engine`, and `fact.issues`
- **AND** stdout SHALL NOT include Chinese prose, ANSI tables, raw template AST dumps, provider payloads, or secrets.

#### Scenario: explain output is a redacted reasoning summary
- **GIVEN** a user runs `pinax template validate video-study --vault ./my-notes --explain`
- **WHEN** validation completes
- **THEN** stdout SHALL include Chinese sections for conclusion, evidence, confidence, risk, tradeoff, and next action
- **AND** it SHALL NOT include full chain-of-thought, raw prompts, hidden system prompts, provider payloads, cookies, Authorization headers, or private tool arguments.

### Requirement: Template commands expose stable human and machine output
Pinax SHALL expose template create, inspect, preview, render, and delete workflows through stable CLI output modes, and SHALL include journal/index/note template metadata without leaking unsafe internals.

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
- **THEN** Pinax SHALL render preview output through the template engine and query service only when required local projections are already available
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

### Requirement: Pinax SHALL support reversible local proof-loop apply

Pinax SHALL provide a CLI-authored restore apply path so a bad local apply can be reverted from an existing snapshot/restore plan without direct file surgery by an agent.

#### Scenario: Restore apply refuses implicit writes

- **GIVEN** a restore plan exists for a vault snapshot
- **WHEN** the user runs restore apply without explicit approval
- **THEN** Pinax SHALL refuse to mutate the vault
- **AND** output SHALL include the exact approval flag or next command required.

#### Scenario: Restore apply restores local Markdown safely

- **GIVEN** a restore plan targets the current vault and a valid snapshot id
- **WHEN** the user runs `pinax version restore apply --yes --plan <path>` or the accepted equivalent command
- **THEN** Pinax SHALL restore local Markdown files from the snapshot plan
- **AND** SHALL write a restore receipt
- **AND** SHALL report `local_write=true` and `remote_write=false`
- **AND** SHALL NOT call provider, cloud sync or MCP write surfaces.

### Requirement: Pinax SHALL enforce shared projection redaction before rendering

Pinax SHALL apply one shared redaction gate to command projections before rendering default, `--json`, `--agent`, `--events`, `--explain` or evidence sidecars.

#### Scenario: Nested projection data is scanned

- **GIVEN** a command projection contains nested facts, actions, evidence, data, error or event payloads
- **WHEN** the projection is rendered
- **THEN** Pinax SHALL redact or reject forbidden protected content before stdout/stderr/evidence persistence
- **AND** forbidden content SHALL include note body sentinels, full body fields, Authorization headers, Bearer tokens, cookies, webhook URLs, provider payloads, raw prompts and hidden prompts.

### Requirement: Pinax SHALL expose a single agent-callable proof loop run command

Pinax SHALL provide one orchestration command for the local proof loop while preserving existing stage commands.

#### Scenario: Proof loop run defaults to preview mode

- **WHEN** an agent runs `pinax proof loop run --vault <vault>`
- **THEN** Pinax SHALL emit one bounded projection with `proof_loop_run_id`, ordered stage facts, evidence paths and next actions
- **AND** it SHALL NOT mutate the vault unless explicit apply flags are present.

#### Scenario: Proof loop run applies only approved safe operations

- **WHEN** an agent runs proof loop run with explicit apply approval
- **THEN** Pinax SHALL take a fresh snapshot before applying allowed repair or organize operations
- **AND** manual-review-only operations SHALL remain next actions instead of being auto-applied.

### Requirement: Proof-loop output contracts SHALL cover all rendering modes

Pinax SHALL contract-test proof-loop stage commands, proof-loop run and restore apply across default, `--json`, `--agent`, `--events` and `--explain` modes.

#### Scenario: Machine modes stay bounded and parseable

- **WHEN** proof-loop commands render machine output
- **THEN** `--json` SHALL emit one valid envelope
- **AND** `--agent` SHALL emit stable key=value facts
- **AND** `--events` SHALL emit start/end NDJSON events
- **AND** no mode SHALL leak note body, token, Authorization header, cookie, raw prompt, hidden prompt or provider payload.

#### Scenario: Explain mode is evidence summary, not chain-of-thought

- **WHEN** proof-loop commands render `--explain`
- **THEN** the output SHALL include conclusion, evidence, confidence or risk where applicable and next action
- **AND** it SHALL NOT include full chain-of-thought, raw prompts or hidden system prompts.

### Requirement: Pinax SHALL use GORM Gen for local index database access

Pinax SHALL route ordinary `.pinax/index.sqlite` projection reads and writes through GORM Gen generated DAO code. The local index remains a rebuildable projection of Markdown vault content, but field references, predicates, ordering and writes SHALL be type-backed rather than hardcoded SQL or direct GORM business chains.

#### Scenario: Index rebuild writes through generated DAO
- **GIVEN** a vault contains notes, tags, links, attachments, properties and assets
- **WHEN** `pinax index rebuild --vault <vault>` updates `.pinax/index.sqlite`
- **THEN** projection rows SHALL be created through generated DAO methods
- **AND** ordinary rebuild code SHALL NOT call `database/sql`, `Raw`, `Exec`, or hardcoded SQL verb strings.

#### Scenario: Search and lookup read through generated DAO
- **GIVEN** the local index exists
- **WHEN** Pinax lists notes, searches, resolves backlinks, checks assets, or serves readonly MCP resources
- **THEN** queries SHALL use generated DAO fields and predicates
- **AND** output ordering, machine fields, and stable error codes SHALL remain compatible with existing behavior.

#### Scenario: Schema exceptions stay centralized
- **GIVEN** Pinax needs connection, migration, transaction, or schema metadata behavior
- **WHEN** GORM Gen cannot express the operation safely
- **THEN** the exception SHALL live in a documented helper or migration boundary
- **AND** guard tests SHALL prevent ordinary index business files from reintroducing raw SQL or direct GORM query chains.

