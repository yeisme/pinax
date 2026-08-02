## ADDED Requirements

### Requirement: Workbench module SHALL consume bounded KB review projections

Pinax SHALL expose versioned, bounded Local API and capability projections for KB overview, source inventory, evaluation suites, question summaries, and run details so `client/yeisme-workbench` can render a review page without reading private files or invoking shell commands.

#### Scenario: Workbench loads KB review overview
- **WHEN** the Pinax Workbench module requests the KB review overview
- **THEN** Pinax SHALL return independent source, Ollama daemon/model, LanceDB sidecar, active generation, index freshness, provider/model/dimension, corpus count, evaluation count, last-run, and next-action facts
- **AND** it SHALL NOT return absolute vault paths, note bodies, vectors, provider config values, credentials, or raw provider payloads.

#### Scenario: Workbench loads one question detail lazily
- **WHEN** a review suite contains up to 50 representative questions and the user selects one question
- **THEN** the client SHALL load suite/question summaries and only the selected question's bounded detail and citations
- **AND** Pinax SHALL NOT return every question's full citation list or every chunk in the initial response.

#### Scenario: Browser does not execute CLI or read LanceDB
- **WHEN** the page needs a rebuild, evaluation, migration, or recovery action
- **THEN** Pinax SHALL return a copyable real `pinax ...` command and the exact disabled reason for P0
- **AND** browser code SHALL NOT spawn a shell, directly mutate `.pinax/**`, or read LanceDB.

### Requirement: KB review projection SHALL represent readiness and failure states truthfully

Pinax SHALL expose separate stable states for source acquisition, Ollama process/model/embed readiness, LanceDB sidecar readiness, generation freshness, retrieval result, evaluation result, and answer-generation availability.

#### Scenario: Port reachability is not reported as retrieval success
- **WHEN** Ollama answers `/api/version` but the exact model is missing, real embedding fails, the sidecar is missing, or no active generation exists
- **THEN** the overview SHALL report the strongest proven layer and the failed downstream layer separately
- **AND** the page SHALL NOT display the KB as ready or searchable.

#### Scenario: Unsupported source remains unsupported
- **WHEN** source inventory contains PDF, webpage, Git repository, Feishu document, or OCR content without an installed Pinax acquire adapter
- **THEN** the source SHALL report `unsupported` or `not_configured` with a real next action
- **AND** it SHALL NOT report ready, indexed, uploaded, or synchronized.

#### Scenario: No-hit and partial results remain explicit
- **WHEN** a retrieval returns zero hits or only part of the requested stages succeeds
- **THEN** run detail SHALL report `no_hits` or `partial`, preserve available bounded citations, and list the failed or missing stage
- **AND** it SHALL NOT synthesize an empty citation or success answer.

#### Scenario: Generation is unavailable
- **WHEN** the MVP has only an embedding model and no answer-generation model
- **THEN** the review projection SHALL report `answer_mode=not_generated`
- **AND** the page SHALL display that only retrieval and citation are being evaluated.

#### Scenario: Identity rerank is represented as passthrough
- **WHEN** Inferrum retrieval uses the current identity rerank stage
- **THEN** the projection SHALL label rerank as `off`, `identity`, or `passthrough`
- **AND** it SHALL NOT label the result as reranked by a model.

### Requirement: KB review page SHALL provide a compact accessible evaluation workflow

The Workbench Pinax module SHALL render KB review inside the existing workspace chrome with one primary scroll container, a compact status header, `问题评测` and `知识源` tabs, one question list, and one active detail flow.

#### Scenario: Desktop layout shows one active detail owner
- **WHEN** the viewport is at least 1280 CSS pixels wide
- **THEN** the page SHALL show a 300–340 pixel question summary column and a flexible active-question detail column
- **AND** it SHALL NOT nest another application sidebar, inspector workbench, free docking surface, or page-inside-page shell.

#### Scenario: Narrow viewport reflows without hidden actions
- **WHEN** the viewport is below 768 CSS pixels
- **THEN** the page SHALL use a single-column flow, card-form citations, horizontally scrollable top-level tabs when needed, and action targets of at least 44 CSS pixels
- **AND** it SHALL preserve visible disabled actions with their exact reasons
- **AND** it SHALL not create horizontal page overflow.

#### Scenario: Keyboard and assistive technology can review a run
- **WHEN** a keyboard or screen-reader user opens the page
- **THEN** tabs SHALL implement the tablist keyboard model, question selection SHALL be operable without a pointer, citation tables SHALL use semantic table markup, status changes SHALL be announced politely, and errors SHALL use an alert role
- **AND** status SHALL not depend on color alone
- **AND** focus SHALL remain stable after status refresh.

### Requirement: P0 KB review surfaces SHALL remain read-only

Pinax SHALL expose P0 KB review routes as bounded GET projections and SHALL keep rebuild, evaluation execution, model pull, source import, and review-decision persistence behind the existing CLI/application service until a separate mutation contract is approved.

#### Scenario: Review page offers real next commands
- **WHEN** the user views an unavailable or stale state
- **THEN** the page SHALL show the corresponding copyable `pinax kb doctor`, `pinax kb rebuild`, `pinax kb evaluate`, or source-import command produced by the server projection
- **AND** the command SHALL use real supported flags and selected vault-safe references.

#### Scenario: Non-GET review request is rejected
- **WHEN** a client sends a mutation request to a P0 KB review route
- **THEN** Pinax SHALL return method-not-allowed or capability-unavailable with a stable error code
- **AND** it SHALL NOT start Ollama work, rebuild a projection, write a review decision, or modify canonical content.

#### Scenario: Cached snapshot is labeled offline
- **WHEN** Workbench displays the last successful bounded snapshot while the Pinax Local API is unreachable
- **THEN** the page SHALL label it as an offline cached snapshot with its timestamp
- **AND** it SHALL NOT display current online/readiness status or enable actions.
