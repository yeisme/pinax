# pinax Spec Delta

## ADDED Requirements

### Requirement: Core note creation

Pinax SHALL create Markdown notes from the CLI while preserving Markdown files as the source of truth.

#### Scenario: Create a note with frontmatter

- **WHEN** a user runs `pinax note new "研究日志" --tags research,pinax --vault <vault>`
- **THEN** Pinax creates a Markdown file under the vault
- **AND** the file contains YAML frontmatter with `schema_version`, `note_id`, `title`, `tags`, `created_at`, and `updated_at`
- **AND** the command returns a structured projection containing the created note path.

### Requirement: Template rendering

Pinax SHALL manage editable Markdown templates and render them without executing code.

#### Scenario: Initialize and render built-in templates

- **WHEN** a user runs `pinax template init --vault <vault>` and `pinax template render mermaid --title "架构" --vault <vault>`
- **THEN** Pinax creates built-in templates under `.pinax/templates/`
- **AND** rendering replaces safe variables such as `{{title}}`, `{{date}}`, `{{datetime}}`, `{{project}}`, and `{{tags}}`
- **AND** the mermaid template contains a Markdown Mermaid code fence.

### Requirement: Hybrid search and local index

Pinax SHALL combine fast full-text search with a local SQLite/GORM index projection.

#### Scenario: Rebuild index and search backlinks

- **WHEN** a vault contains notes using `[[Wiki Link]]` and `#tag`
- **AND** a user runs `pinax index rebuild --vault <vault>`
- **THEN** Pinax stores note, tag, and link projections in `.pinax/index.sqlite` through GORM
- **AND** `pinax search <query> --vault <vault>` reports whether it used `rg` or scan fallback.

### Requirement: Sync planning boundary

Pinax SHALL expose sync plans for Git, S3, and Pinax Cloud without pretending that unimplemented remote writes succeeded.

#### Scenario: Cloud sync reports backend requirement

- **WHEN** a user runs `pinax sync diff --target cloud --vault <vault>`
- **THEN** Pinax returns a plan with `backend_required=true`
- **AND** the projection includes the minimum Pinax Cloud API handoff
- **AND** `pinax sync push --target cloud --vault <vault>` without `--yes` fails with an approval-required error.
