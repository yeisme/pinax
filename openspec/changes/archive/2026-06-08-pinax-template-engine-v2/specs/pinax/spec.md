## ADDED Requirements

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
- **AND** it SHALL render a bounded Markdown preview of the query result
- **AND** it SHALL NOT treat the fenced block as JavaScript, shell, raw SQLite SQL, or arbitrary Markdown to execute.

#### Scenario: inspect query-backed template without full execution
- **GIVEN** a v2 template declares one or more queries
- **WHEN** the user runs `pinax template inspect project-dashboard --vault ./my-notes --json`
- **THEN** Pinax SHALL parse query declarations and return explain-style metadata such as language, selected columns, limit, warnings, and unsupported clauses
- **AND** it SHALL NOT render full query rows, write notes, update `.pinax` assets, call providers, or access network.

#### Scenario: reject unsupported query clauses in template
- **GIVEN** a v2 template declares a query with unsupported SQL clauses such as `JOIN`, subqueries, shell calls, network calls, or JavaScript execution
- **WHEN** the user runs `pinax template validate project-dashboard --vault ./my-notes --json`
- **THEN** Pinax SHALL report `template_query_parse_failed`, `sql_unsupported_clause`, or `sql_forbidden_function`
- **AND** no Markdown file, `.pinax` structured asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: query template output is bounded
- **GIVEN** a query-backed template query matches more rows than the template or command limit allows
- **WHEN** the user runs `pinax template render project-dashboard --vault ./my-notes --json`
- **THEN** Pinax SHALL return only the bounded result set
- **AND** the projection SHALL include page or `has_more` facts when available
- **AND** it SHALL NOT include full note bodies by default.

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
