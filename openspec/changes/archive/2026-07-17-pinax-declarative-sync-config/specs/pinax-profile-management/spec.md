## ADDED Requirements

### Requirement: profile 与仓库 secret identity 解耦

Pinax profiles SHALL store local credential references and unlock identities, while repository declarations SHALL store only stable logical identifiers and provider names.

#### Scenario: 多设备复用声明而不复制本机 profile

- **WHEN** two devices bootstrap the same repository with different local profile names
- **THEN** both SHALL resolve the same declared credential identity to device-local providers
- **AND** repository files SHALL NOT contain either profile's raw credentials

#### Scenario: 已有 encryption secret ref 被复用

- **WHEN** a workspace has an existing encryption secret reference and repository declaration omits a replacement
- **THEN** Pinax SHALL reuse the existing reference
- **AND** SHALL NOT silently rotate the key or create a new incompatible stored secret
