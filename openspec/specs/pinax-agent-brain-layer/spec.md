# pinax-agent-brain-layer Specification

## Purpose
TBD - created by archiving change pinax-agent-brain-layer. Update Purpose after archive.
## Requirements
### Requirement: Pinax SHALL expose an agent brain context bundle from bounded projections

Pinax SHALL provide an Agent Brain context model that combines local memory, semantic KB context, search results, graph evidence, query/database rows, project state, and proof receipts without exposing full private note bodies by default.

#### Scenario: Agent asks for meeting preparation context

- **WHEN** an agent requests context for `prepare for Alice meeting`
- **THEN** Pinax SHALL compose bounded references from `pinax memory context`, `pinax kb context`, `pinax search`, backlinks/link graph, and relevant receipts when available
- **AND** the context bundle schema SHALL be `pinax.agent_brain.context_bundle.v1`
- **AND** the context SHALL include `task`, `entities`, `memory_refs`, `semantic_refs`, `graph_refs`, `query_refs`, `receipts`, `freshness`, `body_exposure`, and `next_actions`
- **AND** the context SHALL include source references, freshness, confidence or ranking reasons, open tasks, and next actions
- **AND** it SHALL NOT include full note bodies, raw prompts, hidden system prompts, provider payloads, Authorization headers, cookies, tokens, private tool arguments, or complete chain-of-thought.

#### Scenario: Context bundle exposes real next commands

- **WHEN** an index is stale, KB projection is missing, provider credentials are missing, or proof review is required
- **THEN** the context bundle SHALL include real commands such as `pinax index refresh --vault ./my-notes --json`, `pinax kb provider doctor openai --vault ./my-notes --json`, or `pinax proof loop run --vault ./my-notes --json`
- **AND** it SHALL NOT expose local execution wrappers, shell aliases, raw tool arguments, or agent-only prefixes.

### Requirement: Answer synthesis SHALL be citation-first and body-safe

Pinax SHALL treat answer synthesis as a bounded projection over evidence, not as unconstrained chat over raw private documents.

#### Scenario: Answer includes claims and citations

- **WHEN** a future answer command or MCP tool returns a synthesized answer
- **THEN** the projection SHALL include answer text, claims, evidence references, freshness, confidence, open questions, cost/provider metadata, and next actions
- **AND** each claim SHALL cite note paths, memory ids, graph edges, query rows, receipt ids, or provider-safe citations
- **AND** unsupported claims SHALL appear as open questions or missing evidence rather than confident conclusions.

#### Scenario: Synthesis preserves body exposure

- **WHEN** the answer is generated from notes or KB chunks
- **THEN** the output SHALL quote only bounded snippets unless the user explicitly requests local body exposure through an approved command
- **AND** MCP, Local API, Web, and `--agent` output SHALL NOT default to full body exposure.

### Requirement: Agent Brain SHALL keep provider and cost state visible

Pinax SHALL expose provider/model/source type and cost class for embedding, rerank, and synthesis workflows without revealing credentials or raw provider payloads.

#### Scenario: Missing provider credential returns doctor action

- **WHEN** answer synthesis, KB rebuild, semantic context, or rerank needs a provider that is not configured
- **THEN** Pinax SHALL return a stable diagnostic with credential source type and a concrete next action such as `pinax kb provider doctor openai --vault ./my-notes --json`
- **AND** stdout, stderr, events, MCP payloads, fixtures, screenshots, and evidence SHALL NOT include raw credential values.

#### Scenario: Local provider is distinguished from metered provider

- **WHEN** a provider is local-only, such as Ollama
- **THEN** Pinax SHALL mark it as local-only or local-service-backed
- **AND** when a provider may incur network or usage cost, Pinax SHALL expose a bounded cost class such as `low`, `metered`, or `unknown` rather than silently calling it.

### Requirement: MCP and Local API brain surfaces SHALL default to readonly and scoped projections

Pinax SHALL expose Agent Brain surfaces through local MCP and Local REST/RPC as registered capabilities that are readonly by default and backed by the same bounded projections as the CLI.

#### Scenario: MCP brain tools are readonly

- **WHEN** an MCP client lists or calls Agent Brain tools such as brain context, brain answer, sources, or maintenance plan
- **THEN** those tools SHALL return bounded projections by default
- **AND** they SHALL NOT write Markdown, `.pinax/**`, SQLite/GORM, LanceDB, provider state, sync state, Git state, or remote services.


