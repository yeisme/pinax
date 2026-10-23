# 人类阅读写入 API 与受监督写会话

2026-10-02 · **已实现（W02–W06 交付，experimental）**：五个受监督 writer HTTP 端点、typed client 写调用与同 identity 恢复已落地并通过 component 证据；旧 Agent/CLI/MCP/URI 合同不变。实施 owner 为 [pinax-human-reading-write-v1](../../openspec/changes/pinax-human-reading-write-v1/)。DriveBridge 是首个消费者，不是笔记 owner；本文是写路由、scope、提案门、快照与恢复语义的唯一详细合同。

## 1. 基线与不变量

写入门在 Pinax 既有体系中已经存在——approval/write scope、expected_revision、snapshot_required、operation/receipt、unknown 恢复——本合同只把它们投影到人类会话，不发明第二套写入门。

- **write scope 独立授予**：`writer.capture`、`writer.editor`、`writer.organizer` 由 owner 授予、受监督会话 bootstrap 显式携带；与 reader scope 同注册表但互不升级。旧 read/write/admin token、temp token、MCP 凭据永远不可达 writer 路由（`writer_disabled`/`insufficient_scope`），请求字段不构成授权。
- **一切编辑经提案**：人类编辑以 proposal（change + preview_digest）为唯一入口；apply 只接受 propose 返回的未变更解析形 change（规范 note_ref + 钉住的 expected_revision）与其摘要。篡改摘要 → 409 `preview_mismatch`，无部分应用。
- **authorization 是调用方断言，不是已验证批准**：apply 携带 `explicit_instruction|reviewed_proposal`；任何声称"host 已批准"的值（如 `host_approved`）在 schema 层拒绝。宿主门 = 授予 writer scope 的受信本机会话。
- **revision/快照/operation 语义不降级**：expected_revision 漂移 → 409 `revision_conflict`；非 create 的 apply 先建快照（receipt_ref=`snapshot:` 前缀）；organize apply 走 approval_and_snapshot，snapshot_message 缺省由 plan_id 派生。每个 capture/apply 发 operation receipt；unknown 走同 identity 重放或 owner-local operation.show/reconcile，禁止换 identity 盲重试。
- 结构写（folder/inbox REST）、operation.show/reconcile、sync 为无缺口项，本合同不复制；如人类会话需要，按 scope 投影引用既有合同。
- 不提供 MCP 自批路径、普通文件覆盖旁路、收藏/进度/阅读状态。

## 2. 写公开面

前缀 `/v1/writer`。原五个提案／写入接口只允许 POST，JSON 请求体（≤128 KiB）；新增 editor-source 仅 GET，日记首建 propose/apply 为独立 POST。成功沿用 `pinax.projection.v1`，data 为冻结 schema。原五个 binding 自 W06 起 available；editor-source 在本次 N01/N02 组件验证后可用，均有 `internal/api/http:rest.writer.*` backing。

