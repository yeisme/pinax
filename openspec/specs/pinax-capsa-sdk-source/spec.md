# pinax-capsa-sdk-source Specification
## Purpose
TBD - created by archiving change pinax-capsa-sdk-source-migration. Update Purpose after archive.
## Requirements
### Requirement: Pinax SHALL 仅切换 Capsa local replace source

Pinax SHALL 保持 public module path `github.com/yeisme/capsa` 和既有 Go import；其本地 replace SHALL 指向 `../../backend-server/capsa/sdk`，且 SHALL NOT 指向 `../../shared/capsa`。

#### Scenario: 本地开发解析 SDK

- **WHEN** Pinax 编译使用 Capsa SDK 的命令或包
- **THEN** Go module SHALL 从 `backend-server/capsa/sdk` 解析 SDK source
- **AND** import path SHALL 保持 `github.com/yeisme/capsa`。

### Requirement: Pinax SHALL 保持同步运行时合同

本 source migration SHALL NOT 改变 wire/schema、crypto salt、state paths、daemon naming 或 vault 同步、设备和加密合同。

#### Scenario: 既有本地状态继续运行

- **WHEN** 用户在 source migration 后执行既有 vault 同步
- **THEN** 系统 SHALL 使用相同的协议、加密与状态路径语义
- **AND** SHALL NOT 要求因 SDK source 变化改名 daemon 或迁移状态。

### Requirement: Pinax legacy salt SHALL 要求显式 re-push

对使用 `pinax-cloud-sync-salt-v1` 写入的既有远端对象，Pinax SHALL 将 source migration 与数据重新推送区分；Capsa SDK 使用 `capsa-sync-salt-v1` 时，重新推送 SHALL 从完整本地 vault 显式发起。

#### Scenario: legacy 对象需要重新推送

- **GIVEN** 远端对象由旧 Pinax-local crypto path 写入
- **WHEN** 用户迁移到 canonical Capsa SDK source
- **THEN** 系统 SHALL NOT 静默重写远端对象或删除本地状态
- **AND** 用户可从完整本地 vault 执行 `pinax sync push --target capsa --yes` 进行显式 re-push。

### Requirement: Cleanup 与 Scaena SHALL 延后处理

Pinax SHALL NOT 在本 change 删除 `shared/capsa`；Scaena SHALL 通过独立 consumer change 迁移。

#### Scenario: Pinax change 完成

- **WHEN** Pinax local replace 已迁移并通过验证
- **THEN** `shared/capsa` SHALL 继续保留
- **AND** 当前 dirty worktree SHALL NOT 被回滚。
