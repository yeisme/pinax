## ADDED Requirements

### Requirement: 声明 repository-encrypted credential mode

Pinax SHALL allow a repository declaration to select encrypted repository credentials without changing the default device-profile workflow.

#### Scenario: 默认保持 device profile
- **WHEN** an existing declaration omits `credential_mode`
- **THEN** Pinax SHALL treat it as `device-profile`
- **AND** existing AWS shared profile and default credential chain behavior SHALL remain unchanged

#### Scenario: 显式 repository-encrypted 模式
- **WHEN** declaration sets `credential_mode=repository-encrypted` and references a credential identity
- **THEN** bootstrap and sync SHALL require the matching encrypted credential bundle
- **AND** SHALL NOT silently fall back to another local AWS account or profile when unlock fails

### Requirement: Bootstrap 可选执行 pull

Pinax SHALL extend repository bootstrap with an explicit pull option while preserving existing compile-only behavior.

#### Scenario: 未指定 pull
- **WHEN** 用户运行现有 `pinax sync repo bootstrap --device <id> --yes --json` without `--pull`
- **THEN** Pinax SHALL keep the existing compile-only bootstrap behavior
- **AND** SHALL provide a follow-up pull command

#### Scenario: 指定 pull
- **WHEN** 用户增加 `--pull`
- **THEN** Pinax SHALL execute pull only after declaration、secrets、runtime and protected-path validation succeeds
- **AND** new-device policy SHALL remain pull-only regardless of local directory contents

### Requirement: Repository sync capability gate

Pinax SHALL allow a repository declaration to require additive sync capabilities before compiling or executing repository-encrypted S3 configuration.

#### Scenario: 当前二进制满足声明能力
- **WHEN** declaration requires `repository-encrypted-s3-v1`、`capsa-remote-commit-v1` and `pull-only-bootstrap-v1`
- **THEN** doctor、apply、bootstrap、diff、pull、push and daemon SHALL validate all required capabilities before remote access
- **AND** output SHALL expose only capability names and support status

#### Scenario: 当前二进制缺少能力
- **WHEN** any declared capability is unsupported
- **THEN** Pinax SHALL fail closed with `sync_capability_unsupported`
- **AND** SHALL report `remote_write=false`
- **AND** SHALL provide a real upgrade or device-profile rollback command