| 路径 | capability / scope | 请求 | data / 行为 |
| --- | --- | --- | --- |
| `/notes/{object_id}/editor-source`（GET） | `writer.editor_source` / `writer.editor` **且** `reader.body` | revision 必填（完整 sha256）；拒绝额外或重复 query | 无损正文、body_digest、受保护 metadata、64KiB 源文件预算；不读取导出或 AST，不写 owner 状态 |
| `/journals/daily:propose`（POST） | `writer.daily.propose` / `writer.editor` + `reader.body` + `reader.metadata` | date、IANA timezone、operation_id | 无副作用首建预览；固定 UUIDv7/相对目标/模板摘要/创建时间，before/after 与实际发布一致 |
| `/journals/daily:apply`（POST） | `writer.daily.apply` / 同上 | 原 proposal（内含 preview_digest）+ authorization | 先快照、原 ledger、无覆盖发布、ApplyReceipt；同日期防重，无自动重试 |
| `/capture` | `writer.capture` | title 必填（≤512），body ≤64 KiB，tags ≤32×128，slug/dir；**头部** X-Pinax-Operation-ID + Idempotency-Key 必填 | CaptureReceipt：operation 回执 + note_id/path；同 identity 重放同一回执，不二次写 |
| `/proposals` | `writer.propose`（只读） | change（action=create/append/replace/tags/archive + 受限字段） | ProposalPreview：解析后 change（含 expected_revision）+ sha256 preview_digest + before/after + state；不写 vault |
| `/proposals:apply` | `writer.apply` | propose 返回的 change + preview_digest + authorization | ApplyReceipt：operation 视图（status/receipt_ref/resource_ref/revision_after）；非 create 先快照 |
| `/organize/plan` | `writer.organize.plan`（只读规划） | note_refs 可选（1–100，canonical note_id）；缺省整库 | OrganizePlan：plan_id/status/created_at/expires_at/operation_count/operations[]；一律落盘 `.pinax/organize-plans/` 供 host 审阅 |
| `/organize/apply` | `writer.organize.apply` | plan_id 必填（原样 apply），snapshot_message/allow_stale 可选 | OrganizeApplyReceipt：operation_id=plan_id（恢复句柄）、snapshot_id、status；被消费 plan 重放显式失败 |

错误表冻结于 transport catalog（`insufficient_scope`、`writer_disabled`、`writer_request_invalid`、`operation_identity_required`、`idempotency_conflict`、`preview_mismatch`、`authorization_required`、`revision_conflict`、`plan_not_found`、`plan_stale`、`snapshot_required`、`outcome_unknown`、`reconcile_required` 等）；HTTP 映射见 `internal/api/writer.go` 的 `writerErrorStatus`。响应一律 `Cache-Control: private, no-store`。

### 2.1 编辑源码增量（2026-10-04）

实施 owner：[pinax-notebook-consumer-v1](../../openspec/changes/pinax-notebook-consumer-v1/)。`WriterEditorSource(ctx, objectID, revision)` 返回 `pinax.writer.editor_source.v1alpha1`，仅当前 revision 可编辑；版本过期为 409，完整源文件超过64KiB为413，不给截断源码。正文保持首尾空白与换行；metadata 只返回 note_id/title/tags/status/kind，不给任意 frontmatter 或绝对路径。

`frontmatter_protected=true`。当旧 collaboration splitter 不能保证正文边界一致（如部分 CRLF frontmatter），返回 `editable=false`、`readonly_reason=frontmatter_unsupported`；客户端必须保持只读，不能走普通文件覆盖。源码读取不 init/index refresh/snapshot；随后仍以同一 revision 生成提案并确认 apply。body_digest、对象身份、revision、预算由 typed client 再校验。

已验：service/HTTP/client 的权限交集、撤销、no-store、版本冲突、符号链接/隐藏项、重复身份、空正文/字节预算、固定源码→提案→保存→新版本读取→原 operation 回执。证据 `temp/integration-test-runs/20261004T190821Z-3975523/`。DriveBridge/Electron 集成、标签/图/日记/增强节点/跨会话恢复尚未因此完成。

### 2.2 日记首建增量（2026-10-05）

`WriterDailyPropose(ctx, date, timezone, operationID)` → 人审阅 `before/after` → `WriterDailyApply(ctx, proposal, authorization)`。日记写入需要三项 scope 的交集，不能凭 editor 或 reader 单项启用。仅允许 POST、闭对象 JSON、no-store；拒绝未知 query/字段。完整源文件仍不超过64KiB；原 proposal 的 JSON apply 请求不超过128KiB（预览先检查转义膨胀），不会给出无法原样提交的截断提案。

预览不 init、刷新索引、创建快照或保存正文。模板只能是内置或已有 `journal.daily`，不执行查询、创建后 action 或任意函数。交互模板接受普通变量以及固定请求日期的 `date()`；循环、递归、printf 等未纳入有界子集，返回 `template_unsupported`。旧模板引擎与 CLI 不改义。CRLF/BOM frontmatter 或不支持的 metadata 明确拒绝，不把元数据当正文；空模板的预览与发布正文也逐字一致。

