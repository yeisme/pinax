# MVP 范围

MVP 分四个阶段推进：

| 阶段 | 目标 | 验证 |
| --- | --- | --- |
| 本地 Vault 工作台 | `init`、`vault validate`、daily/inbox、`note list/show`、`pinax note links`/`pinax note backlinks`/`pinax note orphans`、`search --link-target`、attachments、saved views、index/search、Markdown import/export、`metadata plan/apply`、`repair plan/apply`、`organize plan/list/apply`、`version snapshot` | `go test ./...` 和 command-level tests |
| CLI-backed Provider Pull | 外部 CLI capability probe、fake executable fixture、`sync diff`、`sync pull --dry-run` | provider 和 sync fixture tests |
| Agent/MCP Read and Plan | project board workspace、共享 `NoteDisplay`、`pinax mcp serve` 的只读 resources/tools、localhost REST/RPC projection adapter、handoff、triage dry-run | MCP frame、REST/RPC component 和 output contract tests |
| Controlled Apply | action file apply、本地写入审批、event evidence、handoff | dry-run/yes gate 和 redaction tests |

参考 GBrain 的 “agent brain layer” 方向后，Pinax 的 MVP 应增加一个明确的 **Agent Brain MLP** 视角：先把现有 local vault、memory ledger、KB context、link graph、query/database views 和 MCP 只读 surface 组合成 agent 可消费的长期记忆入口，而不是先做托管知识库或通用聊天 UI。

Agent Brain MLP 的最小闭环：

| 步骤 | 真实命令 | 输出边界 |
| --- | --- | --- |
| 导入资料 | `pinax import markdown ./source --dry-run --vault ./my-notes --json`，确认后 `pinax import markdown ./source --group research --kind reference --status active --conflict rename --yes --vault ./my-notes --json` | dry-run 不写；apply 通过 service 写 note 和 import receipt。 |
| 建索引和语义投影 | `pinax index refresh --vault ./my-notes --json`，`pinax kb rebuild --backend lancedb --provider ollama --model nomic-embed-text --vault ./my-notes --json` | KB rebuild 写本地 projection；provider/key 只显示来源，不回显 secret。 |
| 结构化长期记忆 | `pinax memory capture --type decision --subject alice --object "Preferred concise async updates" --source notes/meetings/alice.md --vault ./my-notes --json` | 写 `.pinax/memory/ledger.sqlite`，必须带 source；recall/context 不输出私密全文。 |
| Agent 查询上下文 | `pinax memory context "prepare for Alice meeting" --entity alice --limit 12 --vault ./my-notes --agent`，`pinax kb context "prepare for Alice meeting" --limit 8 --vault ./my-notes --json` | 返回 bounded facts、ranking reason、evidence refs 和 next actions。 |
| 关系和事实校验 | `pinax note backlinks "Alice" --vault ./my-notes --json`，`pinax search "Alice" --link-target notes/people/alice.md --vault ./my-notes --json`，`pinax graph query --kind technique --match storyboard --vault ./my-notes --json` | 返回 bounded relationship/prompt-graph evidence，不加载全量图，不自动修复。 |
| Agent 接入 | `pinax mcp serve --vault ./my-notes` | MCP 默认只读，降级 body mode，不能写 vault。 |
| 维护和压缩 | `pinax proof loop run --vault ./my-notes --json` | 先诊断、计划、snapshot requirement 和 receipt；apply 必须显式 `--apply --yes`。 |

这个 MLP 的验收标准不是“搜索命中很多”，而是 agent 能回答带引用的问题：对象是谁、最近发生了什么、有哪些未完成事项、证据来自哪里、哪些信息可能过期、下一步应该运行什么命令。答案综合本身仍属于 preview/experimental 能力；在没有正式 synthesis contract 之前，CLI/API 应优先返回 memory/kb/graph/search 的 bounded context，让上层 agent 自行合成并引用 evidence。

Daily briefing workflow 是后续 agent workflow 切片。它必须建立在 local vault、research evidence ledger、review queue 和 delivery receipt 之上，不应变成独立 news bot。类似 GBrain 的 dream cycle 可以作为更晚的 maintenance loop：实体合并、引用修复、记忆去重、过期检测、矛盾提示和摘要压缩都必须先产出 reviewable plan，不得夜间静默改写 note body。

当前 MVP 的第一轮外部评估优先服务真实 Markdown vault：先让用户安全连接、capture daily/inbox，建立 SQLite/GORM local index，按 tag/group/folder/kind/status 搜索和浏览，保存常用视图，检查 resolved/broken/ambiguous links、orphan notes 和 attachments，按 `--link-target` 搜索，导入和导出 Markdown bundle，补充 metadata，生成 repair/organize plan 和 project board plan，然后在显式 version snapshot 保护后执行本地变更。Project board 是本地 project workbench，不是 remote Todo provider；`project board plan --save` 写 review snapshot，weekly planning 可以读取 board counts，但不会自动把全部 item 写入外部 task system。

未来 Web/Open Design 工作只是同一套 local projection 上的客户端合同，不是真源替代，也不表示当前 CLI 已实现 Web UI。Kanban、知识图谱、搜索和无限画布方向见 [Pinax Web 开放设计](./web-open-design.md)，Pinax 侧合同由 OpenSpec `pinax-web-open-design-client-contracts` 跟踪；真正客户端源码必须由未来独立客户端子项目拥有。
