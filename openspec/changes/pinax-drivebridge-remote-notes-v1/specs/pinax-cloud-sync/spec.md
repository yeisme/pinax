## ADDED Requirements

### Requirement: DriveBridge attach 不得充当 Capsa transport

Pinax Cloud Sync（Capsa）、Remote API Mode 与 DriveBridge 工作副本绑定 SHALL 作为不同模式报告。DriveBridge attach 或 `bind-working-copy` SHALL NOT 被文档、help、status 或输出称为 Capsa transport，除非后续独立 change 明确把 DriveBridge 空间用作 Capsa blob 存放处。Capsa `pinax capsa backend set` 与 `pinax sync push|pull` SHALL 保持原义。

#### Scenario: Capsa push 与 DriveBridge 上传分开

- **WHEN** vault 同时配置了 Capsa s3-direct 与 DriveBridge visible-working-copy
- **THEN** 两条路径的 status SHALL 分开展示
- **AND** 一条成功 SHALL NOT 把另一条标成已同步
- **AND** DriveBridge 成功 SHALL NOT 被描述为 Capsa revision commit

#### Scenario: Capsa rclone OneDrive 不是明文工作副本

- **WHEN** 用户使用 `pinax capsa backend set rclone --remote onedrive:PinaxSync`
- **THEN** Pinax SHALL 仍按 Capsa 密文协议同步
- **AND** doctor SHALL NOT 因此报告 `drivebridge_content_mode=provider-plaintext`

### Requirement: Capsa remote_write 不得因 DriveBridge 成功而发出

Capsa `remote_write=true` SHALL 仅在既有耐久 revision commit 与本地 sync-state receipt 之后发出。DriveBridge attach、hydrate、list、stat 或文件上传成功 SHALL NOT 使任何 Capsa/sync 投影发出 `remote_write=true`。

#### Scenario: DriveBridge hydrate 不抬升 Capsa remote_write

- **WHEN** 用户在已 attach 的 vault 上成功执行 `pinax storage hydrate`
- **THEN** 该命令的 projection SHALL NOT 设置 Capsa `remote_write=true`
- **AND** 既有 Capsa sync-state SHALL 不变

#### Scenario: DriveBridge 上传成功不是 Capsa commit

- **WHEN** DriveBridge 将某 Markdown 文件传输到已 attach 空间成功
- **THEN** `pinax sync status` SHALL NOT 报告新的 Capsa revision
- **AND** SHALL NOT 发出 `remote_write=true`
