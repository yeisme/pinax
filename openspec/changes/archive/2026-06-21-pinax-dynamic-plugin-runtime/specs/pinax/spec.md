# pinax 增量规格

## MODIFIED Requirements

### Requirement: Pinax prioritizes local notebook core before external extensions
Pinax SHALL provide a complete local-first notebook core before relying on external provider, cloud sync, AI automation, or dynamic plugin capabilities.

#### Scenario: Notebook core commands require no plugins
- **WHEN** a user runs daily, inbox, organization view, link, backlink, attachment, saved view, import, export, query, Dataview-compatible query, or publish preview commands against a valid local vault
- **THEN** Pinax SHALL NOT require dynamic plugins, JavaScript, Python, WASM, firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, cookies, or external network access.

#### Scenario: Plugin hooks cannot replace core semantics
- **WHEN** a plugin contributes a hook for query, template, export, publish, diagnostic, or action planning
- **THEN** Pinax SHALL preserve built-in command behavior and use plugin output only through documented capability extension points
- **AND** plugin failure SHALL NOT corrupt or replace built-in notebook core behavior.

