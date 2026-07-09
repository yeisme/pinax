## MODIFIED Requirements

### Requirement: 端侧加密保护明文

Manifest 和 blob SHALL 使用 client-side encryption；明文 SHALL NOT 离开本地设备的 explicit local content flows. 加密密钥 SHALL 从 secret reference 解析出真实密钥值，SHALL NOT 使用引用字符串本身作为密钥材料。

#### Scenario: 加密 manifest 和 blob

- **WHEN** 客户端上传 manifest 或 blob
- **THEN** 数据 SHALL 使用端侧加密 envelope
- **AND** 后端或 direct object store SHALL 只看到 encrypted payload 和非敏感 revision metadata

#### Scenario: 对象 key 和 metadata 不暴露路径

- **WHEN** direct transport 写入对象 key、metadata、revision 或 head
- **THEN** object key SHALL use protocol/layout ids such as `head.json`, `revisions/`, `manifests/sha256/`, and `blobs/sha256/`
- **AND** SHALL NOT contain plaintext note path, plaintext note body, raw token, Authorization header, Cookie, raw secret ref, provider stderr, or provider payload

#### Scenario: 脱敏验证

- **WHEN** 测试、stdout、stderr、event、receipt、fixture、backend log 或 object metadata 写入 Cloud Sync 相关数据
- **THEN** 不得暴露 plaintext note body、plaintext path in protected surfaces, raw token, Authorization header, Cookie, raw secret ref, provider stderr, or provider payload

#### Scenario: profile:// 引用解析为真实密钥

- **GIVEN** `secret_ref` 配置为 `profile://tencent-cos-pinax`
- **WHEN** Pinax 执行 Cloud Sync 加密操作
- **THEN** `DeriveKey` SHALL 通过 `ResolveSecretRef` 解析 `profile://` 引用为真实的 `aws_secret_access_key`
- **AND** SHALL NOT 使用字面字符串 `profile://tencent-cos-pinax` 作为 PBKDF2 输入
- **AND** 如果 profile 不存在或无法解析，SHALL 返回错误而非回退到弱密钥

#### Scenario: env:// 和 keychain:// 引用继续工作

- **GIVEN** `encryption_secret_ref` 配置为 `env://PINAX_SYNC_SECRET`
- **WHEN** Pinax 执行 Cloud Sync 加密操作
- **THEN** `DeriveKey` SHALL 从环境变量解析出真实密钥值
- **AND** SHALL NOT 在 projection、receipt 或 log 中暴露密钥值

#### Scenario: 弱加密密钥警告

- **GIVEN** 用户运行 `pinax capsa backend set s3` 未指定 `--encryption-secret-ref`
- **AND** `secret_ref` 不是 `env://` 或 `keychain://` scheme
- **WHEN** Pinax 配置 Capsa Sync backend
- **THEN** projection SHALL 包含 `weak_encryption_key` 警告
- **AND** 警告 SHALL 建议设置 `--encryption-secret-ref env://PINAX_SYNC_SECRET`

### Requirement: Revision CAS 提交准入

Cloud push SHALL only report a remote write after a compare-and-swap revision commit succeeds and local sync-state evidence is written. 当 `--yes` 已确认且 commit 因 `revision_conflict` 失败时，push SHALL 自动拉取远端修订、重建计划并重试一次。

#### Scenario: durable commit 后 remote_write=true

- **WHEN** missing encrypted blobs and encrypted manifest have been uploaded
- **AND** transport atomically commits a new revision against the observed base revision
- **THEN** CLI MAY output `remote_write=true`
- **AND** local sync-state/run evidence SHALL include backend kind, device/workspace ids, revision id, manifest id, status, and timestamp without leaking secrets

#### Scenario: plan/blob upload 不算 remote write

- **WHEN** CLI only generated a plan, performed dry-run, uploaded blobs, uploaded a manifest, or hit an unsupported transport path
- **THEN** CLI SHALL output `remote_write=false`
- **AND** SHALL NOT create a dummy revision or claim sync success

#### Scenario: Base revision 失配时自动 rebase

- **GIVEN** 客户端基于 base revision `rev_a` 提交 commit
- **AND** transport current revision is `rev_b`
- **AND** 用户已通过 `--yes` 确认同步写入
- **WHEN** transport 返回 `revision_conflict`
- **THEN** CLI SHALL 自动拉取 `rev_b`，重建 push plan，并重试一次 commit
- **AND** 如果重建后的 plan 无冲突且 commit 成功，SHALL 输出 `remote_write=true`
- **AND** 如果重建后的 plan 有冲突，SHALL 返回 `conflict_required` 并包含冲突处理 next actions

#### Scenario: 无 --yes 时不自动 rebase

