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
| NoteList/SearchResults | items、next_cursor 可空、observed_at、index_watermark、index_state（status=ready/partial/stale/index_unavailable + watermark + generation）：索引新鲜度显式可读，消费方无需解码 opaque watermark 即可区分 fresh 与 stale；索引检查严格只读（mode=ro，绝不迁移/回填/修复 owner 索引） |
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
- source 超 8 MiB 或解析超预算返回 413 `document_too_large` / `document_budget_exceeded`，错误details带固定revision、source_size_bytes与可用source-export action；显式导出每个Range最大1MiB。附件每个Range最大8MiB；无Range仅在总大小不超过对应上限时返回200，否则416并提供总大小，消费者分段流式保存。不以返回半篇代替全文，不支持格式受权下载，不进在线转换服务。
- document 首次请求冻结可解析的 content revision；后续页重新校验同版。外部编辑后，如原版可从现有 snapshot 读取则继续原版并提示 current_revision；否则 409 `revision_unavailable`，用户选择重开当前版，不能混页或偷偷创建 snapshot。
- 原始 HTML 为转义文本；GFM 表格/任务列表支持只读显示；wiki/ref/asset/fence 语义由 Pinax resolver 产生。fence 仅既有注册的只读 owner handler，有独立预算与错误，不加载执行代码。remote 图片不自动请求。

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
