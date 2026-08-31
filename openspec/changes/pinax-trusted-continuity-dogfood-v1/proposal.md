## Why

Pinax 已有 experimental `pinax continue`、handoff、proposal/review 和 provider-neutral memory runtime，但真实 `yeisme-notes` 中尚未建立 `project:pinax` 连续性闭环：用户切换 Codex 与 Claude Code 时仍需手工指定 vault/scope、重新解释进度，也没有可信结果反馈能证明这套体验值得继续投入。现在需要冻结功能扩张六周，把已有底层能力收敛为“继续这个项目”这一条可重复、可核验、无静默长期写入的个人工作流。

## What Changes

- 新增 user-level repository binding：由 CLI/service 将当前 Git repository/worktree 映射到一个已注册 vault 和 bounded scope；禁止默认跨 vault 搜索，缺失或歧义时返回明确状态与唯一下一步。
- 修改 experimental `pinax continue`：保留所有显式参数的既有语义；仅当当前 repository 已显式绑定时，无参数调用才自动解析 vault/scope，并生成面向人的紧凑 Resume Card。
- Resume Card 固定展示 objective、current state、key decisions、blockers/conflicts、一个 recommended next action，以及真实 evidence freshness/source status；stale、conflict、missing 必须可见，不能伪装为已验证事实。
- 新增 additive `pinax continue checkpoint`，在干净结束、暂停或切换 Agent 时创建 bounded handoff；决定、偏好和可复用经验只能生成 proposal，不能自动成为 confirmed memory，也不能保存 transcript、raw prompt 或 chain-of-thought。
- 新增 opt-in continuity run receipt、四值 outcome feedback（`trusted|corrected|wrong_project|insufficient`）和本地报告；旧的只读 `continue` 调用默认不新增遥测写入，Codex/Claude operator 通过显式 `--record-run` 使用该能力。
- 修改 Memory Inbox 产品节奏：影响当前继续工作的 pending item 可以 inline 提示，其余进入每周 review；报告记录 review 用时，目标为每周不超过 5 分钟。
- 首轮 dogfood 仅使用 Pinax repository，但覆盖 implementation/debugging、product/spec/docs、release/operations 三类真实任务，并同时使用 Codex 与 Claude Code。Go 门槛为至少 30 个 continuity loops、trusted rate ≥80%、source resolvability ≥95%、silent durable writes = 0、每周 review ≤5 分钟。
- 明确记录单仓库样本的局限：`wrong_project=0` 不能证明跨项目自动路由；跨项目 binding/routing 仍是未验证能力，不得据此宣称成熟。
- 现有 action-capture canary 继续完成 2026-08-23 至 2026-09-05 的固定观察窗口并产出 Go/Iterate/Stop receipt，随后归档；本 change 不新增飞书/task provider、稳定 `intent=action_capture` 或其他命令面。
- 全部 CLI、JSON、`--agent`、stored schema 和配置均采用 additive experimental 演进；本 change 没有 breaking change，不弃用现有 `agent`、`memory`、`brain`、`continue` 或 `review` 合同。

## Capabilities

### New Capabilities

- `continuity-workspace-binding`: 当前 Git repository/worktree 到已注册 vault 与 bounded scope 的 user-level、CLI-authored 绑定、解析、诊断和安全回退合同。
- `continuity-dogfood-evidence`: opt-in run receipt、四值用户反馈、任务分类、review 时长、六周指标报告和 Go/Iterate/Stop 门禁。

### Modified Capabilities

- `agent-continuity-experience`: 增加自动绑定解析、Resume Card、可信 freshness/source 状态、bounded checkpoint 和 personal-first continuation workflow。
- `memory-review-inbox`: 增加与当前 continuation 相关的 inline 提示、其余项目的每周 review 节奏和用时证据，保持 canonical lifecycle/proof service 不变。

## Impact

- CLI/output：`internal/cli/continue_review_cmd.go`、`internal/output/` 和 command contract tests；所有新增 flag、subcommand、facts、actions 与 data fields 均为 optional/additive，继续标记 `experimental=true`。
- Application/domain：新增 binding resolution、checkpoint orchestration、continuity receipt/report service；复用现有 handoff、proposal、review、source resolver 和 output projection，不复制 memory ranking/lifecycle。
- Persistence/config：binding 使用 user-level versioned registry 并由 CLI/service 写入；continuity receipt/report 使用 GORM additive table/index，不硬编码 SQL，不复制正文或 source body。
- Source evidence：扩展 bounded repository evidence resolution，并以真实 evidence 时间计算 freshness；缺失、stale、ambiguous 分开计数。
- Split owner：Codex 与 Claude Code 的自然语言触发、安装和 CLI 调用编排归根仓库 `.skills/yeisme/pinax-agent/`，通过生成后的 `.agents/skills` 与 `.claude/skills` 消费 Pinax 合同；Pinax core 不嵌入 runtime-specific plugin。
- Tests/evidence：补充 Go unit、command contract、testscript/process e2e、Codex/Claude fixture adapter 和真实 dogfood evidence；integration/e2e 运行写入 `temp/integration-test-runs/<run-id>/`。
- Maturity：`pinax-agent-memory-runtime` 现有 Codex + Ordo 四周 stable gate 不变。本 change 只验证 personal continuity UX，不能替代或关闭该 runtime gate。