`preview_digest` 是服务内存密钥的 HMAC，绑定 vault、session、binding、scopes、原 operation 和完整提案；是完整性证明，不是人批准的证明。apply 校验后持有现有 vault operation lock，先查原 ledger。新操作检查模板、目标及同日历史 ledger：已成功同日（包括后来移动模板路径）不能用新键再建；未明状态先 inspect/reconcile。目标发布使用同目录临时文件 fsync 后无覆盖 hard-link，不支持 hard-link 的文件系统 fail closed，不回退覆盖 rename，不生成日期后缀副本。只写指定日期，不附带更新今天 captures；随后复用 record gate/index。

恢复纪律：此新增 client **不自动重发**。服务已经发布但响应丢失时，仍有原提案的同会话客户端可显式原样 apply，以回放原回执。错误响应 schema、错身份及传输歧义都保留原 operation 并返回 unknown。正文/提案不进入 ledger。进程中断后，owner `pinax operation describe <operation-id> --vault <fixture-vault> --json` 与 `pinax operation reconcile <operation-id> --vault <fixture-vault> --json` 复用既有合同；reconcile 和 daily apply 使用同一锁，仅在目标对象+完整内容digest+匹配record证据一致时确认成功，不重建正文或捏造record。缺record、内容变化或证据不足继续 `reconcile_required`。新确认的成功回执清除旧unknown错误标记。

限制：本切片只保证新增daily接口间的同日并发；旧CLI/外部编辑器并发写入仍需后续组合验收。模板改路径后，reader lookup 尚不能从历史ledger重定位旧对象，apply会拒绝重复创建但须由owner定位旧对象。服务重启/换会话后不伪造旧session回放；跨会话受监督查询/恢复仍待后续合同。日记正文读取/编辑的系统页兼容、DriveBridge宿主确认UI、Electron及三平台验收均未因此完成。

组件证据：`temp/integration-test-runs/20261005T074452Z-3521605/`，7个包、31个顶层测试通过，六件套与脱敏扫描完整。editor-source回归 `20261005T073504Z-3021529`，导航回归 `20261005T073506Z-3024058`。定向lint为0 issues；这不是全仓final gate或三平台实机证据。

极早期进程中断若停在accepted、尚无预备事实，现状为安全阻断新建并要求owner检查，不伪造已取消/已成功；具体恢复处置登记N04.3。

### 2.3 日记正文编辑增量（2026-10-06，HTTP 与 typed client 已验）

三个独立入口均要求 `writer.editor`、`reader.body`、`reader.metadata`：

- `GET /v1/writer/journals/daily/editor-source`：闭 query `date/timezone/object_id/revision`，原文件无损源码，复用64KiB完整source预算与metadata保护；重复或额外字段拒绝。
- `POST /v1/writer/journals/daily/edit:propose`：闭JSON `date/timezone/object_id/revision/operation_id/body`；只替换正文，返回 `pinax.writer.daily_edit_proposal.v1alpha1`，含 `preview` 的exact before/after/canonical change及外层 `preview_digest`。
- `POST /v1/writer/journals/daily/edit:apply`：原 `proposal` 与 `authorization`，返回既有ApplyReceipt，capability=`writer.daily.edit`、binding=`rest.writer.daily.edit`。`request_digest`匹配**外层日记HMAC**，不是内部 `preview.preview_digest`。

日记HMAC绑定session/binding/scopes/vault/日期/原对象与完整差异；内部摘要仅为既有保存引擎规范差异。两者不可互换。提案不写vault，apply在原锁内复查日期/ID/revision并先snapshot。写前持久预期source与record摘要；恢复只据原object+字节+record证据，缺证保持unknown，不补正文/事件。同会话原提案可显式重放原回执；无自动重发、无跨会话升权。外部编辑器最后检查→rename窗口不等全局CAS，D03保护仍独立。

