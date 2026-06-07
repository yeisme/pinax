# pinax Spec Delta

## ADDED Requirements

### Requirement: Template authoring from the CLI

Pinax SHALL let users create editable Markdown templates through CLI commands while keeping templates as local text files.

#### Scenario: Create a template from a file

- **WHEN** a user runs `pinax template create meeting --from ./meeting.md --vault ./my-notes --json`
- **THEN** Pinax writes `.pinax/templates/meeting.md` through the application service
- **AND** stdout contains one JSON projection for `template.create`
- **AND** the projection contains the template name and path
- **AND** no cloud backend, provider credential, or network connection is required.

#### Scenario: Create a template from inline body

- **WHEN** a user runs `pinax template create daily-review --body "# {{date}}" --vault ./my-notes --json`
- **THEN** Pinax writes `.pinax/templates/daily-review.md`
- **AND** the body is stored as plain Markdown text
- **AND** no shell, script, environment variable, or network interpolation is executed.

#### Scenario: Reject unsafe template names

- **WHEN** a user runs `pinax template create ../bad --body "x" --vault ./my-notes --json`
- **THEN** Pinax fails with stable error code `invalid_template_name`
- **AND** no file outside `.pinax/templates/` is created.

### Requirement: Template variables are safe and explicit

Pinax SHALL render templates using explicit text variables without executing code.

#### Scenario: Render custom variables

- **GIVEN** `.pinax/templates/meeting.md` contains `# {{title}}\n客户: {{client}}`
- **WHEN** a user runs `pinax template render meeting --title "客户会议" --var client=Acme --vault ./my-notes --json`
- **THEN** the rendered body contains `# 客户会议`
- **AND** the rendered body contains `客户: Acme`
- **AND** the command does not execute scripts, shell commands, environment lookups, or network calls.

#### Scenario: Missing variables fail clearly

- **GIVEN** `.pinax/templates/meeting.md` contains `客户: {{client}}`
- **WHEN** a user runs `pinax template render meeting --vault ./my-notes --json`
- **THEN** Pinax fails with stable error code `template_variable_missing`
- **AND** the error names the missing variable without printing secrets or raw provider payload.

### Requirement: Notes can be generated from custom templates

Pinax SHALL let `note new` consume custom templates and variable values.

#### Scenario: Create note from custom template

- **GIVEN** `.pinax/templates/meeting.md` contains `# {{title}}\n客户: {{client}}`
- **WHEN** a user runs `pinax note new "客户会议" --template meeting --var client=Acme --tags meeting,client --vault ./my-notes --json`
- **THEN** Pinax creates a Markdown note under `notes/`
- **AND** the note has Pinax frontmatter with `schema_version`, `note_id`, `title`, `tags`, `created_at`, and `updated_at`
- **AND** the note body contains the rendered custom template content.

### Requirement: Template validation reports actionable results

Pinax SHALL validate templates before generation so malformed templates do not silently create broken notes.

#### Scenario: Validate a valid Mermaid template

- **GIVEN** `.pinax/templates/diagram.md` contains a closed Mermaid code fence
- **WHEN** a user runs `pinax template validate diagram --vault ./my-notes --json`
- **THEN** Pinax returns `status=success`
- **AND** the projection includes facts for template name, variables, and issues count.

#### Scenario: Detect unclosed fences

- **GIVEN** `.pinax/templates/bad.md` contains an unclosed Markdown code fence
- **WHEN** a user runs `pinax template validate bad --vault ./my-notes --json`
- **THEN** Pinax fails or returns `status=partial` with issue code `template_fence_unclosed`
- **AND** `pinax note new --template bad` SHALL NOT create a note unless validation passes.

### Requirement: Template deletion is explicit and safe

Pinax SHALL protect templates from accidental deletion.

#### Scenario: Delete custom template with approval

- **GIVEN** `.pinax/templates/meeting.md` exists
- **WHEN** a user runs `pinax template delete meeting --vault ./my-notes --yes --json`
- **THEN** Pinax deletes only `.pinax/templates/meeting.md`
- **AND** it records a redacted event through the application service.

#### Scenario: Reject delete without approval

- **GIVEN** `.pinax/templates/meeting.md` exists
- **WHEN** a user runs `pinax template delete meeting --vault ./my-notes --json`
- **THEN** Pinax fails with stable error code `approval_required`
- **AND** the template file remains unchanged.

