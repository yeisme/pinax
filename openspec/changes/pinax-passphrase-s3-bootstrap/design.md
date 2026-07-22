## Context

Pinax 当前已经具备三层声明式同步模型：仓库声明 `.pinax/pinax-sync.yaml`、仓库密文 `.pinax/pinax-sync.secrets.yaml` 和本机运行态 `.pinax/cloud/`。第一版 unlock provider 只有 `fake` 与 `env`；direct S3 transport 通过 AWS SDK default credential chain 或 shared profile 获取凭据。因此，仓库虽然能够描述 bucket、endpoint、workspace 和 logical identity，却还不能让普通用户在新 Mac 上只凭 vault 口令完成 bootstrap。

根级 `openspec/changes/portable-encrypted-project-bootstrap/` 规定通用 envelope、passphrase/Keychain、rekey 和 cross-language execution boundary 由 `cli/credentialctl` 所有。Pinax 是第一 canary，只实现 Pinax-specific secret schema、Capsa runtime compiler、S3 adapter injection、pull-only bootstrap、doctor 和 evidence；不得复制公共 crypto 或 Keychain adapter。

本变更不复制整个 `~/.aws/credentials` 文件，也不把 AWS 配置文件作为普通 vault 内容同步。它把 S3/COS credential 作为 typed secret bundle 加密后提交，由 Pinax 在命令边界解锁并直接注入 AWS SDK。这样既保留“类似 Ansible Vault 的密文随 Git”体验，也避免路径、profile 合并、重复 section 和明文 materialize 问题。

## Product Scope

### Target User

- 已有私有 Git vault 和 S3/COS Capsa remote，希望在新 Mac 上低门槛恢复笔记的单用户。
- 需要在多台个人设备之间共享同一 vault，但不希望逐台手工维护 `~/.aws/credentials` 的开发者。

### Job To Be Done

用户 clone 私有 vault 后，输入一次独立 vault 解锁口令，即可验证仓库密文、生成本机 pull-only runtime、连接 S3/COS、拉取并解密笔记；整个过程不把真实 credential 或 passphrase 写入 Git、CLI 输出和运行证据。

### MVP Goals

- 支持 `passphrase-v1` 和 macOS Keychain 两种生产 unlock 路径。
- 支持 repository-encrypted S3/COS static credential bundle。
- 支持新设备 `bootstrap --pull`，默认 pull-only，禁止首次 remote delete/push。
- 支持 password rekey，不旋转 Capsa 内容加密 key。
- 保留现有 AWS profile 模式，并提供可逆迁移。

### Non-Goals

- 不提交或自动合并完整 `~/.aws/config`、`~/.aws/credentials`。
- 不在本变更实现 AWS STS/OIDC 登录、云 KMS、组织级 RBAC、多人密钥共享或服务端 secret escrow。
- 不自动轮换 COS SecretId/SecretKey，也不自动轮换 Capsa 内容加密 key。
- 不让 daemon 在没有 Keychain/secret manager 的情况下弹出交互式口令提示。
- 不改变远端 object layout、encrypted manifest、blob envelope 或 revision CAS 协议。

## User Workflow And State Transitions

```mermaid
stateDiagram-v2
    [*] --> Cloned: git clone
    Cloned --> Locked: declaration + ciphertext present
    Locked --> Unlocking: bootstrap reads TTY/keychain
    Unlocking --> Locked: wrong password / tamper / missing credential
    Unlocking --> RuntimeReady: validate + compile pull-only runtime
    RuntimeReady --> Pulling: --pull confirmed
    Pulling --> Ready: remote revision decrypted and applied
    Pulling --> RuntimeReady: transport or key mismatch failure
    Ready --> Rekeying: secret rekey requested
    Rekeying --> Ready: envelope replaced atomically
    Ready --> ProfileFallback: operator selects device profile rollback
    ProfileFallback --> Ready: old AWS profile path active
```

推荐的新设备命令：

```bash
git clone https://github.com/yeisme/yeisme-notes.git yeisme-notes
cd yeisme-notes
pinax sync repo bootstrap \
  --device "$(scutil --get LocalHostName)" \
  --unlock prompt \
  --remember-keychain \
  --pull \
  --yes \
  --json
```

`--unlock prompt` 从 controlling TTY 读取且关闭 echo；prompt 不进入 stdout。`--json` stdout 始终只包含一个 JSON envelope。无 TTY 时命令返回 `sync_repo_unlock_required`，并提示使用 Keychain ref 或 passphrase file，而不是从 stdin 猜测输入来源。

