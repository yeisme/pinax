# 人类阅读 API 与受监督本机会话

2026-10-01 · **已实现（P02–P07 交付，experimental）**：七个只读 reader HTTP 端点、typed client 与受监督本机会话已落地并通过 component 证据；旧 Agent/CLI/MCP/URI 合同不变。实施 owner 为 [pinax-human-reading-api-v1](../../openspec/changes/pinax-human-reading-api-v1/README.md)。DriveBridge 是首个消费者，不是笔记 owner；本文是新路由、类型、预算和生命周期的唯一详细合同，不是可直接调用的现有 API 清单。

## 1. 基线与不变量

现有 `note.read` 有 REST/RPC，`note.list` 有 RPC；`note.search`、`pinax.render_note` 不能据 CLI/MCP 标签认定已有 HTTP。现有 `render.view.v1alpha1` 保持 64 KiB/512 节点、card/context/body 及原脱敏规则。本包新增人类阅读合同，复用 application service、版本解析、关系和 asset 校验，不调大旧 Agent 树、不恢复 MCP Apps。

- Markdown vault 是正文真源；索引可重建，读取不能自动 refresh/rebuild、初始化 vault、snapshot、挂载、同步或写 proposal。
- body 读取必须有 owner 授予的 `reader.body` scope；metadata、relations、assets 分开授权。请求字段 `audience=human` 不构成授权，renderer 不得自选 principal/scopes。
- 人类正文可包含用户实际写入的代码、路径、术语，不套用旧 Agent SafeText 正则整段抹除；HTML 仍是不可信文本。凭据配置、`.pinax/**`、隐藏文件和任意宿主路径不成为正文资源。
- 消费者不得从私有数据库、vault 目录扫描或文件降级路径补齐缺失能力。API 不可用必须返回不可用，与成功空列表分开。
- 本包不定义收藏/进度、不执行写入审批，不改变 Hermes 已授权工具集。

## 2. 新公开读取面

以下路径已实现（P02–P06），前缀 `/v1/reader`，只允许 GET。成功沿用 `pinax.projection.v1`，具体 payload 放 data；字节 endpoint 除外。新 capability/请求/响应 schema 必须进入现有 transport catalog 与 `pkg/pinaxclient`；生成 OpenAPI 及消费者类型，不另手写 route registry。七个 binding 自 P06 起 available（backing=`internal/api/http:rest.reader.*`）；旧 `remote_command_unsupported` 保留。

| 路径 | capability / scope | 请求 | data / 行为 |
| --- | --- | --- | --- |
| `/source` | `reader.source` / `reader.metadata` | 无 | SourceDescriptor：session、binding、display_name、capabilities、limits、index 状态；无绝对路径 |
| `/notes` | `reader.notes` / `reader.metadata` | project_id/tag/status 可选，sort、limit、cursor | NoteCard[] 与 next_cursor、observed_at、index_watermark；默认 updated_desc，最后以 object_id 排序 |
| `/search` | `reader.search` / `reader.body` | q（trim 后 1–512 字符），同上筛选与分页 | 命中 NoteCard、至多 320 字符安全 HTML 转义片段；关键词检索，不启动模型；查询不进日志 |
| `/notes/{object_id}/document` | `reader.document` / `reader.body` | revision、cursor、anchor、outline_cursor 可选；cursor与anchor互斥 | DocumentPage；首请求可解析当前 revision，后续必须固定；目录和正文同版 |
| `/notes/{object_id}/links` | `reader.links` / `reader.relations` | revision 必填，direction=outgoing/backlinks，limit、cursor | LinkPage；固定源版和关系水位，目标重新授权；不返回不可见目标的名字/计数 |
| `/assets/{asset_ref}/content` | `reader.asset` / `reader.assets` | note_id、note_revision 必填；HTTP Range | owner 解析的附件字节；验证引用确属该笔记及该内容身份，206/416；无 bearer URL |
| `/notes/{object_id}/source` | `reader.source_export` / `reader.body` | revision 必填；Range 可选 | 显式人类下载原文；流式 UTF-8 Markdown，不加载进 renderer；不绕过当前权限 |

列表sort固定为 `updated_desc|created_desc|title_asc`，搜索固定 `relevance_desc`，同分以object_id升序；多tag为AND，project_id/status只接受owner现有合法值，未知值400。relations排序为source/target object_id+anchor，cursor绑定direction和水位。document请求携带cursor时revision必须与cursor一致；独立outline_cursor读取同版目录，不推进正文cursor。

