## Why

历史上，新 Mac 即使已经 clone 私有 vault，仍必须手工创建 `~/.aws/credentials`，并从旧设备单独迁移 Capsa 内容解密密钥。这个流程既不符合“仓库自描述 + 一个 vault 口令完成 bootstrap”的目标，也容易把 COS 凭据、同步密钥和设备运行配置混为一谈。

Pinax 现已具备声明式同步配置、仓库密文 secrets、生产级 passphrase/macOS Keychain 解锁、typed S3 credential bundle、AWS SDK 内存凭据注入和 clone 后 pull-only bootstrap。运营者已报告一台 Mac 可以完成该工作流，但该正向观察尚不是可复核的发布支持证据。根级 `openspec/changes/portable-encrypted-project-bootstrap/` 已指定 `cli/credentialctl` 为通用 crypto/unlock owner；本变更只负责把该公共能力接入现有 `pinax sync repo`，同时保留 AWS shared profile 旧流程作为兼容与回滚路径。

## What Changes

- 接入 `credentialctl` 提供的 versioned project-secrets library/CLI，实现生产级 `passphrase-v1`、macOS Keychain、rekey 和 scoped resolution；Pinax 不复制 KDF、AEAD 或 Keychain 实现。
- 增加 typed S3/COS credential bundle，支持 `access_key_id`、`secret_access_key` 和可选 `session_token`，密文可随 Git 提交，明文不进入 CLI 参数、仓库、日志、receipt 或 generated runtime config。
- 扩展 direct S3 transport，使其可以把解锁后的 credential bundle 直接注入 AWS SDK credential provider，不要求生成或修改 `~/.aws/credentials`。
- 扩展 `pinax sync repo bootstrap`：支持安全 prompt/keychain 解锁、可选 `--pull`，新设备始终先生成 pull-only runtime，再拉取和解密远端笔记。
- 将 repository unlock 统一到 remote-aware `diff`、`pull`、`push` 和 daemon run：显式 flag 优先，其次使用仓库作用域 macOS Keychain；repository-encrypted 模式不得因为缺少 unlock source 而静默回退到本机 AWS profile。
- 为真实 S3 备份定义 durable commit gate：只有 manifest CAS commit 完成、返回新 revision 且 `remote_write=true` 才算备份成功；dry-run、blob upload、backend list 和本地 receipt 均不是完成证据。
- 在仓库声明中增加 additive capability gate，使只具备旧规划路径、会返回 `real remote writes are not wired yet` 的 Pinax 二进制在读取 repository-encrypted 配置时 fail closed，并给出升级命令，而不是生成成功外观的 projection。
- 增加 passphrase rekey、错误口令、密文篡改、credential 缺失、Keychain 不可用和旧 profile 回退的诊断与恢复合同。
- 为 `yeisme-notes` 提供从 device-local AWS profile 迁移到 repository-encrypted credential bundle 的 runbook；迁移不旋转远端内容加密 key、不删除远端 revision。
- 新增证据分级的 macOS 支持毕业门：将“某台 Mac 可用”记录为 observed，只有发布候选二进制、Keychain bootstrap、双向 COS round-trip、恢复场景和脱敏 evidence 均通过后，才可对实际验证的 `darwin/<arch>` 元组声明 Supported。
- 保留既有 `pinax capsa backend set s3 --profile ...` 和 AWS shared profile 行为；本变更不删除现有 flag、provider、schema 字段或运行路径。

## Capabilities

### New Capabilities

- `pinax-passphrase-s3-bootstrap`: 定义仓库口令解锁、Keychain 记忆、typed S3/COS credential bundle、SDK 内存注入、pull-only bootstrap、rekey 和恢复行为。

### Modified Capabilities

- `pinax-declarative-sync-config`: 声明式仓库配置可选择 repository-encrypted credential identity，并允许 bootstrap 使用生产 unlock provider 和可选 pull。
- `pinax-cloud-sync`: direct S3 transport 可以消费解锁后的临时 credential provider，同时保持端侧内容加密、revision CAS 和 pull-only 新设备语义。
- `pinax-profile-management`: 明确 device-local AWS profile 与 repository-encrypted credential 的解析优先级、兼容和回滚路径。

## Impact

- `github.com/yeisme/credentialctl/pkg/projectsecrets`: versioned project-secrets dependency；具体实现由 `cli/credentialctl/openspec/changes/credentialctl-project-secrets-envelope/` 所有。
- `internal/remote`: Pinax envelope compatibility adapter、typed credential bundle 和 scoped secret resolver，不拥有生产 KDF/AEAD/Keychain。
- `internal/app`: bootstrap/unlock 编排、S3 credential 生命周期、pull-only bootstrap、doctor 和迁移诊断。
- `internal/cli`: `sync repo credential`、`sync repo secret rekey`、bootstrap additive flags、`diff|pull|push` 统一 unlock flags、安全 stdin/TTY 输入和稳定输出合同。
- `internal/remote/s3_backend.go`: 显式 AWS SDK credentials provider 注入，同时保留 shared profile/default chain。
- `internal/app/sync_daemon.go`: 非交互 Keychain unlock、degraded state 和禁止无 credential 远端写入。
- `internal/redaction` 与 receipt：新增 credential、passphrase、Keychain 和 bootstrap facts 的集中脱敏。
- `tests/e2e`、`internal/remote`、`internal/app`: cryptographic tamper、CLI contract、跨设备 bootstrap、fake S3/COS transport 和 secret leakage tests。
- `internal/testkit`、`Taskfile.yml` 与 release/onboarding 文档：记录发布候选 provenance、macOS platform tuple、双向阶段结果和脱敏 evidence；不把一台设备的结果扩展为全部 macOS 架构支持。
- `docs/commands/sync.md`、`docs/commands/capsa.md`、release/onboarding 文档和 `yeisme-notes` migration runbook。
