## Why

原 `pinax-drivebridge-workbench-v1` 将消费者和 CLI 自身发布资格混写。客户端任务迁回 DriveBridge 后，云同步、Obsidian 兼容与恢复仍须由 Pinax 保留，不能丢弃或交给客户端。

## What Changes

- 原样接收 R02/R03 的 pending 验收，不降低门槛，不新建同步实现。
- 将真实 endpoint/跨设备/Capsa C3 与真实 vault/Proof Loop/恢复分开记录。
- 只由 Pinax 的公开 CLI 和证据入口验收；DriveBridge/Electron、UI、安装包和消费者采用不作为本 change 通过条件。

## Capabilities

### New Capabilities

- `pinax-release-evidence-boundary`: 独立保存 Pinax 自身的真实同步与 vault 资格。

### Modified Capabilities

无。现有合同和 Preview/experimental 等级保持。

## Impact

只接收任务、说明和证据链接；无运行时、凭据、真实 vault 或远端状态修改。
