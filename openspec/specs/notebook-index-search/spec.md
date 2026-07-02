# notebook-index-search Specification

## Purpose

定义 Pinax 本地 SQLite/GORM 索引、搜索过滤和 agent organize 计划的稳定行为。Markdown vault 是真源，索引是可重建 projection。
## Requirements
### Requirement: Local index database is initialized and rebuilt through Pinax
Pinax SHALL create and maintain a local SQLite/GORM index projection for notebook search and organization without making the database the source of truth, SHALL support incremental maintenance after the initial rebuild, and SHALL distinguish system index pages from ordinary user notes.

#### Scenario: Rebuild index with root-layout system index pages classified
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL scan registered Pinax Markdown notes inside the vault boundary and rebuild note, text, tag, token, link, attachment, and dimension count projections through GORM
- **AND** notes with `kind: index` under `index/` SHALL be classified as system index pages
- **AND** system index pages SHALL be excluded from ordinary note statistics, orphan detection, and default recent-note index page queries unless explicitly included.

#### Scenario: Rebuild keeps legacy notes index pages compatible
- **GIVEN** an older vault contains a registered system index page under `notes/index/`
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL classify the legacy page as a system index page for compatibility
- **AND** it SHALL NOT move, rewrite, or delete the legacy page during rebuild.

### Requirement: Search uses local index with safe fallbacks
Pinax SHALL search the local notebook using the index when fresh and degrade to local scan or ripgrep fallback when needed, while preserving system index page filtering semantics.

#### Scenario: Default search excludes system index pages
- **GIVEN** `index/home.md` is a registered Pinax note with `kind: index` and `status: system`
- **WHEN** a user runs `pinax search "首页" --vault ./my-notes --json`
- **THEN** Pinax SHALL exclude the system index page from ordinary search results by default
- **AND** stdout facts SHALL identify whether system notes were included or excluded.

### Requirement: Search filters cover notebook organization dimensions
Pinax SHALL let users combine full-text query with local notebook filters, including bidirectional link relationship filters.

#### Scenario: Filter search by organization dimensions
- **WHEN** a user runs `pinax search "设计" --group work --folder architecture --kind reference --status active --vault ./my-notes --json`
- **THEN** Pinax SHALL return only matching notes
- **AND** JSON facts SHALL include stable keys for group, folder, kind, and status filters.

#### Scenario: Filter search by links and attachments
- **WHEN** a user runs `pinax search "" --link-target "Auth" --has-attachment --vault ./my-notes --json`
- **THEN** Pinax SHALL return notes with matching resolved or unresolved link targets and at least one attachment reference
- **AND** each result SHALL include link and attachment summary counts.

#### Scenario: Filter search by resolved backlink target
- **WHEN** a user runs `pinax search "" --link-target note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return notes whose outgoing link graph resolves to the selected note
- **AND** facts SHALL include `link_target`, `resolved`, `broken`, `ambiguous`, `engine`, and `index_status`.

#### Scenario: Filter search by ambiguous link target
- **WHEN** a user runs `pinax search "" --link-target "会议" --vault ./my-notes --json`
- **AND** multiple notes can satisfy the target
- **THEN** Pinax SHALL fail with stable error code `link_target_ambiguous` or return partial facts with candidate paths
- **AND** it SHALL NOT choose one target automatically.

#### Scenario: Invalid search filter fails clearly
- **WHEN** a user runs `pinax search "x" --updated-after not-a-date --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `invalid_date_filter`
- **AND** no index database or Markdown file SHALL be modified.

### Requirement: Agent organize suggestions are reviewable plans
Pinax SHALL let agents generate local organize suggestions as reviewable plans rather than directly editing notes.

#### Scenario: Generate organize suggestions
- **WHEN** an agent runs `pinax organize suggest --vault ./my-notes --save --json`
- **THEN** Pinax SHALL read notes and index projection through application services
- **AND** it SHALL save `.pinax/organize-plans/<plan_id>.json` through the service
- **AND** the plan SHALL include operations with kind, mode, risk, path, target, reason, and evidence.

