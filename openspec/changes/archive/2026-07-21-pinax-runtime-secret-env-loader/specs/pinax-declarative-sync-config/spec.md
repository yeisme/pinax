## ADDED Requirements

### Requirement: 加密配置支持 env 资产

Declarative sync configuration SHALL allow a logical env asset identity while keeping the encrypted dotenv path fixed and the plaintext runtime path local-only.

#### Scenario: 声明 env identity

- **WHEN** `pinax-sync.yaml` references an env identity
- **THEN** bootstrap SHALL resolve `.pinax/pinax-sync.env.age` through the same unlock provider
- **AND** SHALL not copy plaintext values into `pinax-sync.yaml` or generated repository metadata
