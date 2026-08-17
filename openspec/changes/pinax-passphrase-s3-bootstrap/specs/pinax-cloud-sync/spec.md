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

### Requirement: Repository credential 覆盖所有远端同步命令

Cloud Sync SHALL resolve repository-encrypted credentials consistently for remote-aware diff, pull, push and daemon runs.

#### Scenario: 显式或 Keychain 解锁远端操作
- **WHEN** active declaration uses `repository-encrypted` and a command needs remote head、manifest、blob or commit access
- **THEN** Pinax SHALL use the explicit unlock source or repository-scoped Keychain default
- **AND** SHALL NOT fall back to an AWS shared profile、default credential chain or unrelated AWS environment credentials
- **AND** `diff`、`pull` and `push` SHALL expose symmetric unlock flags

#### Scenario: daemon 无法非交互解锁
- **WHEN** daemon cannot resolve a Keychain、0600 file or approved secret-manager source
- **THEN** daemon SHALL enter a degraded state and skip Pull and Push
- **AND** SHALL NOT open a TTY prompt
- **AND** SHALL report `remote_write=false` and a redacted recovery action

### Requirement: Remote-aware 备份计划

Cloud Sync SHALL distinguish a remote-aware plan from a cached local plan.

#### Scenario: diff 或 push dry-run 已读取远端
- **WHEN** Pinax successfully authenticates and reads the remote head and manifest
- **THEN** output SHALL report `remote_checked=true` and the observed remote revision
- **AND** the plan MAY be used as the approval input for a subsequent push

#### Scenario: 只能读取本地 cache
- **WHEN** unlock、network or provider access is unavailable and only cached state can be inspected
- **THEN** output SHALL report `remote_checked=false` and `diff_scope=cached`
- **AND** SHALL NOT present the plan as proof that a backup can be safely committed

### Requirement: Durable S3 backup commit evidence

Cloud Sync SHALL treat a backup as complete only after the remote manifest revision is durably committed and verified.

#### Scenario: 有变化的 push 成功
- **WHEN** all missing blobs are uploaded and the remote manifest CAS commit succeeds
- **THEN** output SHALL report `remote_write=true`、a non-empty `revision_id` and committed manifest identity
- **AND** Pinax SHALL read back the remote head before reporting backup completion

#### Scenario: 无变化且远端已一致
- **WHEN** remote-aware comparison proves local manifest equals the current remote head
- **THEN** output SHALL report `up_to_date=true`、`remote_checked=true` and `remote_write=false`
- **AND** this SHALL be distinguishable from an unwired、blocked、dry-run or failed remote write

#### Scenario: 仅 blob 上传或本地 receipt 成功
- **WHEN** blob upload succeeds but manifest commit/read-back does not succeed
- **THEN** output SHALL report backup incomplete and `remote_write=false`
- **AND** SHALL NOT claim durable backup completion
