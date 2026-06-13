## Why

`cli/pinax` Taskfile 简洁，缺少 lint 配置和热加载。根 `go-dev-toolchain-quality` 要求新增 golangci-lint v2 配置、补齐 Taskfile 任务，且不新增无意义的 Air（CLI-only 项目无长期运行入口）。

## What Changes

- 新增 `.golangci.yml`，覆盖基线 linters。
- 补齐 `deps`、`mod-check`、`lint`、`run` 任务。
- 明确热加载不适用（CLI-only 项目，多个短生命周期命令）。

## Capabilities

### Modified Capabilities

- `pinax`: Go 开发工具链标准化，新增 lint 配置和 Taskfile 补齐。

## Impact

- 新增文件：`.golangci.yml`。
- 修改文件：`Taskfile.yml`、`AGENTS.md`。

## Non-Goals

- 不新增 Air 热加载。Pinax 是 CLI-only 项目，没有长期运行服务入口。
- 不改变 Pinax 的笔记、vault 或索引功能。
