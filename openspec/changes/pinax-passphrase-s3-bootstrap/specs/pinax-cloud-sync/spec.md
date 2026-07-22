## ADDED Requirements

### Requirement: Direct S3 临时 credential provider

Cloud Sync SHALL allow direct S3 transport to consume an explicitly unlocked runtime credential provider without materializing AWS credential files.

#### Scenario: 使用 repository credential
- **WHEN** the active declaration uses `repository-encrypted` credentials
- **THEN** the S3 adapter SHALL pass an in-memory AWS SDK credentials provider to the client config
- **AND** SHALL NOT set process-global AWS credential environment variables
- **AND** SHALL NOT persist plaintext credentials in `.pinax/cloud/` or user-visible evidence

#### Scenario: credential 生命周期结束
- **WHEN** a sync command or daemon run completes
- **THEN** Pinax SHALL release references to the runtime credential snapshot
- **AND** SHALL NOT cache plaintext credential values in sync receipts, backend registries or debug logs

### Requirement: 新设备禁止远端写入

Cloud Sync SHALL preserve pull-only safety when bootstrap is combined with an immediate pull.

#### Scenario: bootstrap pull
- **WHEN** a device has no trusted local sync receipt and runs bootstrap with pull
- **THEN** the transport SHALL execute no remote write operation
- **AND** output SHALL report `remote_write=false`
- **AND** local deletions SHALL NOT be uploaded as tombstones

