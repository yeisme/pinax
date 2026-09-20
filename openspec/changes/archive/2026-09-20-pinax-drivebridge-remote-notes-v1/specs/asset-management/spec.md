## ADDED Requirements

### Requirement: 附件可登记 DriveBridge 文件版本引用

Pinax 附件与资产 MAY 在 CLI-authored asset metadata 中登记已钉住的 DriveBridge 文件版本引用（含 file id、version 与 SHA-256）。该引用 SHALL NOT 取代 vault 内可迁移文件作为附件真源，SHALL NOT 把 DriveBridge list 结果当作 Pinax note 投影。需要字节时 SHALL 再经 Pinax 既有 attach/asset 流程落地。

#### Scenario: 登记已钉住的附件引用

- **WHEN** 某附件文件已有 `drivebridge://` 版本引用且用户经 Pinax 命令登记该附件
- **THEN** asset metadata SHALL 保存 file id、version 与 sha256
- **AND** stdout SHALL NOT 包含附件 payload 字节
- **AND** 笔记身份 SHALL 仍由 Pinax `note_id` 管理

#### Scenario: 源文件被替换后旧引用失效

- **WHEN** 网盘或对象存储侧同路径文件内容变化，已钉住 version/sha256 不再匹配
- **THEN** 消费该引用 SHALL 失败为 `file_version_changed`
- **AND** 已落地的 vault 附件副本 SHALL 保持不变

### Requirement: 跨项目绑定同一附件引用不得共用笔记身份

Pinax 附件若已是 DriveBridge 文件版本，Scaena 或其他消费者 SHALL 能绑定同一引用。跨项目 SHALL NOT 因内容摘要相同而共用 Pinax `note_id` 或绕过各自授权。Pinax 笔记正文与 proof 记录 SHALL 仍只在 Pinax。

#### Scenario: Scaena 钉住笔记附件为材料引用

- **WHEN** 某附件已有 `drivebridge://` 版本引用且 Scaena 项目绑定覆盖该前缀
- **THEN** Scaena 可钉住该版本为材料引用
- **AND** Pinax 笔记正文与 proof 记录仍只在 Pinax
- **AND** Scaena SHALL NOT 获得该笔记的 `note_id` 作为自己的领域身份

#### Scenario: 撤销后新绑定失败

- **WHEN** Pinax detach 或 DriveBridge 撤销覆盖该附件前缀的绑定
- **THEN** 其他项目新的 list/stat/import SHALL 被拒绝
- **AND** 已在 Pinax vault 内的附件文件 SHALL 仍按 Pinax asset 规则使用
