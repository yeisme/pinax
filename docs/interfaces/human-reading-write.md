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

前缀 `/v1/writer`，只允许 POST，JSON 请求体（≤128 KiB）。成功沿用 `pinax.projection.v1`，data 为冻结回执 schema。五个 binding 自 W06 起 available（backing=`internal/api/http:rest.writer.*`）。

| 路径 | capability / scope | 请求 | data / 行为 |
| --- | --- | --- | --- |
| `/capture` | `writer.capture` | title 必填（≤512），body ≤64 KiB，tags ≤32×128，slug/dir；**头部** X-Pinax-Operation-ID + Idempotency-Key 必填 | CaptureReceipt：operation 回执 + note_id/path；同 identity 重放同一回执，不二次写 |
| `/proposals` | `writer.propose`（只读） | change（action=create/append/replace/tags/archive + 受限字段） | ProposalPreview：解析后 change（含 expected_revision）+ sha256 preview_digest + before/after + state；不写 vault |
| `/proposals:apply` | `writer.apply` | propose 返回的 change + preview_digest + authorization | ApplyReceipt：operation 视图（status/receipt_ref/resource_ref/revision_after）；非 create 先快照 |
| `/organize/plan` | `writer.organize.plan`（只读规划） | note_refs 可选（1–100，canonical note_id）；缺省整库 | OrganizePlan：plan_id/status/created_at/expires_at/operation_count/operations[]；一律落盘 `.pinax/organize-plans/` 供 host 审阅 |
| `/organize/apply` | `writer.organize.apply` | plan_id 必填（原样 apply），snapshot_message/allow_stale 可选 | OrganizeApplyReceipt：operation_id=plan_id（恢复句柄）、snapshot_id、status；被消费 plan 重放显式失败 |

错误表冻结于 transport catalog（`insufficient_scope`、`writer_disabled`、`writer_request_invalid`、`operation_identity_required`、`idempotency_conflict`、`preview_mismatch`、`authorization_required`、`revision_conflict`、`plan_not_found`、`plan_stale`、`snapshot_required`、`outcome_unknown`、`reconcile_required` 等）；HTTP 映射见 `internal/api/writer.go` 的 `writerErrorStatus`。响应一律 `Cache-Control: private, no-store`。

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
- schema/digest 身份：OpenAPI golden `sha256:bc8cc605…`；manifest 由 transport catalog 生成（binding available + backing 可追溯）。

## 7. 验证入口

```bash
go test ./internal/transportcatalog/... -run TestWriterContract -count=1
go run ./tools/testkit/integrationevidence --profile human-reading-write-capture   # W02
go run ./tools/testkit/integrationevidence --profile human-reading-write-edit      # W03
go run ./tools/testkit/integrationevidence --profile human-reading-write-organize  # W04
go run ./tools/testkit/integrationevidence --profile human-reading-write-client    # W05
go run ./tools/testkit/integrationevidence --profile human-reading-write-api       # W06
go run ./tools/testkit/integrationevidence --profile human-reading-write-quality   # W07（task check，无 race 阶段）
openspec validate pinax-human-reading-write-v1 --strict --no-interactive
```
