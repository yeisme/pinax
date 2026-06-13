## ADDED Requirements

### Requirement: Go 开发工具链标准化落地

本项目 SHALL 按照根 go-dev-toolchain-quality 设计落地 golangci-lint v2 配置、Taskfile 任务语义对齐和可选热加载。

#### Scenario: lint 配置覆盖基线

- **WHEN** 本项目完成 .golangci.yml 配置
- **THEN** 配置 SHALL 覆盖基线 linters（errcheck、govet、ineffassign、staticcheck、unused、misspell、revive）和 formatters（gofmt、goimports）
- **AND** golangci-lint run --new-from-rev=HEAD~ SHALL 退出码 0

#### Scenario: Taskfile 任务语义对齐

- **WHEN** 本项目完成 Taskfile 更新
- **THEN** Taskfile SHALL 提供 deps、mod-check、fmt、fmt-check、lint、test、build、ci/check 任务
- **AND** task ci SHALL 至少执行格式检查、lint、测试和构建

#### Scenario: 项目特有任务保留

- **WHEN** 本项目已有特有任务
- **THEN** 工具链落地 MUST 保留这些特有任务
- **AND** 只允许为命名一致性增加别名或补充说明
