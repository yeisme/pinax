## Why

Pinax 已经具备本地 Markdown vault、索引、静态发布和同步能力，但还缺少一条轻量的“文档发布维护”路径：把一篇或一组 Pinax note 发布到 Notion 页面或飞书 Docs，并在后续本地内容变化时判断外部文档是否过期、应该新建、更新、重新链接或解除链接。

这个能力不是站点发布、不是评论同步、不是飞书/Notion 的完整协作客户端。Pinax 只维护本地 note 与外部文档之间的发布关系、内容摘要、发布收据和安全状态；评论、批注、协作者权限、页面内块级微调等平台原生操作由 agent 根据 skill 直接调用原生 CLI，例如 Notion CLI 或 `lark-cli`。

## What Changes

- 新增 `document-publish-maintenance` 能力，作为 `pinax publish doc ...` 命令族的合同。
- 初期支持两个文档发布目标：`notion-page` 和 `lark-doc`。
- 新增发布 profile、provider doctor、prepare、push、status、list、link、unlink 命令合同。
- 新增 provider adapter 边界：Pinax 优先调用外部 CLI adapter，不默认直接嵌入 Notion 或飞书 SDK。
- 新增 publish package、mapping、receipt、external status 和 stale 检测模型。
- 机器输出遵守 Pinax CLI output contract：`--json` 单 envelope、`--agent` 稳定 key=value、`--events` NDJSON、`--explain` 脱敏说明。
- 集成测试使用 fake Notion/lark executables、fixture vault 和临时目录，不依赖真实 token、真实公网或用户 vault。

## Non-Goals

- 不实现 Notion comment、飞书评论、批注、协作者权限、审批流或通知流。
- 不把外部文档作为 vault 真源，不从 Notion/飞书反向同步正文覆盖本地 note。
- 不实现完整 Notion database、飞书 Wiki、飞书表格、飞书多维表格或知识库空间管理。
- 不复用 Hugo/GitHub Pages static publish 的主题、站点、搜索索引和 deploy 逻辑。
- 不让 agent 手写 `.pinax/**` mapping、receipt、event log 或 provider profile。
- 不在首版实现真实 provider SDK；若外部 CLI 能力不足，先通过 adapter error 和 OpenSpec 后续变更处理。

## Capabilities

### New Capabilities

- `document-publish-maintenance`: 定义 Pinax note 到外部文档目标的 prepare、push、status、link/unlink、mapping、receipt、provider adapter 和 agent handoff 行为。

### Modified Capabilities

- 无。现有 `static-site-publishing` 保持站点/Markdown bundle 发布语义，不承担 Notion/飞书文档维护。

## Impact

- CLI：新增 `pinax publish doc ...` 命令族，并保持 `pinax publish` 现有静态发布命令兼容。
- App service：新增 document publish use cases，命令层只做参数校验和输出模式选择。
- Domain：新增 document publish profile、target、package、mapping、receipt、provider health、external status、stale reason 等模型。
- Provider adapter：新增 CLI-backed adapter 接口，首版通过 fake executable 完成测试，真实接入分别映射到 Notion CLI 和 `lark-cli`。
- Output：新增 command projection，默认中文摘要，机器协议字段保持英文。
- Redaction：所有 provider stdout/stderr、raw payload、token、Authorization/Cookie、外部 CLI 参数和错误必须脱敏后才能进入输出、receipt、events 和测试证据。
- Tests：新增 unit、command、testscript e2e、fake executable、contract tests 和 integration evidence 输出要求。
