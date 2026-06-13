## ADDED Requirements

### Requirement: Folder lifecycle events update local projections
Pinax SHALL update local folder, note, asset, and link projections from structured folder lifecycle events emitted by Pinax commands or APIs.

#### Scenario: Folder create updates folder projection
- **WHEN** a user runs `pinax folder create projects/research --purpose notes --vault ./my-notes --json`
- **THEN** Pinax SHALL emit or process a `folder_created` index event with folder path, purpose, managed status, and evidence source `pinax_command`
- **AND** `pinax folder list --include-empty --vault ./my-notes --json` SHALL include the new folder without requiring manual index file edits.

#### Scenario: Folder rename updates affected note projections
- **WHEN** a folder rename moves registered notes from `inbox/` to `archive/`
- **THEN** Pinax SHALL preserve note identity, update note path and folder properties, and mark affected relative link or attachment projections stale when necessary
- **AND** the command projection SHALL include index update or stale-index facts.

#### Scenario: Folder delete removes empty folder projection
- **WHEN** a CLI-authored empty folder is deleted through `pinax folder delete inbox --empty-only --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL remove or tombstone the folder projection and registry entry
- **AND** it SHALL NOT remove note, asset, or link projections unrelated to that folder.