source export 不是文件系统 API，不接受路径或输出目录。保存位置由桌面原生保存对话框交给 DriveBridge 本机服务；该 endpoint 只发送获授权字节。普通读取不自动下载。

### 类型与身份

| 类型 | 必需字段与约束 |
| --- | --- |
| SourceDescriptor | schema_version=`pinax.reader.source.v1alpha1`、session_id、binding_id、display_name、capabilities、limits、index_state、observed_at；binding_id 只来自受信父进程，不证明跨设备身份 |
| NoteCard | object_id、title、revision（sha256）、uri、status、project_id 可空、tags、updated_at；无全文、无绝对路径；title 不可用时有明确状态 |
| NoteList/SearchResults | items、next_cursor 可空、observed_at、index_watermark、index_state（status=ready/partial/stale/index_unavailable + watermark + generation）：索引新鲜度显式可读，消费方无需解码 opaque watermark 即可区分 fresh 与 stale；索引检查只读私有有界副本（main+WAL合计64MiB、两次稳定观察；不在源vault创建WAL/SHM，绝不迁移/回填/修复 owner 索引；活跃变更或超预算报告index_unavailable） |
| DocumentPage | schema_version=`pinax.reader.document.v1alpha1`、object_id、revision、current_revision、uri、nodes、outline、outline_cursor 可空、next_cursor 可空、complete、content_bytes、observed_at、warnings；complete 只在全文最后页且无未说明丢弃时为 true |
| DocumentNode | type、anchor、text/children、可选 ref 或 owner fence payload；标准 paragraph/heading/list/quote/code/table/image/link；未知 type 显示 unsupported，不执行插件 |
| LinkPage | object_id、revision、direction、items、next_cursor、relations_watermark、observed_at；关系投影水位不是正文 revision |
| AssetRef | opaque asset_ref、mime、size_bytes、digest、availability；只能经 owner 字节入口读取，不携带本机路径 |

`pinax://note/<id>?v=<sha256>` 不改格式。跨 vault 身份由 DriveBridge 的 source binding 包装；同 ID 不同绑定是不同目标，不能用标题、路径或 URI 猜测同一对象。

本包不要求对旧 vault 写新 UUID。binding 是 DriveBridge 本机实例登记的随机稳定 ID；Pinax 会话在已批准 canonical root 上运行，返回同一 binding_id 与新 session_id。root 移动/替换或 clone 要重新验证绑定，不把它们默认为同一 vault。云同步 workspace ID 不自动充当本地 identity。

### 分页、预算与渲染

- 列表/搜索/关系：默认 50、上限 200，limit 越界 400；cursor 为 opaque、绑定会话、scope、查询/排序、索引代次。过期 410，不能悄悄重复第一页。无索引为 `index_unavailable`，过期为 partial/stale 并带水位，不自动写索引。
- 文档：每页最多 256 KiB UTF-8 文本、512 节点（含嵌套节点），完整响应最多 1 MiB；可交互解析源文件上限 8 MiB、全篇解析节点上限 65536。达到任何预算都必须显式说明，不能以静默截断宣称完成。
- 页界按完整块切分；单个超长 code/paragraph/table 使用 `continuation` 节点，携带稳定 block_anchor、fragment_index、last_fragment、原 block type 与必要 table header/code language。UTF-8、转义实体和 inline token 不能从中间切断；回放不得重复块。长 inline token 可降为转义文本并返回 warning。
- outline 与正文锚点绑定 revision；目录单页上限 512 条，响应有独立 outline_cursor。anchor 请求定位同一 revision 的页，不在 renderer 扫全篇寻找。
- 跨笔记 `[[目标#^块ID|名称]]` 与 Markdown `target.md#%5E块ID` 使用 owner 的固定 revision＋anchor。N04.15 支持顶层段落末尾、前置空白的字面 `^ID`（1–64 ASCII 字母/数字/短横线）；不移除源标记、不新增节点类型。重复/缺失 ID 不退回全文；代码、链接标签、HTML、标题、列表/引用嵌套、转义和其他复杂表示不猜测。生成 anchor 不是块 ID 的跨版本身份，必须与原 revision 配对；原版不可恢复时返回409。目标解析沿标题引用共用32目标/累计8MiB/65536节点预算，不执行 fence。N04.16 同时支持同笔记简写 `[[#标题]]`、`[[#^块ID]]`、Markdown `#fragment` 及显式自身目标＋片段，始终使用正在展示的 source revision；读取历史 snapshot 时不拿当前 notes 条目的锚点替代。两层嵌入与 DriveBridge 消费另验。
- source 超 8 MiB 或解析超预算返回 413 `document_too_large` / `document_budget_exceeded`，错误details带固定revision、source_size_bytes与可用source-export action；显式导出每个Range最大1MiB。附件每个Range最大8MiB；无Range仅在总大小不超过对应上限时返回200，否则416并提供总大小，消费者分段流式保存。不以返回半篇代替全文，不支持格式受权下载，不进在线转换服务。
- document 首次请求冻结可解析的 content revision；后续页重新校验同版。外部编辑后，如原版可从现有 snapshot 读取则继续原版并提示 current_revision；否则 409 `revision_unavailable`，用户选择重开当前版，不能混页或偷偷创建 snapshot。
- 原始 HTML 为转义文本；GFM 表格/任务列表支持只读显示；wiki/ref/asset/fence 语义由 Pinax resolver 产生。fence 仅既有注册的只读 owner handler，有独立预算与错误，不加载执行代码。remote 图片不自动请求。

