## Why

Pinax 当前配置入口主要依赖命令行参数和少量环境变量，输出主题颜色也硬编码在 `internal/output`，难以为不同终端、用户偏好和 vault 项目提供一致的可配置体验。

本变更补齐 Viper 驱动的分层配置设计，并在不破坏机器输出合同的前提下，为默认人类输出提供可配置颜色主题和基于 Glow/Glamour 的 Markdown 渲染。

## What Changes

- 新增 `internal/config` 配置层，使用 Viper 读取内置默认值、用户级配置、项目级配置、环境变量和显式命令行 flag。
- 定义配置优先级：显式命令行 flag > 环境变量 > 项目级配置 > 用户级配置 > 内置默认值。
- 增加用户级配置路径和项目级 `.pinax/config.yaml` 合并策略，普通只读命令不得隐式写配置文件。
- 增加 `PINAX_` 环境变量映射，兼容 `NO_COLOR`、`EDITOR` 和后续 `PAGER`。
- 增加 `pinax config path/get/doctor/set/unset` 规划，其中 `set/unset` 必须由 CLI/service 写入结构化配置。
- 引入可配置输出主题，按 role 管理颜色，而不是在 renderer 中散落具体色值。
- 引入 `github.com/charmbracelet/glamour` 作为 Markdown 正文渲染组件，默认只作用于人类 summary 输出。
- 保持 `--json`、`--agent`、`--events` 和 `--explain` 的机器输出合同稳定，机器 stdout 不输出 ANSI、pager 或人类装饰文本。
- 不引入新的 TUI 默认体验；现有 Bubble Tea journal pager 不作为本变更目标，可在后续显式 `--pager` 设计中处理。

## Capabilities

### New Capabilities

- `configuration-layer`: 覆盖 Viper 配置加载、路径、优先级、环境变量、显式 flag overlay、配置命令和校验行为。
- `configurable-output-rendering`: 覆盖可配置主题颜色、Markdown 渲染、默认人类输出美化和机器输出隔离行为。

### Modified Capabilities

无。本变更通过新增 delta spec 描述新增行为，不重写现有 `pinax` 或 `note-command-ux` 基准要求。

## Impact

- 影响 `cmd/pinax`：新增全局配置相关 flag，将命令层从直接变量读取逐步迁移到 typed config 和显式 flag overlay。
- 影响 `internal/config`：新增 Viper 配置模块、默认值、路径解析、环境变量映射、合并、校验和配置写入服务。
- 影响 `internal/output`：新增 RenderOptions、主题 role、Markdown renderer，并让默认 summary 消费最终输出配置。
- 影响 `internal/app`：配置命令需要通过 application service 创建和修改 CLI-authored structured assets。
- 影响依赖：新增 Viper 和 Glamour；不得引入外部 `glow` 或 `gum` 二进制运行时依赖。
- 影响测试：需要覆盖配置优先级、环境变量、配置写入、机器输出无 ANSI、默认 Markdown 渲染和 `NO_COLOR` 行为。
