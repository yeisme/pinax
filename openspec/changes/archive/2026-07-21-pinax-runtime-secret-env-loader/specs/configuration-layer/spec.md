## ADDED Requirements

### Requirement: runtime env overlay 安全

Configuration SHALL resolve decrypted Pinax env values only for an explicit allowlist and SHALL preserve the precedence of explicit flags and process environment.

#### Scenario: 非 allowlist env 不注入

- **WHEN** the encrypted env asset contains an undeclared key
- **THEN** Pinax SHALL reject or omit it according to the typed policy
- **AND** SHALL report the key name without its value
- **AND** SHALL not pass the entire decrypted environment to child processes
