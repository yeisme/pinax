# Hermes × Pinax 个人 Action Capture Canary

本运行手册用于验证一个严格限界的个人效率闭环：用户在 Hermes 中表达行动，Pinax 提供只读、有来源的项目上下文，Hermes 显示预览并等待明确确认，外部任务系统负责真正写入和状态管理。

这不是 Pinax 的飞书集成。Pinax 不保存 task ID 镜像，不同步负责人、截止时间或状态，也不解析飞书 payload。

## 系统边界

| 系统 | 负责 | 不负责 |
|---|---|---|
| Pinax | bounded context、source refs、memory、handoff、proposal/review | provider SDK、任务创建、任务状态同步 |
| Hermes | 对话、行动提取、预览、确认、工具编排 | 项目决定和执行经验的长期事实源 |
| Hermes 网关或 `lark-cli` | 确认后的外部任务写入和状态事实 | Pinax 长期记忆 |

## 注册只读 Pinax MCP

先找到实际 vault：

```bash
pinax vault list --json
```

在当前 Hermes profile 中注册 stdio MCP；把 `/absolute/path/to/vault` 换成真实绝对路径：

```bash
hermes mcp add pinax --command pinax --args mcp serve --vault /absolute/path/to/vault
hermes mcp test pinax
```

Canary 只需要三个 Pinax 工具。禁用同一 MCP server 的其他工具：

```bash
hermes tools disable --platform cli \
  pinax:pinax.search \
  pinax:pinax.brain.context \
  pinax:pinax.brain.answer \
  pinax:pinax.brain.sources \
  pinax:pinax.brain.maintenance_plan \
  pinax:pinax.query.run \
  pinax:pinax.database.view.show \
  pinax:pinax.database.view.render \
  pinax:pinax.note.read \
  pinax:pinax.note.links \
  pinax:pinax.note.backlinks \
  pinax:pinax.note.context \
  pinax:pinax.vault.graph_summary \
  pinax:pinax.project.board \
  pinax:pinax.task.adopt_plan \
  pinax:pinax.organize.plan \
  pinax:pinax.git.snapshot_plan
```

确认剩余 MCP 工具为：

- `pinax.agent.context`
- `pinax.agent.memory_recall`
- `pinax.agent.handoff_read`

检查实际过滤结果：

```bash
hermes tools list --platform cli
hermes tools list --platform feishu
```

## 记忆治理

Hermes memory 只保留用户偏好、交互习惯和工具路由。项目决定、执行经验、约束和重要结果以 Pinax 为准，并必须经过 proposal/review。

启用 Hermes memory 写入审批：

```bash
hermes config set memory.write_approval true
hermes config get memory.write_approval
```

期望输出为 `true`。

## Action Capture Skill

个人 Skill 位于 active profile：

```text
$HOME/.hermes/profiles/personal/skills/pinax-action-capture/
```

用 Hermes 检查是否被发现：

```bash
hermes skills list
```

Skill 的固定行为是：

1. 读取三个 Pinax 只读工具的最小必要上下文。
2. 显示标题、可观察结果、背景、建议截止时间、约束、来源、缺失信息和 `awaiting confirmation`。
3. Pinax 不可用时标记 `Pinax context: unavailable` 或 `Source status: ungrounded`。
4. 修改预览不等于授权；只有明确确认才能调用已配置的外部任务工具。
5. 完成结果只有通过 Pinax proposal/review 才能成为长期记忆。

## Canary Recorder

`pinax-action-canary` 是 Hermes 个人适配层的本地 CLI，不是 Pinax 稳定产品命令。它通过 CLI 创建 JSON/JSONL，避免 Agent 手写结构化资产，只保存离散指标与外部 reference digest。

初始化固定两周窗口：

```bash
pinax-action-canary init --start-date 2026-08-10 --days 14
pinax-action-canary self-test
```

每次用户表达行动时先开始计时：

```bash
pinax-action-canary begin
```

得到 `preview_id` 后，在首次预览显示时记录来源与缺失状态：

```bash
pinax-action-canary preview \
  --preview-id preview_example \
  --source-total 2 \
  --source-openable 2 \
  --context-status grounded \
  --modifications 0 \
  --missing-information no
```

每次用户要求一次实质修改，追加 revision：

```bash
pinax-action-canary revise --preview-id preview_example
```

明确确认并由外部工具返回结果后记录写入；`--external-ref` 在落盘前会被哈希：

使用 `lark-cli` 时，先对确认后的字段做 dry-run，并使用 `preview_id` 作为 idempotency key：

```bash
LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1 \
LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1 \
lark-cli auth status --json --verify

lark-cli task +create \
  --as user \
  --summary "<confirmed title>" \
  --description "<bounded confirmed context>" \
  --idempotency-key preview_example \
  --dry-run \
  --json
```

如果用户表达“给我创建任务”，必须先从 `lark-cli auth status` 取得当前用户的 `openId`，并在确认后的 create 命令中加 `--assignee <open_id>`。审阅 dry-run 后，只有用户再次明确确认，才可去掉 `--dry-run` 执行真实写入。

外部工具返回结果后，再记录 canary 事件：

```bash
pinax-action-canary create \
  --preview-id preview_example \
  --confirmed yes \
  --result created \
  --external-ref external-task-reference \
  --duplicate no \
  --erroneous no
```

若用户拒绝创建：

```bash
pinax-action-canary create \
  --preview-id preview_example \
  --confirmed no \
  --result not_created
```

Recorder 会拒绝 `confirmed=no` 且 `result=created` 的事件。

重要任务完成并产生 Pinax 提案后记录：

```bash
pinax-action-canary completion \
  --preview-id preview_example \
  --important yes \
  --proposal-generated yes

pinax-action-canary review \
  --preview-id preview_example \
  --result accepted
```

普通执行噪声使用 `--important no --proposal-generated no`，不得为了提高指标而生成长期记忆。

## 报告与决策

```bash
pinax-action-canary report --week 1 --json
pinax-action-canary report --week 2 --json
pinax-action-canary report --json
```

报告必须同时给出分母和 known bias。决策门槛：

- 至少 70% 预览只需零次或一次实质修改。
- 至少 90% Pinax 来源可打开。
- 未确认即创建任务为 0。
- 重复或错误任务不超过 5%。
- 预览延迟中位数低于 30 秒。
- 至少 50% 重要完成任务形成有价值且通过审阅的提案。
- 每周至少四天主动使用。

北极星是 `trusted_action_loops`，即完成并经审阅沉淀为可信记忆的行动闭环数。任务数、命令量和集成数不是成功指标。

## 分层诊断

遇到失败时依次检查，避免把不同层的问题混为一谈：

```bash
hermes mcp test pinax
hermes skills list
pinax-action-canary self-test
```

以上通过后，再分别检查 Hermes 模型 inference 和外部任务 writer。MCP 连接成功但对话超时，不等于 Pinax context 失败；外部 writer 未配置也不应阻塞普通的未落地任务预览。

## 回滚

```bash
hermes mcp remove pinax
```

随后禁用或删除用户级 `pinax-action-capture` Skill 和 `pinax-action-canary` 本地命令即可。回滚不修改 Pinax vault，也不删除或关闭任何已存在的外部任务。