#### Scenario: Agent output exposes low-token organize facts
- **WHEN** an agent runs `pinax organize suggest --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable key=value lines for plan id, operation counts, automatic count, manual review count, risk counts, and save path when present
- **AND** stdout SHALL NOT include localized prose, raw prompts, provider payloads, or secrets.

#### Scenario: Apply saved organize plan with snapshot protection
- **WHEN** a user runs `pinax organize apply --plan organize_123 --yes --snapshot-message "整理前快照" --vault ./my-notes --json`
- **THEN** Pinax SHALL ensure a Git snapshot exists or create one with the supplied message
- **AND** it SHALL apply only approved low-risk operations through the application service
- **AND** it SHALL refresh the local index after successful writes.

#### Scenario: Reject stale organize plan
- **WHEN** a saved organize plan source facts no longer match the current vault
- **AND** a user runs `pinax organize apply --plan organize_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `plan_stale`
- **AND** stdout SHALL include an action recommending `pinax organize suggest --save`.

### Requirement: Organize suggestions are explainable and conservative
Pinax SHALL base automatic organization suggestions on local evidence and avoid high-risk mutations.

#### Scenario: Suggest metadata and path operations with evidence
- **WHEN** `pinax organize suggest --vault ./my-notes --json` analyzes notes with missing kind, missing status, or mismatched folder
- **THEN** suggested operations SHALL include evidence from title, tags, project metadata, current path, links, or saved views
- **AND** each operation SHALL be classified as automatic, manual_review, low, medium, or review risk.

#### Scenario: High-risk operations require manual review
- **WHEN** organize suggestion detects duplicate titles, possible merges, destructive deletes, body link rewrites, or broad folder moves
- **THEN** Pinax SHALL emit manual_review operations
- **AND** `organize apply` SHALL NOT perform those operations automatically.

### Requirement: Query planner uses safe indexed execution
Pinax SHALL plan database queries from a validated query AST and execute them through repository boundaries with parameter binding.

#### Scenario: Plan property-filtered table query
- **WHEN** a user runs `pinax query explain 'SELECT title, due FROM notes WHERE tags CONTAINS "project" AND status = "active" AND due <= date(today) ORDER BY due ASC LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL report source filters, property filters, selected indexes, selected properties, sort keys, limit, and fallback warnings
- **AND** it SHALL NOT expose raw SQL text by default.

#### Scenario: Execute selected property query
- **WHEN** a table query selects `title`, `status`, and `due`
- **THEN** the repository SHALL load only those selected properties plus row identity and required sort/filter fields
- **AND** it SHALL NOT load note bodies by default.

#### Scenario: Reject unsafe query at planner boundary
- **WHEN** query validation detects unsupported clauses, forbidden functions, path escape attempts, or excessive limits
- **THEN** Pinax SHALL fail before repository execution with a stable error code
- **AND** no index database or Markdown file SHALL be modified.

### Requirement: Query pagination and limits are stable
Pinax SHALL support stable limits and cursor pagination for database query results.

#### Scenario: Default query limit applies
- **WHEN** a query omits `LIMIT`
- **THEN** Pinax SHALL apply a safe default result limit
- **AND** stdout SHALL include an action or cursor for retrieving more rows when more matches exist.

#### Scenario: Cursor returns next page
- **WHEN** a query result includes `has_more=true`
- **AND** a user reruns the query with the returned cursor
- **THEN** Pinax SHALL return the next stable page for the same query shape
- **AND** stdout SHALL include updated pagination facts.

### Requirement: Search explains record consistency risks
Pinax SHALL expose whether search results came from consistent ledger-backed records, unregistered Markdown files, missing files, or stale projections.

#### Scenario: Search returns consistency facts
- **WHEN** a user runs `pinax search "设计" --vault ./my-notes --json`
- **THEN** each result SHALL include stable consistency facts such as `record_status`, `lifecycle_status`, `record_version`, `ledger_seq`, `index_epoch`, `content_hash`, `path_status`, `version_backend`, `revision_id`, and `worktree_state`
- **AND** results with unresolved ledger or frontmatter issues SHALL include a next action for record status or repair planning.

#### Scenario: Search excludes deleted records by default
- **GIVEN** the record ledger contains trashed or deleted notes
- **WHEN** a user runs `pinax search "设计" --vault ./my-notes --json`
- **THEN** Pinax SHALL exclude deleted lifecycle states by default
- **AND** it SHALL include them only when the user selects an explicit trash, deleted, or all lifecycle filter.

### Requirement: Search supports version-aware queries
Pinax SHALL support version-aware local search by using version backend evidence and SHALL clearly report when historical content cannot be read by the configured backend.

#### Scenario: Search at current HEAD
- **GIVEN** the vault is Git-backed and the current HEAD is readable
- **WHEN** a user runs `pinax search "设计" --at HEAD --vault ./my-notes --json`
- **THEN** Pinax SHALL search content associated with HEAD or a HEAD index snapshot
- **AND** each result SHALL include revision id, file blob id when available, ledger sequence, index epoch, and whether dirty worktree changes were excluded.

#### Scenario: Search including dirty changes
- **GIVEN** the vault has uncommitted Markdown changes
- **WHEN** a user runs `pinax search "设计" --include-dirty --vault ./my-notes --json`
- **THEN** Pinax SHALL search the current working tree projection
- **AND** each affected result SHALL include `worktree_state=dirty` and diff summary hash facts.

#### Scenario: Search changed notes since revision
- **GIVEN** the version backend can list changed paths since revision `abc123`
- **WHEN** a user runs `pinax search "索引" --changed-since abc123 --vault ./my-notes --json`
- **THEN** Pinax SHALL restrict candidate parsing to notes changed since that revision before applying query filters
- **AND** stdout SHALL include changed path count, indexed candidate count, backend revision facts, and fallback status.

#### Scenario: Historical search unavailable
- **GIVEN** the version backend cannot read files at revision `abc123`
- **WHEN** a user runs `pinax search "索引" --revision abc123 --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `version_read_unavailable`
- **AND** no index database, Markdown file, record asset, Git state, provider state, or remote service SHALL be modified.