## Decisions

### 1. 复用现有 secrets asset，而不是提交 AWS 文件

继续使用 `.pinax/pinax-sync.secrets.yaml`。`SecretEntry` 增加可选 `kind`、`format` 和 `version` metadata；已有 entry 缺少这些字段时继续按既有 string secret 解析。

S3 credential bundle 的解密后逻辑结构为：

```json
{
  "access_key_id": "<redacted>",
  "secret_access_key": "<redacted>",
  "session_token": "<optional-redacted>"
}
```

整个 JSON payload 作为单个 secret entry 加密。结构校验只报告缺失字段名和 credential kind，不回显值。声明层只保存 `credential_id` 和 credential mode，不保存 credential value。

### 2. Passphrase envelope 由 credentialctl 提供

Pinax 通过 versioned `credentialctl/pkg/projectsecrets` API 读取、写入、解锁和 rekey repository envelope。Argon2id、XChaCha20-Poly1305、随机 DEK、associated data、minimum KDF threshold 和 tamper error taxonomy 由 credentialctl change 定义并测试。

Pinax 只传入稳定的 `project_id=pinax`、repository id、entry name、identity、kind、format 和 capability grant，并把公共 resolver 的 redacted result 映射到 Pinax projection。错误口令、metadata 损坏、cross-repository copy 或 authentication failure 均由公共 owner fail-closed；Pinax 不捕获后自行生成替代 key。

### 3. Keychain 是 credentialctl unlock source，不是仓库资产

在 macOS 上，`--remember-keychain` 调用 credentialctl Keychain adapter，将 unlock secret 写入 repository-scoped account。仓库只保存非敏感 keychain account hint/digest，不保存 Keychain value。

- Keychain 写入必须由用户显式批准。
- `doctor` 只报告 `configured=true|false`、service、account digest 和可访问性，不显示值。
- daemon 只能使用已配置 Keychain/secret manager；不得打开 TTY prompt。
- Keychain 不可用时，交互式命令可以回退到 prompt；非交互命令安全失败。

### 4. S3 credential 直接注入 AWS SDK

`S3BackendOptions` 增加可选 credentials provider。repository-encrypted mode 下，应用服务在单次命令或 sync run 开始时解锁 bundle，构造 AWS SDK `StaticCredentialsProvider`，并把 provider 传给 `config.LoadDefaultConfig`。

明文 credential 的生命周期被限制在不可变 runtime snapshot 和 AWS SDK provider 中：

- 不设置全局 `AWS_ACCESS_KEY_ID` 环境变量。
- 不生成 `~/.aws/credentials`。
- 不写入 `.pinax/cloud/config.yaml`、stored secret YAML、receipt 或 daemon log。
- run 结束后释放引用；Go 无法保证即时物理清零，因此不得创建额外字符串副本或 debug dump。

### 5. Credential mode 与兼容优先级

声明增加可选 `credential_mode`，值为：

- `device-profile`：默认值，保持现有行为，通过 endpoint/profile 或 AWS default chain 解析。
- `repository-encrypted`：必须从声明引用的 encrypted credential bundle 解析；缺失或解锁失败时不得静默回退到本机其他 AWS credential。

显式选择 `repository-encrypted` 后，profile query 仅作为非敏感 provider hint，不作为隐式 credential fallback。这样可以避免新 Mac 恰好使用错误 AWS account。

旧 `pinax capsa backend set s3 --profile ...`、现有 config 和 `fake`/`env` provider 全部保持工作。迁移采用 expand-then-contract，不重命名或删除旧字段。

### 6. Bootstrap 保持 pull-only 和原子性

`bootstrap --pull` 分成四个提交点：

1. 只读加载并验证 declaration、secrets envelope、Git ignore 和 protected paths。
2. 解锁 secrets，验证 encryption key identity 与 credential bundle。
3. 原子生成本机 runtime config/source marker，标记 `new_device_mode=pull-only`。
4. 在用户提供 `--yes` 时执行 pull；只有远端 revision 解密并成功应用后才写成功 receipt。

任何阶段失败都不得 push、remote delete、替换远端 head 或生成新的内容加密 key。runtime 已生成但 pull 失败时，状态保持可诊断的 `runtime_ready`，用户可以修复网络后重试 pull。

### 7. 安全输入与 CLI 兼容

新增安全输入面：

```bash
pinax sync repo credential set s3 \
  --name tencent-cos-pinax \
  --provider passphrase-v1 \
  --stdin \
  --vault ./yeisme-notes \
  --json
```

