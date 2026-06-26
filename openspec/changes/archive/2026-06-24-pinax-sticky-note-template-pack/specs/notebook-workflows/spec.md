# notebook-workflows Delta Spec

## MODIFIED Requirements

### Requirement: Built-in note templates cover common workflows

Pinax SHALL provide built-in sticky short-document templates for inbox capture without turning them into managed project board items.

#### Scenario: Sticky template creates a short inbox note

- **WHEN** a user runs `pinax note add "临时线索" --template sticky.capture --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note under `inbox/sticky/`
- **AND** the note frontmatter SHALL include `kind: sticky`, `status: inbox`, and tags including `sticky` and `capture`
- **AND** the template body SHALL remain a short capture note without creating todo checkboxes, `board_column`, or managed project item metadata.

#### Scenario: Sticky project signal keeps project context without becoming a board item

- **WHEN** a user runs `pinax note add "子项目看板线索" --template sticky.project_signal --project research --folder inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note in the project inbox path
- **AND** the note SHALL keep `kind: sticky` and `status: inbox`
- **AND** it SHALL NOT write `board_column` or `kind: task`; creating a movable board item SHALL continue to require `pinax project item add`.

### Requirement: Template recommendation helps users choose templates

Pinax SHALL recommend sticky templates from local metadata for Chinese short-document intents.

#### Scenario: Recommend sticky templates by intent

- **WHEN** a user runs `pinax template recommend --intent "便签" --vault ./my-notes --json`
- **THEN** Pinax SHALL recommend `sticky.capture` as the primary template
- **AND** project signal intents such as `子项目看板线索` SHALL recommend `sticky.project_signal`
- **AND** recommendation SHALL remain metadata-only, local, and read-only.
