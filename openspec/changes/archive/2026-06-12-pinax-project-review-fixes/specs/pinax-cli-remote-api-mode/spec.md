## ADDED Requirements

### Requirement: RPC note reads are bounded by default

`Pinax.Note.Read` SHALL use the shared bounded note-display projection by default so remote note reads do not expose full note bodies accidentally.

#### Scenario: RPC Note.Read defaults to bounded display

- **GIVEN** `pinax api serve` is running for a vault containing note `note_123`
- **WHEN** an RPC client calls `Pinax.Note.Read` without an explicit body-capable display/request
- **THEN** the response SHALL be a `note.show` Projection using bounded `NoteDisplay` facts such as title, path, note id, status, tags, updated time, excerpt, and link/context counts when available
- **AND** the response SHALL NOT include full note body content.

#### Scenario: RPC Note.Read body exposure is explicit

- **GIVEN** `pinax api serve` is running for a local vault containing note `note_123`
- **WHEN** an RPC client calls `Pinax.Note.Read` with an explicit body-capable display/request
- **THEN** the response MAY include the note body in the same Projection shape used by local CLI JSON output
- **AND** the handler SHALL still apply the normal local API auth, write/read gates, redaction, and output-contract rules.

### Requirement: REST error statuses map stable Projection errors

REST adapters SHALL map known Projection error classes to appropriate HTTP statuses while preserving the failed Projection envelope as the response body.

#### Scenario: REST validation error returns 400 with Projection body

- **GIVEN** a REST request has invalid parameters or an invalid vault object reference
- **WHEN** Pinax rejects the request before command execution
- **THEN** the HTTP status SHALL be `400 Bad Request`
- **AND** the response body SHALL be a failed Projection with stable `error.code`, English `error.message`, optional `error.hint`, and next actions when useful.

#### Scenario: REST authorization and write gates use non-500 statuses

- **GIVEN** a REST request fails because of missing auth, insufficient scope, disabled writes, or missing approval
- **WHEN** Pinax returns the failed Projection
- **THEN** missing/invalid auth SHALL map to `401 Unauthorized`
- **AND** insufficient scope and write-disabled failures SHALL map to `403 Forbidden`
- **AND** approval-required failures SHALL map to `400 Bad Request`
- **AND** the response body SHALL remain the failed Projection envelope.

#### Scenario: REST not found, conflict, and unavailable errors are distinguishable

- **GIVEN** a REST request fails after routing to an application service
- **WHEN** the Projection error is a missing target, revision/concurrent-write conflict, unsupported route/capability, backend unavailable, or unexpected internal failure
- **THEN** missing targets SHALL map to `404 Not Found`
- **AND** revision or concurrent-write conflicts SHALL map to `409 Conflict`
- **AND** unsupported route/capability SHALL map to `404 Not Found` or `405 Method Not Allowed` according to the registered route/method
- **AND** backend unavailable SHALL map to `503 Service Unavailable`
- **AND** unexpected internal failures SHALL map to `500 Internal Server Error`
- **AND** clients SHALL still use the Projection `error.code` as the stable machine contract.