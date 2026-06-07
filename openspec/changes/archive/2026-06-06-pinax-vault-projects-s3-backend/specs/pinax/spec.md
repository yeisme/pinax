# pinax Delta Specification

## ADDED Requirements

### Requirement: Pinax manages multiple projects inside one vault

Pinax SHALL allow a local vault to contain multiple named projects through CLI-authored structured metadata.

#### Scenario: creating a project
- **GIVEN** a Pinax vault exists
- **WHEN** the user runs `pinax project create research --name "研究" --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update `.pinax/projects.json` through the application service
- **AND** stdout SHALL contain one JSON envelope with `command=project.create`, `status=success`, project facts, and a runnable next action

#### Scenario: listing projects
- **GIVEN** a vault has project metadata
- **WHEN** the user runs `pinax project list --vault ./my-notes --json`
- **THEN** stdout SHALL contain project records and the current project without reading note bodies

#### Scenario: switching current project
- **GIVEN** a vault has a project with slug `research`
- **WHEN** the user runs `pinax project switch research --vault ./my-notes`
- **THEN** Pinax SHALL update only the current project pointer in `.pinax/projects.json`
- **AND** it SHALL append redacted event evidence

### Requirement: Pinax stores backend configuration for local and S3 storage

Pinax SHALL configure storage backend metadata through CLI commands without requiring real network access or persisted provider secrets.

#### Scenario: configuring S3 backend
- **GIVEN** a Pinax vault exists
- **WHEN** the user runs `pinax storage set-s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault ./my-notes --json`
- **THEN** Pinax SHALL write `.pinax/storage.json` through the application service
- **AND** stdout SHALL contain a JSON envelope with backend facts and no credentials

#### Scenario: diagnosing S3 backend configuration
- **GIVEN** a vault has S3 backend metadata
- **WHEN** the user runs `pinax storage doctor --vault ./my-notes --json`
- **THEN** Pinax SHALL validate required fields without connecting to S3
- **AND** it SHALL report expected credential source without printing secret values
