# go-dev-toolchain Specification

## Purpose
TBD - created by archiving change pinax-go-dev-toolchain. Update Purpose after archive.
## Requirements
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

### Requirement: Pinax GitHub CI mirrors the local quality gate

Pinax SHALL provide a GitHub Actions CI workflow for repository changes that touch the Pinax subproject or its workflow definition, and the workflow SHALL run from the Pinax subproject working directory using the same quality gate expected locally.

#### Scenario: CI runs the Pinax check gate for subproject changes

- **GIVEN** a push or pull request changes files under `cli/pinax/**` or the Pinax CI workflow file
- **WHEN** the Pinax CI workflow runs
- **THEN** it SHALL set up Go, Task, golangci-lint, and OpenSpec dependencies
- **AND** it SHALL run the Pinax quality gate that covers formatting, linting, tests, build, and `openspec validate --all`
- **AND** it SHALL NOT require provider credentials, user vaults, or network calls to external note providers.

#### Scenario: CI scope stays inside the Pinax subproject

- **GIVEN** the workflow is triggered from the repository root
- **WHEN** it runs Pinax commands
- **THEN** command execution SHALL use `cli/pinax` as the working directory
- **AND** generated build artifacts, coverage files, and test evidence SHALL NOT be committed by the workflow.

### Requirement: Pinax release workflow uses Pinax tags

Pinax SHALL provide a GitHub Actions release workflow that is triggered only by Pinax release tags and builds release artifacts through the Pinax Go CLI release toolchain.

#### Scenario: Release workflow triggers on Pinax semantic tags

- **GIVEN** a tag matching `pinax/v*.*.*` is pushed
- **WHEN** the Pinax release workflow runs
- **THEN** it SHALL set up the Go release environment
- **AND** it SHALL run a release check or packaging step for the Pinax CLI
- **AND** it SHALL keep non-Pinax tags from triggering Pinax release packaging.

#### Scenario: Release workflow separates validation from publishing

- **GIVEN** the release workflow is being changed or reviewed
- **WHEN** validation is required without publishing a real release
- **THEN** the workflow SHALL support a check or snapshot path that validates release configuration without publishing artifacts
- **AND** real publishing SHALL require the Pinax release tag trigger.