### Requirement: Index maintenance is incremental and bounded
Pinax SHALL optimize index maintenance by skipping unchanged notes, batching writes, bounding worker concurrency, and exposing progress and freshness diagnostics.

#### Scenario: Incremental refresh skips unchanged notes
- **GIVEN** the index has recorded ledger sequence, content hashes, file sizes, modified times, and version evidence for notes
- **WHEN** a user runs `pinax index refresh --vault ./my-notes --json`
- **THEN** Pinax SHALL parse only notes with changed ledger facts, changed version evidence, changed mtime/size, changed content hash, or missing projection rows
- **AND** stdout SHALL include scanned count, changed count, skipped count, indexed count, batch count, and duration facts.

#### Scenario: Rebuild reports checkpoint progress
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL process notes in batches and expose progress facts for total candidates, completed candidates, current batch, ledger sequence, index epoch, and last checkpoint
- **AND** a failed rebuild SHALL leave enough checkpoint or status evidence for a later command to explain whether it must restart or can resume.

#### Scenario: Low-memory index mode bounds concurrency
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --memory-budget low --json`
- **THEN** Pinax SHALL use bounded parse workers, bounded result queues, smaller SQLite batches, and streaming file reads
- **AND** it SHALL NOT retain all Markdown bodies, snippets, binary payloads, or historical revision contents in memory.

#### Scenario: SQLite projection uses batched writer semantics
- **WHEN** Pinax rebuilds or refreshes the local index
- **THEN** parse, hash, token, link, and property extraction MAY run concurrently
- **AND** SQLite projection writes SHALL be committed through bounded batch transactions that preserve one index epoch transition per completed batch or rebuild.

### Requirement: Search latency favors projection reads over full scans
Pinax SHALL serve ordinary search, list, table, and database-view queries from SQLite projections when fresh and SHALL use full Markdown scans only as an explicit fallback.

#### Scenario: Fresh search avoids full vault scan
- **GIVEN** `.pinax/index.sqlite` is fresh for the current ledger sequence and version evidence
- **WHEN** a user runs `pinax search "设计" --vault ./my-notes --json`
- **THEN** Pinax SHALL query the SQLite/FTS projection rather than scanning every Markdown file
- **AND** stdout SHALL include `engine=index`, index epoch, ledger sequence, and freshness facts.

#### Scenario: Fallback scan is bounded and explicit
- **GIVEN** the index is missing or stale
- **WHEN** a user runs `pinax search "设计" --allow-scan --memory-budget low --vault ./my-notes --json`
- **THEN** Pinax SHALL run a bounded local scan with limited workers and streaming reads
- **AND** stdout SHALL include `engine=scan`, index status, scanned count, and warning facts.

### Requirement: Index sync reconciles file facts
Pinax SHALL reconcile current vault file facts with indexed file facts to detect external changes.

#### Scenario: Hash unchanged skips work
- **WHEN** a Markdown file has unchanged path, size, modified time, and content hash compared with the index record
- **THEN** `pinax index sync` SHALL skip parsing and writing that note
- **AND** it SHALL report the skip count in machine output.

#### Scenario: Content changed updates self projection
- **WHEN** a Markdown file path is unchanged but content hash changes
- **THEN** `pinax index sync` SHALL update that note's text, token, tag, link, attachment, property, and dimension projection
- **AND** it SHALL update affected incoming link state when title, aliases, note id, or path-derived fields changed.

### Requirement: Index maintains vault object lookup projections
Pinax SHALL extend the local index projection to support note, asset, unmanaged Markdown, and vault file lookup while preserving Markdown and CLI-authored assets as the source of truth.

#### Scenario: Lookup a note or asset by filename
- **WHEN** a user runs `pinax index lookup yeisme --scope all --vault ./my-notes --json`
- **THEN** stdout SHALL include ranked candidates from registered notes, adoptable Markdown files, assets, and vault files
- **AND** each candidate SHALL include object kind, path, managed status, match fields, score, index status, and version evidence when available.

#### Scenario: Ordinary search still excludes unmanaged files by default
- **WHEN** a vault contains unmanaged `yeisme.md` and a user runs `pinax search yeisme --vault ./my-notes --json`
- **THEN** Pinax SHALL exclude unmanaged Markdown from ordinary note search results
- **AND** stdout SHALL include an action recommending `pinax index lookup yeisme --scope all` or `pinax record adopt yeisme --plan` when unmanaged candidates exist.

#### Scenario: Lookup supports asset filters
- **WHEN** a user runs `pinax index lookup diagram --kind asset --media-type image/png --vault ./my-notes --json`
- **THEN** Pinax SHALL return matching asset candidates without reading binary payloads into stdout
- **AND** facts SHALL include asset count, engine, index status, and media filters.

#### Scenario: Rebuild indexes attachment links
- **GIVEN** registered notes reference local attachments through Markdown links or Obsidian wiki embeds
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL rebuild `assets`, `asset_links`, and `vault_files` projections through GORM
- **AND** the projection SHALL preserve source note path, raw reference, resolved asset path, media type, link style, status, and line number when available.

#### Scenario: Attachment relationship commands use fresh index
- **GIVEN** `.pinax/index.sqlite` is fresh
- **WHEN** a user runs `pinax note attachments "认证方案" --vault ./my-notes --json` or `pinax asset backlinks diagram.png --vault ./my-notes --json`
- **THEN** Pinax SHALL answer from indexed attachment projections
- **AND** it SHALL NOT rescan every Markdown file or hash every asset during the query.

#### Scenario: Attachment query falls back safely when index is stale
- **GIVEN** the index is missing or stale
- **WHEN** a user runs `pinax asset orphans --vault ./my-notes --json`
- **THEN** Pinax MAY use a bounded local scan fallback
- **AND** stdout SHALL include `index_status` and a next action for `pinax index refresh --vault ./my-notes --json`.

### Requirement: Version-aware index refresh uses VersionBackend candidates
Pinax SHALL route changed-since and revision-aware index refresh through VersionBackend capabilities instead of shelling out to Git or parsing Git porcelain in command/application layers.

#### Scenario: Refresh changed paths since revision
- **WHEN** a user runs `pinax index refresh --changed-since abc123 --vault ./my-notes --json`
- **THEN** Pinax SHALL ask the active VersionBackend for changed path candidates
- **AND** it SHALL refresh only supported note, asset, and vault file projections for those candidates.

#### Scenario: Changed-since unsupported fails clearly
- **WHEN** a user runs `pinax index refresh --changed-since abc123 --vault ./my-notes --json`
- **AND** the active VersionBackend does not support changed path queries
- **THEN** Pinax SHALL fail with stable error code `version_changed_paths_unavailable`
- **AND** it SHALL not modify index, Markdown, record, asset, provider, or version state.

### Requirement: Shared resolver drives note, record, asset, and version commands
Pinax SHALL provide a shared resolver for vault object references so command behavior is consistent across lookup, note, record, asset, metadata, and version workflows.

#### Scenario: Resolver returns candidates for readonly commands
- **WHEN** a readonly command resolves `yeisme` and multiple candidates match
- **THEN** Pinax SHALL return ranked candidates with object kind, path, match fields, managed status, and next actions
- **AND** it MAY return partial status if the command can still provide useful readonly results.

#### Scenario: Resolver rejects ambiguous write targets
- **WHEN** a writing command resolves `yeisme` and multiple candidates match
- **THEN** Pinax SHALL fail before writing with stable error code `vault_object_ref_ambiguous`
- **AND** no Markdown file, asset file, index row, record event, version snapshot, or provider state SHALL be modified.

### Requirement: Index commands guide local maintenance decisions
Pinax SHALL make `pinax index` a decision-oriented maintenance surface that explains the current index state, affected workflows, and the safest next command without requiring users to infer state transitions from implementation details.

#### Scenario: Default index command summarizes status
- **WHEN** a user runs `pinax index --vault ./my-notes`
- **THEN** Pinax SHALL render a concise Chinese summary containing the index status, index path, note count when available, freshness evidence, affected workflows, and one recommended next command
- **AND** it SHALL NOT write `.pinax/index.sqlite`, Markdown files, event files, Git state, provider state, or remote services.

#### Scenario: Default index command preserves machine contracts
- **WHEN** a user runs `pinax index --vault ./my-notes --json` or `pinax index --vault ./my-notes --agent`
- **THEN** Pinax SHALL emit the same command projection contract as an index summary command with stable English keys including `index_status`, `path`, `schema_version`, `notes`, `recommended_action`, and `writes=false`
- **AND** localized Chinese labels SHALL NOT appear in `--agent` keys or JSON field names.

#### Scenario: Missing index recommends bounded recovery
- **WHEN** the index database is missing and a user runs `pinax index --vault ./my-notes`
- **THEN** Pinax SHALL recommend `pinax index refresh --vault ./my-notes` for ordinary recovery when the vault size is within the lazy refresh budget
- **AND** it SHALL recommend `pinax index rebuild --vault ./my-notes` when a full rebuild is required.

### Requirement: Index refresh is the default low-cost maintenance action
Pinax SHALL provide `pinax index refresh` as the preferred low-cost maintenance command for reconciling the local index projection when the vault can be repaired incrementally.

#### Scenario: Refresh skips unchanged notes
- **WHEN** a user runs `pinax index refresh --vault ./my-notes --json`
- **THEN** Pinax SHALL scan registered Pinax note facts, skip unchanged notes using ledger sequence, content hash, modified time, size, schema version, and projection row evidence where available
- **AND** stdout SHALL include stable facts for scanned notes, changed notes, skipped notes, indexed notes, deleted rows, failed rows, batch count, duration, and final `index_status`.

#### Scenario: Refresh creates missing index safely
- **WHEN** `.pinax/index.sqlite` is missing and a user runs `pinax index refresh --vault ./my-notes --json`
- **THEN** Pinax SHALL create the index database through the application service and index registered Pinax notes only
- **AND** unmanaged Markdown files without Pinax frontmatter SHALL remain excluded.

#### Scenario: Refresh reports partial failures without hiding them
- **WHEN** one or more notes cannot be parsed or indexed during `pinax index refresh`
- **THEN** Pinax SHALL return `status=partial`
- **AND** stdout SHALL include failed count, redacted evidence, affected paths, and next actions for `pinax index doctor` or `pinax index rebuild`.

### Requirement: Index doctor explains freshness and integrity problems
Pinax SHALL provide `pinax index doctor` to diagnose index availability, schema compatibility, freshness, row consistency, and projection health without mutating vault content.

#### Scenario: Doctor diagnoses stale index
- **WHEN** registered note facts differ from indexed facts and a user runs `pinax index doctor --vault ./my-notes --json`
- **THEN** Pinax SHALL report `status=partial`, issue counts grouped by code and severity, stale evidence, affected paths, and a recommended action
- **AND** it SHALL NOT modify the index database unless the user explicitly chooses a repair or refresh command.

#### Scenario: Doctor diagnoses unreadable index
- **WHEN** `.pinax/index.sqlite` exists but cannot be opened or migrated
- **AND** a user runs `pinax index doctor --vault ./my-notes --json`
- **THEN** Pinax SHALL report stable issue code `index_unreadable`
- **AND** it SHALL include a safe next action for `pinax index repair --kind recreate` or `pinax index rebuild` without printing raw stack traces or secrets.

#### Scenario: Doctor emits explainable human output
- **WHEN** a user runs `pinax index doctor --vault ./my-notes`
- **THEN** Pinax SHALL render Chinese sections for 状态, 问题, 证据, 影响, and 推荐下一步
- **AND** machine keys such as `schema_version` or `index_status` SHALL be localized in the default human output.

### Requirement: Index repair is bounded to projection-safe operations
Pinax SHALL provide index repair operations only for projection-safe maintenance and SHALL avoid changing Markdown note bodies, record ledger assets, Git state, provider state, or remote services.

#### Scenario: Repair previews projection-safe operations
- **WHEN** a user runs `pinax index repair --vault ./my-notes --dry-run --json`
- **THEN** Pinax SHALL return a repair preview with operation kind, mode, risk, target path, reason, and evidence
- **AND** `writes=false` SHALL be present in facts.

#### Scenario: Repair requires explicit approval for writes
- **WHEN** a user runs `pinax index repair --vault ./my-notes --kind recreate --json` without `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** no index database, Markdown file, event file, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Repair recreates corrupt projection only after approval
- **WHEN** `pinax index doctor` reports a corrupt or unreadable projection
- **AND** a user runs `pinax index repair --vault ./my-notes --kind recreate --yes --json`
- **THEN** Pinax SHALL move or remove only the local index projection according to the selected repair policy, rebuild registered Pinax notes, and report final `index_status=fresh` when successful
- **AND** stdout SHALL include evidence for the old projection handling and the rebuilt index path.

