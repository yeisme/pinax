## MODIFIED Requirements

### Requirement: 端侧加密保护明文

Manifest 和 blob SHALL 使用 client-side encryption；明文 SHALL NOT 离开本地设备。加密密钥 SHALL 从 secret reference 解析出真实密钥值，SHALL NOT 使用引用字符串本身作为密钥材料。

#### Scenario: 加密 manifest 和 blob

- **WHEN** 客户端上传 manifest 或 blob
- **THEN** 数据 SHALL 使用端侧加密 envelope
- **AND** 后端或 direct object store SHALL 只看到 encrypted payload 和非敏感 revision metadata

#### Scenario: profile:// 引用解析为真实密钥

- **GIVEN** `secret_ref` 配置为 `profile://tencent-cos-pinax`
- **WHEN** Pinax 执行 Cloud Sync 加密操作
- **THEN** `DeriveKey` SHALL 通过 `ResolveSecretRef` 解析为真实的 `aws_secret_access_key`
- **AND** SHALL NOT 使用字面字符串 `profile://tencent-cos-pinax` 作为 PBKDF2 输入
- **AND** 如果 profile 不存在或无法解析，SHALL 返回错误

#### Scenario: 弱加密密钥警告

- **GIVEN** 用户运行 `pinax capsa backend set s3` 未指定 `--encryption-secret-ref`
- **AND** `secret_ref` 不是 `env://` 或 `keychain://` scheme
- **WHEN** Pinax 配置 Capsa Sync backend
- **THEN** projection SHALL 包含 `weak_encryption_key` 警告

### Requirement: Revision CAS 提交准入

Cloud push SHALL only report a remote write after a compare-and-swap revision commit succeeds. 当 `--yes` 已确认且 commit 因 `revision_conflict` 失败时，push SHALL 自动拉取远端修订、重建计划并重试一次。

#### Scenario: durable commit 后 remote_write=true

- **WHEN** transport atomically commits a new revision against the observed base revision
- **THEN** CLI MAY output `remote_write=true`

#### Scenario: Base revision 失配时自动 rebase

- **GIVEN** 用户已通过 `--yes` 确认同步写入
- **WHEN** commit 返回 `revision_conflict`
- **THEN** CLI SHALL 自动拉取远端修订，重建 push plan，并重试一次 commit
- **AND** 如果重建后的 plan 无冲突且 commit 成功，SHALL 输出 `remote_write=true`
- **AND** 如果重建后的 plan 有冲突，SHALL 返回 `conflict_required`

#### Scenario: 无 --yes 时不自动 rebase

- **GIVEN** 用户运行 `pinax sync push` 未带 `--yes`
- **WHEN** commit 返回 `revision_conflict`
- **THEN** CLI SHALL 直接返回 `revision_conflict` 错误

### Requirement: 本地后台实时同步进程

Pinax SHALL provide a managed local sync daemon. 后台 daemon 子进程 SHALL 继承父进程的环境变量。Pinax SHALL 提供 `daemon install`/`uninstall` 命令生成 OS 服务单元。

#### Scenario: 后台 daemon 继承环境变量

- **GIVEN** `PINAX_SYNC_SECRET` 设置在当前 shell 环境中
- **WHEN** the user runs `pinax sync daemon start --target capsa --vault <vault> --yes`
- **THEN** daemon 子进程 SHALL 继承 `PINAX_SYNC_SECRET` 及其他所有环境变量

#### Scenario: Linux systemd 服务安装

- **GIVEN** 用户在 Linux 上运行 `pinax sync daemon install --vault ./my-notes`
- **THEN** SHALL 在 `~/.config/systemd/user/capsa-sync-<slug>.service` 写入服务单元
- **AND** SHALL NOT 自动启动服务
- **AND** projection SHALL 包含 `systemctl --user enable --now capsa-sync-<slug>`

#### Scenario: macOS launchd 服务安装

- **GIVEN** 用户在 macOS 上运行 `pinax sync daemon install --vault ./my-notes`
- **THEN** SHALL 在 `~/Library/LaunchAgents/com.yeisme.capsa-sync.<slug>.plist` 写入 plist
- **AND** SHALL NOT 自动加载服务

#### Scenario: 卸载服务单元

- **WHEN** 用户运行 `pinax sync daemon uninstall --vault ./my-notes`
- **THEN** Pinax SHALL 删除服务单元文件
- **AND** SHALL NOT 停止正在运行的服务
