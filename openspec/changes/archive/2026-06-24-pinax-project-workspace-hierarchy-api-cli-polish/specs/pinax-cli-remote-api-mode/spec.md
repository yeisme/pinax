## ADDED Requirements

### Requirement: Project workspace REST routes SHALL be projection adapters

Pinax SHALL expose project workspace routes through the local REST/RPC projection adapter without creating a separate remote project management model.

#### Scenario: Read project workspace routes
- **WHEN** local API clients call `GET /v1/projects`, `GET /v1/projects/{project}`, `GET /v1/projects/{project}/subprojects`, `GET /v1/projects/{project}/subprojects/{subproject}`, or `GET /v1/project-items/{item_id}`
- **THEN** Pinax SHALL route through the application service
- **AND** responses SHALL be Pinax projection envelopes
- **AND** handlers SHALL NOT directly parse Markdown, read `.pinax` structured assets, query GORM repositories, call Git, or invoke providers.

#### Scenario: Project board route supports optional subproject query
- **WHEN** a local API client calls `GET /v1/projects/research/board?subproject=stock-learning&note_display=card`
- **THEN** the response SHALL match the bounded projection returned by `pinax project board show research --subproject stock-learning --note-display card --vault <vault> --json`
- **AND** omitting `subproject` SHALL preserve existing project-wide board behavior.

### Requirement: Project workspace write routes SHALL be gated

Pinax SHALL expose only controlled write-plan or confirmed write routes for project workspace operations.

#### Scenario: Readonly API rejects workspace writes
- **GIVEN** `pinax api serve --vault yeisme-notes` is running in default readonly mode
- **WHEN** a client calls `POST /v1/project-items?project=research&subproject=stock-learning&yes=true`
- **THEN** Pinax SHALL return a failed projection with `error.code=write_disabled`
- **AND** no Markdown file, `.pinax` asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Allow-write API still requires confirmation
- **GIVEN** `pinax api serve --vault yeisme-notes --allow-write` is running
- **WHEN** a client calls a project workspace write route without `yes=true`
- **THEN** Pinax SHALL return `approval_required`
- **AND** it SHALL include a runnable CLI next action when useful.

#### Scenario: High-risk workspace write requires snapshot
- **GIVEN** a project workspace write would archive, delete, batch-change, or rewrite managed Markdown
- **WHEN** the request includes `yes=true` but no recent snapshot evidence exists
- **THEN** Pinax SHALL return `snapshot_required`
- **AND** the response body SHALL remain a failed Pinax projection envelope.

### Requirement: Project workspace capabilities SHALL be discoverable

Pinax SHALL publish project workspace routes and RPC methods from the shared route registry.

#### Scenario: API route list includes workspace capabilities
- **WHEN** the user runs `pinax api routes --vault yeisme-notes --json`
- **THEN** project workspace routes SHALL include route id, surface, method, path or RPC method, command, capability id, schema version, readonly, body_allowed, approval_required, snapshot_required, and stable errors.

#### Scenario: OpenAPI export includes workspace routes
- **WHEN** the user runs `pinax api schema export --format openapi --vault yeisme-notes --json`
- **THEN** project workspace REST routes SHALL appear in `data.schema.paths`
- **AND** operations SHALL include `x-pinax-command`, `x-pinax-capability`, `x-pinax-readonly`, `x-pinax-body-allowed`, `x-pinax-approval-required`, and `x-pinax-snapshot-required`.

