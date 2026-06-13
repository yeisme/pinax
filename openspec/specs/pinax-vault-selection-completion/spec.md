# pinax-vault-selection-completion Specification

## Purpose

Define Pinax vault selector registration, default vault resolution, shell completion, and remote discovery cache behavior so local note workflows can switch vaults by stable aliases without network or secret side effects during completion.
## Requirements
### Requirement: Named local vault registry

Pinax SHALL provide CLI-authored named local vault aliases and a selected default vault.

#### Scenario: Register local vault alias

- **GIVEN** a valid Pinax vault exists at `./work-notes`
- **WHEN** the user runs `pinax vault register ./work-notes --name work --default`
- **THEN** Pinax SHALL store the absolute path under alias `work` in the user vault registry
- **AND** Pinax SHALL select `work` as the default vault
- **AND** the output SHALL be available as a normal projection in all supported output modes.

#### Scenario: Use selected vault without flag

- **GIVEN** alias `work` is registered and selected as the default vault
- **WHEN** the user runs a vault-scoped command such as `pinax note list` without `--vault`
- **THEN** Pinax SHALL resolve the command against the `work` vault
- **AND** an explicit `--vault`, `PINAX_VAULT`, or user config `vault` SHALL override the registry default.

### Requirement: Vault selector completion

Pinax SHALL provide completion for the persistent `--vault` flag without remote side effects.

#### Scenario: Complete local aliases

- **GIVEN** local aliases `work` and `personal` are registered
- **WHEN** the shell requests completion for `pinax note list --vault <TAB>`
- **THEN** Pinax SHALL return `work` and `personal` with descriptions
- **AND** completion SHALL not write registry/cache files.

#### Scenario: Complete cached remote selectors

- **GIVEN** the remote vault cache contains selector `cloud:team`
- **WHEN** the shell requests completion for `--vault cloud:<TAB>`
- **THEN** Pinax SHALL return `cloud:team` with profile/workspace metadata
- **AND** completion SHALL not perform network calls, resolve secrets, or print tokens.

### Requirement: Remote vault discovery cache

Pinax SHALL refresh remote vault discovery explicitly and store only redacted metadata for completion.

#### Scenario: Refresh remote cache from profile endpoint

- **GIVEN** a profile named `cloud-work` has an endpoint and secret reference
- **WHEN** the user runs `pinax vault remote refresh --profile cloud-work`
- **THEN** Pinax SHALL fetch the remote vault list from the endpoint
- **AND** Pinax SHALL write selectors and metadata to the user cache
- **AND** Pinax SHALL NOT write the raw token, Authorization header, cookies, or provider payload bodies to the cache or output.

#### Scenario: List remote cache offline

- **GIVEN** a remote cache exists
- **WHEN** the user runs `pinax vault remote list --profile cloud-work`
- **THEN** Pinax SHALL read only the local cache
- **AND** Pinax SHALL output cached selectors, labels, profile, workspace, and fetched timestamp.

### Requirement: Vault-aware note completion

Pinax SHALL use the resolved vault selector for note command completions.

#### Scenario: Complete note refs from selected alias

- **GIVEN** alias `work` points to a vault containing note `Alpha`
- **WHEN** the shell requests completion for `pinax note show --vault work <TAB>`
- **THEN** Pinax SHALL resolve `work` to its local path and return `Alpha` note candidates.

#### Scenario: Default alias drives note commands

- **GIVEN** alias `work` is selected as the default vault
- **WHEN** the user runs `pinax note list` without `--vault`
- **THEN** Pinax SHALL use the `work` vault.

