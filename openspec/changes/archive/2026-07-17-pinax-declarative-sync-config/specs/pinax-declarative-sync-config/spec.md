## ADDED Requirements

### Requirement: 仓库声明同步配置

Pinax SHALL support a CLI-authored, versioned repository sync manifest that describes backend topology without storing plaintext credentials or device runtime state.

#### Scenario: 初始化仓库配置

- **WHEN** 用户运行 `pinax sync repo init --vault ./my-notes --json`
- **THEN** Pinax SHALL 创建或更新 `.pinax/pinax-sync.yaml`
- **AND** SHALL 写入 schema version、backend、workspace、secret identity 和 sync policy
- **AND** SHALL NOT 写入 SecretKey、token、密码或解密后的 encryption key

#### Scenario: 配置文件可跨平台复用

- **WHEN** 同一仓库在 Linux、macOS、Windows 或远程开发容器中被 checkout
- **THEN** 声明配置 SHALL 不包含平台绝对路径、daemon PID、临时目录或设备唯一 receipt
- **AND** 各平台 SHALL 能通过 bootstrap 生成本地运行配置

### Requirement: 加密 secrets 资产

Pinax SHALL support a repository-tracked encrypted secrets asset whose plaintext is available only during an authenticated runtime unlock.

#### Scenario: 加密 secrets 不泄露明文

- **WHEN** 用户运行 `pinax sync repo secret set --vault ./my-notes --name personal-sync-key --json`
- **THEN** Pinax SHALL 创建或更新加密 secrets asset
- **AND** 明文 SHALL NOT appear in stdout、stderr、日志、run receipt、vault content 或 generated runtime config
- **AND** 加密文件 SHALL have restrictive local permissions before being committed

#### Scenario: 无 bootstrap 身份时安全失败

- **WHEN** 新设备没有可用 keychain、age identity、secret manager identity 或显式一次性解锁输入
- **THEN** bootstrap SHALL fail with `sync_repo_unlock_required`
- **AND** SHALL NOT create a usable backend config with guessed or newly generated encryption key
- **AND** 输出 SHALL include a runnable recovery command

### Requirement: 本机运行配置编译

Pinax SHALL compile repository declarations and unlocked secrets into the existing local Capsa runtime state without overwriting device-owned state incorrectly.

#### Scenario: 新设备 bootstrap

- **WHEN** 用户运行 `pinax sync repo bootstrap --vault ./my-notes --device desktop-1 --json`
- **THEN** Pinax SHALL validate and decrypt repository configuration
- **AND** SHALL write the local `.pinax/cloud/config.yaml` through the application service
- **AND** SHALL set the requested unique device id
- **AND** SHALL preserve local sync receipts and daemon state only when they belong to the same workspace and device

#### Scenario: 配置漂移检测

- **WHEN** repository declaration differs from local generated runtime config
- **THEN** `pinax sync repo doctor --vault ./my-notes --json` SHALL report `sync_repo_config_drift`
- **AND** SHALL identify source and redacted field names
- **AND** SHALL NOT silently mutate local runtime state

### Requirement: 安全的 plan/apply 生命周期

Pinax SHALL separate repository sync configuration planning from applying changes.

#### Scenario: plan 无写入

- **WHEN** 用户运行 `pinax sync repo plan --vault ./my-notes --json`
- **THEN** Pinax SHALL report intended config, secret and device changes
- **AND** SHALL NOT modify repository files, local runtime config, remote objects or vault content

#### Scenario: apply 需要显式确认高风险变化

- **WHEN** apply would change workspace, backend namespace, encryption key identity or enable remote deletion
- **THEN** Pinax SHALL require `--yes` or an equivalent explicit approval
- **AND** SHALL write a redacted receipt with the changed field names
- **AND** SHALL keep the previous generated runtime config restorable

### Requirement: 应用和租户命名空间

Pinax SHALL provide stable declarative namespace fields while treating full multi-tenant authorization as a separate server capability.

#### Scenario: 应用使用独立 workspace

- **WHEN** a repository declares `tenant_id`, `app_id` and `workspace_id`
- **THEN** the effective remote namespace SHALL be deterministic and collision-resistant
- **AND** two applications SHALL NOT silently share a workspace unless explicitly allowed

#### Scenario: 多租户边界透明

- **WHEN** a direct S3 backend is used for multiple tenants
- **THEN** Pinax SHALL expose tenant/app/workspace facts in doctor and plan output
- **AND** SHALL state that direct transport does not provide server-side RBAC, quota, audit or tenant authorization
