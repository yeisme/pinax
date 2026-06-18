# Pinax 定位重写：Agent-Safe Knowledge Control Plane

## Why

当前 Pinax README 和产品文档把项目定义为"local-first Markdown notes CLI"，这是一个实现形态描述，不是用户购买理由。用户不会因为这个描述选择 Pinax 而非 Obsidian/Logseq/Notion。

真正的用户痛点是：我有一个真实、本地、私密的 Markdown 知识库，想让 AI 帮我整理/检索/修复/同步，但不敢让 AI 直接改文件、不敢把明文交给云、不想被托管平台锁死。

CEO review（2026-06-17）建议把定位从"笔记 CLI"收窄成"agent-safe proof loop for your Markdown vault"：所有首屏、README、demo 围绕一个场景——agent 安全诊断、整理、同步真实 vault，每次写入都有 plan、snapshot、receipt、restore。

## What changes

1. 重写 `README.md` 第一屏：从"local-first Markdown notes CLI"改为"agent-safe knowledge control plane for your Markdown vault"
2. 重写 `docs/overview/product-positioning.md`：收窄定位、明确差异化（agent-safe proof loop）、明确竞品关系（complements Obsidian/Logseq, avoids Notion lock-in）
3. 重写 `docs/README.md` 首页：突出 proof loop 主线
4. 新增 `docs/overview/agent-safe-boundary.md`：解释 plaintext boundary、MCP bounded context、Cloud no-exec/no-plaintext invariant
5. 更新 `README.zh-CN.md` 保持中英文同步
6. 压缩信息架构：把产品复杂度收成三个可复述概念（Local Vault 是真源 / Proof Loop 保护 agent 写入 / Cloud Sync 只协调密文）
7. 不改命令行为、不改协议字段、不改代码——纯文档/定位变更

## Out of scope

- 改变任何 CLI 命令行为或输出合同
- 新增命令或 flag
- 改变 OpenSpec spec delta 中的行为要求
- 翻译内部中文开发文档

## Impact

- `cli/pinax/README.md`
- `cli/pinax/README.zh-CN.md`
- `cli/pinax/docs/README.md`
- `cli/pinax/docs/overview/product-positioning.md`
- 新增 `cli/pinax/docs/overview/agent-safe-boundary.md`
- OpenSpec `pinax` spec（定位描述 delta）
