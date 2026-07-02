# Pinax 首次用户 Proof Loop Readiness

## 为什么

Pinax 已经具备本地 vault、bounded projection、repair/organize plan、version snapshot、restore、integration evidence 和 release packaging 等基础能力。当前最大问题不是缺功能，而是外部用户无法在 5 分钟内稳定复现一个可信闭环：安装 Pinax，连接真实 Markdown vault，预览 agent-safe proof loop，保存计划，创建快照，应用低风险修复，并证明可回滚。

如果继续横向扩展 Cloud Sync、插件、KB、Publish 或 Planning，产品心智会被稀释。下一步应把已有能力收束成一个可安装、可演示、可测试、可发布的黄金路径。

## 做什么

本变更将 Pinax 的首次用户体验固化为一个可验证的 Proof Loop readiness 交付包：

1. 收口当前进行中的 TaskBridge daily todolist 和 release 文档改动，避免在脏工作区上叠加主线。
2. 建立 deterministic demo vault，稳定包含可诊断、可计划、可修复、需人工 review、可回滚的样例问题。
3. 固化黄金路径命令，覆盖 `init -> note/inbox/journal -> proof loop preview -> repair plan -> snapshot -> apply -> restore`。
4. 强化 JSON、agent、events、stderr、receipt、evidence 的 body-leak 和 secret redaction 合同。
5. 确保 `task test:integration` 生成项目本地 redacted evidence。
6. 增加 release archive 安装后的 smoke 验证，证明用户不需要源码开发环境也能跑通核心路径。

## 不做什么

- 不新增 Cloud Sync transport 或 Pinax Cloud 后端能力。
- 不新增插件 runner、Publish target、KB provider、Memory recall 算法或 Dashboard UI。
- 不把 TaskBridge daily planning 扩展为新的产品主线，只收口已有进行中改动。
- 不绕过 CLI/application service 手写 `.pinax/**` 结构化资产。
- 不把 README 变成全功能手册，README 只承载黄金路径和差异化定位。

## 用户结果

新用户完成安装后，应能运行真实命令并看到三件事：

1. Pinax 读取输出默认是 bounded projection，不泄漏完整 note body。
2. Pinax 写入必须经过 plan、snapshot、receipt 和显式确认。
3. Pinax 写坏后能通过受控 restore 路径恢复，而不是靠手工文件手术。

## 成功标准

- `openspec validate pinax-first-user-proof-loop-readiness --strict` 通过。
- `task check` 通过。
- `task test:integration` 通过，并在 `temp/integration-test-runs/<run-id>/` 写入 redacted evidence。
- release archive smoke 能跑通 `pinax version`、`pinax init`、`pinax note add`、`pinax proof loop run --json`。
- README 和 quickstart 首屏优先展示 Proof Loop 黄金路径，Cloud/Plugin/Publish/KB 等高级能力下沉到文档入口。

