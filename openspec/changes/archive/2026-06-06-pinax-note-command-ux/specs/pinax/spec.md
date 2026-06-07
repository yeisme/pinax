## ADDED Requirements

### Requirement: Pinax note command is ergonomic and backwards compatible
Pinax SHALL expose an ergonomic note command surface while preserving existing `note new`, `note list`, and `note show` behavior.

#### Scenario: Note help shows daily workflow commands
- **WHEN** a user runs `pinax note --help`
- **THEN** help output SHALL include create/new, list, show/read, open/edit, rename, move, archive, delete, and tag commands
- **AND** help text SHALL describe local Markdown note management.

#### Scenario: Existing note commands remain valid
- **WHEN** a user runs existing commands `pinax note new`, `pinax note list`, or `pinax note show`
- **THEN** Pinax SHALL preserve backwards-compatible behavior and output contract unless the user selects new flags.

#### Scenario: Note commands require no provider credentials
- **WHEN** a user runs note creation, listing, reading, editing, tagging, archiving, or deletion commands against a valid local vault
- **THEN** Pinax SHALL NOT require firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, or external network access.
