# Pinax Agent 优化设计

- 状态：设计稿（design-only）。本文是**消费者视角的需求清单 + 对活跃 change 的增量**，不与 `pinax-agent-memory-runtime` 平行造轮子；任何实施须遵守 [Roadmap](./roadmap.md) 2026-10-10 前的连续性主线。
- 日期：2026-09-23
- 关联：[agent-memory-runtime proposal](../../openspec/changes/pinax-agent-memory-runtime/proposal.md)、[可信连续性 PRD](./trusted-agent-continuity.md)、[Markdown 预览设计](./markdown-rendered-preview-design.md)、[cache contract](../interfaces/cache-contract.md)

## 1. 目标

让 Agent（Claude Code、Codex、Cursor、DSH 工作台内 agent）对 vault 的每一次读写都满足四条：

1. **便宜**：默认 bounded 读取，token 花在刀刃上。
2. **可提案**：写入门永远走 proposal + proof loop，Agent 只能 propose。
3. **可交接**：会话间上下文来自 context pack，而不是重新解释。
4. **可核验**：产出是引用而非正文粘贴，每次写入有 receipt。

前三条 Pinax 已有大半件；本设计是把它们拧成一致面并补缺口。

## 2. 现状与活跃 change 地图

| 能力 | 现状 | 活跃 change |
| --- | --- | --- |
| bounded 读取（card/detail/context） | 已支持 | — |
| proof loop（plan→snapshot→apply→receipt→restore） | 已支持 | — |
| 只读 MCP | 已支持 | — |
| principal/scope/proposal/handoff/feedback、context compiler | 规划中 | `pinax-agent-memory-runtime` |
| inbox 判断 | 进行中 | `pinax-inbox-judgment-v1`、`pinax-real-judgment-acceptance-v1` |
| 连续性 dogfood | 进行中 | `pinax-trusted-continuity-dogfood-v1` |
| mounted vault | 进行中 | `pinax-mounted-vault-local-v1` |
| DSH pane gated actions（inbox.capture、sync.run） | 已支持 | — |

## 3. 六条优化线

### 3.1 读取经济：card-first 两段式

- 固化读取模式：`search` → cards → 命中后 `note read --display context` → 仅在确需全文时 `--display body`。
- MCP 面按此拆两级工具：`pinax.search_cards`（卡片+锚点）与 `pinax.read_note(display=…)`；`display=body` 是独立工具，进审计。
- 给 context 请求加预算参数（`--budget <tokens>`，agent-memory-runtime 的 context compiler 已计划 budget 编译，这里是对齐而非新增）。
- 度量：`card_hit_rate`（cards 命中后不再读 body 的比例）与 `body_read_ratio` 进入 dogfood 指标；不达标说明卡片字段不够用，先改卡片再谈加预算。

### 3.2 提案写入：propose-only 面补齐 note 域

agent-memory-runtime 已规划 `pinax.agent.*` proposal 面（memory 生命周期）；本设计补的是**note 域**的同一纪律：

| proposal | 风险级 | 路径 |
| --- | --- | --- |
| `propose_inbox_capture` | 低 | 映射既有 gated action `inbox.capture`，receipt 即回执 |
| `propose_journal_append` | 低 | proof loop 轻量路径 |
| `propose_note_draft` | 低 | 新建 draft，不碰既有 note |
| `propose_note_edit` | 中 | target `object_id` + `expected revision` + 结构化 diff/正文 + rationale；plan_stale 拒绝漂移 |
| `propose_organize/repair` | 高 | 既有 plan → 人工审批 → apply，不变 |

规则：中风险以上需用户确认；自动 apply 只允许低风险清单；所有 proposal 过 `RejectHandwrittenMetadata`；失败回 restore 已有路径。**Agent 永远拿不到绕过 proof gate 的直写工具。**

### 3.3 Context pack 标准化

