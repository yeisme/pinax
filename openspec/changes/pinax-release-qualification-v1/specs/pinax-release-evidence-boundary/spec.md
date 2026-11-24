## ADDED Requirements

### Requirement: Pinax 真实发布证据独立保存

Pinax SHALL 保留真实 Cloud Sync、跨设备、Capsa 合同、Obsidian vault 兼容与 Proof Loop restore 的 pending 验收，且 SHALL NOT 以客户端发布作为自身工程完成门。

#### Scenario: 接收混合任务

- **WHEN** 原客户端混合 change 的 R02/R03 迁入
- **THEN** 保留原未完成状态和双向出处，只有原定义的真实证据齐全才能勾选

#### Scenario: 本地回归通过

- **WHEN** fixture、本地远端和恢复合同通过
- **THEN** 只记录本地工程结果，真实平台、用户观察和云端事实继续保持 pending/unknown
