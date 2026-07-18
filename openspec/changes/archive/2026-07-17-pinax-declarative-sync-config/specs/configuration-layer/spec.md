## ADDED Requirements

### Requirement: 项目配置支持声明式同步层

Pinax configuration SHALL resolve repository sync declarations as a distinct, validated project layer without allowing environment variables or flags to silently replace secret identities.

#### Scenario: 同步声明优先于本机生成配置

- **WHEN** repository declaration and generated runtime config both exist
- **THEN** `sync repo doctor` SHALL compare them using typed normalized values
- **AND** `sync repo apply` SHALL be the only normal path that regenerates runtime sync state

#### Scenario: 明文敏感字段拒绝进入声明配置

- **WHEN** `.pinax/pinax-sync.yaml` contains secret-like keys or literal credentials
- **THEN** validation SHALL fail with a stable error code
- **AND** SHALL report the field path with redaction
- **AND** SHALL recommend `pinax sync repo secret set` or an external credential reference
