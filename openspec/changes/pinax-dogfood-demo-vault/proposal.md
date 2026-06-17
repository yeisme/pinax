# Pinax Dogfood Demo Vault

## Why

CEO review（2026-06-17）建议把 proof loop demo 做成唯一主线，作为销售、文档、测试和用户访谈的共同核心。需要一个 fixture messy vault 演示完整 aha moment：

> agent 整理真实 vault → Pinax 先给 plan/snapshot/receipt → agent 看不到不该看的明文 → 确认后应用 → 错了可以恢复

当前没有标准 demo vault，每次展示都需要手动创建测试数据。

## What changes

1. 创建 `examples/messy-vault/` 包含有意制造的问题（broken links、orphan notes、duplicate titles、missing metadata、empty notes、stale notes）
2. 创建 `examples/messy-vault/README.md` 说明 demo 场景和预期 proof loop 行为
3. 添加 testscript E2E 测试验证 demo vault 能跑通完整 proof loop（diagnose → plan → snapshot → apply → restore）
4. 创建 `docs/demo-proof-loop.md` 记录 demo 脚本和讲解要点
5. 确保 demo vault 不含真实敏感数据、不含 provider credentials

## Out of scope

- Cloud Sync demo（preview 阶段非主线）
- MCP server demo（单独文档/视频）
- 自动化视频/动画录制
- 真实 Obsidian vault 导入

## Impact

- `cli/pinax/examples/messy-vault/`（新增 fixture vault）
- `cli/pinax/examples/messy-vault/README.md`（新增）
- `cli/pinax/tests/e2e/demo_proof_loop_test.go`（新增 testscript）
- `cli/pinax/tests/e2e/testdata/demo/scripts/`（新增 testscript 文件）
- `cli/pinax/docs/demo-proof-loop.md`（新增）
- OpenSpec `pinax` spec（demo vault 行为 delta）
