# pinax-drivebridge-remote-notes Specification

## Purpose
TBD - created by archiving change pinax-drivebridge-remote-notes-v1. Update Purpose after archive.
## Requirements
### Requirement: 原存储入口必须保留并继续完善

Pinax SHALL 保持 `pinax storage set local`、`pinax storage set s3`、`pinax storage status`、`pinax storage doctor`、`pinax capsa backend set` 与 `pinax backend add` 为 canonical 入口，并继续作为领域写入与配置路径。DriveBridge attach SHALL 是加法。本包 SHALL NOT 删除、重命名或强制迁移这些命令。未 attach 时，这些命令 SHALL 与现网行为一致，且 SHALL NOT 要求已安装 DriveBridge。

#### Scenario: 未 attach 时 storage set s3 仍只走自己的 profile

- **WHEN** vault 已执行 `pinax storage set s3` 且从未 attach DriveBridge，本机也未安装 DriveBridge
- **THEN** `pinax storage status` 与 `pinax storage doctor` SHALL 按原 `pinax.storage.v1` 合同工作
- **AND** SHALL NOT 失败为 `drivebridge_not_installed`
- **AND** `pinax capsa backend set` 与 `pinax backend add` SHALL 仍可配置

#### Scenario: 禁止拆掉 owner 存储改成只认 DriveBridge

- **WHEN** 某实现提议删除 `pinax storage set s3` 或 `pinax capsa backend set`，改让用户只配置 DriveBridge 桶
- **THEN** 该变更为 breaking，本包拒绝
- **AND** 必须保留原命令并另增 attach

#### Scenario: 旧 Capsa rclone 仍可用

- **WHEN** 用户继续运行 `pinax capsa backend set rclone --remote onedrive:PinaxSync`
- **THEN** Capsa 密文同步 SHALL 按原合同工作
- **AND** SHALL NOT 要求先执行 DriveBridge bind 或 `bind-working-copy`

### Requirement: 优先零拷贝 attach 已有 local 根或 S3 前缀

Pinax 远程笔记第一支持路径 SHALL 是 `pinax storage attach-drivebridge --space <space>`：挂接 **已经存在的** `storage set local` 根目录或 `storage set s3` 的同一 bucket/prefix。Attach SHALL NOT 复制笔记字节，SHALL NOT 要求用户再创建一个 DriveBridge 桶，SHALL NOT 在 `init` 或日常 `note add` 时静默上传整个 vault。location 必须与当前 storage profile 一致，否则 SHALL 返回 `drivebridge_location_mismatch` 且不创建部分 attach 记录。

#### Scenario: attach 已有 S3 不是新网盘

- **WHEN** vault 已 `pinax storage set s3 --bucket notes --prefix pinax/` 且用户执行 `pinax storage attach-drivebridge --space pinax-vault`
- **THEN** Pinax SHALL 请求 DriveBridge adopt 同一 bucket/prefix
- **AND** `pinax storage status` SHALL 仍显示原 s3 profile
- **AND** SHALL NOT 要求用户再配一个 DriveBridge 桶

#### Scenario: attach 本地 vault 零拷贝

- **WHEN** vault 已 `pinax storage set local --root /abs/vault` 且 attach 同一根目录
- **THEN** DriveBridge 空间 SHALL 指向该目录
- **AND** 磁盘上 SHALL NOT 出现第二份笔记副本

#### Scenario: 位置不一致拒绝

- **WHEN** attach 声明的 space location 与当前 `pinax storage` 的 bucket、prefix 或 local root 不一致
- **THEN** Pinax SHALL 返回 `drivebridge_location_mismatch`
- **AND** SHALL NOT 写入 `.pinax/drivebridge-attach.yaml`
- **AND** 原 storage profile SHALL 不变

#### Scenario: DriveBridge 未安装时 attach 失败且不破坏原配置

- **WHEN** 用户在未安装 DriveBridge 的机器上运行 `pinax storage attach-drivebridge --space pinax-vault`
- **THEN** 命令 SHALL 失败为 `drivebridge_not_installed`
- **AND** `.pinax/storage.json` SHALL 保持原样

### Requirement: OneDrive 与 Google Drive 工作副本必须显式 opt-in

把可见工作副本绑到 OneDrive 或 Google Drive SHALL 使用 `pinax storage bind-working-copy --provider onedrive|gdrive --space <space>`。系统 SHALL NOT 在 `init`、`storage set` 或日常 `note add` 时静默把整个 vault 上传到新网盘。未带该 opt-in 时，试图把 OneDrive/GDrive 当作默认 attach 目标 SHALL 返回 `drivebridge_working_copy_opt_in_required`。