## 独立富文档读取（N04.24，experimental）

`GET /v1/reader/notes/{object_id}/rich-document` 已有真实REST binding，要求当前受监督session的 `reader.body`，必填 `revision=sha256:<64hex>`；可选cursor/anchor/outline_cursor，cursor与anchor互斥。query闭合，object_id仅在路径，重复/未知/坏UTF8/坏百分号及超过8KiB query拒绝。响应no-store/nosniff，data为 `pinax.reader.rich_document.v1alpha1` wrapper，含原document与annotations，旧document端点不改。

annotations只提供惰性mermaid/math fence源文本、字面Callout的kind/title/fold，以及原anchor和本页真实page_anchors。source省略等同空字符串。结构跨页可重复相同revision/anchor的Callout上下文，消费者幂等合并，不能把每页重复当多个新块。每篇≤512注释、单payload64KiB/总256KiB、原分页256KiB/512节点、完整响应≤1MiB；超预算413不返回截断成功。rich不执行任何fence handler；源文本中的URL、HTML或命令不是授权，图形消费者须独立做安全渲染。

rich游标与旧document/outline隔离，绑定object/binding/session/scopes/revision及可见来源水位。引用目标或可见集合变化使旧cursor拒绝，不混新布局；可显式重新打开可恢复的原snapshot，不能自动换head。普通system/daily权限不扩大。N04.25 已提供 `Client.ReaderRichDocument(ctx, ReaderDocumentRequest)` 和typed wrapper/annotation。SDK在原有界transport上进一步封顶1MiB，校固定object/revision/URI、closed schema、节点/注释唯一、page_anchors本页存在、字节预算和非执行语义；不会把source渲染成HTML，也不接受fence/payload。真实多页和snapshot回读、撤权、错误响应正负控制证据为 `temp/integration-test-runs/20261006T125920Z-3177609/`。N04.26 增加完整顶层段落的 `$$…$$` 展示公式（可在同一段落内跨行），source仅剥除外层定界符，保留TeX原bytes/CRLF/空白，旧document文本不改。代码/链接/HTML、残缺/多组定界符、嵌套quote/list、空行跨段不猜测，金额不升级；仍按math annotation预算且不执行TeX。真实HTTP/SDK证据 `temp/integration-test-runs/20261006T130821Z-3620441/`。两层嵌入、行内公式、空行跨段公式以及DriveBridge图形显示仍未完成。

证据 `temp/integration-test-runs/20261006T124931Z-2687333/`：真实两页注释、固定revision、闭query、metadata403/撤权401、旧cursor隔离、wire转义膨胀413。OpenAPI当前 `sha256:2c46c46db61f3a586a312eb1a79d4e06ca5b38ba0175745046c0fa01f0727ac1`，删除新path恢复rich schema基线，再删三个schema恢复旧摘要；reader12/writer14。

## 组合文档与两层嵌入合同（N04.29，HTTP experimental）

本节是独立 `composed-document` 读取面，不是现有rich端点的新字段。登记五个封闭schema：`pinax.reader.composed_document.request.v1alpha1`、`pinax.reader.embed_target.v1alpha1`、`pinax.reader.embed.v1alpha1`、`pinax.reader.node_origin.v1alpha1`、`pinax.reader.composed_document.v1alpha1`。N04.29已提供真实HTTP handler与available `reader.composed_document` / `rest.reader.composed_document`，readonly、body explicit；N04.30提供typed `Client.ReaderComposedDocument(ctx, ReaderDocumentRequest)`。

