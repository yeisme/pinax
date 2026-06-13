## ADDED Requirements

### Requirement: REST and RPC expose inbox and draft readonly projections

Pinax SHALL expose inbox and draft readonly operations through the local REST/RPC projection adapter without creating a separate remote business model.

#### Scenario: REST lists inbox items
- **WHEN** a local REST client calls `GET /v1/inbox?limit=20`
- **THEN** Pinax SHALL return the same projection envelope schema as `pinax inbox list --vault ./my-notes --json`
- **AND** the REST handler SHALL NOT directly parse Markdown, read `.pinax` structured assets, call GORM repositories, or mutate vault state.

#### Scenario: REST shows draft item
- **WHEN** a local REST client calls `GET /v1/drafts/note_123?view=rendered`
- **THEN** Pinax SHALL return a bounded note display projection for the draft
- **AND** the response SHALL include stable facts for note id, path, title, status, lifecycle status, display mode, and body exposure.

#### Scenario: RPC readonly methods match CLI projections
- **WHEN** an RPC client calls `Pinax.Inbox.List`, `Pinax.Inbox.Show`, `Pinax.Draft.List`, or `Pinax.Draft.Show`
- **THEN** the response SHALL use the same projection envelope schema as REST and CLI JSON output
- **AND** RPC SHALL NOT invent a separate response shape for notes, errors, actions, or evidence.

### Requirement: Remote inbox and draft writes stay gated

Pinax SHALL expose inbox and draft write-like operations only as explicit gated projection adapter routes.

#### Scenario: Readonly server rejects inbox capture
- **GIVEN** a user starts the local API server without `--allow-write`
- **WHEN** a REST client calls `POST /v1/inbox:capture` with a title and body
- **THEN** Pinax SHALL return a failed projection with stable error code `write_disabled`
- **AND** no Markdown, `.pinax` asset, index projection, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Write server still requires approval for lifecycle mutation
- **GIVEN** a user starts the local API server with `--allow-write`
- **WHEN** a REST client calls `POST /v1/drafts/note_123:discard` without `yes=true`
- **THEN** Pinax SHALL return a failed projection with stable error code `approval_required`
- **AND** no Markdown, `.pinax` event, record metadata, index projection, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Dry-run remote promote returns plan only
- **GIVEN** a user starts the local API server with `--allow-write`
- **WHEN** a REST client calls `POST /v1/inbox/note_123:promote?to=active&dry_run=true`
- **THEN** Pinax SHALL return a transition plan projection with `writes=false`
- **AND** it SHALL NOT write Markdown, `.pinax` assets, index projection, Git state, provider state, or remote services.

#### Scenario: Approved remote promote calls application service
- **GIVEN** a user starts the local API server with `--allow-write`
- **WHEN** a REST client calls `POST /v1/drafts/note_123:promote?status=active&yes=true`
- **THEN** Pinax SHALL call the same application service used by `pinax draft promote`
- **AND** it SHALL return a projection envelope with old status, new status, path, writes, record event, and index update facts.

### Requirement: Route registry advertises inbox and draft capabilities

Pinax SHALL publish inbox and draft REST/RPC capabilities from the shared route registry so remote clients can discover allowed operations without scraping localized help text.

#### Scenario: API routes includes inbox and draft routes
- **WHEN** a user runs `pinax api routes --vault ./my-notes --json`
- **THEN** the route registry projection SHALL include REST and RPC entries for inbox and draft list, show, capture/create, promote, archive, and discard operations
- **AND** each route SHALL include route id, surface, method, path or RPC method, projection command, capability id, schema version, readonly flag, body allowed flag, approval requirement, snapshot requirement, and stable error codes.

#### Scenario: OpenAPI schema exports review routes
- **WHEN** a user runs `pinax api schema export --format openapi --vault ./my-notes --json`
- **THEN** the exported schema SHALL include inbox and draft REST paths with methods derived from the shared route registry
- **AND** each operation SHALL include `x-pinax-command`, `x-pinax-capability`, `x-pinax-readonly`, `x-pinax-body-allowed`, and `x-pinax-approval-required` metadata.

#### Scenario: Remote errors preserve projection envelopes
- **WHEN** a REST or RPC inbox/draft route receives invalid parameters, unknown note refs, invalid lifecycle targets, disabled writes, or missing approval
- **THEN** Pinax SHALL return a failed projection envelope with a stable error code
- **AND** transport status mapping SHALL NOT replace, localize, or drop the projection error details.
