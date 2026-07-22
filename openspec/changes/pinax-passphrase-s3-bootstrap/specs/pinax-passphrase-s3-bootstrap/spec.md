## ADDED Requirements

### Requirement: 生产级 passphrase secrets envelope

Pinax SHALL support a production `passphrase-v1` unlock provider for repository-tracked encrypted secrets.

#### Scenario: 口令加密 credential bundle
- **WHEN** 用户通过安全 stdin 或交互式 TTY 设置 S3 credential bundle
- **THEN** Pinax SHALL 使用带随机 salt 的内存强 KDF 派生 wrapping key
- **AND** SHALL 使用认证加密保护随机 data encryption key 和 secret entry
- **AND** 仓库文件 SHALL 只包含 ciphertext、versioned crypto metadata 和 redacted identity metadata
- **AND** plaintext SHALL NOT appear in process arguments、stdout、stderr、logs、receipts 或 generated runtime config

#### Scenario: 错误口令或密文篡改
- **WHEN** passphrase 不正确、wrapped key 被修改或 entry authentication 失败
- **THEN** unlock SHALL fail closed with a stable redacted error code
- **AND** SHALL NOT distinguish sensitive cryptographic details in default output
- **AND** SHALL NOT create runtime config、pull remote content or generate a replacement key

### Requirement: macOS Keychain unlock

Pinax SHALL support an explicit macOS Keychain unlock source for repository passphrases.

#### Scenario: 用户同意记住口令
- **WHEN** 用户在 macOS interactive bootstrap 中使用 `--remember-keychain`
- **THEN** Pinax SHALL write the unlock secret to a repository-scoped Keychain item under service `pinax`
- **AND** repository files and runtime receipts SHALL store only a redacted account hint or digest
- **AND** subsequent non-interactive sync runs SHALL resolve the unlock secret without printing it

#### Scenario: Keychain 不可用
- **WHEN** the Keychain is locked, unavailable or missing the repository item
- **THEN** interactive commands SHALL offer a prompt recovery action
- **AND** non-interactive commands SHALL fail with `sync_repo_unlock_required`
- **AND** daemon SHALL NOT open a TTY prompt or continue with partial credentials

### Requirement: Typed S3/COS credential bundle

Pinax SHALL support a typed encrypted credential bundle for S3-compatible transports.

#### Scenario: 设置 static credentials
- **WHEN** 用户运行 `pinax sync repo credential set s3 --name tencent-cos-pinax --stdin --json`
- **THEN** Pinax SHALL accept only `access_key_id`、`secret_access_key` and optional `session_token`
- **AND** SHALL validate required fields before encrypting
- **AND** list/doctor output SHALL show only name、kind、identity、provider and configured status

#### Scenario: 拒绝不安全输入
- **WHEN** credential payload contains unknown fields, empty required values, nested commands, file includes or plaintext declaration fields
- **THEN** Pinax SHALL reject it with a stable field-level error
- **AND** SHALL NOT echo the rejected value

### Requirement: Clone 后 pull-only bootstrap

Pinax SHALL support bootstrapping a new device from repository declaration and encrypted S3 credentials without requiring a pre-existing AWS shared profile.

#### Scenario: 新 Mac 一次 bootstrap 并拉取
- **WHEN** 用户在 clone 后运行 `pinax sync repo bootstrap --device <id> --unlock prompt --remember-keychain --pull --yes --json`
- **THEN** Pinax SHALL unlock the repository secrets、compile local Capsa runtime and inject credentials into the AWS SDK
- **AND** SHALL perform a pull-only sync
- **AND** SHALL NOT write `~/.aws/credentials`、push、remote delete or replace remote head
- **AND** successful output SHALL report `pull_only=true` and `remote_write=false`

#### Scenario: pull 失败后可重试
- **WHEN** runtime compilation succeeds but network, provider auth or remote decryption fails
- **THEN** Pinax SHALL preserve a diagnosable runtime-ready state
- **AND** SHALL not report bootstrap success
- **AND** SHALL provide a runnable doctor and pull retry command

### Requirement: Passphrase rekey

Pinax SHALL support changing the repository unlock passphrase without rotating the Capsa content encryption key or S3 credentials.

#### Scenario: 原子 rekey
- **WHEN** 用户运行 `pinax sync repo secret rekey --provider passphrase-v1 --json`
- **THEN** Pinax SHALL authenticate the old unlock source and wrap the existing data encryption key with the new passphrase
- **AND** SHALL atomically replace the secrets asset after validation
- **AND** SHALL preserve credential digests、logical identities and content encryption key identity

#### Scenario: rekey 中断
- **WHEN** validation, fsync or atomic rename fails
- **THEN** the previous envelope SHALL remain usable
- **AND** Pinax SHALL emit a redacted failure receipt and rollback action

### Requirement: Secret-safe trace and evidence

Pinax SHALL produce reviewable bootstrap and credential evidence without exposing secrets.

#### Scenario: 集成测试证据
- **WHEN** passphrase bootstrap integration or e2e tests run
- **THEN** the runner SHALL write redacted per-run evidence under `temp/integration-test-runs/<run-id>/`
- **AND** SHALL include command、stdout、stderr、environment summary、artifacts and original exit code
- **AND** SHALL scan evidence for passphrase、SecretId、SecretKey、Authorization header and decrypted note payload leakage

