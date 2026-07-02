## ADDED Requirements

### Requirement: Pinax SHALL model durable source notes for external references

Pinax SHALL support durable source notes as long-lived Markdown records for external references such as GitHub repositories, public datasets, documentation sites, tools, and protocol resources.

#### Scenario: Create a GitHub source note from a built-in template
- **WHEN** the user runs `pinax note add "iptv-org/iptv" --template source.github --var url=https://github.com/iptv-org/iptv --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note through the note application service
- **AND** the note SHOULD use a safe vault-relative path under `sources/github/` unless the user provides an explicit destination
- **AND** the note SHALL include durable-source sections for source facts, canonical URLs, use decision, risk and boundary, verification, related notes, and next actions
- **AND** JSON output SHALL preserve the existing Pinax envelope shape and include facts for template name, effective path, kind, status, and tags.

#### Scenario: Explicit note fields override source template defaults
- **WHEN** the user runs `pinax note add "iptv-org/iptv" --template source.github --var url=https://github.com/iptv-org/iptv --dir custom --kind reference --status draft --tags custom/tag --vault ./my-notes --json`
- **THEN** Pinax SHALL prefer the explicit destination, kind, status, and tags over template defaults
- **AND** it SHALL still report that the selected template was `source.github`.

### Requirement: Durable source metadata SHALL be additive and optional

Pinax SHALL allow durable source notes to carry optional metadata such as `source_url`, `last_checked_at`, `source_license`, and `review_after` without requiring existing notes to migrate or breaking ordinary note indexing.

#### Scenario: Existing reference notes remain valid
- **GIVEN** a vault contains an existing note with `kind: reference`, ordinary tags, and a GitHub URL in the body
- **WHEN** the user runs `pinax index refresh --vault ./my-notes`
- **THEN** Pinax SHALL continue indexing and searching the note without requiring durable source metadata
- **AND** it SHALL NOT rewrite the note only because the optional durable-source fields are missing.

#### Scenario: Source metadata remains searchable through existing dimensions
- **GIVEN** a durable source note has `kind: source`, `status: active`, and tags such as `source/github`, `media/iptv`, and `license/cc0`
- **WHEN** the user runs `pinax note list --kind source --tag source/github --vault ./my-notes --json`
- **THEN** Pinax SHALL return the note using existing note list/search projections
- **AND** it SHALL NOT require a separate source database as the content authority.

### Requirement: Metadata and organize plans SHALL suggest durable source improvements safely

Pinax SHALL detect likely external source notes and suggest durable source metadata, path, tag, and review improvements through plan-first workflows.

#### Scenario: Metadata plan suggests source fields without writing
- **GIVEN** a note contains `https://github.com/iptv-org/iptv` but lacks durable source metadata
- **WHEN** the user runs `pinax metadata plan "iptv-org/iptv" --vault ./my-notes --json`
- **THEN** Pinax SHALL suggest optional fields such as `kind: source`, `source_url`, `last_checked_at`, `source_license`, `review_after`, and structured tags
- **AND** it SHALL NOT write Markdown, `.pinax` events, index rows, Git state, providers, or remote services.

#### Scenario: Organize plan suggests source path and manual review items
- **GIVEN** a GitHub source candidate is stored under a generic research path
- **WHEN** the user runs `pinax organize plan --vault ./my-notes --json`
- **THEN** Pinax MAY suggest moving the note to `sources/github/<slug>.md`
- **AND** it SHALL return manual review items when the note lacks use decision, risk and boundary, verification, or related note links
- **AND** it SHALL NOT automatically rewrite the body, split concept notes, delete notes, or create new related notes.

### Requirement: Durable source notes SHALL integrate with graph maintenance

Pinax SHALL use existing note links, backlinks, orphan detection, search, and dataview/query projections to help users maintain durable source notes over time.

#### Scenario: Related notes are checked through existing graph commands
- **GIVEN** a durable source note links to an internal concept note using Markdown or wiki links
- **WHEN** the user runs `pinax note links sources/github/iptv-org-iptv.md --vault ./my-notes --json`
- **THEN** Pinax SHALL report the outgoing note relationship using the existing link projection
- **AND** `pinax note backlinks <concept-note> --vault ./my-notes --json` SHALL be able to show the source note as a backlink.

#### Scenario: Missing related links become review suggestions
- **GIVEN** a durable source candidate has no outgoing or incoming note links
- **WHEN** the user runs `pinax organize plan --vault ./my-notes --json`
- **THEN** Pinax MAY suggest adding related note links as a manual review item
- **AND** it SHALL NOT create speculative concept notes without explicit user action.

### Requirement: Agent review SHALL remain a thin workflow over Pinax commands

Pinax SHALL remain the source of truth for durable source note templates, metadata, organization suggestions, and safe writes. Any future agent skill for long-term note review SHALL guide review and call Pinax commands rather than defining an independent storage contract.

#### Scenario: Agent prepares a long-term note review
- **WHEN** an agent reviews a temporary external URL note for durable storage
- **THEN** it SHOULD inspect the note, explain source-note quality gaps, and propose Pinax commands such as `note tag set`, `note move`, `metadata plan`, `organize plan`, `index refresh`, `note links`, and `note backlinks`
- **AND** it SHALL NOT hand-write `.pinax` structured assets or bypass the Pinax application service for machine-readable metadata.
