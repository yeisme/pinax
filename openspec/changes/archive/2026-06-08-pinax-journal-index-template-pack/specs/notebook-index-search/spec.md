## MODIFIED Requirements

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

## ADDED Requirements

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