### Requirement: Index output remains one projection across modes
Pinax SHALL render index summary, status, refresh, doctor, repair, sync, and rebuild output from a single command projection per command.

#### Scenario: Index commands support structured modes
- **WHEN** a user runs any index maintenance command with `--json`, `--agent`, or `--explain`
- **THEN** Pinax SHALL emit valid mode-specific output from the same projection
- **AND** `--json` stdout SHALL contain JSON only, `--agent` stdout SHALL contain stable key=value lines, and `--explain` SHALL be a Chinese reviewable summary with evidence references.

#### Scenario: Index events stream stays structured
- **WHEN** a user runs a long-running index command with `--events`
- **THEN** Pinax SHALL emit NDJSON start/progress/end or error events with monotonic sequence numbers
- **AND** progress events SHALL include bounded counts without writing ANSI, localized prose, or debug logs to stdout.

### Requirement: Index page templates generate refreshable navigation pages
Pinax SHALL provide built-in index templates that create and refresh local Markdown navigation pages from the current vault index projection.

#### Scenario: Create home index page from template
- **WHEN** a user runs `pinax index page create home --template index.home --vault ./my-notes --json`
- **THEN** Pinax SHALL create `index/home.md` through the application service when it is missing
- **AND** the created note SHALL include Pinax note frontmatter with `kind: index`, `status: system`, and index tags
- **AND** stdout SHALL include page name, template name, note path, managed block count, query count, and index status facts.

