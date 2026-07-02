# cli-tree-ux Specification Delta

## MODIFIED Requirements

### Requirement: Pinax updates command help and completion safely

Pinax SHALL update help and completion behavior to favor the primary command tree without triggering writes or remote operations.

#### Scenario: Completion is lightweight

- **WHEN** shell completion is invoked for Pinax commands
- **THEN** completion handlers SHALL NOT write vault files, `.pinax` metadata, Git state, provider state, or remote systems.

#### Scenario: Help examples use primary paths

- **WHEN** a user reads help examples for vault, journal, storage, or organize workflows
- **THEN** examples SHALL prefer the new primary command paths
- **AND** compatibility paths SHALL be documented only where needed for migration clarity.

#### Scenario: High-value object completion covers local workflow objects

- **GIVEN** a local vault contains projects, project subprojects, folders, backend profiles, prompt assets, plugins, and sync conflict files
- **WHEN** the shell requests completion for matching `project`, `folder`, `backend`, `prompt`, `plugin`, `collection`, `graph`, or `sync conflicts` commands
- **THEN** Pinax SHALL return matching local candidates with short non-sensitive descriptions
- **AND** profile completion SHALL NOT expose endpoints, raw tokens, Authorization headers, cookies, or secret values.

#### Scenario: Path-like flags keep file completion

- **WHEN** the shell requests completion for path-like flags such as `--from`, `--to`, `--api-token-file`, `--root`, or `sync conflicts resolve --merged`
- **THEN** Pinax SHALL leave shell file completion enabled unless the command has an explicit safe object registry for that argument.

#### Scenario: Rendering and enum flags complete statically

- **WHEN** the shell requests completion for bounded enum flags such as `--color`, `--theme`, `--markdown-style`, lifecycle, collection export format, graph node kind, or project board display fields
- **THEN** Pinax SHALL return the documented enum values with `ShellCompDirectiveNoFileComp`.
