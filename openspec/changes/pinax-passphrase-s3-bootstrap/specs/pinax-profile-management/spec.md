## ADDED Requirements

### Requirement: Device profile 与仓库密文 credential 的显式优先级

Pinax SHALL resolve S3 credentials according to the declaration's credential mode rather than silently mixing credential sources.

#### Scenario: device-profile 模式
- **WHEN** credential mode is `device-profile`
- **THEN** Pinax SHALL use the configured shared profile or AWS default credential chain
- **AND** SHALL NOT require repository credential unlock

#### Scenario: repository-encrypted 模式
- **WHEN** credential mode is `repository-encrypted`
- **THEN** Pinax SHALL use only the declared repository credential identity for provider authentication
- **AND** a local profile with the same or different name SHALL NOT override it implicitly

### Requirement: Profile rollback 保持可用

Pinax SHALL keep the existing profile-based S3 configuration as a reversible rollback path.

#### Scenario: 回滚到本机 profile
- **WHEN** an operator changes the declaration back to `device-profile` and reapplies config
- **THEN** existing `pinax capsa backend set s3 --profile <name>` behavior SHALL remain usable
- **AND** the rollback SHALL NOT rotate the content encryption key or delete remote revisions