证据 `20261006T015715Z-3970939`：真实HTTP创建→源码→提案→确认apply→原receipt→新source，旧revision/篡改日期/三scope缺一/撤权/闭query与未知写字段拒。catalog为11个writer binding；OpenAPI新摘要 `sha256:6f173eb3…`，删除三个path与四个schema精确恢复上一版 `sha256:43c4c9cf…`。typed client新增 `WriterDailyEditorSource`、`WriterDailyEditPropose`、`WriterDailyEditApply`，复用无损source校验。提案核日期/对象/原operation/revision/安全相对路径/after=request body；apply校回执外层HMAC、原revision、object ID资源引用、source digest、snapshot与record身份。无自动重发；歧义返回携原operation的unknown，显式原提案回放才查回原结果。正常日记成功回执resource_ref统一为 `pinax://note/<object-id>`，与reconcile一致，旧普通writer不变。SDK真实提交后代理丢响应→unknown→显式原回执→新source回读证据 `20261006T020617Z-238970`；普通editor回归 `020621Z-243617`。不是DriveBridge/Electron实机或跨会话恢复完成证明。

## 3. 受监督写会话

bootstrap 帧（`pinax.local_session.bootstrap.v1alpha1`）扩展位自 W02 开放：scopes 枚举含 writer scopes，`readonly` 变为布尔；**readonly ⇔ writer scope 关联由 bootstrap 读取器强制**——writer 会话必须声明 `readonly=false`（ready 事件如实上报并暴露 writer 路由组），reader-only 会话保持 readonly。canonical note_id 之外的整理选择 → `organize_identity_required`（先 `pinax record adopt`，规划不越权收养）。

## 4. Typed client 与恢复纪律（W05）

`pkg/pinaxclient` 提供 `WriterCapture/WriterPropose/WriterApply/WriterOrganizePlan/WriterOrganizeApply` 与 `WriterAwaitApply` 有界轮询：回执校验 schema_version；歧义传输失败（request_failed/5xx）以**同一 identity** 恰好重发一次（ledger 幂等重放）；持续歧义返回 `mutation_outcome_unknown`（携带原 operation id，禁止换 identity）；organize apply 歧义不自动重试（plan 已被消费，指引 inspect plan）；identity 在本地预校验，HTTP 前失败。

## 5. 演进与回退

全部 additive：reader 只读面与旧 REST/RPC/MCP/URI 合同不变。撤销某个 writer scope 或关闭 writer 路由组即回退到纯只读会话；无数据迁移。apply 半途失败的恢复路径只有 operation receipt 与既有 reconcile，不存在文件覆盖补救。写能力可用性按证据门推进：合同期（W01）binding 保持 planned；W06 组件证据落地后五个 binding 才 available。

## 6. DriveBridge 消费交接摘要（W07）

- DriveBridge 作为受信宿主授予 writer scopes（按用户操作逐项授予：捕获用 `writer.capture`、编辑用 `writer.editor`、整理用 `writer.organizer`），并承担 host 确认流（propose 预览 → 用户确认 → apply）。
- 已验：五个端点在 fixture vault 上的全链（读取→捕获→提案→确认→apply→同 identity 崩溃恢复→整理）与 typed client 恢复纪律（component 证据 `temp/integration-test-runs/20261002T155631Z-229267/`、`20261002T154925Z-3822650/`）。
- 未验：真实用户长周期使用、三平台宿主集成（DriveBridge Electron 侧实现另行验收）、跨会话恢复的用户体验（跨会话同 identity 按设计 fail-closed，恢复走 owner-local operation.show/reconcile）。
- schema/digest 身份：OpenAPI golden 在daily首建增量后为 `sha256:8260370086dcb5a10949fe989a68ef5157b04fea8d9079d4fbb1e0add19cfa39`；删除daily两条path和三个schema精确恢复 `sha256:4c1e027e…`；删除导航增量恢复 editor-source 基线 `sha256:e37403bb…`；测试删除新增 path/schema 后仍恢复 `sha256:bc8cc605…`，旧合同保持不变；manifest 由 transport catalog 生成（binding available + backing 可追溯）。

## 单笔记标签保存边界（N04.14）