#### Scenario: Preview index page without writing
- **WHEN** a user runs `pinax index page preview home --vault ./my-notes --json`
- **THEN** Pinax SHALL render the corresponding index template against bounded query results from the local index projection
- **AND** it SHALL NOT modify notes, `.pinax` structured assets, Git state, provider state, or remote services.

#### Scenario: Refresh index page managed blocks
- **GIVEN** `index/home.md` contains valid Pinax managed blocks
- **WHEN** a user runs `pinax index page refresh home --vault ./my-notes --json`
- **THEN** Pinax SHALL execute the index template's bounded Pinax SQL queries through the query service
- **AND** it SHALL update only matching managed blocks in `index/home.md`
- **AND** it SHALL preserve user-authored Markdown outside managed blocks.

#### Scenario: Refresh refuses missing managed block
- **GIVEN** `index/home.md` does not contain the managed block required by `index.home`
- **WHEN** a user runs `pinax index page refresh home --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `managed_block_missing`
- **AND** it SHALL NOT rewrite the whole file or create a duplicate section
- **AND** stdout SHALL include a next action recommending `pinax index page create home --template index.home --vault ./my-notes --json` for a fresh page or manual block restoration for an existing page.

#### Scenario: Index template query uses Pinax SQL service
- **WHEN** an index template declares a query such as `SELECT title, status, path FROM notes WHERE status = "active" ORDER BY updated_at DESC LIMIT 20`
- **THEN** Pinax SHALL parse and execute the query through the existing Pinax SQL query service and repository boundaries
- **AND** it SHALL NOT concatenate raw SQLite SQL in command handlers, application service business logic, or template functions.

#### Scenario: Create decision index page from template
- **WHEN** a user runs `pinax index page create decisions --template index.decisions --vault ./my-notes --json`
- **THEN** Pinax SHALL create `index/decisions.md` through the application service when it is missing
- **AND** the page SHALL contain managed blocks for proposed decisions, accepted decisions, and decisions needing review.

#### Scenario: Create learning index page from template
- **WHEN** a user runs `pinax index page create learning --template index.learning --vault ./my-notes --json`
- **THEN** Pinax SHALL create `index/learning.md` through the application service when it is missing
- **AND** the page SHALL contain managed blocks for active learning notes, video notes, book notes, and unreviewed highlights where supported by the local index.

#### Scenario: Create meetings index page from template
- **WHEN** a user runs `pinax index page create meetings --template index.meetings --vault ./my-notes --json`
- **THEN** Pinax SHALL create `index/meetings.md` through the application service when it is missing
- **AND** the page SHALL contain managed blocks for recent meetings, open action items, and meetings without linked project where supported by the local index.

#### Scenario: Create research index page from template
- **WHEN** a user runs `pinax index page create research --template index.research --vault ./my-notes --json`
- **THEN** Pinax SHALL create `index/research.md` through the application service when it is missing
- **AND** the page SHALL contain managed blocks for active research topics, evidence notes, and unanswered questions where supported by the local index.

#### Scenario: Refresh all starter index pages is explicit
- **WHEN** a user runs `pinax index page refresh all --vault ./my-notes --json`
- **THEN** Pinax MAY refresh all built-in starter index pages only if the command is implemented as an explicit multi-page workflow
- **AND** it SHALL report each page path, status, changed flag, and error code independently
- **AND** it SHALL NOT silently create or refresh multiple pages from a single-page command such as `pinax index page refresh home --vault ./my-notes --json`.

### Requirement: Managed block patching is conservative
Pinax SHALL treat managed blocks as the only writable region during template refresh and SHALL fail closed when block boundaries are unsafe.

#### Scenario: Duplicate managed block names are ambiguous
- **GIVEN** a note contains two `<!-- pinax:managed name=recent -->` blocks
- **WHEN** Pinax refreshes a template section named `recent`
- **THEN** Pinax SHALL fail with stable error code `managed_block_ambiguous`
- **AND** no note body SHALL be modified.

#### Scenario: Unclosed managed block is invalid
- **GIVEN** a note contains `<!-- pinax:managed name=recent -->` without a matching closing marker
- **WHEN** Pinax refreshes that note
- **THEN** Pinax SHALL fail with stable error code `managed_block_unclosed`
- **AND** no note body SHALL be modified.

### Requirement: Index projection preserves review lifecycle facts

Pinax SHALL project inbox and draft lifecycle metadata into the local SQLite/GORM index while keeping Markdown as the source of truth.

#### Scenario: Rebuild indexes inbox and draft facts
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json` in a vault containing notes with `status: inbox` and `status: draft`
- **THEN** Pinax SHALL rebuild note rows with status, lifecycle status, kind, folder, group/project, note id, title, updated time, and canonical path facts
- **AND** the projection SHALL remain rebuildable from Markdown without requiring `.pinax/index.sqlite` as a source of truth.

