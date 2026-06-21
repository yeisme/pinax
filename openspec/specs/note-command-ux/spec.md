# note-command-ux Specification

## Purpose

描述 Pinax 日常 note 命令的人机工效、引用解析、列表过滤、编辑边界和单笔维护行为，确保 note 创建、读取、编辑、移动、归档、删除和标签操作通过共享 projection 输出且不会泄露完整正文。
## Requirements
### Requirement: Note commands support ergonomic creation
Pinax SHALL let users create notes from multiple safe content sources while preserving Markdown body content as the content source and creating record ledger facts for machine identity and lifecycle.

#### Scenario: Add note through the recommended command
- **WHEN** a user runs `pinax note add "研究日志" --body "正文" --tags research --vault ./my-notes --json`
- **THEN** Pinax SHALL create the same registered Pinax note projection as `pinax note new`
- **AND** stdout SHALL contain one JSON envelope with command `note.new`, created path, note id, record facts, and next actions.

#### Scenario: Create note from inline body
- **WHEN** a user runs `pinax note new "研究日志" --body "正文" --tags research --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note with Pinax frontmatter and the provided body
- **AND** Pinax SHALL append a record ledger event and update the note registry for the created note
- **AND** stdout SHALL contain one JSON envelope with command `note.new`, created path, note id, record version, version backend, revision id, worktree state, title, and next actions.

#### Scenario: Create note from stdin
- **WHEN** a user pipes Markdown to `pinax note create "会议" --stdin --vault ./my-notes --json`
- **THEN** Pinax SHALL read note body from stdin through the command layer
- **AND** the application service SHALL create the note and its record ledger facts without reading external network resources.

#### Scenario: Reject conflicting note body sources
- **WHEN** a user runs `pinax note new "x" --body a --from ./a.md --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `note_source_conflict`
- **AND** no note file or record ledger event SHALL be created.

#### Scenario: Dry run note creation
- **WHEN** a user runs `pinax note new "Draft" --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return planned path, frontmatter, record event preview, registry preview, and body preview
- **AND** it SHALL NOT write Markdown files, `.pinax/` state, Git state, provider state, or remote services.

### Requirement: Note discovery requires explicit Pinax registration
Pinax SHALL treat Markdown files as ordinary notes only when they carry Pinax note frontmatter created or normalized through Pinax commands.

#### Scenario: Ignore unmanaged Markdown in note projections
- **WHEN** a vault contains `notes/raw.md` without `schema_version: pinax.note.v1`
- **AND** a user runs `pinax note list`, `pinax search`, `pinax index rebuild`, or `pinax note show raw.md`
- **THEN** Pinax SHALL NOT include that unmanaged Markdown file in ordinary note projections
- **AND** users SHALL add or import notes through commands such as `pinax note add ...` or `pinax import markdown ... --yes`.

### Requirement: Note creation builds notebook information architecture
Pinax SHALL make newly created notes immediately discoverable through group, folder, kind, tags, daily index, record ledger, and local index projections.

#### Scenario: Create note with group folder and kind
- **WHEN** a user runs `pinax note new "工具笔记" --group work --folder inbox --kind reference --tags pinax,cli --vault ./my-notes --json`
- **THEN** Pinax SHALL create the note under the selected group/project prefix and folder
- **AND** the note frontmatter SHALL include `project`, `folder`, `kind`, and `tags`
- **AND** the record ledger SHALL store the note id, path, lifecycle state, record version, schema version, and content hash
- **AND** the JSON envelope facts SHALL include group, folder, kind, daily index path, record update status, version evidence status, and index update status.

#### Scenario: Created note is added to daily index
- **WHEN** a note is created without `--dry-run`
- **THEN** Pinax SHALL update `notes/daily/YYYY-MM-DD.md` through the application service
- **AND** the daily index SHALL include the note title, path, tags, group, folder, kind, and note id.

#### Scenario: Created note refreshes local index
- **WHEN** a note is created without `--dry-run`
- **THEN** Pinax SHALL refresh `.pinax/index.sqlite` through the GORM index service using ledger and Markdown inputs
- **AND** a following `pinax stats --vault ./my-notes --json` SHALL report `index_status=fresh`.

### Requirement: Note references resolve without requiring exact paths
Pinax SHALL resolve note references by note id, path, `notes/` prefix tolerant path, exact title, or unique title match.

#### Scenario: Show note by unique title
- **WHEN** a vault contains one note titled `研究日志`
- **AND** a user runs `pinax note show "研究日志" --vault ./my-notes --json`
- **THEN** Pinax SHALL read that note and include its path and note id in the projection.

#### Scenario: Ambiguous title returns candidates
- **WHEN** a vault contains multiple notes titled `会议`
- **AND** a user runs `pinax note show "会议" --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `note_ref_ambiguous`
- **AND** the error projection SHALL include candidate paths or note ids.