既有 `writer.propose/apply` 的 `change.action=tags` 继续支持 add/remove/set；沿用原 operation、revision、快照和回执，不另建标签保存 API。propose 和 TagNote 均验证可无损修改表示：仅替换 `tags`、`updated_at`，其余 metadata 与正文逐字节保留。复杂标签目标（如 block list）、CRLF frontmatter、重复键、anchor/alias/merge 等暂拒绝；应用错误为 `frontmatter_unsupported`，旧 HTTP 冻结错误表将其包装为 `writer_unavailable`，不得把拒绝视为保存成功。原子外部编辑 CAS、批量标签计划和跨会话恢复不在该证据内。

## 显式目标标签计划与应用（N04.20，experimental）

三个 POST 已注册为 available REST binding，均要求当前受监督会话同时具有 `writer.editor`、`reader.body`、`reader.metadata`；旧 token 不升级，所有响应 `private, no-store`。

| 路径 | 请求 | 返回 |
| --- | --- | --- |
| `/v1/writer/tags:plan` | `operation_id`、`operation=rename/delete`、规范 `old_tag`、`new_tag`（delete 必须空）、1–32 个 `targets[{object_id,revision}]` | `pinax.writer.tags_plan.v1alpha1`：old/new tag ID、逐项 before/after、固定身份和 HMAC |
| `/v1/writer/tags:apply` | 原 `plan`＋`authorization=reviewed_proposal/explicit_instruction` | `pinax.writer.tags_receipt.v1alpha1` 原父 operation 与逐项结果 |
| `/v1/writer/tags:lookup` | 仅原 `operation_id`，无需计划或 preview key | 同一父回执；每次重新鉴权、只读私有 ledger 快照 |

query（包括裸 `?`）、未知字段、重复/转义等价键、坏UTF8/未配对surrogate、深度>32、token>16384拒绝。plan/apply request≤128KiB，lookup≤2KiB。owner 仍裁决规范标签、目标身份、每源64KiB/选择总1MiB；显式精确标签不是全vault/子树迁移，daily对象仍走专属owner。计划只在内存，不持久正文或标签草稿；HMAC不是人工批准证明。

**HTTP 200 仅表示返回了 operation，不代表写入完成。** 只有 `status=succeeded` 才完成；`accepted/applying/reconcile_required` 保原operation，查看原事实，不能自动换key、重建计划或补写。多文件非事务：先全选预检，再父子operation/原锁/逐项快照/TagNote保存；部分失败保留成功项。原apply重放不继续未完成项。404不证明从未提交；历史回执不授当前文件读取权。不同session/binding/完整scope身份不能认领旧操作，跨会话恢复另验。

N04.21 已提供 `pkg/pinaxclient.WriterTagsPlan`、`WriterTagsApply`、`WriterTagsLookup`（Client 方法），复用原有界、拒重定向 transport。SDK核对请求目标/revision、规范tag及old/new ID、逐项差异、相对安全路径、父operation/digest与唯一子identity；success须全部子成功且result/receipt相同，未完成状态不能冒完成。apply不自动重试；丢响应返回携带原operation的unknown错误，使用lookup查询原事实，无需重交标签草稿。SDK不能独立验证服务端HMAC、原record或人工批准；跨会话与DriveBridge/Electron另验。真实提交后代理503→仅lookup→原receipt→固定新revision读回通过，证据 `temp/integration-test-runs/20261006T121422Z-942451/`。OpenAPI当前 `sha256:08ab4afa4fa2a54f9d348e5a0154e303569101a0c2ff172c329341352c2bf337`，删除三个新path恢复N03.6摘要，再删五schema恢复旧日记摘要。证据 `temp/integration-test-runs/20261006T120404Z-416116/`。

## 跨会话恢复私有授权与只读查询（N03.9 / N04.33–38，experimental）

