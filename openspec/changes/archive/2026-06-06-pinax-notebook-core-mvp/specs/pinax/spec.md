## ADDED Requirements

### Requirement: Pinax prioritizes local notebook core before external extensions
Pinax SHALL provide a complete local-first notebook core before relying on external provider, cloud sync, or AI automation capabilities.

#### Scenario: Notebook core commands require no external credentials
- **WHEN** a user runs daily, inbox, organization view, link, backlink, attachment, saved view, import, or export commands against a valid local vault
- **THEN** Pinax SHALL NOT require firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, cookies, or external network access.

#### Scenario: Notebook core writes stay inside CLI-owned boundaries
- **WHEN** a notebook core command writes notes, attachments, saved views, import receipts, export receipts, or index projections
- **THEN** the write SHALL happen through Cobra command dispatch into `internal/app` services
- **AND** the command layer SHALL NOT hand-write `.pinax` JSON/YAML/JSONL assets.

#### Scenario: Notebook core keeps Markdown portable
- **WHEN** a user opens the vault in a normal Markdown editor
- **THEN** created notes, daily notes, inbox notes, wiki links, Markdown links, and attachment references SHALL remain readable without Pinax running.
