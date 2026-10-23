## Why

Pinax 已经拥有正文、笔记身份、revision、提案／应用、附件输入和 Capsa 同步执行器，但 DriveBridge 仍缺少一份可生成、可验证的 writer 与 sync 控制合同。若客户端自行读取 `.pinax/**`、SQLite 或本机路径，就会复制 Pinax 状态并破坏人工确认、revision guard 和未知 mutation 恢复语义。

本变更把 Pinax 的公开控制面补齐，并让 DriveBridge 只消费 typed contract。Electron 是本阶段唯一允许进行本机 vault 纵切的入口；Web 只做共享组件、合同和 fixture 验收。真实 endpoint、跨设备收敛、平台签名和主力 vault dogfood 继续保持外部门槛。

## What Changes

- 增加 sync status/diff/push/pull/conflicts list/diff/resolve 的 REST、RPC、transport catalog、OpenAPI 和 typed client 合同，复用现有 Capsa application service。
- 增加 DriveBridge 可生成的 Pinax writer/sync owner contract artifact，不让 renderer 访问私有 vault 状态。
- 规定 rich/composed 文档源码优先：预览带 source anchor、origin 和固定 revision，编辑回到原 Markdown source 并经过 proposal → 人工确认 → apply → receipt。
- 规定附件沿用 bounded input request、MIME/大小限制、固定 note identity、revision guard、快照和 receipt；renderer 不传路径、不写 vault。
- 为 DriveBridge 工作台提供 bound → reading → draft → preview → applying → succeeded/conflict/unknown/revoked 状态机和 sync 只读状态面；写入按钮必须经过宿主确认和 owner contract。
- 记录 D14.1/D14.2 的合同与共享组件证据；D03/D08、D14.3、真实 Cloud Sync 和三平台验收不在本变更中伪装完成。

## Capabilities

### New Capabilities

- `sync-control`：公开同步状态、计划、执行和冲突恢复合同，保持 revision、remote_write 和 reconcile_required 语义。
- `notebook-workbench`：源码优先的编辑／预览／附件边界和 DriveBridge 消费者状态机。

### Modified Capabilities

- `writer`：仅补充 DriveBridge 可消费的公开 schema 选择，不改变既有 writer change 枚举、提案／应用或 operation receipt 语义。
- `operation-recovery`：公开 sync mutation 使用既有 run receipt/log 和 operation recovery 入口，原 operation identity 已绑定 receipt；没有确定证据时保持 reconcile_required，不重放 mutation。

## Impact

涉及 `internal/app`、`internal/api`、`pkg/pinaxclient`、OpenAPI/transport catalog、DriveBridge contract generator、desktop IPC catalog、Web shared notebook components 和测试。无新增数据库 schema，不读取或迁移 `.pinax/**`，不执行 race，不访问个人 vault 或真实云端凭据。