#### Scenario: Incremental refresh updates lifecycle changes
- **WHEN** a controlled inbox or draft command changes a note status or path
- **THEN** Pinax SHALL refresh the affected local index projection after the successful write
- **AND** `pinax index status --vault ./my-notes --json` SHALL report fresh or return a stable stale diagnostic with a `pinax index refresh` next action.

#### Scenario: Discarded notes are filterable but not ordinary results
- **WHEN** a user runs `pinax search "草稿" --vault ./my-notes --json`
- **THEN** Pinax SHALL exclude lifecycle status `discarded` by default
- **AND** discarded notes SHALL be returned only when the user selects an explicit `--status discarded`, trash, deleted, or all lifecycle filter.

### Requirement: Search and note list expose review status without hiding user Markdown

Pinax SHALL expose inbox and draft notes in ordinary local queries with stable lifecycle facts, unless the user selects a lifecycle filter that excludes them.

#### Scenario: Ordinary search marks inbox and draft results
- **WHEN** a user runs `pinax search "方案" --vault ./my-notes --json`
- **THEN** matching inbox and draft notes SHALL remain searchable as ordinary Markdown content
- **AND** each result SHALL include stable status and lifecycle status facts so clients can visually group or de-emphasize them.

#### Scenario: Explicit status filter narrows review queue
- **WHEN** a user runs `pinax search "" --status draft --vault ./my-notes --json`
- **THEN** Pinax SHALL return only draft lifecycle notes
- **AND** JSON facts SHALL include `filter.status=draft`, returned count, total count, engine, and index status.

