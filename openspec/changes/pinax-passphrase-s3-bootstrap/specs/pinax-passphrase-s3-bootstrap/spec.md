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

### Requirement: Capsa 内容密钥随仓库密文可移植

Pinax SHALL allow the same repository envelope to carry the existing Capsa content encryption key without exposing it in Git or generated runtime config.

#### Scenario: 新设备恢复内容密钥
- **WHEN** the envelope contains the declared `encryption_key` entry with format `capsa_encryption_key.v1`
- **THEN** bootstrap SHALL authenticate it with the same repository passphrase used for the S3 credential
- **AND** SHALL persist it only to a device-level `stored://` reference
- **AND** SHALL preserve the original key value and remote key identity

#### Scenario: 内容密钥缺失或格式错误
- **WHEN** the declared encryption entry is missing, empty or has another kind/format
- **THEN** bootstrap SHALL fail before runtime compilation and pull
- **AND** SHALL NOT generate a replacement content key

### Requirement: Device profile 一键迁移

Pinax SHALL provide a CLI-authored migration from an existing S3 device profile to the repository-encrypted configuration.

#### Scenario: 迁移现有设备
- **WHEN** 用户运行 `pinax sync repo migrate device-profile --unlock prompt --remember-keychain --yes --json`
- **THEN** Pinax SHALL read the active S3 runtime, configured AWS shared profile and current Capsa content key
- **AND** SHALL author `.pinax/pinax-sync.yaml` plus one `.pinax/project-secrets.yaml` containing `s3_credentials.v1` and `capsa_encryption_key.v1`
- **AND** output SHALL report `remote_write=false` and `key_rotated=false`
- **AND** SHALL NOT contact the remote, write `~/.aws/credentials` or expose plaintext

#### Scenario: 迁移原子失败
- **WHEN** envelope、declaration、Git protected-path update、fsync or atomic rename 中任一步失败
- **THEN** the previous runtime、declaration and encrypted envelope SHALL remain usable
- **AND** Pinax SHALL NOT leave a declaration that references an absent or unverifiable credential entry
- **AND** output SHALL report `remote_write=false`

#### Scenario: 重复执行相同迁移
- **WHEN** the existing repository envelope already contains the same S3 credential identity and Capsa content-key identity
- **THEN** Pinax SHALL return `already_migrated=true`
- **AND** SHALL NOT rotate the DEK、rewrite ciphertext、change remote namespace or contact the remote

#### Scenario: 既有 envelope 身份冲突
- **WHEN** an existing envelope cannot be authenticated or contains another credential、content-key or repository identity
- **THEN** migration SHALL fail with `migration_conflict`
- **AND** SHALL require an explicit rotate/reconcile workflow rather than overwriting the envelope

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

### Requirement: macOS 支持声明受证据与平台元组约束

Pinax release and onboarding documentation SHALL distinguish an observed macOS success from supported repository-encrypted S3/COS synchronization. A support claim SHALL name the verified `darwin/<arch>` tuple rather than extrapolating from a single Mac or from GoReleaser artifact availability.

#### Scenario: 一台 Mac 已可用但证据未完整
- **WHEN** an operator reports that a Mac completed the workflow but no redacted release-candidate evidence covers the full round-trip and recovery matrix
- **THEN** the capability SHALL remain `experimental`
- **AND** documentation MAY record the result as `observed`
- **AND** SHALL NOT label all macOS architectures or installation channels as supported

#### Scenario: 发布候选的只读合同探针
- **WHEN** an operator invokes `task integration:sync-macos-candidate` with a release-candidate binary, safe release provenance and installation channel on a declared `darwin/<arch>` tuple
- **THEN** the runner SHALL write the standard redacted evidence directory and `artifacts/platform-support.json`
- **AND** SHALL record the exact platform tuple, macOS version, release provenance, installation channel and bootstrap/inbound/outbound/recovery command-contract stages
- **AND** SHALL reject a non-Darwin host, missing/unsafe provenance or missing required contract stage, and SHALL NOT persist raw candidate command output or body
- **AND** SHALL NOT open a vault, access a remote or change a support status beyond `candidate`
- **AND** SHALL retain `bootstrap_pull`, `outbound_round_trip` and `recovery_matrix` as `not_run` until the separately authorized real workflow executes

#### Scenario: 发布候选完成 macOS 双向验证
- **WHEN** a release-candidate Pinax binary on one declared `darwin/<arch>` tuple completes existing-device remote-aware preflight, clone-time Keychain bootstrap pull, Mac deliberate durable push, existing-device pull/validation, and required recovery checks
- **THEN** the redacted evidence SHALL identify the release provenance, platform tuple, stage outcomes, revision identities and recovery result
- **AND** the support matrix MAY mark only that tuple as `supported`
- **AND** the Mac bootstrap stage SHALL retain `pull_only=true` and `remote_write=false`
- **AND** a changed Mac push SHALL include `remote_write=true`, a non-empty `revision_id` and read-back confirmation

#### Scenario: 未验证的 macOS 架构或安装渠道
- **WHEN** a GoReleaser archive exists for another macOS architecture or installation channel without matching evidence
- **THEN** release documentation SHALL describe it as `unverified`
- **AND** SHALL NOT infer workflow support from successful compilation, archive extraction or a different platform tuple
