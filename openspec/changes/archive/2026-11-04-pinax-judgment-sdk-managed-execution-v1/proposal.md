# Why

接入公共判断 SDK 的进程内执行与显式配置同步，保持 pinax 的领域边界和现有行为。

# What Changes

- 复用 Aigora judgment-sdk v1；宿主执行、管理端只维护非秘密修订和摘要。
- 收件箱分类和整理建议；vault 写入、tag、move 仍经 owner 保护门。
- 默认 off；只有显式 shadow/assist 才执行。源码实现和验收全部归本 owner。

# Capabilities

## New Capabilities

- `pinax-managed-judgment`: 本项目判断集成与恢复合同。

# Impact

入口 `internal/inboxjudgment`。不部署、不执行付费 Provider、不迁移真实数据。跨项目依赖为 Aigora SDK/管理合同；共享凭据需显式授权，不自动回退。
