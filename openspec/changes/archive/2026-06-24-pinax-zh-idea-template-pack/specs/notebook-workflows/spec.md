# notebook-workflows Delta Spec

## MODIFIED Requirements

### Requirement: Built-in templates support common notebook workflows

Pinax SHALL provide built-in Chinese templates for parked idea seeds and detailed content notes without requiring a dedicated `idea` command.

#### Scenario: Create a parked idea seed from a built-in template

- **WHEN** a user runs `pinax note add "某篇小说是怎么写成的" --template idea.research_seed --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note under `ideas/research/`
- **AND** the note frontmatter SHALL include `kind: idea`, `status: parked`, and tags including `idea` and `research-seed`
- **AND** the template body SHALL use Chinese headings for trigger, value, questions, leads, and related notes without creating todo checkboxes.

#### Scenario: Recommend Chinese templates by intent

- **WHEN** a user runs `pinax template recommend --intent "动漫" --vault ./my-notes --json`
- **THEN** Pinax SHALL recommend an anime-related note template such as `idea.anime_watch` or `media.anime`
- **AND** recommendation SHALL remain metadata-only, local, and read-only.

#### Scenario: Create an ideas index page

- **WHEN** a user runs `pinax index page create ideas --template index.ideas --vault ./my-notes --json`
- **THEN** Pinax SHALL create a managed index page for notes where `kind` is `idea` and `status` is `parked`.
