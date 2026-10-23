## Why

Pinax 的 continuity 核心属于 `cli/pinax`，但“继续这个项目”这一日常入口发生在 Codex 与 Claude Code 对话中。需要一个根仓库拥有、单一来源生成到两个 runtime 的 operator Skill，既保留自然语言体验，又不让 Pinax core 吸收 runtime-specific plugin、安装或提示编排。

## What Changes

- 新增 `pinax-continuity-operator` self-built source Skill，规范 resume、status/binding recovery、checkpoint、relevant review、weekly review 和四值 outcome feedback 的自然语言编排。
- Skill 只调用 `cli/pinax/openspec/changes/pinax-trusted-continuity-dogfood-v1/` 定义并实现的 provider-neutral CLI contract；不读取 Pinax 内部数据库、不手写 vault/registry metadata、不复制 memory lifecycle。
- 通过 root 与 `cli/pinax` 两个受管 profile 和 `scripts/skills.sh`，将同一 source 生成到对应 workspace 的 Codex `.agents/skills` 与 Claude Code `.claude/skills`，禁止维护多份漂移实现。
- 当 repository binding missing/invalid/ambiguous 时，Skill 最多提出一个关键问题或一个 canonical recovery command；不得扫描所有 vault、猜测 project 或执行未经授权的 project switch。
- Substantial session closeout 只生成 bounded checkpoint；durable decision/preference/lesson 只能调用 Pinax proposal/review，不能静默 confirmed。
- 每个 recorded loop 结束时由用户明确选择 `trusted|corrected|wrong_project|insufficient`；Skill 不根据语气、工具成功或任务完成情况推断 outcome。
- 首轮只 dogfood Pinax repository，覆盖 Codex 与 Claude Code；跨项目 routing 保留为 unvalidated，不在本 change 建新 MCP、provider bridge、团队或公共 client。
- 全部改动为 additive；不重命名已有 skill、profile key、Pinax command 或 output field。

## Capabilities

### New Capabilities

- `pinax-continuity-operator-integration`: Codex 与 Claude Code 共用的 Pinax continuity 自然语言 operator、安装同步、错误恢复、checkpoint/review/feedback 调用与跨 owner 验收合同。

### Modified Capabilities

无。

## Impact

- Root skill source：`.skills/yeisme/pinax-agent/pinax-continuity-operator/`。
- Profile/runtime generation：`.skills/profiles/root.txt` 与 `.skills/profiles/targets/cli/pinax.txt`；生成目标为 root 和 `cli/pinax` 各自的 `.agents/skills/` 与 `.claude/skills/`，不得手改生成目录。
- Validation：`scripts/skills.sh validate-custom`、`validate-profiles`、`sync-root`/等价同步命令和 `validate-runtime`。
- Pinax implementation owner：`cli/pinax/openspec/changes/pinax-trusted-continuity-dogfood-v1/`；本 root change 不追踪其代码任务或测试完成状态，只记录 contract dependency 与跨 runtime acceptance。
- Existing owner contract：继续遵守 `openspec/specs/agent-memory-owner-routing/spec.md` 与 `openspec/specs/general-agent-memory-contract/spec.md`；Pinax canonical memory state 和 lifecycle 不迁移到 Skill。