### Requirement: Note list is filterable and scannable
Pinax SHALL let users list notes by useful local vault dimensions.

#### Scenario: List recent notes with filters
- **WHEN** a user runs `pinax note list --tag research --project work --status active --recent --limit 20 --vault ./my-notes`
- **THEN** stdout SHALL contain a concise Chinese human summary and a scannable list of matching notes
- **AND** diagnostics SHALL go to stderr.

#### Scenario: List notes as JSON
- **WHEN** a user runs `pinax note list --tag research --limit 20 --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with filter facts, total count, returned count, and notes
- **AND** stdout SHALL NOT contain human prose outside JSON.

### Requirement: Note list supports notebook organization filters
Pinax SHALL allow note listing to filter by notebook organization dimensions used by the core workflows.

#### Scenario: List notes by folder and kind
- **WHEN** a user runs `pinax note list --group work --folder inbox --kind reference --status active --vault ./my-notes --json`
- **THEN** Pinax SHALL return only notes matching the selected group, folder, kind, and status
- **AND** JSON facts SHALL include each selected filter using stable keys.

#### Scenario: List notes by date range
- **WHEN** a user runs `pinax note list --created-after 2026-01-01 --updated-before 2026-02-01 --vault ./my-notes --json`
- **THEN** Pinax SHALL filter notes by frontmatter or filesystem timestamps when frontmatter is missing
- **AND** invalid date values SHALL fail with stable error code `invalid_date_filter`.

### Requirement: Note commands expose link and attachment subcommands
Pinax SHALL expose link, backlink, orphan, graph-context, and attachment inspection from the note command surface.

#### Scenario: Note help includes relationship commands
- **WHEN** a user runs `pinax note --help`
- **THEN** help output SHALL include links, backlinks, orphans, attach, and attachments commands
- **AND** help text SHALL describe local Markdown note relationships.

#### Scenario: Note relationship commands follow output contract
- **WHEN** a user runs a note relationship command with `--agent` or `--json`
- **THEN** stdout SHALL contain only the selected machine format
- **AND** diagnostics SHALL go to stderr.

#### Scenario: Note links supports relationship filters
- **WHEN** a user runs `pinax note links note_123 --broken-only --kind wiki --vault ./my-notes --json`
- **THEN** Pinax SHALL return only matching outgoing link edges
- **AND** stdout facts SHALL include path, note id when available, links, resolved, broken, ambiguous, ignored, kind filter, and engine.

#### Scenario: Note backlinks supports bounded output
- **WHEN** a user runs `pinax note backlinks note_123 --limit 20 --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable low-token key=value facts for backlink count, returned count, broken count, ambiguous count, index status, and next action when more results exist
- **AND** stdout SHALL NOT include localized prose, raw note bodies, provider payloads, or secrets.

#### Scenario: Ambiguous note reference returns candidates
- **WHEN** a user runs `pinax note links "会议" --vault ./my-notes --json`
- **AND** multiple notes match the note reference
- **THEN** Pinax SHALL fail with stable error code `note_ref_ambiguous`
- **AND** the error projection SHALL include candidate paths or note ids.

