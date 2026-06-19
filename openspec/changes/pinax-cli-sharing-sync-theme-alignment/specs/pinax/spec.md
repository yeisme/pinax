## ADDED Requirements

### Requirement: Pinax remains a CLI-first local vault tool
Pinax SHALL treat the local Markdown vault as the source of truth and SHALL expose cloud, publish and provider integrations as controlled CLI surfaces around that vault.

#### Scenario: Sharing surfaces do not become note sources of truth
- **WHEN** Pinax publishes to GitHub Pages, GitHub Wiki, GitHub Gist, an HTTP endpoint, or a local preview server
- **THEN** the generated artifact SHALL be a delivery surface derived from selected vault content
- **AND** Pinax SHALL NOT treat the delivery surface as authoritative note storage or bypass the vault proof loop for later writes.

#### Scenario: Cloud server is a sync transport boundary
- **WHEN** Pinax syncs through a cloud server transport
- **THEN** the CLI SHALL own local vault selection, local file writes, approval gates, conflict handling, receipts and redacted projections
- **AND** the server SHALL be treated as an optional transport/coordinator for encrypted sync artifacts, not as a plaintext hosted notebook or local tool executor.

#### Scenario: Production server implementation stays out of CLI scope by default
- **GIVEN** a change is scoped to the Pinax CLI repository
- **WHEN** it references Cloud Server support
- **THEN** it SHALL limit implementation to client protocol, transport adapter, fake/local server tests, redaction, and sync-state behavior unless a separate server-owned change explicitly expands scope
- **AND** it SHALL NOT add a long-running hosted note backend inside ordinary CLI app services.

### Requirement: Theme design follows the CLI and publish boundaries
Pinax SHALL keep CLI chrome and publish-site theme design consistent with local-first, work-focused usage.

#### Scenario: CLI theme is concise and operational
- **WHEN** Pinax renders default human output
- **THEN** the output SHALL be concise, Chinese, scannable, and oriented around status, facts, evidence and next action
- **AND** it SHALL avoid marketing copy, decorative noise, or localized text inside machine modes.

#### Scenario: Publish site theme is inspectable and local
- **WHEN** Pinax builds a publish site with the built-in theme
- **THEN** the theme SHALL use local CSS/JS assets and publish-safe data files
- **AND** it SHALL NOT require external fonts, CDN assets, analytics, remote images, the source vault, `.pinax/**`, SQLite, provider credentials, or network access by default.