#### Scenario: Note list remains compatible with status filters
- **WHEN** a user runs `pinax note list --status inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL return the same candidate set as `pinax inbox list --vault ./my-notes --json` except for command name and workflow-specific next actions
- **AND** both projections SHALL use canonical vault-relative paths.

### Requirement: Review index pages are managed system pages

Pinax SHALL generate inbox and draft review pages through the existing index page template mechanism and SHALL classify those pages as system index pages.

#### Scenario: Preview inbox index page without writes
- **WHEN** a user runs `pinax index page preview inbox --template index.inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL render a bounded inbox review page using local index or scan-backed query facts
- **AND** stdout facts SHALL include `writes=false`, template name, target path, managed block count, query count, and returned item count
- **AND** no Markdown, `.pinax` asset, Git state, provider state, remote service, or index projection SHALL be modified.

#### Scenario: Create inbox index page as system note
- **WHEN** a user runs `pinax index page create inbox --template index.inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL create `index/inbox.md` or the safe template-defined path as a registered system index page
- **AND** the frontmatter SHALL classify it as `kind: index` and `status: system`
- **AND** ordinary note statistics, orphan detection, and default search SHALL exclude the created index page.

#### Scenario: Refresh draft index page managed block only
- **GIVEN** `index/drafts.md` contains Pinax managed blocks for draft review content
- **WHEN** a user runs `pinax index page refresh drafts --template index.drafts --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the matching managed blocks through the application service
- **AND** it SHALL preserve user-authored Markdown outside those blocks.