#### Scenario: Explain output summarizes link decisions
- **WHEN** a user runs `pinax note backlinks note_123 --vault ./my-notes --explain`
- **THEN** stdout SHALL contain a Chinese explanation summary with conclusion, evidence, risk, and recommended next action
- **AND** stdout SHALL NOT include full chain-of-thought, raw prompts, hidden system prompts, secrets, or provider payloads.

### Requirement: Note maintenance supports inbox triage semantics
Pinax SHALL let inbox triage reuse safe note move and metadata patch behavior.

#### Scenario: Move note while updating folder and kind
- **WHEN** a user runs `pinax note move note_123 work/ideas --kind reference --status active --vault ./my-notes --json`
- **THEN** Pinax SHALL move the note inside the vault and update selected frontmatter fields
- **AND** it SHALL preserve unknown frontmatter fields where practical.

### Requirement: Note editing uses an explicit editor boundary
Pinax SHALL open notes in an editor only when requested and SHALL keep editor execution testable and local.

#### Scenario: Open existing note in editor
- **WHEN** a user runs `pinax note edit note_123 --editor fake-editor --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve the note inside the vault and execute the editor with the local note path
- **AND** stdout SHALL contain a projection with editor command, note path, and status.

#### Scenario: Missing editor fails clearly
- **WHEN** a user runs `pinax note edit note_123 --vault ./my-notes` and no editor is configured
- **THEN** Pinax SHALL fail with stable error code `editor_not_configured`
- **AND** it SHALL suggest setting `$EDITOR` or passing `--editor`.

### Requirement: Single-note maintenance operations are safe
Pinax SHALL support common single-note maintenance operations while protecting vault boundaries, destructive actions, and index projection consistency.

#### Scenario: Rename note title and path
- **WHEN** a user runs `pinax note rename note_123 "New Title" --vault ./my-notes --json`
- **THEN** Pinax SHALL update frontmatter title and choose a safe target path inside the vault
- **AND** it SHALL fail with stable error code `note_path_conflict` if target path already exists
- **AND** it SHALL process or enqueue a structured index event for the rename with old path, new path, note id, and content hash evidence.

#### Scenario: Archive note without moving file
- **WHEN** a user runs `pinax note archive note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL set frontmatter `status: archived`
- **AND** it SHALL NOT move or delete the Markdown file
- **AND** it SHALL update status property and search/list projection incrementally.

#### Scenario: Delete note moves to trash by default
- **WHEN** a user runs `pinax note delete note_123 --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL move the note to `.pinax/trash/` through the application service
- **AND** it SHALL append redacted event evidence
- **AND** it SHALL process or enqueue a delete index event that removes ordinary note projection and updates affected backlinks.

#### Scenario: Hard delete requires explicit hard approval
- **WHEN** a user runs `pinax note delete note_123 --hard --json` without `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** the note file SHALL remain unchanged.

#### Scenario: Move note updates index facts
- **WHEN** a user runs `pinax note move note_123 archive --vault ./my-notes --json`
- **THEN** Pinax SHALL move the note inside the vault and update selected frontmatter fields when requested
- **AND** JSON facts SHALL include path, note id, index event kind, index update status, and affected projection counts when available.

#### Scenario: Editor mutation refreshes changed note projection
- **WHEN** a user runs `pinax note edit note_123 --editor fake-editor --vault ./my-notes --json`
- **AND** the editor changes the Markdown file
- **THEN** Pinax SHALL detect the changed content hash after editor exit
- **AND** it SHALL update or enqueue incremental projection refresh for that note.

### Requirement: Note tags are manageable from the CLI
Pinax SHALL let users add, remove, and set frontmatter tags without hand-editing machine-readable metadata, and SHALL ensure tag updates cannot corrupt CLI-authored YAML frontmatter or inject unrelated metadata fields.

