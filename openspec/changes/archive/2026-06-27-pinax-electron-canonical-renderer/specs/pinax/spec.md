## MODIFIED Requirements

### Requirement: Pinax prioritizes local notebook core before external extensions

Pinax SHALL provide a complete local-first notebook core before relying on external provider, cloud sync, AI automation, or dynamic plugin capabilities. Publish preview and generated HTML SHALL use the `pinax-web` canonical renderer shared with Electron preview semantics.

#### Scenario: Notebook core commands require no plugins

- **WHEN** a user runs daily, inbox, organization view, link, backlink, attachment, saved view, import, export, query, Dataview-compatible query, or publish preview commands against a valid local vault
- **THEN** Pinax SHALL NOT require dynamic plugins, JavaScript, Python, WASM, firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, cookies, or external network access.

#### Scenario: Canonical renderer stays inside Pinax boundaries

- **WHEN** Pinax renders note preview or generated publish HTML
- **THEN** rendering SHALL use the `pinax-web` canonical renderer contract and bounded projections
- **AND** the renderer SHALL NOT become a separate source of truth for note identity, persistence, provider config, sync state or publish receipts.
