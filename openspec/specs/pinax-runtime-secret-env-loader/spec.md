# pinax-runtime-secret-env-loader Specification

## Purpose
TBD - created by archiving change pinax-runtime-secret-env-loader. Update Purpose after archive.
## Requirements
### Requirement: 加密 dotenv 资产

Pinax SHALL support a repository-tracked encrypted dotenv asset at `.pinax/pinax-sync.env.age` whose plaintext is never required to be committed.

#### Scenario: 创建加密 env 资产

- **WHEN** 用户运行 `pinax sync env init --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update the encrypted env asset through the application service
- **AND** the asset SHALL contain ciphertext and redacted metadata only
- **AND** the plaintext file SHALL NOT be created by default

#### Scenario: 设置 env 值不输出明文

- **WHEN** 用户运行 `pinax sync env set COS_SECRET_KEY --vault ./my-notes --json`
- **THEN** Pinax SHALL encrypt the value using the configured unlock provider
- **AND** stdout、stderr、events、receipts 和 logs SHALL NOT contain the value

### Requirement: 动态运行时加载

Pinax SHALL decrypt and parse the encrypted env asset into an immutable in-memory snapshot at command or sync-run boundaries.

#### Scenario: 命令内存注入

- **WHEN** a sync command resolves a valid encrypted env asset
- **THEN** Pinax SHALL inject only allowlisted keys into the application/provider boundary
- **AND** SHALL NOT require the user to run `source` or `export`
- **AND** explicit command flags and explicitly supplied process environment SHALL take precedence

#### Scenario: daemon reload

- **WHEN** the encrypted env asset changes between daemon sync runs
- **THEN** the daemon SHALL attempt unlock and parse before the next run
- **AND** a successful snapshot SHALL be used atomically by the next run
- **AND** the current run SHALL retain its original snapshot

#### Scenario: reload failure fail-safe

- **WHEN** unlock or parse fails during daemon reload
- **THEN** Pinax SHALL retain the last successful snapshot or enter a structured degraded state if none exists
- **AND** SHALL NOT partially apply the new values or write remote changes using a partial snapshot

### Requirement: 严格 dotenv parser

Pinax SHALL parse only an explicit safe dotenv subset and SHALL NOT execute shell syntax.

#### Scenario: 拒绝命令替换

- **WHEN** an env value contains `$()`, backticks, `${}`, NUL, control characters or an include directive
- **THEN** parsing SHALL fail with a stable error code and line number
- **AND** SHALL NOT disclose the rejected value

### Requirement: 明文 env 默认 Git 忽略

Pinax SHALL maintain a managed Git ignore block for plaintext environment files and runtime materializations.

#### Scenario: 初始化自动忽略

- **WHEN** 用户运行 `pinax sync env init --vault ./my-notes --json`
- **THEN** `.gitignore` SHALL ignore `.env`, `.env.*`, `*.env` and `.pinax/runtime/`
- **AND** SHALL explicitly allow the encrypted asset and non-sensitive `.env.example` templates
- **AND** SHALL preserve unrelated user-authored ignore rules

#### Scenario: 已跟踪 env 文件告警

- **WHEN** Git index already tracks a plaintext env file
- **THEN** `pinax sync env doctor --vault ./my-notes --json` SHALL report `tracked_secret_env`
- **AND** SHALL recommend `git rm --cached -- <path>`
- **AND** SHALL NOT delete the working-tree file automatically

### Requirement: 明文 materialize 显式且受保护

Pinax SHALL not materialize plaintext env files unless the user explicitly requests it.

#### Scenario: 显式 materialize

- **WHEN** 用户运行 `pinax sync env unlock --vault ./my-notes --materialize --json`
- **THEN** Pinax SHALL write only to the managed runtime path with restrictive permissions
- **AND** SHALL report path and permission facts without values
- **AND** the path SHALL be Git-ignored and Capsa-protected

#### Scenario: 清理 materialized env

- **WHEN** 用户运行 `pinax sync env clean --vault ./my-notes --json`
- **THEN** Pinax SHALL remove only Pinax-managed materialized env files
- **AND** SHALL never remove arbitrary user-selected files
