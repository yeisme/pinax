## ADDED Requirements

### Requirement: Pinax maintains a local bidirectional note graph
Pinax SHALL derive a bidirectional note graph from local Markdown notes while keeping Markdown files as the source of truth and SQLite/GORM as a rebuildable projection.

#### Scenario: Build graph from supported Markdown links
- **WHEN** a vault contains notes with `[[Title]]`, `[[Title|Alias]]`, `[[Title#Heading]]`, `[label](relative-note.md)`, and `[label](relative-note.md#heading)`
- **THEN** Pinax SHALL parse those references as note graph edges
- **AND** each edge SHALL preserve source path, raw target, normalized target text, alias when present, heading when present, link kind, and line number when available.

#### Scenario: Ignore non-note links in note graph
- **WHEN** a note contains external URLs, `mailto:` links, pure `#heading` anchors, or non-Markdown attachment references
- **THEN** Pinax SHALL NOT count those references as note graph edges
- **AND** it MAY expose them as ignored link evidence or attachment projection without marking them as broken note links.

### Requirement: Link targets resolve predictably
Pinax SHALL resolve note link targets using a deterministic order and SHALL mark ambiguous targets instead of guessing.

#### Scenario: Resolve target by stable identifiers
- **WHEN** a link target matches a note id, vault-relative path, exact title, or unique case-insensitive title
- **THEN** Pinax SHALL resolve the edge to the matching note path and note id
- **AND** the edge status SHALL be `resolved`.

#### Scenario: Ambiguous title is not guessed
- **WHEN** multiple notes can satisfy the same title or alias target
- **THEN** Pinax SHALL mark the edge status as `ambiguous`
- **AND** JSON output SHALL include candidate note paths or note ids without choosing one automatically.

#### Scenario: Missing target is broken
- **WHEN** no local note can satisfy a note link target
- **THEN** Pinax SHALL mark the edge status as `broken`
- **AND** CLI output SHALL include a stable next action recommending review or repair planning.

### Requirement: Bidirectional graph is available through readonly surfaces
Pinax SHALL expose outgoing links, backlinks, broken links, ambiguous links, and local graph context through readonly CLI and MCP surfaces.

#### Scenario: Agent reads graph context without writing vault
- **WHEN** an agent calls a readonly Pinax CLI command or MCP tool for note link context
- **THEN** Pinax SHALL route through the application service and return graph facts
- **AND** it SHALL NOT modify Markdown files, `.pinax/` state, Git state, provider state, or remote services.

#### Scenario: Graph context is bounded for low-token use
- **WHEN** an agent requests graph context for a note
- **THEN** Pinax SHALL return the target note projection plus bounded outgoing links, backlinks, broken link counts, ambiguous counts, and runnable next actions
- **AND** it SHALL NOT return every note body in the vault by default.

#### Scenario: Graph query uses maintained projection when fresh
- **WHEN** the local index projection is fresh after full rebuild or incremental updates
- **AND** a user or agent requests note links, backlinks, broken links, ambiguous links, or graph context
- **THEN** Pinax SHALL answer from the maintained projection instead of scanning every Markdown note
- **AND** stdout facts SHALL identify the graph engine as `index` or an equivalent stable value.

### Requirement: Link repair remains reviewable
Pinax SHALL make link repair and link rewrite suggestions reviewable plans rather than automatic Markdown body mutations.

#### Scenario: Broken link creates manual review operation
- **WHEN** `pinax repair plan` or `pinax organize suggest` detects a broken or ambiguous note link
- **THEN** the generated operation SHALL use manual review mode for link resolution or link rewrite
- **AND** it SHALL include source path, target text, candidate targets when available, reason, and evidence.

#### Scenario: Dry-run does not write link changes
- **WHEN** a user runs a link repair or organize command with `--dry-run`
- **THEN** Pinax SHALL report the planned link operations
- **AND** it SHALL NOT modify note bodies, index files, event files, Git state, provider state, or remote services.