#### Scenario: Add note tag
- **WHEN** a user runs `pinax note tag add note_123 research --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the target note frontmatter tags through the application service
- **AND** duplicate tags SHALL NOT be added
- **AND** stdout SHALL include stable facts for updated tags and either index update status or explicit stale index status.

#### Scenario: Remove note tag
- **WHEN** a user runs `pinax note tag remove note_123 inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL remove the tag if present
- **AND** it SHALL return a stable projection even if the tag was already absent.

#### Scenario: Reject unsafe tag value
- **WHEN** a user runs `pinax note new "Unsafe" --tags $'safe,bad]\nstatus: archived' --vault ./my-notes --json` or `pinax note tag add note_123 $'bad]\nstatus: archived' --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `invalid_tag`
- **AND** no note file, frontmatter field, record event, index projection, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Tag frontmatter remains parseable YAML
- **WHEN** Pinax writes tags through `note new`, `note tag add`, `note tag remove`, `note tag set`, import defaults, repair apply, or organize metadata operations
- **THEN** the resulting frontmatter SHALL remain valid YAML with `tags` represented as a list of tag strings
- **AND** user-authored unrelated metadata fields SHALL be preserved where practical.

### Requirement: Query commands expose stable database output
Pinax SHALL expose database query, Dataview-compatible query, and table view commands with stable human and machine output.

#### Scenario: Query run supports SQL v2 aggregation
- **WHEN** a user runs `pinax query run 'SELECT status, COUNT(*) AS count FROM notes WHERE tags CONTAINS "project" GROUP BY status LIMIT 20' --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with command `query.run`
- **AND** the envelope SHALL preserve existing top-level fields and existing `query.run` facts
- **AND** `data` SHALL include selected columns, grouped rows, aggregate facts, page facts, engine, and index status
- **AND** stdout SHALL NOT include full note bodies, raw SQL execution traces, secrets, raw prompts, provider payloads, or full chain-of-thought.

#### Scenario: Query explain reports unsupported SQL safely
- **WHEN** a user runs `pinax query explain 'SELECT title FROM notes JOIN tasks ON notes.id = tasks.note_id' --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `sql_unsupported_clause`
- **AND** the next action SHALL recommend a supported `FROM notes` or `FROM tasks` query instead of exposing raw SQLite details.

### Requirement: Note list can include selected properties
Pinax SHALL let note list expose selected database properties while preserving existing note list behavior.

#### Scenario: Note list with selected properties
- **WHEN** a user runs `pinax note list --property status --property due --tag project --vault ./my-notes --json`
- **THEN** Pinax SHALL return matching notes with selected property values
- **AND** existing note identity fields such as path, title, note id, tags, kind, and status SHALL remain stable.

#### Scenario: Invalid property selection fails clearly
- **WHEN** a user runs `pinax note list --property unknown --strict-properties --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `property_not_found`
- **AND** no Markdown file, index database, or structured asset SHALL be modified.

### Requirement: Database command help is user-runnable
Pinax SHALL document database query and Dataview-compatible commands with real local examples and without requiring external provider credentials.

#### Scenario: Database help shows Dataview workflow
- **WHEN** a user runs `pinax dataview --help`, `pinax query --help`, or `pinax database view --help`
- **THEN** help output SHALL include local examples for `dataview run`, `dataview explain`, `query run`, `query explain`, and `database view save/show/render`
- **AND** each example SHALL be directly runnable against a local vault path
- **AND** help output SHALL NOT require Notion API tokens, Obsidian plugin execution, JavaScript, Lark, firecrawl, or external network access.

### Requirement: Completion assists local notebook workflows
Pinax SHALL provide shell completion for database, query, view, search, and note-list workflows using only local vault state and static option sets.

#### Scenario: Complete existing saved views
- **WHEN** a user completes `pinax database view show <TAB>` or `pinax view show <TAB>` against a local vault
- **THEN** completion SHALL list only existing saved view names from CLI-authored view metadata
- **AND** completion descriptions SHOULD include view kind or filter summary when available
- **AND** completion SHALL NOT create, mutate, rebuild, delete, or remotely sync any asset.

