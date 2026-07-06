## Why

当前 `lark-doc` 发布路径已经能把 Pinax Markdown note 上传到飞书 Drive，并通过 mirror layout、vault template 和索引页改善云端结构。但真实验证表明，这条路径创建的是 Drive `file` 类型 Markdown，不是飞书原生 Docs/Docx 富文档：Mermaid、SVG、图片、表格等内容最多被保留为 Markdown 源码，无法保证在飞书端获得可阅读的原生渲染体验。

Pinax 的发布目标应该是“把本地 Markdown vault 作为真源，生成可协作、可阅读、可评论的外部文档副本”。对飞书来说，这意味着 `lark-doc` 的默认发布结果必须是飞书原生文档，而不是 `.md` 文件上传。否则用户看到的仍是源码文件，阅读体验和协作语义都不成立。

## What Changes

- 为 `lark-doc` 新增原生文档 renderer：`renderer=native-docx`，并保留现有 `renderer=markdown-file` 作为 legacy/fallback。
- 将 Markdown note 先解析为 Pinax publish AST，再转换为飞书原生文档块或中间 Docx 导入包，避免把 Markdown 源码直接上传为 Drive file。
- 对 Mermaid、SVG、相对图片和附件提供发布资产管线：可原生表达的内容转换为文档块；不可原生表达的内容预渲染为图片并插入原生文档。
- `lark-doc` 默认 profile 切换到 `renderer=native-docx`；已有 profile 若没有 renderer 字段，读取时按兼容规则推断并允许用户显式迁移。
- mapping、status、list 和输出 facts 增加 `renderer`、`external_object.type=docx|doc|file`、`render_warnings` 等 additive 字段，不删除旧字段。
- index page 也必须由原生文档 renderer 生成，不能继续只维护 `_Pinax Vault Index.md` Markdown file。
- 测试增加 fake `lark-cli` 原生文档导入/块更新脚本和真实飞书 smoke checklist，明确验收“飞书侧是原生文档并能渲染图文”。

## Non-Goals

- 不把飞书作为 Pinax vault 真源，不从飞书反向同步正文覆盖本地 Markdown。
- 不实现飞书评论、批注、权限、协作者、审批和通知流；这些仍由 agent 通过原生 `lark-cli` 操作。
- 不在 Pinax 内直接保存飞书 token、cookies、Authorization header 或 raw provider payload。
- 不要求首版支持所有 Markdown 扩展。首版必须覆盖标题、段落、列表、代码块、引用、表格、链接、图片、Mermaid 和 SVG fallback；其他语法必须产生明确 render warning。
- 不删除现有 Markdown file 发布能力；它作为 fallback/legacy renderer 保留至少一个发布周期。

## Capabilities

### Modified Capabilities

- `document-publish-maintenance`: 将 `lark-doc` 从“Drive Markdown file copy”演进为“飞书原生文档副本”，并定义 renderer、转换、资产、fallback、测试和兼容要求。

## Impact

- CLI/profile：`pinax publish doc profile set lark-doc` 新增可选 `--renderer native-docx|markdown-file`；默认新 profile 使用 `native-docx`。
- Domain：`PublishDocProfile`、`PublishDocPackage`、`PublishDocMapping`、`PublishDocReceipt` 增加 renderer/render result/render warning 等 additive 字段。
- App/render：新增 Markdown publish AST 和 Feishu native document render pipeline。
- Provider adapter：`lark-cli` adapter 新增原生文档 create/update/import 能力探测和调用；缺能力时返回稳定 `provider_capability_missing`，不静默回退。
- Assets：Mermaid/SVG/本地图片需要生成或上传发布资产，写入 package/receipt，不泄漏本地绝对路径和 token。
- Tests：新增 unit、contract、e2e fake CLI、真实 provider smoke checklist 和 integration evidence。
- Docs：更新 `docs/commands/publish.md`，说明 `lark-doc` 默认生成原生飞书 Docs/Docx，Markdown file 只是 fallback。

## Compatibility

本变更按演进策略做 additive 更新：保留 `lark-doc` target id、现有命令、mapping schema version v1 读取能力和 Markdown file renderer。新增 renderer 字段和输出 facts 不破坏旧消费者。若后续要移除 `markdown-file`，必须另开 OpenSpec，定义弃用窗口、迁移命令和 rollback。
