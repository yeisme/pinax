# Spec Delta: Pinax Demo Vault

## ADDED Requirements



### Requirement: Dogfood Demo Vault

Pinax SHALL 提供一个标准 synthetic demo vault fixture，用于 proof loop 演示和 E2E 测试。

#### Scenario: Demo vault 包含可诊断问题

- **GIVEN** `examples/messy-vault/` 存在且包含故意制造的 6 类问题
- **WHEN** 运行 `pinax vault doctor --vault ./examples/messy-vault --json`
- **THEN** 诊断结果包含 broken_links、orphan_notes、missing_metadata、duplicate_titles、empty_notes、stale_notes

#### Scenario: Demo vault proof loop 可完整运行

- **GIVEN** demo vault fixture
- **WHEN** 按顺序执行 diagnose → plan --save → snapshot → apply --yes → restore
- **THEN** 低风险操作（metadata 补全）成功应用
- **AND** manual review 项（broken/orphan/duplicate/empty/stale）不变
- **AND** restore 后文件内容回到 apply 前状态

#### Scenario: Demo vault 不含敏感数据

- **GIVEN** demo vault fixture
- **WHEN** 扫描全部文件
- **THEN** 不包含真实人名、项目名、credentials、tokens、webhook URL
- **AND** `.pinax/config.yaml` 只包含最小配置