#### Scenario: Complete local dimensions and properties
- **WHEN** a user completes flags such as `--tag`, `--group`, `--folder`, `--kind`, `--status`, `--sort`, `--property`, `--column`, or `--type`
- **THEN** completion SHALL return matching local dimensions, inferred properties, or static enum values with concise descriptions
- **AND** it SHALL avoid suggesting missing journal dates, missing views, or non-existent local dimensions.

#### Scenario: Completion degrades without index
- **WHEN** the local index is missing or stale during completion
- **THEN** completion SHALL use cheap local metadata reads or static options when possible
- **AND** it SHALL NOT trigger lazy index rebuild or expensive query execution during shell completion.

### Requirement: Help guides first successful use
Pinax SHALL make common database/search/view workflows discoverable from help output and command errors.

#### Scenario: Query help shows workflow order
- **WHEN** a user runs `pinax query --help`, `pinax database --help`, or `pinax database view --help`
- **THEN** help output SHALL show a short local workflow: inspect/rebuild index, run `query explain`, run `query run`, save a view, show a view
- **AND** each example SHALL be directly runnable against a local vault path.

#### Scenario: Errors include next action
- **WHEN** a query, database view, or property selection fails because an index, schema, view, property, query, approval, or argument is missing
- **THEN** the projection SHALL include a stable error code and a concrete next command
- **AND** machine modes SHALL expose the same next action without parsing localized prose.

### Requirement: Note commands expose index update facts
Pinax SHALL expose stable index update facts for note commands that mutate Markdown files.

#### Scenario: Mutation reports committed index update
- **WHEN** a note mutation command updates the index projection synchronously
- **THEN** stdout facts SHALL include `index_update=committed`, `index_status=fresh`, and the relevant event kind
- **AND** machine output SHALL remain parseable under the CLI output contract.

#### Scenario: Mutation reports queued index update
- **WHEN** a note mutation command cannot wait for incremental index completion
- **THEN** stdout facts SHALL include `index_update=queued` and a next action for `pinax index status --refresh`
- **AND** the command SHALL NOT claim the index is fresh until the update has committed.

### Requirement: Note reference commands use the shared vault object resolver
Pinax SHALL resolve note references consistently across note read/show/link/backlink/mutation commands using the shared resolver.

#### Scenario: Show note by filename stem
- **WHEN** a user runs `pinax note show yeisme --vault ./my-notes --json`
- **THEN** Pinax SHALL match a unique registered note by note id, path, filename, stem, title, or alias
- **AND** stdout SHALL include resolver facts such as match field and candidate count.

#### Scenario: Ambiguous note mutation is rejected
- **WHEN** a user runs `pinax note rename yeisme "New" --vault ./my-notes --json`
- **AND** multiple registered notes match `yeisme`
- **THEN** Pinax SHALL fail with stable error code `note_ref_ambiguous`
- **AND** stdout SHALL include candidates without modifying note files, record events, index rows, or version state.

### Requirement: Metadata planning accepts optional note query
Pinax SHALL allow metadata planning to target one resolved note or adoptable Markdown candidate while preserving full-vault planning when no query is provided.

#### Scenario: Plan metadata for one file
- **WHEN** a user runs `pinax metadata plan yeisme --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve `yeisme` through registered-or-adoptable scope
- **AND** stdout SHALL contain metadata operations only for that resolved object.

#### Scenario: Metadata plan does not adopt unmanaged files implicitly
- **WHEN** `pinax metadata plan yeisme --vault ./my-notes --json` resolves an unmanaged Markdown file
- **THEN** Pinax SHALL report mirror operations and a next action for `pinax record adopt yeisme --plan`
- **AND** it SHALL NOT create record ledger events unless an explicit record adopt apply command is approved.

### Requirement: Note preview shows scannable content and tags
Pinax SHALL render note preview output in default human mode as a concise Chinese metadata summary followed by the preview body and visible tag context.

#### Scenario: Preview note with tags in default mode
- **WHEN** a user runs `pinax note preview "Tagged Preview" --vault ./my-notes`
- **THEN** stdout SHALL include the note title, tag list, rendered view, and preview body
- **AND** diagnostics SHALL go to stderr.

#### Scenario: Preview note keeps machine output structured
- **WHEN** a user runs `pinax note preview "Tagged Preview" --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with command `note.preview`, note metadata, view, and body data
- **AND** stdout SHALL NOT contain human table decoration outside JSON.