请求固定object_id/revision，位置字段沿cursor/anchor/outline_cursor；不允许caller注入depth、vault、session或执行开关。独立GET `/v1/reader/notes/{object_id}/composed-document`，仍需当前reader.body，普通系统页/日记准入不扩大。响应wrapper引用完整原rich_document（身份代表根笔记，节点是本次组合后的投影），另有embeds和origins。旧rich schema、注释枚举、端点、SDK保持原样。

- `embeds[]`：每个嵌入出现一次独立embed_id（不是目标ID，重复引用同对象仍不同），parent_embed_id为空表示直接属于根笔记，source_anchor是父来源版本的原候选锚点。page_anchors为本页实际呈现的占位或后代节点锚点；跨页必要上下文可重复，按binding/root revision/embed_id幂等合并。嵌套项在本页出现时必须同时携带其祖先描述，不能依赖客户端保存上页隐含来源。
- depth=1或2可为expanded/cycle，必须携带target `{object_id, revision, anchor?}`；anchor省略代表全文，提供时必须是目标固定版的owner锚点，不是标题字符串。cycle仅在当前可见且固定身份已确定后标记，按当前祖先对象身份检测，不把重复非循环兄弟引用当循环。depth=3只能是depth_limit，禁止携带target，且不读取第三层目标；第三层描述属于第二层正文中的停止标记，不代表第三层展开。
- unavailable把不存在/不可见/歧义合并，unsupported仅用于可见源语法本身无法支持的选择；两者无target/标题/path/raw，不能通过状态泄漏目标存在性。不能将缺失fragment退回目标全文。附件仍走原附件合同，不把asset_ref解释为笔记身份。
- `origins[]`为本页每一个实际node（含children/continuation）提供唯一映射 `{page_anchor, embed_id, source_anchor}`。embed_id为空为根来源，否则必须对应本页expanded描述；source_anchor指原来源版本的block锚点，fragment映射回原块。占位节点归父来源；重映射后的页面anchor只用于当前组合页，不是可写身份。图形注释继续仅为惰性source，并映射实际页面anchor。
- 最多32个嵌入出现次数（含循环/失败/depth-limit描述），深度上限2；根与展开目标合计8MiB输入、65536节点，重复插入的节点按次数计费。单原候选raw2KiB，整页256KiB文字/512节点、origins≤512、完整projection≤1MiB；所有rich注释仍受原预算。超预算整次错误，不用缺项伪complete；complete仅说明分页结束，不表示每个嵌入都expanded。
- 首次解析目标沿当前可见notes和notelinks owner resolver；目标固定revision必须出现在target。根snapshot不自动构成所有目标的历史快照。后续cursor必须绑定根revision、当前session/scopes/binding、可见corpus水位及组合布局域；目标或可见集变化拒绝旧cursor，不把新head拼进旧分页。不自动创建snapshot/index/写入或执行fence。

Schema只限制形状/枚举/单数组上限，**不证明**完整origin覆盖、父子无环、页锚点归属、权限、UTF8字节累计或source解析语义；这些必须由后续app与SDK验证。N03.8合同通过；N04.28已实现独立application `ReaderComposedDocument` 的两层展开、标题/块fragment、cycle/depth_limit、来源映射、原rich分页与独立cursor。使用当前可见corpus的有界snapshot，第三层不会进一步parse/expand，但不能宣称navigation扫描完全未读取该笔记。应用证据 `temp/integration-test-runs/20261006T154347Z-3083571/`，包含长段落跨页重组与root旧snapshot的循环回指；HTTP现已通过 `20261006T155245Z-3546838`：真实两页祖先来源、嵌入math/depth标记、metadata403/撤权401、闭query400/POST405、目标变源拒旧cursor、旧rich域隔离与JSON膨胀413；响应private,no-store/nosniff，query≤8KiB，成功完整projection≤1MiB。SDK已通过 `20261006T160225Z-4091157`：复用最大1MiB且尊重更小配置的transport、固定root与rich预算验证，增加origin全node唯一覆盖、parent expanded/depth、循环必须指祖先同revision、页锚点只可归占位或展开后代、祖先必须声明其后代页节点；unknown或depth-limit禁止target。真实HTTP多页含cycle/depth/math、变源/旧游标域/撤权拒，以及有效fixture和恶意来源响应反例通过。SDK不独立证明磁盘源内容或授予asset/图形执行权限；DriveBridge消费仍未完成。schema-only阶段OpenAPI摘要为 `sha256:62cbaf566fdcf01e798d2e32f730a8249bf42c4cfaecf09b4d13fd78f55dfd78`，删除这五项必须精确恢复 `sha256:2c46c46db61f3a586a312eb1a79d4e06ca5b38ba0175745046c0fa01f0727ac1`。当前新增GET后OpenAPI摘要为 `sha256:79d729770fdac930042e83eec7ab77414b3c37313a0b8960d6347a4dcfc6b52c`，删除该path精确恢复上述schema-only摘要，再删五schema恢复旧rich；reader13/writer14。

