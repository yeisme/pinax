# core-workflow-stabilization Specification

## Purpose
TBD - created by archiving change pinax-stabilize-core-workflows. Update Purpose after archive.
## Requirements
### Requirement: Core workflow baseline stays green

Pinax SHALL treat the local notebook core workflow as the required green baseline before adding or promoting additional provider, cloud, briefing, dashboard, MCP write, or project-board features.

#### Scenario: Full test and OpenSpec gate passes
- **WHEN** the stabilization change is marked complete
- **THEN** `go test ./...` SHALL pass
- **AND** `task check` SHALL pass when Taskfile dependencies are installed
- **AND** `openspec validate --all` SHALL pass
- **AND** verification evidence SHALL be recorded in `openspec/changes/pinax-stabilize-core-workflows/tasks.md`.

#### Scenario: Current failing baseline is tracked
- **WHEN** a regression from the 2026-06-08 `go test ./...` failure list is fixed
- **THEN** the corresponding task SHALL name the failing package or e2e script
- **AND** it SHALL record the focused command that proves the fix
- **AND** it SHALL not be marked complete solely by changing expectations without confirming the intended user-facing contract.

### Requirement: Note paths are consistent across user-facing surfaces

Pinax SHALL use one stable user-facing note path format across CLI output, JSON facts, agent facts, resolver candidates, record ledger, index rows, search results, MCP payloads, and documentation.

#### Scenario: Resolver accepts compatibility inputs
- **WHEN** a user references the same note as note id, unique title, stem, `foo.md`, or `notes/foo.md`
- **THEN** Pinax SHALL resolve to the same note when the reference is unambiguous
- **AND** output SHALL use the canonical user-facing path
- **AND** ambiguous references SHALL fail clearly rather than choosing an arbitrary candidate.

#### Scenario: Record history uses canonical note path
- **WHEN** a user creates a note and then runs `pinax record history <note-ref> --vault <vault> --json`
- **THEN** record history SHALL find the record using either canonical path or accepted compatibility path
- **AND** the output facts SHALL expose the canonical user-facing path.

### Requirement: Index freshness is updated by controlled writes

Pinax SHALL keep the local SQLite/GORM projection fresh after controlled CLI/service writes, or explicitly mark and explain stale state with safe next actions.

#### Scenario: Query after controlled note write
- **WHEN** a user creates or updates a Pinax note through CLI/service
- **AND** immediately runs `pinax query run ... --vault <vault> --json`
- **THEN** the query SHALL not fail with `property_index_stale` for the just-written note
- **AND** search, note list, and MCP query surfaces SHALL observe the same freshness state.

#### Scenario: Index status after journal or template workflow
- **WHEN** journal, template, note refresh, import, metadata apply, repair apply, organize apply, or index page commands write controlled vault content
- **THEN** `pinax index status --vault <vault> --json` SHALL either report fresh projection
- **OR** report stale/partial with evidence and an action that uses `pinax index refresh` or `pinax index doctor`, not manual `.pinax` editing.

### Requirement: Link graph semantics are engine-independent

Pinax SHALL report equivalent link graph semantics whether links are read from scan fallback or fresh index projection.

#### Scenario: Fresh index and scan fallback agree
- **WHEN** a vault contains resolved links, broken links, ambiguous title links, ignored external links, wiki aliases, headings, and markdown relative links
- **THEN** `pinax note links <note> --json` SHALL report the same resolved, broken, ambiguous, external, and ignored semantics across scan fallback and fresh index engine
- **AND** e2e tests SHALL assert the intended count contract.

### Requirement: OpenSpec owns stabilization execution state

Pinax SHALL track stabilization execution state in the owning OpenSpec change instead of ad hoc docs checklists.

#### Scenario: Active changes are reconciled
- **WHEN** stabilization reaches the planning closeout stage
- **THEN** active Pinax changes SHALL be classified as core dependency, feature continuation, completed, or obsolete
- **AND** completed or obsolete changes SHALL be archived or documented with the reason they remain active
- **AND** README/docs command status SHALL reference current OpenSpec state rather than outdated implementation claims.

