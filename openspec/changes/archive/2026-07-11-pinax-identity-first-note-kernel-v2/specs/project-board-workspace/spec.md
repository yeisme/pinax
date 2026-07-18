## ADDED Requirements

### Requirement: Project decomposition uses stable object identities
Projects, subprojects, managed work items and source notes SHALL carry canonical object IDs. Project membership, parent-child relationships, board columns and classification SHALL be mutable relations and SHALL NOT define object identity.

#### Scenario: Move a note between projects
- **WHEN** a note is reassigned from one project or subproject to another
- **THEN** Pinax SHALL retain the note object ID, update project relations and board projections, and append reviewable ledger evidence.

#### Scenario: Project workspace path changes
- **WHEN** a project or subproject workspace directory is renamed through Pinax
- **THEN** the project object ID and child ownership relations SHALL remain stable and sync SHALL represent the operation as an object move.

### Requirement: Inferred tasks bind to stable source identity before adoption
Task inference SHALL identify its source by object ID plus a stable source anchor, and task adoption SHALL not rely solely on note path and line number.

#### Scenario: Source note moves before task adoption
- **WHEN** an inferred checklist task is previewed, its source note moves, and the user later applies adoption
- **THEN** Pinax SHALL resolve the current source by object ID, detect stale source anchors and either adopt the intended task or require a refreshed plan without creating a duplicate task identity.