## 2.1 笔记导航增量（2026-10-05）

实施 owner 为 [pinax-notebook-consumer-v1](../../openspec/changes/pinax-notebook-consumer-v1/)。以下三个只读 endpoint 已在 service/HTTP/typed client 接通；旧七个 reader 的 schema 与语义不变，source descriptor 的 capabilities 仍是 scope 列表，具体新增动作从 manifest/catalog 发现。

| GET 路径 | 必需 scope | 请求 | 返回与限制 |
| --- | --- | --- | --- |
| `/tags` | `reader.metadata` | limit 默认50/max200、cursor | 规范 tag path、稳定语义 ID、parent_id、virtual、direct_count、descendant_count；水位、total_tags、coverage |
| `/graph` | `reader.metadata` 与 `reader.relations` | mode=source/local；local 的 object_id/revision 必填、hops=1/2；limit=2–200 | 每页最多200节点/400边；两端节点齐全；水位、next_cursor、total_nodes/total_edges、coverage |
| `/journals/daily` | `reader.metadata` | date=YYYY-MM-DD、timezone=IANA | found/missing、固定 note 身份或null；按本地日历日及 owner 模板路径定位，绝不自动创建 |

标签 ID 是规范路径的版本化 SHA-256，不使用 TagRecord 自增行号。大小写保持 owner 规范；改名/移动产生新路径 ID，后续写计划须提供映射。必须与 source/binding 一起引用，不把相同路径跨 vault 当同一标签。计数先按 reader 可见集合过滤，再按 note ID 去重；后代计数排除本节点，但同一笔记多个后代只计一次。虚拟祖先没有直接引用，不是新建的标签对象。

图只投影既有解析成功关系，局部1/2跳使用双向邻接，不推断新关系。分页按边流排序并将孤立节点随后输出，跨页可以重复节点，消费方以 binding+object_id 合并；不会漏掉跨页边。`coverage.page_complete` 只表示页序列完结，`corpus_complete=false` 表示超大正文或不支持数据导致投影不完整。未知或不可见目标的名称、关系和计数不输出。新 cursor 绑定会话、scope、binding、规范请求与内容/路径水位；400/410 不混页。

日记只读检查已存在的 journal.daily 模板 metadata，支持自定义 output path 与 legacy notes/daily 路径；不执行模板查询，不调用 DailyShow/ensureJournalNote/index refresh。未初始化的空目录不产生 `.pinax` 或日记。日期不是 UTC 时间点；标准库随包 tzdata 支持离线 IANA 时区。隐藏/链接日记与缺失同形，重复匹配拒绝。日记首建已另行提供受监督propose/apply，详见[writer合同](human-reading-write.md)；正文读取/编辑的系统页兼容、模板改路径后的旧对象重定位仍是后续任务，不能因为找到身份或可以首建就宣称完整日记工作流已交付。

当前证据：`temp/integration-test-runs/20261005T061039Z-2950026/`，包含真实 httptest、可丢弃 vault、206节点星图和576边密集图分页、8MiB超限覆盖、权限/撤销/隐藏/链接、无索引/只读副作用、日期/时区、typed client 安全拒绝。该导航切片基线OpenAPI digest `sha256:4c1e027e39896451700f55821d7351fcc4ebb3eb8508d62ad2c2f0dce3cb4f4b`；当前加入daily写入后为 `sha256:82603700…`，删除daily写入增量恢复导航基线，再删除三个导航path/schema精确恢复此前 `sha256:e37403bb…`。此为 Pinax 组件证据，不是 DriveBridge/Electron 或三平台验收。

验证入口：`go run ./tools/testkit/integrationevidence --profile notebook-navigation`。

### 日记正文增量（2026-10-06）

