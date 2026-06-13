## ADDED Requirements

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
