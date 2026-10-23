# rendered-view Specification

## Purpose
TBD - created by archiving change pinax-rendered-view-v1. Update Purpose after archive.
## Requirements
### Requirement: The renderer SHALL produce a versioned, bounded rendered projection

Pinax SHALL assemble `render.view.v1alpha1` for a target note as a renderer-agnostic tree with stable anchors, plus bounded panels and proof facts. The projection SHALL respect the existing display depth boundary and SHALL NOT include full note bodies unless display depth `body` is explicitly requested.

#### Scenario: Rendered tree with stable anchors
- **WHEN** a caller renders a note containing headings, a fence and wikilinks with display depth `context`
- **THEN** Pinax SHALL return a tree whose headings, fences and links each carry a stable anchor id
- **AND** the projection SHALL include `ref`, `display`, `badge` and `truncated` facts.

#### Scenario: Display depth boundary holds
- **WHEN** a note is rendered with default display depth
- **THEN** the projection SHALL NOT contain the complete note body
- **AND** requesting body depth SHALL be an explicit parameter that is recorded in output metadata.

#### Scenario: Red line on unsafe values
- **WHEN** any node, panel, badge or warning would carry an absolute path or an unsafe ref substring
- **THEN** Pinax SHALL drop or redact the value and count it in `dropped_unsafe`
- **AND** the projection SHALL never expose filesystem paths or credential-like strings.

### Requirement: The renderer SHALL resolve wikilinks and relations into bounded references

Wikilinks SHALL be resolved through the existing note-links projection. Resolved links SHALL become card references with object id, title, status and version; broken links SHALL become explicit broken markers. Backlinks SHALL be attached as a bounded panel.

#### Scenario: Resolved wikilink becomes a card reference
- **WHEN** a rendered note contains a wikilink that resolves to an existing note
- **THEN** the tree node SHALL carry the target object id, version, title and status
- **AND** it SHALL NOT carry the target's filesystem path.

#### Scenario: Broken wikilink is explicit
- **WHEN** a wikilink does not resolve
- **THEN** the node SHALL be marked `broken`
- **AND** the projection SHALL NOT attempt path inference or silently omit the link.

#### Scenario: Backlinks panel is bounded
- **WHEN** the backlinks panel is assembled
- **THEN** it SHALL contain at most 50 source entities with a `truncated` flag
- **AND** unsafe source refs SHALL be skipped and counted.

### Requirement: The fence registry SHALL render supported fences and explicit failure placeholders

The renderer SHALL support the fences `pinax-database-view`, `pinax-graph`, `pinax-board` and `pinax-timeline`, each backed by an existing read-only projection. A fence whose projection fails SHALL render as an explicit failure placeholder. Unknown fences SHALL pass through unchanged.

#### Scenario: Four fences render from existing projections
- **WHEN** a note contains the four supported fences with valid arguments
- **THEN** each fence node SHALL carry a bounded projection payload with `truncated` facts
- **AND** graph depth SHALL be capped at two hops with node and edge truncation.

#### Scenario: Fence failure is explicit
- **WHEN** a fence argument names a missing saved view, project or produces a projection error
- **THEN** the fence node SHALL render as a failure placeholder with a stable error code and a next action
- **AND** the overall render SHALL NOT silently omit the fence.

#### Scenario: Unknown fence passes through
- **WHEN** a note contains a fence outside the registry
- **THEN** the renderer SHALL keep the fence content verbatim as a plain code node
- **AND** no projection or inference SHALL be attempted.

### Requirement: Asset and external references SHALL be bounded and non-proxying

Vault attachments SHALL resolve to bounded asset references. External schemes SHALL be recognized as external references without fetching, proxying or caching bytes. Unknown schemes SHALL remain literal text.

#### Scenario: Vault attachment resolves to an asset reference
- **WHEN** a rendered note references a vault attachment
- **THEN** the node SHALL carry a `pinax-asset://` reference with mime classification
- **AND** it SHALL NOT carry a filesystem path.

#### Scenario: DriveBridge reference is a status card only
- **WHEN** a note contains a `drivebridge://file/` reference
- **THEN** the renderer SHALL emit an external reference node carrying the reference string verbatim
- **AND** rendering SHALL perform no network or file IO for that reference.

#### Scenario: Unknown scheme stays literal
- **WHEN** a link uses a scheme outside the recognized set
- **THEN** the node SHALL remain literal text without resolution attempts.

### Requirement: Pinax SHALL emit and resolve stable note references

Pinax SHALL emit `pinax://note/<object_id>?v=<revision>` references in dual format and SHALL resolve them with explicit version semantics. Floating references SHALL return a caller-retained `resolved_revision`; subsequent calls SHALL use that supplied baseline to flag drift instead of silently switching versions. Resolution SHALL NOT persist reference state or create snapshots.

#### Scenario: Dual-format reference emission
- **WHEN** a user runs the note reference command with copy
- **THEN** the clipboard SHALL receive a Markdown link and a wikilink for the same object id and revision
- **AND** ambiguous titles SHALL trigger a bounded disambiguation list instead of a wrong match.

#### Scenario: Versioned resolution returns the exact revision
- **WHEN** a reference with an explicit `v` is resolved
- **THEN** Pinax SHALL return the object at that exact revision or an explicit `unresolved` result
- **AND** it SHALL NOT substitute a different revision.

#### Scenario: Floating reference drift is explicit
- **WHEN** a caller supplies the original `resolved_revision` while resolving a floating reference after the target has moved to a newer revision
- **THEN** the result SHALL report the originally resolved revision with `stale=true` and the current revision as a separate fact
- **AND** it SHALL NOT silently re-resolve to the newer revision.

### Requirement: Every rendered projection SHALL carry proof facts

The render footer SHALL carry a proof badge with object id, version, content revision and rendered-at time so any rendered output remains verifiable against the vault.

#### Scenario: Proof badge is present
- **WHEN** any render view is produced at any display depth
- **THEN** the badge SHALL include object id, version, revision and rendered-at timestamp
- **AND** badge values SHALL match the vault record for that revision.

### Requirement: Transports SHALL share one rendered application service

CLI and MCP rendered outputs SHALL be produced by the same application service with equivalent semantics on the same fixture.

#### Scenario: CLI and MCP equivalence
- **WHEN** the same note fixture is rendered through CLI machine output and the MCP render tool
- **THEN** both SHALL return semantically equivalent tree data under transport-specific wrappers
- **AND** the MCP tool SHALL default to non-body display depth.