`GET /v1/reader/journals/daily/document`（capability `reader.daily.document`）要求 `reader.metadata` 与 `reader.body`。必填 `date`、`timezone`、`object_id`、`revision`，可选 `cursor`、`anchor`、`outline_cursor`；query 闭合、禁止重复参数，cursor 与 anchor 互斥。先通过 daily locator 核对日期与对象，再按固定当前 revision 读取，返回 `data.document`，复用 `pinax.reader.document.v1alpha1`。旧普通 document 路由的系统页准入不扩大。SDK 使用 `ReaderDailyDocument`。

每页仍为256KiB/512节点，响应1MiB；旧日记 revision 改变返回409，不自动恢复快照或改读新头。超预算错误不提供普通 source-export 作为日记旁路。所有响应 no-store，每页重验身份/权限，metadata-only不能取得正文。此接口不提供编辑、不执行脚本、不改变source export或资产端点准入。模板与文件路径改变后，有原成功daily ledger可通过当前可见object ID定位；没有历史证据的任意旧CLI路径不猜日期。

证据 `20261006T011744Z-2026636`：真实 typed client/HTTP/可丢弃vault分页、两个scope分别拒绝、重复/未知/坏转义query、撤权、外部改版拒绝、无初始化写入。OpenAPI现 `sha256:43c4c9cf…`，删除一个新path与一个request schema精确恢复上一版 `sha256:82603700…`。日记编辑与跨会话operation恢复仍待，DriveBridge授权与实机集成另验。

## 3. 授权、缓存与失败

reader scope 只由 owner 显式启用。新增本地 human-reader 会话与既有 Agent token、temp-token、MCP collaboration/body/write 开关隔离；旧身份不能靠请求字段升级。能力发现同时取 implemented binding ∩ session scopes ∩ 资源可见性。正文、片段和资产不自动进入审计或搜索日志。

新 reader 响应 `Cache-Control: private, no-store`；不继承旧 `/v1/notes/` 的 30 秒策略，不使用浏览器磁盘缓存。桌面首片只在当前阅读会话保留有界内存数据；logout/binding/session 变化清空。附件 bytes 在范围/权限复验后按流读取，MIME 嗅探不升级可执行内容，`nosniff`；asset-ref 不是持久授权。

| HTTP / code | 含义与恢复 |
| --- | --- |
| 400 `reader_request_invalid` / `reader_cursor_invalid` | 非法 limit、query、revision 或篡改 cursor；修参数，不猜另一 vault |
| 401 既有 token 错误 | 停止读取、清会话正文，重新建立获授权会话 |
| 403 `insufficient_scope` / `reader_disabled` | 有效身份缺 reader 权限；不降级到旧 read/file 接口 |
| 404 `reader_object_not_found` | 不存在与不可见对象同形状；不泄漏标题、候选数 |
| 409 `revision_unavailable` / `source_changed` | 原版不可达或 root 改变；固定旧位置提示，显式重新读取/绑定 |
| 410 `reader_cursor_expired` | 会话、scope、水位过期；重新查询，清选择，不从下一页拼接 |
| 413 `document_too_large` / `document_budget_exceeded` | 显式原文下载；不标 complete |
| 416 `reader_range_invalid` | 越界或超字节预算；客户端修正 Range |
| 422 `reader_unsupported` | 协议或资源格式不支持；安全禁用 |
| 503 `index_unavailable` / `reader_unavailable` | 索引/owner 不可用，与 empty 分开；无自动修复写入 |

旧 endpoint 的错误与缓存不借本包改义。scope/对象校验在每次读取和 Range 继续时执行；不得用缓存名/索引计数泄漏不可见笔记。

## 4. 受监督本机会话（已实现）

新增 `pinax api serve` 的 opt-in `--session-stdin` 模式（P05 已实现，不改旧 flags/default）；stdout 事件 schema 为 `pinax.local_session.event.v1alpha1`。

