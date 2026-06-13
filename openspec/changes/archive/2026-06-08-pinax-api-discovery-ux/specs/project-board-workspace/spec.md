## ADDED Requirements

### Requirement: Pinax API routes human output is scannable

Pinax SHALL render `pinax api routes` default human output with enough route detail for users to inspect local REST/RPC capabilities without switching to JSON.

#### Scenario: API routes summary includes endpoint evidence

- **WHEN** a user runs `pinax api routes --vault ./my-notes`
- **THEN** stdout SHALL include a Chinese summary and route count facts
- **AND** stdout SHALL include readable route evidence containing REST method/path or RPC method name plus the projection command.

#### Scenario: API routes machine output remains complete

- **WHEN** a user runs `pinax api routes --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `command=api.routes`
- **AND** `data.routes` and `data.capabilities` SHALL remain the machine-readable route registry for scripts and agents.