#### Scenario: 未绑定不上传

- **WHEN** 本地 vault 从未执行 `attach-drivebridge` 或 `bind-working-copy`
- **THEN** `pinax note add` SHALL 只写本地 vault
- **AND** DriveBridge 空间 SHALL 无新文件

#### Scenario: 拒绝把网盘当默认 attach

- **WHEN** 用户对尚未 `bind-working-copy` 的 vault 请求 attach 到 OneDrive 或 Google Drive 空间
- **THEN** Pinax SHALL 返回 `drivebridge_working_copy_opt_in_required`
- **AND** SHALL NOT 上传 vault 内容

#### Scenario: 显式 opt-in 后 doctor 声明明文

- **WHEN** vault 已 `pinax storage bind-working-copy --provider onedrive --space notes-copy`
- **THEN** `pinax storage doctor` 与 `pinax vault doctor` SHALL 报告 `drivebridge_content_mode=provider-plaintext`
- **AND** SHALL NOT 报告 Capsa encrypted 或 `opaque_encrypted`

### Requirement: DriveBridge 只提供文件面不得取代笔记核

Pinax SHALL 继续拥有笔记身份、Markdown 真源、proof loop、索引与 bounded projection。DriveBridge SHALL 只提供文件身份、版本、传输与一等后端连接。DriveBridge 工具或 CLI 列出的 `.md` SHALL NOT 被描述为 Pinax note card、detail 或 context，且 SHALL NOT 承诺完整正文。新增或修改笔记 SHALL 经 Pinax 命令/API（含 snapshot/proof 门）。

#### Scenario: DriveBridge 列出 Markdown 文件

- **WHEN** Agent 对已 attach 空间 `list` 到 `.md` 文件
- **THEN** 结果 SHALL 是 DriveBridge 文件引用
- **AND** SHALL NOT 作为 Pinax note 投影返回
- **AND** SHALL NOT 承诺包含完整正文

#### Scenario: 写入仍走 Pinax

- **WHEN** 用户或 Agent 要新增或修改笔记
- **THEN** 必须经 Pinax 命令/API（含 snapshot/proof 门）
- **AND** 只向 DriveBridge 上传 Markdown SHALL NOT 被报告为笔记已保存

### Requirement: DriveBridge 不得写入保护路径

Adopted 空间内，Pinax SHALL NOT 请求 DriveBridge 写入 `.pinax/**`、SQLite/WAL、token 文件或 Capsa 清单/密钥信封。若 DriveBridge 对此类路径返回 `owner_protected`，Pinax SHALL 原样表面该错误且本地保护文件不变。用户内容路径允许文件面 list/stat/transfer；领域对象的创建与语义变更 SHALL 仍经 Pinax 命令。Attach 记录 SHALL 由 Pinax CLI/application service 写入 `.pinax/drivebridge-attach.yaml`，schema 为 `pinax.drivebridge_attach.v1`。

#### Scenario: 拒绝改写 .pinax

- **WHEN** Agent 对已 attach 的 vault 空间请求经 DriveBridge 上传或删除 `.pinax/config.yaml`
- **THEN** 操作 SHALL 失败为 `owner_protected`
- **AND** 本地 `.pinax/config.yaml` SHALL 不变

#### Scenario: attach 记录由 CLI 写入

- **WHEN** `pinax storage attach-drivebridge --space pinax-vault --json` 成功
- **THEN** Pinax SHALL 通过 application service 写入 `.pinax/drivebridge-attach.yaml`
- **AND** 记录 SHALL 包含 `consumer=pinax`、`kind`、`space`、`location`、`purpose` 与幂等键
- **AND** stdout SHALL NOT 包含凭据明文

#### Scenario: detach 不影响 owner 存储

- **WHEN** 用户运行 `pinax storage detach-drivebridge --vault ./my-notes --json`
- **THEN** attach 记录 SHALL 被移除
- **AND** `pinax storage status` SHALL 仍成功并显示原 local 或 s3 profile
- **AND** 笔记数据 SHALL 仍在原目录或原桶

### Requirement: doctor 与 status 必须诚实区分模式

`pinax storage status`、`pinax storage doctor` 与 `pinax vault doctor` SHALL 用英文 facts 报告 DriveBridge 状态，且 SHALL 区分：未 attach、adopt 本地、adopt 同一 S3、明文网盘工作副本、以及 Capsa 密文前缀。Capsa 配置与 DriveBridge attach SHALL 分开展示。成功的 DriveBridge 传输 SHALL NOT 把 Capsa 标成已同步。Human chrome 可用中文；fact 名、错误码与 `drivebridge_content_mode` 枚举 SHALL 为英文。

