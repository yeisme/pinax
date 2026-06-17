# Tasks: Pinax Agent-Safe Positioning Rewrite

Owner: `cli/pinax`  
Priority: P1  
Execution rule: 纯文档变更，不改代码行为。完成后 `task check` 应无回归。

## 0. 基线

- [ ] 0.1 Owner: `cli/pinax`; Lane: sequential; Depends on: none; Scope: 记录当前 README/product-positioning/docs README 的定位措辞基线。Validation: `openspec validate pinax-agent-safe-positioning-rewrite --strict`. Expected: 变更校验通过。Failure re-check: 不混合代码变更。
- [ ] 0.2 Owner: `cli/pinax`; Lane: A; Depends on: 0.1; Scope: 确认 CEO review 核心叙事已记录在 design.md 中，三概念（Local Vault 真源 / Proof Loop 保护 / Cloud Sync 密文）和竞品关系表已定义。Validation: 检查 design.md 内容完整性。Expected: design.md 包含叙事、三概念、竞品表、README 结构、boundary 文档大纲。

## 1. README 重写

- [ ] 1.1 Owner: `cli/pinax`; Lane: A; Depends on: 0.2; Scope: 重写 `README.md` 第一屏：一句话定位 + agent-safe proof loop 场景 + the aha moment 代码块（proof loop run → plan → snapshot → apply → restore）。Acceptance: 第一屏 30 秒内可理解核心价值；所有命令可运行。Validation: 人工审阅。Expected: 第一屏不再是功能列表。
- [ ] 1.2 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: 在 README 新增 "Why Pinax" 段落：三个差异化点（proof loop 安全写入、plaintext boundary、self-hosted encrypted sync）+ 竞品互补关系一句话。Acceptance: 不与 Obsidian/Notion 正面对比功能清单。Validation: 人工审阅。Expected: 定位清晰互补。
- [ ] 1.3 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: 把现有 five core workflows 和 detailed workflows 下沉为 README H2 段落，保留所有命令示例不变。Acceptance: 命令示例未被修改或删除。Validation: `grep -c 'pinax ' README.md` 前后一致。Expected: 命令数量不减少。
- [ ] 1.4 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: 同步更新 `README.zh-CN.md` 保持中文版定位一致。Acceptance: 中英文第一屏叙事一致。Validation: 人工审阅。Expected: 双语同步。

## 2. 产品定位文档

- [ ] 2.1 Owner: `cli/pinax`; Lane: B; Depends on: 0.2; Scope: 重写 `docs/overview/product-positioning.md`：一句话定位改为 agent-safe knowledge control plane，明确目标用户（AI-heavy 开发者、隐私敏感技术人、Obsidian 工程派、自托管小团队），明确 non-goals 不变。Acceptance: 不引入 Notion/collaboration/web/mobile 方向。Validation: `openspec validate pinax-agent-safe-positioning-rewrite --strict`。Expected: 校验通过。
- [ ] 2.2 Owner: `cli/pinax`; Lane: B; Depends on: 2.1; Scope: 新增 `docs/overview/agent-safe-boundary.md`：解释 plaintext boundary（CLI 默认不泄露 full body）、MCP bounded context、Cloud no-exec/no-plaintext invariant、encrypted envelope、proof loop 写入控制链。Acceptance: 文档只描述已实现行为，不承诺未实现能力。Validation: 人工审阅 + 交叉验证 capabilities response 字段。Expected: 文档与代码一致。

## 3. 文档首页

- [ ] 3.1 Owner: `cli/pinax`; Lane: B; Depends on: 1.1; Scope: 更新 `docs/README.md` 首页：突出 proof loop 主线，链接到新的 agent-safe-boundary 文档。Acceptance: 首页引导读者理解 proof loop 而非功能清单。Validation: 人工审阅。Expected: 主线清晰。

## 4. 验证

- [ ] 4.1 Owner: `cli/pinax`; Lane: sequential; Depends on: all previous; Scope: 运行 `task check` 确认文档变更无代码回归。Acceptance: lint/test/build/openspec 全绿。Validation: `task check`。Expected: 36/36 openspec, 0 lint issues, tests pass, build success。
- [ ] 4.2 Owner: `cli/pinax`; Lane: sequential; Depends on: 4.1; Scope: 运行 `openspec validate pinax-agent-safe-positioning-rewrite --strict` 和 `openspec validate --all --strict`。Acceptance: 校验通过。Validation: 同上。Expected: 全部通过。
