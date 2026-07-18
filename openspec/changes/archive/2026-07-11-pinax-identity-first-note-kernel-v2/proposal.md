## Why

Pinax 已具备项目分解、内容分类、Tag、SQL/Dataview、双链、Agent proof loop 和多端加密同步，但机器身份仍未完全脱离文件路径：缺失 `note_id` 时仍通过路径哈希补全，SQLite note 投影以 path 为主键，Cloud Sync 普通 manifest entry 也以 path 标识文件。这使重命名、移动、跨设备合并、链接稳定性和 Agent 审批都无法共享同一个可靠对象身份。

本变更以用户真实笔记工作流为核心，把 Pinax 固化为一个 UUID-first 的本地优先笔记软件内核：Markdown 保持可携带内容真源，Record Ledger 持有机器身份与生命周期，SQLite 提供可重建查询投影，Agent 通过受控服务操作对象，多端同步按对象身份和 revision 收敛。

## What Changes

- 为 vault、note、asset、project、folder、view、template 等受管对象定义统一 `object_id`，默认使用一次性分配并永久持久化的 UUIDv7；`path` 降级为可变 locator，不再承担身份职责。
- 将现有 `note_id` 明确为 note 对象的稳定 `object_id` 兼容字段；禁止业务流程通过 `stableNoteID(path)` 在生命周期中重新推导身份。
- 增加旧 vault 身份审计、迁移计划、snapshot、显式 apply、receipt 和 restore 约束；旧 `note_<sha1>` ID 在迁移窗口内可读取，但新对象不得继续生成路径派生身份。
- 将 Note/Tag/Link/Task/Asset 等 SQLite 投影迁移为 object-first 关系：对象 ID 是关联键，path 是唯一且可变的查询字段；索引重建不得改变对象身份。
- 将已解析双链固化为 `source_object_id -> target_object_id`，同时保留用户原始 `[[Title]]`、alias、heading 或 Markdown target，确保目标重命名和移动后关系仍可追踪并生成可审阅 rewrite plan。
- **BREAKING**：新增 Cloud Sync manifest v2，普通 entry 必须包含 `object_id`、`object_kind`、`current_path`、content revision 和 device facts；rename/move 表达为同一对象的位置变化，不再默认退化为 delete+create。
- 统一 sync 冲突分类：相同 `object_id` 的并发内容修改是 revision conflict，不同 `object_id` 占用相同 path 是 path collision，删除通过 `object_id + tombstone_id` 传播。
- 让 Agent、CLI、MCP 和 REST/RPC 共用对象解析、计划、审批、snapshot、apply、ledger event、sync revision 和 receipt 链路；Agent 不得直接改写 `.pinax/**` 或临时生成对象身份。
- 保持现有项目分解、分类、Tag、SQL/Dataview 和双链用户入口，不扩展新的内容维度、编辑器、发布平台或协作产品。

## Capabilities

### New Capabilities

- `vault-object-identity`: 定义 Pinax vault object 的 UUID 身份、路径定位、revision、device、迁移和兼容合同。

### Modified Capabilities

- `vault-record-ledger`: Record Ledger 必须一次性分配并持有对象身份，frontmatter 只作为可携带镜像，rename/move 不得改变 ID。
- `notebook-index-search`: 本地索引、Tag 和搜索关系改为 object-first，索引重建保持身份稳定并支持按 ID、path、项目和分类查询。
- `note-bidirectional-links`: 已解析双链必须以 source/target object ID 持久化，路径或标题变化不得破坏关系身份。
- `database-views-query`: SQL/Dataview 查询继续暴露 path/title/tag 等用户字段，同时提供稳定 object ID 并用对象关联键执行内部 join。
- `project-board-workspace`: project、subproject、board item 和来源 note 使用稳定对象身份，移动或重分类不得生成新的逻辑对象。
- `pinax-cloud-sync`: Cloud Sync 升级为 object-first manifest v2、对象级 rename/move、revision conflict、path collision 和 UUID tombstone 收敛。
- `pinax-agent-safe-proof-loop`: Agent 写入计划必须解析并锁定对象 ID，审批后写入 ledger 与 sync evidence，禁止 path-only 或隐式身份写入。

## Impact

- 主要影响 `internal/domain`、`internal/app`、`internal/records`、`internal/index`、`internal/cloudsync`、`internal/syncplan`、`internal/api`、`internal/mcp`、`internal/output` 和 `cmd/pinax`。
- 需要新增身份分配器、legacy ID 兼容映射、索引 schema 迁移、manifest v1/v2 读取边界、对象级冲突状态机和迁移测试夹具。
- `.pinax/records/**`、frontmatter `note_id`、SQLite 索引、sync manifest、tombstone、conflict、receipt 和 API projection 合同会受到影响；真实凭据、明文私密正文和 provider payload 不得进入迁移或测试证据。
- 现有 CLI 命令名称和项目/分类/Tag/SQL/Dataview/双链工作流保持兼容；任何结构化资产迁移必须由 Pinax CLI 或 application service 完成。
