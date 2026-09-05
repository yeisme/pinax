# builtin-template-variable-contract Specification

## Purpose
TBD - created by archiving change pinax-template-variable-contract-v1. Update Purpose after archive.
## Requirements
### Requirement: Built-in template metadata declares referenced required variables

Pinax SHALL declare `url` as a required variable for each built-in note template whose body unconditionally reads `.Vars.url`.

#### Scenario: Inspecting a URL-backed built-in template

- **WHEN** an operator inspects `learning.video`, `learning.source`, or `source.github`
- **THEN** the template metadata SHALL contain a required `url` variable with a non-empty description
- **AND** the normalized `variable_schema` and `required_variables` projection SHALL contain `url`

#### Scenario: Completing template variables

- **WHEN** shell completion requests variables for one of those templates
- **THEN** Pinax SHALL return `url=` as a required string with its description

#### Scenario: Templates without declared variables remain compatible

- **WHEN** an existing built-in note template does not pass variable metadata to the generator
- **THEN** its generated frontmatter and render behavior SHALL remain unchanged

