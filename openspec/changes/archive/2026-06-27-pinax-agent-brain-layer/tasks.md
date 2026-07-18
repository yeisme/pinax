# 任务

## 0. 全局约束

- Owner: `cli/pinax`。本 OpenSpec 只负责 Pinax 侧 CLI/API/MCP/projection/proof-loop 合同；未来 Web/desktop client、hosted team backend、OAuth provider 或 gateway runtime 必须由对应独立 owner 承接。
- 合同策略: 全部 stable surface 只做 additive；不得删除、重命名、重定义现有 command、flag、JSON envelope 顶层字段、`--agent` key、MCP tool、API route、config/profile/registry key、DB schema 或 public Go API。
- 写入边界: `.pinax/**`、SQLite/GORM、LanceDB、memory ledger、events、receipts、sync state、provider config、token/profile metadata 必须由 CLI/application service 写入，不允许 Web/Agent/MCP 手写。
- 输出边界: 默认只输出 bounded projection；不得在 stdout/stderr/events/MCP/API/test fixtures/evidence 中输出完整 note body、raw prompt、hidden system prompt、provider payload、Authorization header、cookie、token、private tool arguments 或完整 chain-of-thought。
- Provider/cost: embedding、rerank、LLM 和 local model 调用必须显示 provider/model/source type；付费或网络调用必须有 cost class、doctor next action 或 user-visible confirmation。
- 集成证据: 新增 integration/component/e2e 入口必须写入 `temp/integration-test-runs/<run-id>/`，至少包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json` 和 `artifacts/`，并脱敏。
- 完成门禁: 每阶段运行 focused tests；收口运行 `openspec validate pinax-agent-brain-layer --strict && openspec validate --all --strict`；触及 Go 代码后运行 `task check`。

## P0: Agent Brain MLP 合同基线

- [x] **0.1 审计现有 Pinax brain building blocks**
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: none
  - Scope: 对齐 `memory`、`kb`、search、query/dataview、note links/backlinks/orphans、graph、project board、briefing、MCP、Local API、proof loop、Web/Open Design，形成 capability matrix。
  - Files: `docs/commands/README.md`、`docs/commands/memory.md`、`docs/commands/kb.md`、`docs/commands/search.md`、`docs/commands/graph.md`、`docs/commands/mcp.md`、`docs/overview/product-positioning.md`、`docs/product/mvp-scope.md`、`docs/product/web-open-design.md`、`openspec/specs/*/spec.md`。
  - Acceptance: matrix 覆盖 ingest、memory、semantic KB、graph、query/database views、answer synthesis、MCP/API、maintenance/dream cycle、team/scopes、provider/cost；每项标记 `implemented`、`needs-contract`、`needs-implementation`、`future-owner`。
  - Validation command: `rg -n 'memory context|kb context|pinax mcp serve|Answer synthesis|Agent Brain|dream cycle|provider/cost' docs openspec/changes/pinax-agent-brain-layer`
  - Expected result: 设计和任务中能检索到每类能力及边界。
  - Failure re-check: 如果某能力已有独立 OpenSpec，引用现有 spec，不重复定义实现细节。
  - Completion note: 已在 `design.md` 建立 capability matrix，并在命令文档中区分 current building blocks、`planned` 命令和 `future-owner` surfaces。

- [x] **0.2 固化 Agent Brain capability discovery 命名**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 0.1
  - Scope: 设计 additive capability ids，例如 `brain.context.bundle`、`brain.answer.preview`、`brain.maintenance.plan`、`brain.sources.list`、`brain.provider.cost_status`，并定义是否 CLI-only、MCP、REST/RPC 或 future-client-only。
  - Files: `internal/app/remote.go`、`internal/domain/remote.go`、`internal/app/remote_test.go`、`docs/commands/api.md`、`openspec/changes/pinax-agent-brain-layer/specs/pinax-agent-brain-layer/spec.md`。
  - Acceptance: `pinax api routes --vault ./my-notes --json` 能发现 brain capability 元数据；旧 routes 字段保持不变；planned capability 可用 `local_only_reason=future-contract` 或 `future-owner` 标记。
  - Validation command: `go test ./internal/app -run 'Remote|Capability|Brain|OpenAPI' -count=1`
  - Expected result: focused tests 通过；OpenAPI 只为实际 REST route 输出 path，不为未实现 capability 伪造 route。
  - Failure re-check: 如果暂不实现 registry，只在 spec 中保留 planned id，并推迟到 P1 implementation task。
  - Completion note: 已在 `RemoteCapabilities()` 添加 `brain.context.bundle`、`brain.answer.preview`、`brain.maintenance.plan`、`brain.sources.list`、`brain.provider.cost_status` 的 additive planned capability metadata，统一 `local_only_reason=future-contract`、`ui_group=agent.brain`、`body_exposure_default=none`；focused test 确认这些 capability 不注册假 REST/RPC route，OpenAPI 不输出 `/brain` path。

- [x] **0.3 定义 Agent Brain context bundle schema**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 0.1
  - Scope: 复用 `pinax.agent_context.v1`、memory recall signals、KB hit metadata、graph evidence、query rows 和 proof receipts，定义 `pinax.agent_brain.context_bundle.v1`。
  - Files: `internal/domain/types.go`、`internal/app/agent_context.go`、`internal/output/render.go`、`cmd/pinax/*_test.go`、`docs/overview/agent-safe-boundary.md`。
  - Acceptance: context bundle 包含 `task`、`entities`、`memory_refs`、`semantic_refs`、`graph_refs`、`query_refs`、`receipts`、`freshness`、`body_exposure`、`next_actions`；默认不含完整正文。
  - Validation command: `go test ./internal/app ./internal/output ./cmd/pinax -run 'AgentBrain|ContextBundle|Memory|KB|Graph|Redaction' -count=1`
  - Expected result: focused tests 通过；body sentinel 不出现在 bounded bundle。
  - Failure re-check: 如果 schema 过大，先输出 memory/kb/graph refs 和 next actions，query/receipts 作为 optional fields。
  - Completion note: 已新增 `pinax.agent_brain.context_bundle.v1` domain schema 和 `BuildAgentBrainContextBundle` bounded builder；字段覆盖 `task`、`entities`、`memory_refs`、`semantic_refs`、`graph_refs`、`query_refs`、`receipts`、`freshness`、`body_exposure`、`next_actions`。Focused test 使用 body/evidence sentinel 证明 bundle 不复制 snippets 或 raw evidence。

## P1: Answer synthesis 和证据合同

- [x] **1.1 设计 answer preview CLI/API 合同**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 0.3
  - Scope: 决定命令入口。推荐 `pinax brain answer <question> --vault ./my-notes --json`，如团队决定复用现有命令，则必须更新本 OpenSpec。首版只做 preview/read-only。
  - Files: `cmd/pinax/brain_command_test.go`、`internal/cli/brain_cmd.go`、`internal/app/brain_answer.go`、`internal/domain/types.go`、`docs/commands/brain.md`。
  - Acceptance: answer projection 包含 `schema_version=pinax.agent_brain.answer.v1`、`answer`、`claims[]`、`sources[]`、`open_questions[]`、`next_actions[]`、`cost`、`body_exposure`；无 evidence 的 claim 不得输出为确定结论。
  - Validation command: `go test ./internal/app ./internal/output ./cmd/pinax -run 'BrainAnswer|Synthesis|Evidence|Citation|OpenQuestion|Redaction' -count=1`
  - Expected result: focused tests 通过；无 provider 时返回 bounded plan 或 provider doctor next action。
  - Failure re-check: 如果 LLM synthesis 未准备好，先实现 extractive answer preview，不调用 provider。
  - Completion note: 已新增 `pinax brain answer <question> --vault <vault> --json` read-only extractive preview；输出 `pinax.agent_brain.answer.v1`，包含 `answer`、`claims[]`、`sources[]`、`open_questions[]`、`next_actions[]`、`cost`、`body_exposure` 和 `context_bundle`。首版复用 search bounded contexts，不调用 provider、不写 vault；CLI test 覆盖 body/provider/Auth sentinel 不泄漏。

- [x] **1.2 Provider/rerank/cost 可见性**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.1
  - Scope: 为 answer/kb/rerank/provider 调用定义 `cost_class`、`provider_id`、`model`、`local_only`、`network_required`、`credential_source`、`dry_run_available`。
  - Files: `internal/app/kb.go`、`internal/semantic/`、`internal/output/render.go`、`cmd/pinax/kb_command_test.go`、`docs/commands/kb.md`、`docs/commands/brain.md`。
  - Acceptance: OpenAI/Gemini/Ollama/fake provider 输出不泄密；缺 credential 给 `pinax kb provider doctor <provider> --vault ./my-notes --json` next action；answer preview 显示成本等级。
  - Validation command: `go test ./cmd/pinax ./internal/app ./internal/output -run 'Provider|Cost|CredentialSource|BrainAnswer|Redaction' -count=1`
  - Expected result: focused tests 通过；stdout/stderr 不含 raw token/provider payload。
  - Failure re-check: 如果成本估算无法精确，使用枚举 `none|local|low|metered|unknown`，不展示虚假价格。
  - Completion note: `pinax brain answer` 现在输出 `cost_class=none`、`provider_id=extractive`、`model=none`、`local_only=true`、`network_required=false`、`credential_source=none`、`dry_run_available=true`；既有 KB provider list/doctor 合同继续覆盖 OpenAI/Gemini/Ollama/fake provider 状态、credential source 和 redaction。

- [x] **1.3 Answer evidence contract tests**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.1
  - Scope: 增加递归红线测试：answer/context/MCP/API 不得泄露 full body、raw prompt、hidden system prompt、provider payload、token。
  - Files: `cmd/pinax/brain_command_test.go`、`internal/mcpserver/server_test.go`、`internal/api/http_test.go`、`internal/output/*_test.go`。
  - Acceptance: 测试 fixture 包含 body sentinel、provider payload sentinel、Authorization sentinel；bounded answer 不出现这些 sentinel。
  - Validation command: `go test ./cmd/pinax ./internal/mcpserver ./internal/api ./internal/output -run 'Brain|Answer|MCP|API|Redaction|BodyExposure' -count=1`
  - Expected result: focused tests 通过；失败时指出具体泄漏路径。
  - Failure re-check: 如果现有 renderer 不支持递归扫描，先补测试 helper，不放宽安全断言。
  - Completion note: 已新增 `cmd/pinax/brain_command_test.go` 覆盖 answer preview body/provider/Auth sentinel；既有 MCP/API/output redaction tests 覆盖 bounded display、Authorization/token/provider payload 递归脱敏。目标 focused validation 通过。

## P2: MCP/HTTP/权限和团队知识库合同

- [x] **2.1 扩展 MCP tool 分组和只读 answer tools**
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 1.1
  - Scope: 新增或规划 MCP tools：`pinax.brain.context`、`pinax.brain.answer`、`pinax.brain.sources`、`pinax.brain.maintenance_plan`；默认只读，write tools 不在本阶段启用。
  - Files: `internal/mcpserver/server.go`、`internal/mcpserver/server_test.go`、`docs/commands/mcp.md`、`docs/interfaces/remote-api-contract.md`。
  - Acceptance: MCP `tools/list` 返回 brain tools，tool schema 明确 body exposure、cost、scope；`tools/call` 不写 vault；请求 full body 被拒绝或降级。
  - Validation command: `go test ./internal/mcpserver -run 'Brain|ToolsList|Readonly|BodyExposure|MaintenancePlan' -count=1`
  - Expected result: focused tests 通过；MCP response 不含 full note body。
  - Failure re-check: 如果 tool 数量接近 30+，按分组分页或 capability discovery 输出，不一次性塞入不稳定工具。
  - Completion note: 已新增 stdio MCP tools `pinax.brain.context`、`pinax.brain.answer`、`pinax.brain.sources`、`pinax.brain.maintenance_plan`；Tool metadata additive 声明 `readonly`、`body_exposure`、`cost_class`、`scope` 和 input schema。Calls 复用 bounded answer/context projection 或返回 proof-loop plan-only next action，不写 vault，不暴露 full body。

- [x] **2.2 HTTP MCP / OAuth / scopes / rate limit 作为 future-owner handoff**
  - Owner: `cli/pinax` for contract; future owner: `mcp/gateway` or hosted/backend subproject
  - Lane: C
  - Depends on: 2.1
  - Scope: 定义 HTTP MCP、OAuth、scope、rate limit、team permission 的接口要求和边界，不在 `cli/pinax` 内实现 hosted backend。
  - Files: `docs/interfaces/remote-api-contract.md`、`docs/product/web-open-design.md`、`openspec/changes/pinax-agent-brain-layer/design.md`、future root/backend OpenSpec handoff。
  - Acceptance: 文档明确 single-user local mode、team/company KB mode、OAuth/rate-limit backend owner；无 owner 时不得把 team mode 标为 production-ready。
  - Validation command: `rg -n 'OAuth|scope|rate limit|team|future owner|HTTP MCP' docs openspec/changes/pinax-agent-brain-layer`
  - Expected result: 检索命中 handoff 和边界；没有假命令示例。
  - Failure re-check: 如果需要实际 gateway 代码，创建独立 owner OpenSpec，不在 Pinax CLI change 中实现。
  - Completion note: 已在 `design.md`、`docs/interfaces/remote-api-contract.md`、`docs/commands/mcp.md` 和 `docs/product/web-open-design.md` 明确 HTTP MCP/OAuth/rate limit/team backend 为 `future-owner`，未新增 Go 实现。

- [x] **2.3 Team/company KB permission model spec**
  - Owner: `cli/pinax` for local projection contract; future owner for hosted policy
  - Lane: C
  - Depends on: 2.2
  - Scope: 定义 `principal`、`workspace`、`source_acl`、`visibility`、`redaction_policy`、`audit_ref` 等字段如何进入 bounded projection。
  - Files: `openspec/changes/pinax-agent-brain-layer/specs/pinax-agent-brain-layer/spec.md`、`docs/overview/agent-safe-boundary.md`。
  - Acceptance: 没有 ACL proof 时，team answer 只能返回 `permission_unknown` 或 local-only；不得跨用户合成公司知识。
  - Validation command: `openspec validate pinax-agent-brain-layer --strict`
  - Expected result: spec 能表达 single-user 与 team mode 的不同边界。
  - Failure re-check: 如果 ACL 字段会影响 DB schema，后续实现必须新建 migration design，并按 expand-first 执行。
  - Completion note: 已在 delta spec 和 agent-safe 文档定义 `principal`、`workspace`、`source_acl`、`visibility`、`redaction_policy`、`audit_ref`，缺 ACL proof 时返回 bounded failure。

## P3: Ingest、sync 和 maintenance/dream cycle

- [x] **3.1 Ingest pipeline contract**
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 0.3
  - Scope: 统一 Markdown import、capture、future email/calendar/webhook/shortcut/zapier intake 的 source identity、receipt、dedupe、body exposure 和 proof gate。
  - Files: `docs/commands/import.md`、`docs/commands/inbox.md`、`docs/commands/journal.md`、`docs/commands/briefing.md`、`internal/app/noteops/`、future `internal/app/brain_ingest.go`。
  - Acceptance: 所有 ingest 先支持 dry-run/preview；确认写入必须产生 receipt；外部 source payload 不进入 stdout/stderr/evidence 原文。
  - Validation command: `go test ./internal/app ./cmd/pinax -run 'Import|Inbox|Journal|Briefing|Ingest|Receipt|Redaction' -count=1`
  - Expected result: focused tests 通过；外部集成使用 fake fixtures，不访问真实邮箱/日历/webhook。
  - Failure re-check: 当前未实现的外部源只能进入 docs/spec 的 future integration matrix，不出现 fake production command。
  - Completion note: 已在 import docs 增加 Agent Brain ingest contract matrix，覆盖 Markdown import、inbox、journal/briefing 和 future email/calendar/webhook/shortcut/Zapier intake 的 source identity、dry-run/preview、receipt/dedupe/body exposure/redaction 边界；focused Import/Inbox/Journal/Briefing tests 通过，未新增真实外部集成。

- [x] **3.2 Maintenance/dream cycle plan-only implementation**
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 1.3
  - Scope: 设计并实现 `brain maintain` 或 proof loop extension，输出 entity merge、citation repair、memory dedupe、stale fact、contradiction、summary compression candidates。
  - Files: `cmd/pinax/brain_command_test.go`、`internal/app/brain_maintenance.go`、`internal/domain/types.go`、`docs/commands/brain.md`、`docs/commands/proof.md`。
  - Acceptance: `pinax brain maintain --vault ./my-notes --dry-run --json` 不写 vault；`--save-plan` 只写 CLI-authored plan evidence；apply 不属于默认路径。
  - Validation command: `go test ./internal/app ./cmd/pinax -run 'BrainMaintain|DreamCycle|PlanOnly|Contradiction|Dedupe|Snapshot|Redaction' -count=1`
  - Expected result: focused tests 通过；维护建议包含 risk、evidence、next command。
  - Failure re-check: 如果 plan schema 过宽，先支持 stale memory + duplicate memory + citation repair 三类。
  - Completion note: 已新增 `pinax brain maintain --dry-run --json` 和 `--save-plan`；plan schema 为 `pinax.agent_brain.maintenance_plan.v1`，默认 `writes=false`，覆盖 stale memory、duplicate memory、citation repair 三类候选，包含 risk/evidence/next action。`--save-plan` 仅写 `.pinax/brain-maintenance-plans/*.json` CLI-authored plan evidence；MCP maintenance tool 复用同一 service。

- [x] **3.3 Sync and rebuild policy for brain projections**
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 3.1
  - Scope: 明确 memory ledger、KB/LanceDB、graph projection、answer cache、maintenance plan 在 Cloud Sync 中的权威性和重建策略。
  - Files: `docs/architecture/cloud-sync-design.md`、`docs/commands/memory.md`、`docs/commands/kb.md`、`docs/commands/graph.md`、`openspec/specs/pinax-cloud-sync/spec.md`。
  - Acceptance: Markdown/source notes 和 receipts 是权威；KB/vector/graph/answer cache 是本地可重建 projection；Cloud Sync 不上传明文 vectors/raw body/provider payload。
  - Validation command: `rg -n 'memory ledger|LanceDB|answer cache|rebuildable projection|Cloud Sync' docs openspec/specs/pinax-cloud-sync/spec.md`
  - Expected result: docs/spec 明确各 projection 的 sync/rebuild 策略。
  - Failure re-check: 如果某 projection 必须跨设备同步，另开加密 envelope + migration OpenSpec。
  - Completion note: 已在 Cloud Sync architecture/spec、memory/kb/graph docs 和 `design.md` 记录 source/evidence/rebuildable projection 分类；未实现跨设备 memory/answer-cache sync。

## P4: 收口和归档

- [x] **4.1 文档和命令地图收口**
  - Owner: `cli/pinax`
  - Lane: docs
  - Depends on: 0.1, 1.1, 2.1, 3.2
  - Scope: 新增 `docs/commands/brain.md`，更新 `docs/commands/README.md`、`docs/overview/product-positioning.md`、`docs/product/mvp-scope.md`、`docs/product/web-open-design.md`，说明 Agent Brain 是 staged capability。
  - Files: listed above
  - Acceptance: 文档包含真实可运行命令；未实现命令明确标为 planned；没有把 hosted/team/OAuth/rate-limit 写成当前支持。
  - Validation command: `rg -n 'pinax brain|planned|Agent Brain|memory context|kb context|pinax mcp serve|proof loop' docs openspec/changes/pinax-agent-brain-layer`
  - Expected result: 检索命中设计、命令文档和 OpenSpec。
  - Failure re-check: 如果 `pinax brain` 尚未实现，示例必须在 planned section，当前命令示例只用现有真实命令。
  - Completion note: 已新增 `docs/commands/brain.md`；`pinax brain answer ...` 已标为 implemented extractive preview，其余未实现 `pinax brain ...` 示例仍标为 `planned`，hosted/team/OAuth/rate-limit 保持 `future-owner`。

- [x] **4.2 最终验证和归档准备**
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: all implementation/docs tasks
  - Scope: 验证 OpenSpec、focused tests、full quality gate、evidence redaction，准备 archive。
  - Files: `openspec/changes/pinax-agent-brain-layer/*`、相关 docs/tests/code。
  - Acceptance: `openspec validate pinax-agent-brain-layer --strict` 通过；`openspec validate --all --strict` 通过；触及 Go 代码后 `task check` 通过；所有 integration evidence 脱敏。
  - Validation command: `openspec validate pinax-agent-brain-layer --strict && openspec validate --all --strict`
  - Expected result: OpenSpec strict validation 通过；无 active unrelated failure。
  - Failure re-check: 如果全量 validation 因无关 active change 失败，记录 change 名称、错误摘要和本变更 focused validation 结果。
  - Completion note: `openspec validate pinax-agent-brain-layer --strict`、`openspec validate --all --strict` 和 `task check` 均通过；已准备归档。