1. DriveBridge只启动已验证随包binary，固定readonly、loopback、随机port 0；不接caller的executable/argv/env。绑定预检只授reader.metadata，不读取正文；人类确认scope与guard完成后以新bearer创建body/relations/assets会话，不把预检token原地升权。Pinax依据owner侧bootstrap grants裁决，不信任HTTP中的audience声明。
2. 私有 stdin 首帧是版本化 `pinax.local_session.bootstrap.v1alpha1`（最多 16 KiB，10 秒内）：session_nonce、256-bit 随机 bearer、binding_id、已选择的 canonical vault root、allowlisted reader scopes。secret 不在 argv/env/文件/日志；root 只在受信宿主与服务边界内使用。未知字段/版本/非法 root 均拒绝，不允许 bootstrap 改变监听主机或启用 mutation。
3. Pinax 打开已有 vault并验证权限；不会 init 或迁移。stdout 发脱敏 `ready`（session_id、session_nonce、binding_id、loopback 地址、contract digest、readonly=true；nonce只作握手相关标识，不是凭据）及 error/shutdown，不返回 bearer、root、正文；stderr 也不输出临时 token。宿主最长等待 15 秒后显示失败并回收自有子进程。
4. stdin 保持打开作为 liveness，首帧后不再接命令。EOF 停止接新请求，最多 5 秒排空并关闭自身连接；Pinax 复用既有 SQLite DSN/CloseAll，不删除强杀后 WAL。
5. 当前会话 bearer 只在内存，退出失效。握手校验 nonce 对应的会话身份、地址确为 loopback，禁止重定向。会话不能注册系统服务、监听 LAN、公网或接收远程 reader。
6. 普通 CLI、Hermes MCP 与此会话可并存：不抢 owner 写锁、不杀外部进程。索引 busy 是有界失败，不触发第二个 writer；测试覆盖正常并发但禁止 race。

Go宿主复用 `github.com/yeisme/pinax/pkg/localsession` 的 `Bootstrap`、`Event`；原API类型是该公开类型的alias，不复制wire结构。`DecodeReady`接受有界原始stdout帧，严格核对nonce/binding/预期manifest digest/readonly、拒重复或大小写变体字段/null/未知字段，地址仅接受当前owner固定的 `http://127.0.0.1:<port>`。它不负责spawn/权限授予，也不把error消息传给renderer。预期digest来自随包版本的公开 `pinax api manifest --json` 导出；不是OpenAPI digest，不能拿ready自身digest作为预期值。此包不进入Web类型产物。

绑定所需 root 验证由宿主原生选择、DriveBridge 自有 roots/mount 登记和 Pinax 只读来源校验共同完成，不向 public reader 输出路径列表。无法证明 overlay/content roots 全覆盖时 DriveBridge 必须拒绝该绑定；不读取 `.pinax` 来猜。

## 5. 演进、生成与回退

- 新路由、新 schema、新 opt-in 会话均 additive；旧 render.view、NoteDisplay、URI、CLI/RPC/MCP 命名和 flags 保留。
- 机器 schema/SDK/TS/fixture 由 Pinax transport catalog 生成；消费者只能引用固定版本/摘要，不手工维护第二份 DTO。新增机器产物的可用性由真实实现和 conformance 决定。
- 关闭 reader capability 或撤销会话即可回退，保留原 Markdown 与现有 API。没有真实数据迁移、token 轮转或废弃发布。本包不修改普通 temp-token 模式的既有日志行为；新的 supervised 模式必须独立满足零秘密日志要求，旧日志治理另行处理。
- 详细任务在本 owner P01–P09；契约、应用/API、生命周期各自 focused 验证，再做 Pinax component。DriveBridge UI 与三平台整机验收不在 Pinax 勾选。

## 6. 验证入口与机器合同资产（P01 已登记）

文档校验与合同漂移门：

```bash
openspec validate pinax-human-reading-api-v1 --strict --no-interactive
git diff --check
go test ./internal/transportcatalog/... -run "TestReaderContract|TestOpenAPI" -count=1
```

P01 已把本合同冻结为 owner 机器合同并登记全部证据 profile；P02–P06 已实现全部七个端点、typed client 与受监督会话，P07 通过 Pinax component 全链证据：

- 机器 schema：`pinax.reader.source|note_card|note_list|search_snippet|search_results|outline_entry|document_node|document|link_item|link_page|asset_ref|source_export.v1alpha1`、七个 `pinax.reader.*.request.v1alpha1` 与 `pinax.local_session.bootstrap.v1alpha1`，全部在 `internal/transportcatalog/schema/reader.go` 注册进默认 registry，闭对象、正负 fixture 由 `TestReaderContract` 钉住；预算/长度/枚举以 const 冻结。
- transport catalog 条目：`reader.source|notes|search|document|links|asset|source_export` 七个 experimental 只读 capability 与 `rest.reader.*` 七个 **available** GET binding（backing=`internal/api/http:rest.reader.*`，readiness ready/exploratory），经 `internal/transportmanifest.Compile` 进入全部 manifest 面；`available_surfaces=[rest]`，OpenAPI 生成 `/v1/reader/*` 七条只读路径。
- 证据 profile（`tools/testkit/integrationevidence`）：`human-reading-api|human-reading-access|human-reading-document|human-reading-resources|human-reading-session|human-reading-client|human-reading-quality`，各 lane 全部已跑通（见 tasks.md 各任务 evidence）；quality 走 `task check`（无 race 阶段，已静态核对）。
- 生成/漂移命令：`pinax api manifest --json`（reader binding 随 manifest digest 变化）、`pinax api schema export --format openapi --vault <vault> --json`（OpenAPI digest 含 reader paths/components）与 `go test ./internal/transportcatalog/... -run "TestReaderContract|TestOpenAPI" -count=1` 合同门。

