## Why

DriveBridge 全站复刻与真实 Pinax 接入已获批准。现有 reader document 是安全阅读投影，source-export 是导出合同，两者都不能替代无损编辑源码、标签树、关系图、日记定位和原操作恢复。必须在 Pinax owner 增量补合同，避免客户端反拼 AST 或直写 vault。

根级来源：`../../../../openspec/changes/drivebridge-pinax-reading-integration-v1/`；消费方既有计划为 `../drivebridge/openspec/changes/drivebridge-desktop-knowledge-reader-v1/`、`../drivebridge/openspec/changes/drivebridge-personal-workspace-rebuild-v1/`。本次用户确认的范围包括全部 Web 页面、Electron 宿主、真实 Pinax 读写与可丢弃测试 vault，不访问个人 vault。

## What Changes

- 新增 `GET /v1/writer/notes/{object_id}/editor-source`、封闭 schema、OpenAPI、typed client 和真实服务验证。要求 `writer.editor` 与 `reader.body`，以 object ID 与固定 revision 读取原始正文；保护 frontmatter，沿用旧提案／应用／回执写入门。
- 后续增量仍须完成 reader tags、reader graph、daily 定位及首次创建提案、标签计划／应用、rich-document 和受监督 operation 查询／恢复，不因首个接口完成而删除或降级这些需求。
- 复用现有 runner，为当前切片注册 `notebook-editor-source` profile；所有测试仅使用合成 vault，不运行 race。
- 不改旧 reader/writer/CLI/MCP 行为，不把 Electron、UI、DriveBridge 本机 guard 或缓存归入 Pinax。

## Capabilities

### New Capabilities

- `notebook-editor-source`：人类编辑会话的固定 revision、无损正文和受保护 metadata 投影。
- `notebook-navigation`：标签层级、来源／局部关系图、日期与 IANA 时区日记定位的只读合同。
- `notebook-daily-write`：日记首建提案、人确认、同日期无覆盖发布和原operation恢复。
- 其余写入、增强节点与恢复合同仍保留在 N03/N04；各接口完成规范和冻结前不得宣称已实现。

### Modified Capabilities

无破坏性更改。既有 reader/writer 合同保持原义。

## Impact

涉及 `internal/transportcatalog`、`internal/app`、`internal/api`、`pkg/pinaxclient`、`tools/testkit/integrationevidence`。不新增依赖和持久化 schema；只读请求不初始化 vault、不刷新索引、不创建快照。DriveBridge／Electron 消费仍为 `INTEGRATION_PENDING`。
