## MODIFIED Requirements

### Requirement: Electron client consumes Pinax bounded projection only

Pinax SHALL expose Electron/workbench-facing state through Local REST/RPC, CLI JSON, MCP/dashboard shared projections, or copyable real `pinax ...` commands, and SHALL NOT require the client to read `.pinax/**`, SQLite, LanceDB, token files, provider config, sync state, or other structured assets directly.

#### Scenario: Electron client discovers workbench capabilities

- **WHEN** a client runs `pinax api routes --vault ./my-notes --json`
- **THEN** the projection SHALL identify registered capabilities relevant to workbench screens
- **AND** each capability SHALL expose enough bounded metadata for a client to understand readonly/write mode, body exposure default, required approval, required snapshot, and local-only status when applicable.

#### Scenario: Client source ownership stays separate

- **WHEN** the Electron client implementation begins
- **THEN** the source SHALL live in an independent client subproject
- **AND** `cli/pinax` SHALL remain responsible for stable commands, Local REST/RPC, projections, permission gates, publish planning, redaction contracts and receipts.

### Requirement: Electron shell is a thin orchestration surface

Pinax SHALL treat Electron as the desktop runtime for UI consistency, while keeping product policy and write authority in the Go sidecar.

#### Scenario: Electron does not own domain logic

- **WHEN** the client displays notes, boards, graph data, search results, publish plans, provider status or proof plans
- **THEN** those facts SHALL come from Pinax service projections
- **AND** Electron SHALL NOT implement a parallel persistence model for notes, boards, graph, search, canvas, provider state, publish state or proof receipts.

#### Scenario: Electron security boundary is explicit

- **WHEN** the client exposes local capabilities to the renderer process
- **THEN** the preload bridge SHALL expose only typed Pinax API calls and copy-command helpers
- **AND** arbitrary shell execution, raw filesystem access, token access and direct structured asset writes SHALL remain unavailable to the renderer.

### Requirement: Canonical renderer is shared by Electron preview and static publish

Pinax SHALL use one canonical renderer contract for interactive preview and static HTML publishing.

#### Scenario: Renderer packages share one AST semantics

- **WHEN** the Electron preview and static publisher render the same note fixture
- **THEN** they SHALL share the same normalized Markdown/AST semantics for GFM, frontmatter, headings, wikilinks, attachments, managed blocks, dataview/database-view placeholders, code highlighting and redaction markers
- **AND** renderer fixture tests SHALL fail when preview and static output diverge semantically.

#### Scenario: Renderer consumes projection data, not vault internals

- **WHEN** the renderer needs links, backlinks, graph edges, dataview/database results, source facts, search snippets or managed block output
- **THEN** it SHALL consume bounded projection data from the Go sidecar
- **AND** it SHALL NOT read `.pinax/**`, SQLite, LanceDB, token files, provider config, raw sync state or arbitrary vault paths directly.

#### Scenario: Renderer avoids executable Markdown by default

- **WHEN** a note contains Markdown content
- **THEN** the canonical renderer SHALL treat it as Markdown plus controlled Pinax extensions
- **AND** it SHALL NOT execute MDX components, arbitrary imports, inline scripts, environment reads, network fetches or raw provider payloads from note content.
