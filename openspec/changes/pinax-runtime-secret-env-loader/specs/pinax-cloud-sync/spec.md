## MODIFIED Requirements

### Requirement: env secrets 不进入内容 manifest

Cloud Sync SHALL treat encrypted and plaintext env assets, materialized runtime files and their caches as protected paths.

#### Scenario: manifest 排除 env secrets

- **WHEN** a vault contains `.env`, `.env.local`, `.pinax/pinax-sync.env.age` and `.pinax/runtime/pinax-sync.env`
- **THEN** none of these files SHALL be uploaded as ordinary plaintext content entries
- **AND** sync receipts SHALL report counts and redacted paths only