#### Scenario: Missing managed block blocks refresh
- **GIVEN** `index/inbox.md` does not contain the required Pinax managed block
- **WHEN** a user runs `pinax index page refresh inbox --template index.inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with a stable managed block error code
- **AND** it SHALL NOT guess an insertion point, rewrite the whole file, or create a duplicate section.

### Requirement: Local index projects bidirectional links consistently
Pinax SHALL keep link projection behavior consistent with the note graph query behavior.

#### Scenario: Index rebuild preserves enhanced wiki link fields
- **WHEN** `pinax index rebuild --vault ./my-notes --json` indexes notes with wiki aliases, headings, broken links, and ambiguous targets
- **THEN** the `links` and `backlinks` query sources SHALL expose the same resolved path, raw target, alias, heading, status, evidence, and line facts as `pinax note links`.

#### Scenario: Incremental refresh reclassifies affected links
- **WHEN** a note title, alias, path, or note id changes
- **THEN** Pinax SHALL reclassify affected outgoing and incoming link projection rows
- **AND** it SHALL turn previously resolved links into `broken` or `ambiguous` when appropriate without rewriting note bodies.

#### Scenario: Fresh index engine is truthful
- **WHEN** the local index is fresh and link graph commands report `facts.engine=index`
- **THEN** their output SHALL come from projection data compatible with the shared link graph rules
- **AND** scan fallback SHALL only be used when the index is missing, stale, or unavailable.

### Requirement: Search engine selection is explicit and internal by default
Pinax SHALL support explicit search engine selection without requiring external search binaries.

#### Scenario: Native search does not require ripgrep
- **WHEN** a user runs `pinax search "design" --engine native --vault ./my-notes --json`
- **THEN** Pinax SHALL search registered Markdown notes using its built-in native engine
- **AND** stdout facts SHALL include `engine_requested=native` and `engine=native`
- **AND** Pinax SHALL NOT require `rg`, `fzf`, or `bat` to be installed.

#### Scenario: Index search uses SQLite token candidates
- **WHEN** a user runs `pinax search "design" --engine index --vault ./my-notes --json`
- **AND** the SQLite index is fresh
- **THEN** Pinax SHALL use the indexed `search_token_records` projection to select candidate notes
- **AND** it SHALL load note text only for candidate result projection and snippets
- **AND** it SHALL NOT perform a full Markdown body scan or require external search binaries.

#### Scenario: Index-only search fails without fallback writes
- **WHEN** a user runs `pinax search "design" --engine index --vault ./my-notes --json`
- **AND** the index is missing or stale without `--allow-stale`
- **THEN** Pinax SHALL fail or return partial output with a stable index maintenance action
- **AND** it SHALL NOT perform a native fallback silently.

### Requirement: Search lazy-index policy is bounded
Pinax SHALL make search-time index loading explicit and bounded.

#### Scenario: Lazy index off never writes the index
- **WHEN** a user runs `pinax search "design" --lazy-index off --vault ./my-notes --json`
- **AND** the index is missing or stale
- **THEN** Pinax SHALL return native search results or an index-only error according to `--engine`
- **AND** it SHALL NOT create or modify `.pinax/index.sqlite`.

#### Scenario: Auto lazy index defers over-budget refresh
- **WHEN** search detects more changed notes than the lazy refresh budget
- **THEN** Pinax SHALL defer index maintenance, return bounded search output, and include an action for `pinax index refresh --vault <vault> --json`
- **AND** stdout facts SHALL include `lazy_index.deferred=true`.