#### Scenario: 未 attach 的 doctor 不假装已托管

- **WHEN** vault 仅配置 `storage set local` 且无 attach 记录
- **THEN** doctor/status SHALL 报告 `drivebridge_attached=false` 与 `drivebridge_content_mode=none`
- **AND** SHALL NOT 要求 DriveBridge 已安装

#### Scenario: adopt S3 与 storage profile 一致

- **WHEN** attach 的 bucket/prefix 与 `storage set s3` 一致
- **THEN** doctor SHALL 报告 `drivebridge_attached=true`、`drivebridge_kind=s3`、`drivebridge_content_mode=adopt_s3`、`drivebridge_location_match=true`
- **AND** 原 s3 profile facts SHALL 仍存在

#### Scenario: Capsa 密文前缀不得报成明文笔记

- **WHEN** 被 adopt 的 S3 前缀实际是 Capsa 加密 blob
- **THEN** doctor SHALL 报告 `drivebridge_content_mode=opaque_encrypted`
- **AND** Pinax SHALL NOT 把这些对象当笔记投影或明文工作副本

#### Scenario: 双路径 status 分开

- **WHEN** vault 同时配置了 Capsa s3-direct 与 DriveBridge `bind-working-copy`
- **THEN** 两条路径的 status SHALL 分开展示
- **AND** 一条成功 SHALL NOT 把另一条标成已同步

### Requirement: 换机 hydrate 钉住版本并重建索引

另一台已连接同一 DriveBridge 空间的机器 SHALL 能通过 `pinax storage hydrate --space <space>` 重建工作副本并刷新 Pinax 索引。hydrate SHALL 钉住观察到的文件版本（file id、version、sha256）。冲突与损坏 SHALL 交给 Pinax 既有冲突/repair 入口。DriveBridge SHALL NOT 自动合并正文。本机 local adopt 在目标机器看不见该目录时 SHALL 失败为 `drivebridge_local_unreachable`，不得把本机路径报告为远端可读文件。

#### Scenario: 第二台设备经同一 S3 前缀 hydrate

- **WHEN** 设备 B 对已 attach 的同一 S3 space/prefix 执行 `pinax storage hydrate --space pinax-vault` 且权限有效
- **THEN** 本地 SHALL 出现工作副本
- **AND** `pinax vault validate` 可通过，索引可 `pinax index refresh`
- **AND** 笔记 id 仍由 Pinax 管理
- **AND** 输出 SHALL NOT 因 hydrate 成功而设置 Capsa `remote_write=true`

#### Scenario: hydrate 遇到版本变化

- **WHEN** hydrate 中途远端文件相对钉住版本变化
- **THEN** 该文件 SHALL 失败为 `file_version_changed`
- **AND** Pinax SHALL NOT 静默用新字节覆盖未提交的本地编辑

#### Scenario: 本机 local adopt 不能在看不见磁盘的机器上 hydrate

- **WHEN** 设备 B 对 `kind=local` 的 attach 执行 hydrate，且该绝对目录在设备 B 上不可见
- **THEN** Pinax SHALL 返回 `drivebridge_local_unreachable`
- **AND** SHALL 提示改用已 attach 的 S3 或显式 `bind-working-copy`
- **AND** SHALL NOT 声称 MCP 已能读取客户端磁盘

### Requirement: 附件 DriveBridge 版本引用可被其他项目绑定

Pinax 附件与资产若已是 DriveBridge 文件版本，Scaena 或其他消费者 SHALL 能绑定同一 `drivebridge://` 引用。跨项目 SHALL NOT 因摘要相同而共用笔记身份或绕过各自授权。Pinax 笔记正文与 proof 记录 SHALL 仍只在 Pinax。

#### Scenario: Scaena 使用笔记附件引用

- **WHEN** 某附件已有 `drivebridge://` 版本引用且 Scaena 项目绑定覆盖该前缀
- **THEN** Scaena 可钉住该版本为材料引用
- **AND** Pinax 笔记正文与 proof 记录仍只在 Pinax
- **AND** 两边 SHALL NOT 共用 `note_id`

#### Scenario: 撤销绑定不收回已落地副本

- **WHEN** 用户 detach 或 DriveBridge 撤销该附件前缀绑定
- **THEN** 新的跨项目读取/导入 SHALL 被拒绝
- **AND** 已在 Pinax vault 内的附件文件 SHALL 仍按 Pinax 规则使用