`pkg/localsession.RecoveryLookupGrant` 与 `DecodeRecoveryLookupGrant` 定义独立的 `pinax.local_session.recovery_lookup.v1alpha1`，**N04.36/37已提供只读恢复HTTP与typed SDK，DriveBridge仍未接入**；N04.33/34提供查询内核与会话联动，N04.35提供显式启用的独立私有握手。旧 bootstrap 中新增 `recovery_grant` 字段仍被 schema 和读取器拒绝；授权只能走独立握手。

此帧只能由受信宿主在新的明确用户批准后，通过专用私有通道提供；它不是 bearer、签名票据或 renderer 可以提交的授权。`ValidFor` 只校验一致性，不证明批准、通道来源、磁盘身份或 ledger 所有权。

- 封闭平坦 JSON ≤4096 bytes，每字段必填，拒重复/未知/大小写伪装字段、非字符串、坏UTF8、surrogate、控制字符及尾随内容。错误只有 `recovery_grant_invalid`，不回显帧。
- 绑定新 `session_nonce`、当前 `binding_id`、owner-issued `vault_identity_digest`，再精确绑定原 `operation_id/capability_id/operation_binding_id/principal_digest/scope_digest/request_digest`。vault摘要由N04.33固定文件身份后的随机观察绑定发行；不等跨进程稳定身份或任意路径hash证明。
- 第一合同仅 `action=lookup`，允许普通编辑、日记首建/编辑、标签父操作四种既有 capability；不开放 tags child、capture/organize、apply/retry/reconcile。其它场景仍保留既有owner恢复入口，不删除需求。
- UTC canonical RFC3339Nano 时间，`issued_at ≤ now < expires_at`，最长5分钟；每次实际查询必须重新验时、验当前会话撤权和全部原row身份，不能只在启动时验证。
- 不恢复旧会话、旧bearer、旧HMAC或写scope，不改变ledger principal；回执事实不授当前正文权。原始帧及旧身份摘要不进入renderer、日志或持久恢复槽。

N04.33已实现进程内 `app.RecoverySession`：固定root/祖先/owner目录/ledger的文件身份并生成随机观察绑定，显式安装最多32个不可替换的精确lookup grant；仅当前session查询，使用私有只读快照核原principal/scope/capability/binding/request，返回原View而非推断成功。查询前后重核文件身份/有效期，撤权、观察到路径替换或过期会永久失效恢复session。临时真实ledger及原恢复e2e证据 `20261006T164411Z-2012374`；不证明瞬间替换恢复ABA或跨平台实机安全。该内核不自动授予授权；N04.35/36分别接入私有握手与HTTP查询。

N04.34已把内核接入 `ReaderSessionStore` 的进程内私有入口：一次性 `PrepareRecovery`、私有批准 `AuthorizeRecovery`、实际当前bearer `LookupRecovery`；`Revoke` 与过期检查同步撤销owner许可。注册拒重复bearer覆盖，Create/Verify不共享可变scope，避免身份别名修改。registry→owner串行锁使Revoke返回后不能再准入；不是对进行中snapshot的瞬时中断。证据 `165203Z-2411462`（session）/`165211Z-2421079`（writer回归），日期均20261006。

N04.35 私有启动方式：`pinax api serve --session-stdin --session-recovery-stdin`。先发旧bootstrap，读取独立 `recovery_challenge`（schema=`pinax.local_session.recovery_challenge.v1alpha1`，nonce/binding/vault观察摘要；公开 `DecodeRecoveryChallenge` 验证），经宿主明确批准后以一行newline终止JSON回复精确lookup grant；收到原ready才算服务启动。grant等待10秒、≤4096bytes，EOF/取消/非法拒；握手后stdin只允许空白或EOF。不启用新flag的旧宿主不接收新challenge。挑战属于私有host协议，不转renderer或日志；它不是授权或真实用户批准证明。

真实CLI合法授权→ready→EOF与非法apply授权拒证据 `20261006T170049Z-2856346`，组件含原row读取/源树无写。任意阻塞stdout不具10秒整体startup保证，取消后的阻塞reader由关闭私有管道/进程退出回收。该证据本身不覆盖公开恢复查询；N04.36补充如下HTTP链。

