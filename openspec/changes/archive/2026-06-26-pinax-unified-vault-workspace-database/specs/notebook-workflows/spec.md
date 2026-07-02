# notebook-workflows Delta Spec

## ADDED Requirements

### Requirement: Pinax SHALL provide an Obsidian compatibility pack

Pinax SHALL support common Obsidian-style Markdown vault structures as local source material while keeping Pinax-owned metadata, repairs, views, and receipts behind CLI/application service boundaries.

#### Scenario: Obsidian-style vault can be inspected safely

- **GIVEN** a vault contains Markdown notes, wikilinks, aliases, headings, properties/frontmatter, daily notes, attachments, templates, `.obsidian/**`, and plugin metadata
- **WHEN** the user runs `pinax vault doctor --vault ./my-notes --json`
- **THEN** Pinax SHALL inspect supported note, link, property, attachment, template, and index facts
- **AND** it SHALL treat `.obsidian/**` and unknown plugin metadata as non-Pinax-owned inputs unless a future explicit importer is selected
- **AND** it SHALL NOT rewrite Obsidian config or plugin metadata.

#### Scenario: Link repair is plan-first

- **WHEN** Pinax finds broken, ambiguous, or conflicting wikilinks in an Obsidian-style vault
- **THEN** `pinax repair plan --vault ./my-notes --json` SHALL report candidates, risks, and proposed edits without modifying note bodies
- **AND** applying a repair SHALL require explicit approval and snapshot protection according to the normal proof loop.

#### Scenario: Properties remain user-editable Markdown

- **WHEN** a user edits note frontmatter properties in Obsidian or a text editor
- **THEN** Pinax SHALL read and index those properties as source facts
- **AND** Pinax SHALL NOT overwrite unknown properties unless a user-approved metadata or repair plan explicitly owns the change.

### Requirement: Obsidian-style graph and backlink facts SHALL be bounded

Pinax SHALL expose graph, backlinks, orphan notes, unresolved references, aliases, headings, and block references as bounded facts suitable for agents and dashboards.

#### Scenario: Backlink projection includes ambiguity facts

- **WHEN** a user runs `pinax note backlinks <target> --vault ./my-notes --json`
- **THEN** stdout SHALL include backlink count, resolved count, broken count, ambiguous count, candidate paths or note ids when applicable, alias facts when applicable, and index status
- **AND** it SHALL NOT automatically choose between ambiguous candidates.

#### Scenario: Graph projection is body-safe

- **WHEN** a user runs `pinax graph show --vault ./my-notes --json` or an equivalent graph command
- **THEN** stdout SHALL include bounded node and edge facts, graph scope, filters, warnings, and next actions
- **AND** it SHALL NOT include full note bodies, provider payloads, raw prompts, secrets, or hidden system prompts.

### Requirement: Obsidian-compatible workflows SHALL remain local-first

Pinax SHALL let users use Obsidian-like workflows without requiring Obsidian itself, external plugins, cloud services, provider credentials, or network access for core local behavior.

#### Scenario: Daily notes and templates work without external plugins

- **WHEN** the user runs `pinax journal daily open --template journal.daily --vault ./my-notes --json`
- **THEN** Pinax SHALL create or show a local daily note from inspectable templates
- **AND** it SHALL NOT require Obsidian, DataviewJS, Templater, Lark, Notion, Pinax Cloud, provider tokens, cookies, or network access.

#### Scenario: Publish plan treats vault as source and output as artifact

- **WHEN** the user runs `pinax publish plan --profile public --target github-pages --vault ./my-notes --json`
- **THEN** Pinax SHALL plan a generated publish artifact from local vault content and configured profile
- **AND** GitHub Pages, Wiki, or other publish targets SHALL NOT become the note source of truth.

#### Scenario: Plugin failures cannot replace core behavior

- **WHEN** a Pinax plugin or Obsidian-origin plugin metadata is present but invalid, disabled, or unsupported
- **THEN** core local commands for note list/show, search, query, backlinks, vault doctor, database view render, project board show, and publish plan SHALL continue to work or return bounded warnings
- **AND** plugin failure SHALL NOT corrupt Markdown, `.pinax/**`, index, sync state, provider state, or Git state.
