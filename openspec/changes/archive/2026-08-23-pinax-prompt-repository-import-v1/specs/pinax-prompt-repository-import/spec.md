## ADDED Requirements

### Requirement: Pinax SHALL separate local Prompt Vault and federated catalog semantics

Existing `pinax prompt search|show|resolve` SHALL continue to operate on Pinax-owned local PromptAssets. Federated repository operations SHALL be exposed below `pinax prompt repository` and `pinax prompt catalog` and SHALL use public promptrepo contracts.

#### Scenario: User searches both surfaces

- **WHEN** the user runs local `prompt search`
- **THEN** Pinax SHALL return only local Prompt Vault assets
- **AND** `prompt catalog search` SHALL be required for external/user/official repositories.

### Requirement: Repository profiles SHALL remain shared and credential-safe

Pinax SHALL read and mutate user RepositoryProfiles through the shared promptrepo engine or organization profiles through Template Registry service mode. Pinax SHALL NOT copy profile state, source credentials or Registry private records into the vault or Pinax database.

#### Scenario: Repository added through Sonora is listed in Pinax

- **WHEN** both consumers use embedded mode for the same OS user
- **THEN** Pinax SHALL list the same shared profile
- **AND** SHALL NOT require a second add operation.

### Requirement: Inspect, validate and preview SHALL be provider-free and body-free

Pinax SHALL use exact TemplateAddress and verified TemplateContract for inspect, validate and preview. These operations SHALL make zero Provider calls and zero durable writes by default. Output and evidence SHALL not contain template body, rendered body, input values, credentials or private paths.

#### Scenario: Preview inputs are complete

- **WHEN** a valid input map is provided
- **THEN** preview SHALL report readiness, rendered digest/size and `provider_calls=0`
- **AND** a body sentinel SHALL be absent from all renderers and evidence.

### Requirement: Catalog install SHALL create a rights-permitted local draft

Pinax SHALL materialize a verified body only after explicit install and only when rights and contract permission allow local import or copy. It SHALL create a Pinax-owned PromptAsset with lifecycle `draft`, a versioned body, exact source refs and digests through the existing application service and GORM repository.

#### Scenario: Import is permitted

- **WHEN** the user installs an allowed template
- **THEN** Pinax SHALL create a draft local asset addressable by `pinax://prompt/<id>`
- **AND** SHALL NOT automatically mark it tested, accepted or promoted.

#### Scenario: Template is preview-only

- **WHEN** the permission excludes local copy
- **THEN** install SHALL fail closed with a stable rights reason and next action
- **AND** no empty or partial PromptAsset SHALL be written.

### Requirement: Conflicts SHALL not overwrite active local versions

If a target local ID exists with a different digest, Pinax SHALL return an explicit conflict plan and require keep-existing, side-by-side, fork-local or reject behavior. It SHALL NOT silently replace the current version.

#### Scenario: Same local ID has different content

- **WHEN** catalog install resolves a different template digest
- **THEN** the command SHALL return conflict facts without writing
- **AND** the existing PromptAsset and current version SHALL remain unchanged.