stdin payload 使用严格 JSON object，仅接受 allowlisted fields；交互模式可以分别 prompt `SecretId`、`SecretKey` 和 vault passphrase。现有 `sync repo secret set --value` 保留兼容，但 human help 和文档不再推荐；从首个实现版本开始输出一次脱敏 deprecation warning，至少保留两个 minor release，最早不早于 `v0.4.0` 移除。

新增和扩展的 CLI surface 均为 additive：

- `pinax sync repo credential set|list|remove`
- `pinax sync repo secret rekey`
- `pinax sync repo bootstrap --unlock --unlock-ref --passphrase-file --remember-keychain --pull`

现有 command、flag、JSON envelope 和 `--agent` key 不删除、不重命名。新 facts 必须是 optional，并保持 `spec_version=1.0`，除非输出合同评审要求 minor bump。

## Trace, Audit And Evidence

每次 bootstrap/rekey/credential mutation 生成 redacted projection 和本机 receipt，允许记录：

- command、status、error code、provider、credential kind、workspace、device、repository digest。
- `unlock_source=prompt|keychain|file|env`，但不记录 ref value 或文件内容。
- `credential_source=repository-encrypted|device-profile`。
- `pull_only=true`、planned/applied file counts、remote revision id、`remote_write=false`。
- envelope schema/KDF profile id 和 key id digest，不记录 salt、wrapped key、ciphertext 或 plaintext。

所有 integration/component/e2e 入口必须把脱敏证据写入 `temp/integration-test-runs/<run-id>/`，至少包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json` 和 `artifacts/`，失败时保留原 exit code。

## Compatibility, Migration And Rollback

### Contract Classification

| Surface | Classification | Policy |
| --- | --- | --- |
| CLI commands/flags | Additive | 保留现有命令与 flag；新 facts optional。 |
| secrets asset | Additive v1 metadata | 旧 `fake`/`env` envelope 继续可读；新 provider 需要支持该 capability 的 Pinax 版本。 |
| sync declaration | Additive optional key | `credential_mode` 默认 `device-profile`。 |
| credentialctl library/CLI | New experimental dependency | Pinax 声明支持的 project-secrets contract range；不复制实现。 |
| S3 adapter API | Internal additive | options 增加 provider，不改变既有 constructor。 |
| AWS profile workflow | Unchanged | 继续作为兼容和 rollback path。 |

### Migration

1. 在已有设备读取当前 S3 profile 和 Capsa encryption key reference，但不输出明文。
2. 使用安全 stdin/prompt 写入 repository-encrypted S3 credential bundle，并用同一 repository unlock provider 加密既有内容加密 key。
3. 将 declaration 的 `credential_mode` 切换为 `repository-encrypted`，先运行 plan/doctor。
4. 在第二台 fixture/Mac 上 clone，执行 `bootstrap --pull`，验证笔记数量、revision 和 key identity。
5. 第二设备验证成功后才提交并推广新的 onboarding；旧 AWS profile 暂不删除。

### Deprecation Window

`--value` 至少保留两个 minor release，最早 `v0.4.0` 才能移除；移除前必须有独立 OpenSpec、consumer inventory 和 release note。本 change 不移除它。

### Rollback

- 将 declaration 的 `credential_mode` 恢复为 `device-profile`。
- 使用现有 `pinax capsa backend set s3 --profile <name> ...` 重新生成本机 runtime config。
- 保留原 Capsa encryption key，继续读取同一远端 revision；不回滚或删除远端对象。
- 仓库密文 bundle 可以保留但不再引用，或通过 Pinax CLI remove 后提交；不得手工编辑 envelope。

## Risks And Open Decisions

- Argon2id 参数在低内存机器上的可用性需要 benchmark；参数可以写入 envelope，但最低安全阈值不能被仓库配置降级。
- Keychain CLI adapter 需要处理无 GUI session、锁定 keychain 和 codesign 差异；失败必须可区分为“不可用”而非“credential 错误”。
- Git 历史会永久保留旧 ciphertext；rekey 后旧 passphrase 仍可能解开旧 commit，因此高风险 credential rotation 必须同时撤销旧 COS key。
- 单用户 shared passphrase 不等于多人授权。多人/设备撤销、每设备 recipient 和硬件密钥属于后续 age/X25519 capability。
- `yeisme-notes` 迁移必须在真实 COS 上做第二设备 restore evidence，但真实 secret 不得进入 evidence bundle。
