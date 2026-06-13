## ADDED Requirements

### Requirement: Pinax exposes hidden API schema discovery alias

Pinax SHALL preserve `pinax api schema export` as the primary API schema path while accepting a hidden root `pinax schema export` compatibility path for users who naturally search for schema from the root command tree.

#### Scenario: Root schema alias exports API schema

- **WHEN** a user runs `pinax schema export --format openapi --vault ./my-notes --json`
- **THEN** stdout SHALL contain the same JSON envelope command and facts as `pinax api schema export --format openapi --vault ./my-notes --json`
- **AND** the command SHALL NOT write vault files, `.pinax` metadata, Git state, provider state, or remote systems.

#### Scenario: Root schema alias stays hidden from primary help

- **WHEN** a user runs `pinax --help`
- **THEN** root help SHALL NOT list `schema` as a primary command
- **AND** `pinax schema --help` SHALL show a runnable `pinax schema export` example.