后续复用现有 Go testing/httptest、testscript、`tools/testkit/integrationevidence`。P02–P06就绪后P07运行 `human-reading-api`，P08才进入 `human-reading-quality` 稳定质量门；不新增平行框架。纯渲染/校验函数的定向单元可用Go testing；HTTP+app/repository、SDK+服务或完整进程即为integration/component，不能因为放在internal目录就算unit。任何component/e2e须由现有evidence runner包装，六件套落在本项目 `temp/integration-test-runs/<run-id>/`，测试正文仅合成 fixture。所有错误保留原退出码，无 race、真实模型、真实同步或用户 vault。

## 7. DriveBridge 消费交接摘要（P08）

- **schema 引用**：消费者只引用固定版本 schema——payload `pinax.reader.{source,note_list,search_results,document,link_page,asset_ref,source_export}.v1alpha1`、请求 `pinax.reader.*.request.v1alpha1`、会话引导 `pinax.local_session.bootstrap.v1alpha1` 与 stdout 事件 `pinax.local_session.event.v1alpha1`；全部由 transport catalog 生成，不手写第二套 DTO/TS。
- **固定 digest**：本切片 transport manifest `sha256:77272867fa7a801b34482dee5917937200b58a67ee49bfdb50a84e0eded4bde8`（`pinax api manifest --json` 的 `digest`）；OpenAPI `sha256:07458efaaffb4473eee6eb2582b61948584510f3e9b10a044dc83d21f54811ed`（`pinax api schema export --format openapi --vault <vault> --json` 的 `schema_digest`）。manifest 中 7 个 reader capability 均为 `available_surfaces=["rest"]`、`availability=available`、readiness `ready/exploratory`。
- **已验**（Pinax 本机、合成 fixture 证据）：七端点只读全链（source/notes/search/document/links/asset/source-export）；256KiB/512节点页按 UTF-8 字节计（含嵌套子树）、8MiB/65536 总预算与 320 字符片段；continuation 按精确字节偏移续读、页中长块字节级无损重组、超页块显式 warning 且不宣称 complete；固定 revision：变更后的 pinned revision 从既有 snapshot 续读（current_revision 提示 live 内容），不可读才 409 revision_unavailable；cursor 绑定 session/查询/内容指纹水位——同数量内容变更亦过期（400/410），outline cursor 签名绑定 session+revision；`private, no-store`+nosniff+Range 206/416（asset 8MiB、export 1MiB 窗口），asset 读取以打开描述符全量 SHA256 校验内容身份（同长度替换/检查后符号链接替换均拒绝）；受监督会话单帧引导/ready/shutdown/EOF 等待排空完成后才发 shutdown（超时强制关闭自有连接）/10s 首帧时限；旧 token 不升级、reader bearer 不进旧面；CLI/MCP 同 vault 并发；pkg/pinaxclient typed calls（大小上限、拒绝 redirect、未知版本安全失败、无 local fallback、显式 Range 窗口可在 1MiB/8MiB 上限之上分窗读取并在首窗 Content-Range 发现总大小，字节端点描述符携带资源总量与 ETag 内容 digest）。索引门只读（noteindex.InspectReadOnly：mode=ro 打开，无迁移/AutoMigrate/身份回填/任何持久化写入；异常索引报告 stale/partial 而非被修复，读取前后索引文件字节不变）；notes/search 响应与 typed 结果显式携带 index_state（fresh→ready、漂移→stale、投影不一致→partial 均成功返回带水位，绝不转为错误）；只读检查一次性释放连接池与文件句柄（长会话翻页/搜索句柄有界）。
- **未验**（不冒称）：DriveBridge UI/三平台整机、真实长时间自然阅读、云同步 workspace 当本地身份、任何写入门/收藏/进度。上述均不在本 change 勾选范围。（snapshot 续读已实现并测试：显式 pinned revision 与分页 cursor 均从既有 snapshot 继续；本面从不创建 snapshot。）
