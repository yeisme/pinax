## MODIFIED Requirements

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