### Requirement: Note tag dimensions are visually scannable
Pinax SHALL make default note tag dimension output scannable with count, percentage, and a plain-text heat bar while preserving stable machine output.

#### Scenario: List note tags in default mode
- **WHEN** a user runs `pinax note tags --vault ./my-notes`
- **THEN** stdout SHALL include a concise Chinese summary and a table with tag value, count, percentage, and heat bar
- **AND** the table SHALL remain readable without ANSI color.

#### Scenario: List note tags as JSON
- **WHEN** a user runs `pinax note tags --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with dimension facts and item counts
- **AND** stdout SHALL NOT include the human-only heat bar text as localized prose.

### Requirement: Note properties are manageable from the CLI
Pinax SHALL let users set and remove non-reserved note frontmatter properties through CLI commands backed by the application service.

#### Scenario: Set note property
- **WHEN** a user runs `pinax note property set note_123 priority 2 --vault ./my-notes --json`
- **THEN** Pinax SHALL update the target note frontmatter with `priority: 2`
- **AND** stdout SHALL contain one JSON envelope with command `note.property`, property name, operation, record facts, and index update facts.

#### Scenario: Remove note property
- **WHEN** a user runs `pinax note property remove note_123 priority --vault ./my-notes --agent`
- **THEN** Pinax SHALL remove the `priority` frontmatter field when present
- **AND** stdout SHALL include stable agent facts for command, operation, property, and index update status.

#### Scenario: Reserved property is rejected
- **WHEN** a user runs `pinax note property set note_123 tags urgent --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with a stable validation error
- **AND** it SHALL NOT bypass the dedicated tag management commands for structured tags.

### Requirement: Tag taxonomy supports controlled bulk updates
Pinax SHALL support controlled bulk tag rename and delete operations across registered notes while requiring an explicit preview or confirmation mode.

#### Scenario: Dry-run tag rename
- **WHEN** a user runs `pinax note tags rename old new --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL report matched and changed note counts without modifying Markdown files, `.pinax/` state, provider state, or remote services.

#### Scenario: Confirmed tag rename
- **WHEN** a user runs `pinax note tags rename old new --yes --vault ./my-notes --agent`
- **THEN** Pinax SHALL update matching note frontmatter tags through the application service
- **AND** stdout SHALL include stable facts for old tag, new tag, matched count, changed count, write status, record event count, and index update status.

#### Scenario: Confirmed tag delete
- **WHEN** a user runs `pinax note tags delete stale --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL remove that tag from all registered notes that contain it
- **AND** it SHALL refresh the local index projection after writing changed notes.

#### Scenario: Bulk tag write requires confirmation
- **WHEN** a user runs `pinax note tags delete stale --vault ./my-notes --json` without `--dry-run` or `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** no Markdown file, record ledger, index database, provider state, or remote service SHALL be modified.

### Requirement: Folder taxonomy supports controlled bulk rename
Pinax SHALL support controlled bulk folder rename across registered notes while preserving Markdown files as the source of truth.

#### Scenario: Dry-run folder rename
- **WHEN** a user runs `pinax note folders rename inbox archive --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL report matched note count, changed note count, old folder, new folder, and planned target paths
- **AND** it SHALL NOT modify Markdown files, record ledger, index database, provider state, or remote services.