### Requirement: Ingest sources SHALL enter the vault through service-owned receipts

Pinax SHALL import or capture Markdown notes, meeting records, text exports, and document bundles through service-owned commands and receipts before they become searchable brain context.

#### Scenario: Import is planned before write

- **WHEN** a user imports a directory of notes or documents
- **THEN** Pinax SHALL support a dry-run or preview plan before writing
- **AND** confirmed writes SHALL create normalized Markdown or bounded assets through the application service and write redacted receipt evidence
- **AND** source payloads, provider stderr, webhook tokens, email headers with secrets, or raw external API payloads SHALL NOT be emitted to machine output.

### Requirement: Dream cycle maintenance SHALL be plan-first and reviewable

Pinax SHALL support Agent Brain maintenance as a reviewable planning workflow, not as an invisible background rewrite.

#### Scenario: Maintenance proposes reviewable operations

- **WHEN** Pinax detects duplicate entities, broken citations, stale facts, superseded memories, contradictions, or compression candidates
- **THEN** it SHALL emit a maintenance plan with operation kind, risk, evidence, affected sources, and next action
- **AND** it SHALL NOT modify note bodies, memory records, graph projections, KB projections, or structured assets unless the user explicitly applies an approved plan through a service-owned command.

#### Scenario: Apply requires proof loop protections

- **WHEN** a maintenance operation would write Markdown, `.pinax/**`, memory ledger, graph projection, KB projection, provider state, sync state, or receipts
- **THEN** Pinax SHALL require approval, snapshot or equivalent restore evidence, receipt, and restore hint
- **AND** high-risk rewrites, deletions, entity merges, and contradiction resolutions SHALL remain manual review items until a dedicated apply contract exists.

### Requirement: Brain projections SHALL declare sync and rebuild authority

Pinax SHALL classify each Agent Brain data product as source of truth, receipt evidence, or rebuildable local projection.

#### Scenario: Semantic and graph projections are rebuildable

- **WHEN** Cloud Sync or export considers `.pinax/kb/`, graph projection files, answer caches, or derived indexes
- **THEN** Pinax SHALL treat them as local rebuildable projections unless a later encrypted sync contract explicitly says otherwise
- **AND** Cloud Sync SHALL NOT upload plaintext vectors, raw note bodies, raw provider payloads, or provider credentials.

#### Scenario: Brain projection authority is explicit

- **WHEN** Agent Brain context, answer, maintenance, export, or Cloud Sync logic classifies data products
- **THEN** Markdown notes and user assets SHALL be treated as local source-of-truth content
- **AND** import/proof/sync/maintenance receipts SHALL be treated as service-owned evidence with redaction boundaries
- **AND** SQLite/GORM indexes, KB/LanceDB vectors, graph projections, and answer caches SHALL be treated as rebuildable local projections
- **AND** each next action SHALL use real commands such as `pinax index refresh --vault ./my-notes --json`, `pinax kb refresh --vault ./my-notes`, or `pinax graph rebuild --vault ./my-notes --json`.

#### Scenario: Planned answer cache is not synced as private model state

- **WHEN** a future implementation persists an answer cache or synthesis trace
- **THEN** the cache SHALL be local and rebuildable by default
- **AND** it SHALL NOT sync raw prompts, hidden system prompts, raw provider payloads, private tool arguments, full note bodies, Authorization headers, cookies, tokens, or complete chain-of-thought.

#### Scenario: Memory and receipts preserve evidence boundaries

- **WHEN** memory records or maintenance receipts are used as Agent Brain evidence
- **THEN** they SHALL include source citations and lifecycle state
- **AND** they SHALL avoid raw prompts, provider payloads, full note bodies, Authorization headers, cookies, tokens, hidden system prompts, private tool arguments, and complete chain-of-thought.

### Requirement: Future clients SHALL consume Agent Brain contracts without owning business rules

Future local clients SHALL consume Pinax Agent Brain projections rather than duplicating vault parsing, graph construction, memory ranking, provider calls, or proof-loop business rules.

#### Scenario: Client uses discovery before brain features

- **WHEN** a future client needs Agent Brain capabilities
- **THEN** it SHALL discover supported commands/routes/tools through `pinax api routes --vault ./my-notes --json`, OpenAPI export, MCP tools list, or documented CLI command projections
- **AND** it SHALL NOT read `.pinax/**`, SQLite/GORM databases, LanceDB files, provider config, token files, sync state, or receipts directly.