N04.36：`GET /v1/reader/recovery/operation?operation_id=<original-id>`，capability=`reader.recovery.lookup`，data复用 `pinax.operation.v1` 原View。只收单值闭query operation_id（ASCII opaque≤128），query≤2KiB，拒body、未知/重复/坏UTF8/percent；POST不能安装grant。当前受监督bearer加精确私有grant才有权限，不靠reader.metadata/body授历史回执权。成功响应≤128KiB、private,no-store/nosniff；查询失败统一403 recovery_lookup_unavailable，不区分未批准/foreign/missing，不能据此推断从未提交。200只表示读到原事实，accepted/applying不是完成，保原request_digest/status。

新进程真实私有握手→GET原accepted回执及foreign operation拒证据 `20261006T171000Z-3318134`；组件含未授权、非法输入、撤权401与源树不变。OpenAPI+writer回归 `171022Z-3342893`，reader14/writer14，新digest `sha256:4c85fe7909c8575546deee3ae2d5f0d85c20f30e5d3ab1e6e1617d1824f595a8`；删除单path与单request schema精确恢复 `79d729770…` 及旧增量链。

N04.37：`Client.ReaderRecoveryLookup(ctx, ReaderRecoveryHandle)` 返回既有typed `Operation`。handle必须包含原operation_id/capability_id/binding_id/request_digest，只有operation_id发给HTTP，其余只用于客户端核对原身份，不构成授权。复用拒重定向GET、成功/错误响应128KiB上限并尊重更小调用方配置；原closed schema外加拒idempotency_key、result≤64KiB/receipt≤16KiB、时间格式、revision摘要、状态与reconcile/required_action/completion关系。保留原状态不自动retry/reconcile，失败不证明未提交。结果JSON保持惰性，客户端不能独立证明磁盘证据或人批准，也不根据操作日期顺序假设系统时钟单调。

SDK真实HTTP未批准拒→新grant后原accepted→owner applying→succeeded事实→撤权拒，以及五状态正控制/恶意身份与预算/redirect/503不重发/坏handle零网络，通过 `20261006T171835Z-3778189`。真实状态转换由测试owner ledger执行，不冒一次完整编辑保存或DriveBridge重启实机。

N04.38 补充真实保存证据 `20261006T172611Z-4159514`：旧CLI受监督writer实际创建日记、固定源码、exact提案和edit apply，代理只在owner真实提交后丢弃响应，SDK保unknown且apply一次；旧进程退出，新只读CLI经私有grant查询同一operation，原回执全部事实保持，另授reader.body后固定revision读回，正文恰好写一次。新会话恢复期间完整vault树不变，旧bearer、其他operation、旧提案写入均拒。两种回执schema分别验证，其余字段按JSON语义相等。测试host在apply前保留旧批准摘要和会话身份，不靠事后读取ledger伪造handle。

此证据不代表真实用户批准、DriveBridge/Electron、强杀或三平台验收。后续准入需分别落实：host人工确认后发行精确回执授权的受信来源；最后DriveBridge人工确认和重启链。reconcile会更新operation状态，必须独立授权且沿原证据引擎，不因lookup合同就绪而自动执行。D03与三平台验收仍独立阻塞。

## 7. 验证入口

```bash
go test ./internal/transportcatalog/... -run TestWriterContract -count=1
go run ./tools/testkit/integrationevidence --profile notebook-editor-source
go run ./tools/testkit/integrationevidence --profile notebook-daily
go run ./tools/testkit/integrationevidence --profile human-reading-write-capture   # W02
go run ./tools/testkit/integrationevidence --profile human-reading-write-edit      # W03
go run ./tools/testkit/integrationevidence --profile human-reading-write-organize  # W04
go run ./tools/testkit/integrationevidence --profile human-reading-write-client    # W05
go run ./tools/testkit/integrationevidence --profile human-reading-write-api       # W06
go run ./tools/testkit/integrationevidence --profile human-reading-write-quality   # W07（task check，无 race 阶段）
openspec validate pinax-human-reading-write-v1 --strict --no-interactive
```