#### Scenario: Confirmed folder rename
- **WHEN** a user runs `pinax note folders rename inbox archive --yes --vault ./my-notes --agent`
- **THEN** Pinax SHALL move matching note files into the target folder and update frontmatter `folder: archive`
- **AND** stdout SHALL include stable facts for old folder, new folder, matched count, changed count, write status, record event count, and index update status.

#### Scenario: Folder rename requires confirmation
- **WHEN** a user runs `pinax note folders rename inbox archive --vault ./my-notes --json` without `--dry-run` or `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** no Markdown file, record ledger, index database, provider state, or remote service SHALL be modified.

#### Scenario: Folder rename rejects path conflicts before writing
- **WHEN** a confirmed folder rename would overwrite an existing note path or make two notes target the same path
- **THEN** Pinax SHALL fail with stable error code `note_path_conflict`
- **AND** it SHALL NOT partially move notes or update frontmatter.

### Requirement: Dataview-compatible query commands are available
Pinax SHALL provide a safe Dataview-compatible command surface for common Markdown database queries while compiling every query into Pinax's bounded query engine.

#### Scenario: Run Dataview table query
- **WHEN** a user runs `pinax dataview run 'TABLE title, status, due FROM #project WHERE status != "done" SORT due ASC LIMIT 20' --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with command `dataview.run`
- **AND** facts SHALL include language `dataview`, result kind `table`, source `notes`, row count, columns, engine, index status, and page facts
- **AND** rows SHALL contain bounded note/database projections without full note bodies.

#### Scenario: Run Dataview task query
- **WHEN** a user runs `pinax dataview run 'TASK FROM #project WHERE !completed SORT due ASC LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL return task rows derived from local Markdown task list items
- **AND** each row SHALL include source note path, line number, task text, completion state, due/scheduled dates when available, and stable note identity
- **AND** the command SHALL NOT call external Todo providers or write the vault.

#### Scenario: Reject DataviewJS
- **WHEN** a user runs `pinax dataview run 'dataviewjs await fetch("https://example.test")' --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `dataviewjs_unsupported`
- **AND** no network, shell, JavaScript, provider, `.pinax` metadata, Git, or remote state SHALL be touched.

### Requirement: Database views support Dataview-style result kinds
Pinax SHALL let users save and render local database views for table, list, task, calendar, and board-style projections through CLI-authored view metadata.

#### Scenario: Save Dataview-backed database view
- **WHEN** a user runs `pinax database view save project-dashboard --query 'TABLE title, status FROM #project LIMIT 20' --language dataview --kind table --vault ./my-notes --json`
- **THEN** Pinax SHALL write or update `.pinax/views.json` through the application service
- **AND** the view definition SHALL record query language, query text, result kind, selected columns, display options, created/updated timestamps, and schema version
- **AND** stdout SHALL include command `database.view.save`, write facts, and a next action to show or render the view.

#### Scenario: Render saved database view as Markdown
- **WHEN** a user runs `pinax database view render project-dashboard --format markdown --vault ./my-notes --json`
- **THEN** Pinax SHALL execute the saved query through the same bounded query service
- **AND** `data` SHALL include a Markdown artifact or rendered block plus row/page facts
- **AND** rendering SHALL NOT write note files unless a separate explicit refresh/apply command is used.

### Requirement: Dataview managed blocks refresh safely
Pinax SHALL support managed Dataview-style fenced blocks in notes without allowing unmanaged body rewrites.

#### Scenario: Preview Dataview block without writing
- **WHEN** a note contains a fenced `pinax-dataview` query block
- **AND** a user runs `pinax note preview Dashboard --vault ./my-notes --json`
- **THEN** Pinax SHALL render a bounded preview of the query output
- **AND** no Markdown file, `.pinax` metadata, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Refresh managed Dataview block
- **WHEN** a note contains a `pinax:managed` Dataview block
- **AND** a user runs `pinax note refresh Dashboard --rendered --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the managed block output generated by Pinax
- **AND** stdout SHALL include changed block count, query count, index status, and record/index update facts
- **AND** user-authored prose outside the managed block SHALL remain unchanged.

