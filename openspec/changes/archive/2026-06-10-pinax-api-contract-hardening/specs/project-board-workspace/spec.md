## ADDED Requirements

### Requirement: Pinax API OpenAPI schema is derived from the route registry

Pinax SHALL derive local API OpenAPI REST paths, methods, operation ids, and Pinax extension metadata from the same remote route registry returned by `pinax api routes`.

#### Scenario: OpenAPI method matches registered REST method
- **GIVEN** a REST route is registered with `surface=rest`, `method=POST`, and path `/v1/project-items/{ref}:{action}`
- **WHEN** a user runs `pinax api schema export --format openapi --vault ./my-notes --json`
- **THEN** the exported OpenAPI schema SHALL include `/v1/project-items/{ref}:{action}` with a `post` operation
- **AND** it SHALL NOT export that route as a `get` operation.

#### Scenario: Every registered REST route appears in OpenAPI
- **GIVEN** REST routes are registered in the remote route registry
- **WHEN** OpenAPI schema is exported
- **THEN** each registered REST route with a path SHALL appear exactly once under `data.schema.paths`
- **AND** the operation method SHALL equal the lowercase registered route method.

#### Scenario: OpenAPI operation includes Pinax route metadata
- **GIVEN** a REST route is exported to OpenAPI
- **WHEN** a client inspects the operation object
- **THEN** the operation SHALL include `operationId`, `x-pinax-command`, `x-pinax-capability`, `x-pinax-readonly`, `x-pinax-body-allowed`, `x-pinax-approval-required`, and `x-pinax-snapshot-required`
- **AND** those values SHALL match the registered route and capability metadata.

### Requirement: Pinax local REST routes preserve projection error envelopes

Pinax SHALL map local REST transport status codes without replacing or dropping the command projection error envelope.

#### Scenario: Unknown REST route returns projection 404
- **WHEN** a local REST client requests an unknown API path
- **THEN** Pinax SHALL return HTTP 404
- **AND** the response body SHALL be a failed projection envelope with `error.code=route_not_found`.

#### Scenario: Unsupported REST method returns projection 405
- **WHEN** a local REST client uses an unsupported method for a registered path
- **THEN** Pinax SHALL return HTTP 405
- **AND** the response body SHALL be a failed projection envelope with `error.code=method_not_allowed`.

#### Scenario: REST route registry matches handler behavior
- **GIVEN** routes are listed by `pinax api routes --json`
- **WHEN** contract tests exercise representative HTTP requests for each registered REST route
- **THEN** each route SHALL reach the handler for its projection command
- **AND** the response body SHALL be valid JSON using the Pinax projection envelope.

### Requirement: Pinax local RPC methods preserve registry contracts

Pinax SHALL keep registered RPC methods aligned with the RPC dispatcher and equivalent REST capability metadata.

#### Scenario: Every registered RPC method is dispatchable
- **GIVEN** RPC routes are registered in the remote route registry
- **WHEN** contract tests call each registered RPC method with representative fixture params
- **THEN** the dispatcher SHALL return a Pinax projection envelope
- **AND** it SHALL NOT return `rpc_method_not_found` for any registered method.

#### Scenario: Unknown RPC method returns stable error
- **WHEN** a local RPC client calls an unknown method
- **THEN** Pinax SHALL return a failed projection envelope with `error.code=rpc_method_not_found`
- **AND** the error hint SHALL direct the user to inspect `pinax api routes`.

#### Scenario: REST and RPC capability metadata stays aligned
- **GIVEN** a capability is exposed by both REST and RPC surfaces
- **WHEN** capabilities are listed
- **THEN** the REST and RPC routes SHALL point to the same projection command and response schema version
- **AND** any difference in readonly, body exposure, approval, or snapshot requirements SHALL be explicit in route or capability metadata.

### Requirement: Pinax remote write gates use explicit transport status without side effects

Pinax SHALL keep remote write-like REST and RPC operations behind dry-run, approval, and snapshot gates, and SHALL use explicit transport status while preserving failed projection details.

#### Scenario: Approval gate has failed projection and no writes
- **GIVEN** a project item exists in a fixture vault
- **WHEN** a REST client calls `POST /v1/project-items/{ref}:archive` without approval fields
- **THEN** Pinax SHALL return a non-2xx transport status with `error.code=approval_required`
- **AND** no Markdown file, `.pinax` asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Snapshot gate has failed projection and no writes
- **GIVEN** a project item exists in a fixture vault and approval is supplied
- **WHEN** a REST or RPC client requests an archive or risky move without required snapshot evidence
- **THEN** Pinax SHALL return a non-2xx transport status with `error.code=snapshot_required`
- **AND** the projection SHALL include a runnable `pinax version snapshot` next action or hint
- **AND** no Markdown file, `.pinax` asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Remote gate output is redacted
- **WHEN** REST or RPC gate responses include evidence, hint, actions, or diagnostics
- **THEN** stdout, stderr, response body, test fixtures, and integration evidence SHALL NOT include provider tokens, Authorization headers, cookies, webhook URLs, raw provider payloads, or hidden prompt content.

### Requirement: Pinax API serve lifecycle output is mode-safe

Pinax SHALL treat `pinax api serve --readonly` as a long-running local server command with explicit stdout and stderr behavior for each output mode.

#### Scenario: Serve requires readonly mode
- **WHEN** a user runs `pinax api serve --vault ./my-notes` without `--readonly`
- **THEN** Pinax SHALL return a failed projection with `error.code=readonly_required`
- **AND** it SHALL NOT start a server.

#### Scenario: Serve binds localhost and reports URL on stderr by default
- **WHEN** a user runs `pinax api serve --vault ./my-notes --readonly --port 0`
- **THEN** Pinax SHALL bind to `127.0.0.1`
- **AND** default human mode SHALL report the local URL on stderr
- **AND** stdout SHALL NOT contain logs, banners, or non-structured progress.

#### Scenario: Serve events mode emits lifecycle events
- **WHEN** a user runs `pinax api serve --vault ./my-notes --readonly --port 0 --events`
- **THEN** stdout SHALL be NDJSON events containing `start` and `ready`
- **AND** shutdown or startup failure SHALL emit `shutdown` or `error`
- **AND** diagnostics and logs SHALL remain on stderr.

#### Scenario: Serve machine modes do not mix logs into stdout
- **WHEN** a user runs `pinax api serve` with `--json` or `--agent`
- **THEN** Pinax SHALL either emit one startup projection and keep stdout otherwise quiet, or return a stable `unsupported_output_mode` failed projection
- **AND** it SHALL NOT mix local URL logs, diagnostics, or human prose into machine stdout.
