# 产品定位

Pinax 是 **面向 Markdown vault 的 agent-safe 知识控制平面**。它让 AI 可以安全地读取、诊断、修复和同步真实本地知识库，同时保证每一次 agent 写入都可审计、可预览、可回滚。

参考 GBrain 这类 “agent brain layer” 项目后，Pinax 的长期定位可以更明确：Pinax 不只是笔记 CLI，而是给 Claude Code、Codex、Cursor、OpenClaw、Hermes 和本地 MCP client 使用的 **私有知识大脑控制层**。它把 notes、meeting notes、emails/imported markdown、project board、memory ledger、KB projection、link graph、database views 和 proof receipts 组织成 agent 可查询、可引用、可维护的长期上下文。

一句话定位：**Pinax 让 AI 安全操作你的私有知识库，并把它变成可审计的 agent brain；它不是另一个笔记应用，也不是另一个云端 silo。**

Pinax 的 answer layer 必须比普通搜索更进一步，但不能绕过安全边界：搜索返回候选，`memory context` 返回结构化事实，`kb context` 返回语义上下文，graph/query 返回关系证据，最终给 agent 的综合答案必须带来源、置信度、新鲜度和下一步命令，而不是把完整私密正文倾倒给模型。

```bash
pinax import markdown ./source --dry-run --vault ./my-notes --json
pinax index refresh --vault ./my-notes --json
pinax memory context "prepare for Alice meeting" --entity alice --limit 12 --vault ./my-notes --agent
pinax kb context "prepare for Alice meeting" --limit 8 --vault ./my-notes --json
pinax graph query --kind technique --match storyboard --vault ./my-notes --json
pinax mcp serve --vault ./my-notes
```

## 三个可重复概念

1. **Local Vault 是真源**：Markdown 文件永远是真源；SQLite 和 `.pinax/` 是可重建投影。
2. **Proof Loop 保护每次 agent 写入**：Capture -> Retrieve -> Diagnose -> Plan -> Snapshot -> Apply -> Restore。
3. **Share 和 Sync 是表面，不是真源**：publish targets 是生成出来的 delivery artifacts；Cloud Sync 协调 encrypted revisions，不保存明文 note，也不执行本地工具。
4. **Answer Synthesis 是受控 projection，不是自由聊天**：agent 可以请求“明天见 Alice 前我需要知道什么”，但 Pinax 返回的必须是 bounded answer、引用、过期风险、open tasks 和真实 next command，而不是无来源总结或 raw note dump。

## 目标用户

- **AI-heavy developers**：用 agent 操作真实 Markdown knowledge base，需要每次写入都经过 plan gate、snapshot 保护和可回滚链路。
- **Agent builders / MCP integrators**：需要把 Claude Code、Codex、Cursor、Hermes、OpenClaw 等 agent 接到同一个长期记忆层，同时保持本地权限、引用证据和 write gate。
- **隐私敏感的技术工作者**：不愿把明文笔记交给 hosted platform，希望有 self-hosted encrypted sync。
- **Obsidian engineering power users**：希望在既有 vault 上叠加可编程、agent-safe 的维护和修复层，而不是换一个 note editor。
- **自托管小团队**：需要 portable、auditable 的 Markdown vault 和分布式 encrypted sync，不想采用 hosted collaboration workspace。

## 竞品定位

Pinax 不竞争 note-editing UX 或功能清单。它处在不同层级。

| 竞品 | 关系 | Pinax 不做什么 |
| --- | --- | --- |
| Obsidian | **Complement**：作为既有 vault 的 agent-safe 维护层 | 不替代 editor UI |
| Logseq | **Differentiate**：不复制 outliner/graph UI | 不复制 outliner model |
| Notion | **Avoid**：不做团队协作或 cloud workspace | 不做 cloud lock-in 或 hosted vault |
| Reflect | **Differentiate**：更可编程、可验证 | 不竞争个人 note-taking 手感 |
| GBrain 类 agent brain | **Learn and constrain**：学习 “Search gives raw pages, brain gives cited answers” 的产品层，但坚持 Markdown/local-first/proof loop | 不把 agent synthesis 变成无引用、无权限、无成本提示的 hosted brain |

## 初始重点

- 初始化和验证本地 Markdown vault。
- 通过 proof loop 创建、capture、整理和检索 notes。
- 管理本地 project workspaces、task boards 和 task adoption plans，但不把 Pinax 变成 remote issue tracker。
- 复用 database saved views，把 bounded table、board、list、calendar 和 tab projection 贯穿 CLI、Markdown rendered notes、dashboard、MCP、REST/RPC 和 remote CLI mode。
- Preview Obsidian-style vault：wikilinks/backlinks、properties、daily managed blocks、templates、attachments、dataview blocks、canvas/plugin ignore 和 publish plans，但不拥有 Obsidian plugin state。
- 通过 `pinax version` 管理 note versions、rollback plans 和 changed-path evidence；Git 是可选 backend，不是用户可见工作流名称。
- 通过 Pages、Wiki、Gist、HTTP endpoints 和 loopback preview 等 reviewed publish surfaces 分享本地 notes。
- 通过 CLI-backed Provider adapter 和 local-first Pinax Cloud distributed sync 同步本地文件。
- 通过稳定的 `--agent` / `--json` output 服务 agent workflows。
- 通过 `memory`、`kb context`、`graph query`、`query/dataview` 和 future answer synthesis，把 raw retrieval 升级成带引用的 agent context。
- 未来 Web/Open Design client 可以提供 Kanban、graph、search 和 canvas workbench surfaces，但必须消费 Pinax CLI/API projections，并把写入保持在 proof loop 内。

成熟度标签：

- 成熟：local vault、proof loop、bounded note display、project workspace/board、database saved view render、link/backlink graph projections、asset doctor/repair plan、local REST/RPC route discovery，以及 read-only MCP/dashboard projections。
- 预览：Obsidian compatibility、Cloud Sync transports、Remote API Mode command coverage、publish targets、sync daemon operation、structured memory ledger 和 KB semantic context。
- 实验：answer synthesis、entity-resolution maintenance、provider automation、briefing delivery、dynamic plugin runners 和 hosted/team surfaces。

Cloud Sync 定位：Pinax Cloud 是同步协调器，不是 note 真源。每台用户设备都保留可离线使用的本地 vault；server transport 只保存 encrypted sync artifacts，并排序 revisions，让设备安全收敛。

## 非目标

- 不把长期 daemon 作为 MVP 必需能力。
- 不把 Feishu、Notion 或其他外部平台当作 note 真源。
- 不把 Pinax Cloud 做成集中式明文 note editor 或 hosted vault 真源。
- 不把 “agent brain” 做成默认托管多用户知识库；团队权限、OAuth、rate limit 和 hosted surfaces 必须另有显式设计。
- 默认不直接维护外部平台 native API SDK。
- 不允许 agent 手写 machine-readable metadata。
- 不构建 team collaboration workspace、hosted web/mobile editor 或 Notion-style cloud product。