- **GIVEN** 用户运行 `pinax sync push` 未带 `--yes`
- **WHEN** commit 返回 `revision_conflict`
- **THEN** CLI SHALL 直接返回 `revision_conflict` 错误
- **AND** SHALL 建议用户运行 `pinax sync --yes`（双向同步）或先 `pinax sync pull`

#### Scenario: 自动 rebase 后仍冲突

- **GIVEN** push 自动 rebase 重试一次后仍返回 `revision_conflict`
- **THEN** CLI SHALL 返回 `revision_conflict` 错误
- **AND** SHALL NOT 无限重试
- **AND** SHALL 建议用户手动检查冲突

### Requirement: 本地后台实时同步进程

Pinax SHALL provide an explicitly managed local sync daemon for a configured vault. The daemon SHALL reuse the existing Cloud Sync push/pull/conflict engine and SHALL NOT introduce a separate synchronization protocol or bypass existing approval, receipt, redaction, and `remote_write=true` rules. 后台 daemon 子进程 SHALL 继承父进程的环境变量。

#### Scenario: 前台运行 daemon 启动后立即同步

- **GIVEN** a vault has a configured Cloud Sync backend
- **WHEN** the user runs `pinax sync daemon run --target cloud --vault <vault> --yes`
- **THEN** Pinax SHALL start a local daemon runner for that vault
- **AND** it SHALL immediately execute one startup sync cycle before waiting for the next poll interval
- **AND** that cycle SHALL pull a newer remote revision before pushing local dirty content
- **AND** it SHALL persist redacted daemon events under `.pinax/sync-daemon/events.jsonl`.

#### Scenario: 机器输出保持稳定

- **WHEN** the user runs `pinax sync daemon run --target cloud --vault <vault> --yes --json`
- **THEN** stdout SHALL remain one final JSON envelope for `sync.daemon.run`
- **AND** intermediate progress SHALL NOT be mixed into JSON stdout.

#### Scenario: 后台 daemon 继承环境变量

- **GIVEN** `PINAX_SYNC_SECRET` 设置在当前 shell 环境中
- **WHEN** the user runs `pinax sync daemon start --target capsa --vault <vault> --yes`
- **THEN** daemon 子进程 SHALL 继承 `PINAX_SYNC_SECRET` 及其他所有环境变量
- **AND** 加密操作 SHALL 正常执行而非因缺少密钥而失败

### Requirement: OS 服务注册

Pinax SHALL provide `pinax sync daemon install` and `pinax sync daemon uninstall` commands to generate and remove platform-appropriate OS service units for the sync daemon.

#### Scenario: Linux systemd 用户服务

- **GIVEN** 用户在 Linux 上运行 `pinax sync daemon install --vault ./my-notes`
- **WHEN** Pinax 生成 systemd 用户服务单元
- **THEN** SHALL 在 `~/.config/systemd/user/pinax-sync-<vault-slug>.service` 写入服务单元文件
- **AND** 单元 SHALL 配置 `Restart=on-failure` 和 `RestartSec=10`
- **AND** SHALL NOT 自动启动服务
- **AND** projection SHALL 包含启用命令 `systemctl --user enable --now pinax-sync-<vault-slug>`

#### Scenario: macOS launchd plist

- **GIVEN** 用户在 macOS 上运行 `pinax sync daemon install --vault ./my-notes`
- **WHEN** Pinax 生成 launchd plist
- **THEN** SHALL 在 `~/Library/LaunchAgents/com.yeisme.pinax-sync.<vault-slug>.plist` 写入 plist 文件
- **AND** plist SHALL 配置 `RunAtLoad=true` 和 `KeepAlive` on unsuccessful exit
- **AND** SHALL NOT 自动加载服务
- **AND** projection SHALL 包含加载命令 `launchctl load ~/Library/LaunchAgents/...`

#### Scenario: Windows 暂不支持

- **GIVEN** 用户在 Windows 上运行 `pinax sync daemon install`
- **THEN** CLI SHALL 返回 `platform_unsupported`
- **AND** SHALL 建议 user 使用 Windows Task Scheduler

#### Scenario: 卸载服务单元

- **GIVEN** 用户之前通过 `install` 生成了服务单元
- **WHEN** 用户运行 `pinax sync daemon uninstall --vault ./my-notes`
- **THEN** Pinax SHALL 删除服务单元文件
- **AND** SHALL NOT 停止正在运行的服务（用户需手动 `systemctl stop` 或 `launchctl unload`）

#### Scenario: 不在单元文件中存储明文密钥

- **GIVEN** `PINAX_SYNC_SECRET` 通过 `env://` 引用
- **WHEN** Pinax 生成服务单元
- **THEN** SHALL NOT 在单元文件中写入明文密钥值
- **AND** SHALL 使用 `EnvironmentFile` (Linux) 或建议用户在 shell profile 中设置 (macOS)