- 单入口 `pinax agent context --task "<desc>" [--budget N]`：编译 = 目标 note cards + backlink/graph 邻域 + project board 状态 + 近期 ledger 摘要 + continuity checkpoint（continue contract）。
- pack 带 `revision` 与 `freshness`：revision 未变可整体复用（挂 [cache contract](../interfaces/cache-contract.md)），stale 项显式标注。
- 与 agent-memory-runtime 的 context compiler **同一实现**；本条是 CLI 消费出口的约束，不是第二个 compiler。

### 3.4 引用互通：产出引用而非正文

- Agent 任务产出统一记 `pinax://note/<object_id>?v=<revision>`（格式见[预览设计 §5](./markdown-rendered-preview-design.md)），消费方按需解析卡片——省 token 且可核验。
- 资产闭环：agent 经 DriveBridge 上传文件 → note 内嵌 `drivebridge://file/<id>` → 渲染面显示状态卡。Pinax 与 DriveBridge 引用体系互不代理、互不感知对方内部状态。
- DSH pane gated actions 在 P2（预览设计）时评估增加 `note.create_draft`，仍走 receipt。

### 3.5 可观测与验收

沿用连续性 dogfood 的硬指标（trusted rate ≥80%、source resolvability ≥95%、每周 review ≤5 分钟、silent confirmed writes = 0），追加四项 agent 经济指标：

| 指标 | 定义 | 目标线（试运行） |
| --- | --- | --- |
| `card_hit_rate` | cards 命中后未读 body 的占比 | ≥60% |
| `context_pack_reuse` | revision 未变复用 pack 的占比 | ≥40% |
| `proposal_accept_rate` | 用户确认通过的比例（过低=proposal 质量，过高=确认形同虚设） | 50–90% 区间外都要复盘 |
| `plan_stale_rate` | note_edit proposal 因漂移被拒占比 | 持续走高说明并发写热点，需要提示用户拆任务 |

### 3.6 技能路由与合同纪律

- 新工具面落地的同一 change 内更新 `pinax-agent-router` 路由表（skill frontmatter 变更走 evolutionary-change-policy 门）。
- 所有新 CLI/MCP 字段先落 `cli-output-contract`（`--agent`/`--json`/`--events`/`--explain` 四模一致）再放量。
- proposal 漂移时的失败文案必须给"下一步动作"（重新读 card → 重新 propose），不是裸错误码。

## 4. 与活跃 change 的关系

| 本设计条目 | 归属 | 动作 |
| --- | --- | --- |
| 3.1 / 3.3 | `pinax-agent-memory-runtime` | 以需求附录并入该 change 的 context compiler 与 MCP 面任务，不另立 |
| 3.2 note 域 proposal | 新 change `pinax-agent-note-proposal-v1`（10-10 后） | additive MCP/CLI，复用 proof loop |
| 3.4 | 预览设计 P1 的 `pinax://` change | 同一 change 内带上 `--agent` 出口 |
| 3.5 | dogfood change | 指标进 evidence profile，不改合同 |
| 3.6 | 各 change 收尾项 | 检查清单化 |

## 5. 非目标

- 不开放 agent 直接写文件或手写结构化元数据的任何通道。
- 不做 agent 自主审批（确认永远来自用户或明确的受信规则，不做"agent 批 agent"）。
- 不在本设计内新增 provider/向量集成（roadmap 明确暂停）。
- 不改动既有 `memory`/`brain` 稳定面；收敛另立 change。

## 6. 开放问题

1. 低风险自动 apply 的清单是否需要 per-vault 配置（`agent_auto_apply` 策略键）还是全局硬编码——建议首版硬编码 + 显式错误，配置面留到有真实需求。
2. `proposal_accept_rate` 过高时如何区分"proposal 真的好"与"用户疲劳性确认"——可能需要抽查式 review 而非全量确认。
3. context pack 是否进 Capsa 同步（跨设备缓存）——涉及密文边界，暂缓。
