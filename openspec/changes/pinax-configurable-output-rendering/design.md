## Context

Pinax 已有 Cobra 命令入口、`domain.Projection` 和 `internal/output` 多模式 renderer。当前默认人类输出使用 `lipgloss` 和 `lipgloss/table`，颜色值直接硬编码在 `internal/output/render.go`；`PINAX_COLOR` 已经存在，但没有用户级/项目级配置来源，也没有 typed config 校验。项目文档已经预留 `internal/config` 作为 Viper defaults、env、project config 和 validate 的归属。

本变更把配置读取、环境变量、项目配置和命令行显式覆盖统一到 `internal/config`，并让输出层消费最终 `RenderOptions`。Markdown 美化使用 Glow 背后的 `github.com/charmbracelet/glamour` 库，而不是运行外部 `glow` 或新增 TUI。

## Goals / Non-Goals

**Goals:**

- 建立 Viper 驱动、typed config 输出的配置层。
- 支持用户级配置、项目级 `.pinax/config.yaml`、`PINAX_` 环境变量和显式命令行 flag。
- 确保 flag 默认值不会覆盖配置文件值，只有显式提供的 flag 才参与最高优先级 overlay。
- 支持可配置输出主题、颜色开关、宽度和 Markdown 渲染风格。
- 让 `note show/read`、journal show、`template show/render` 在默认人类模式下使用 Glamour 渲染 Markdown 正文。
- 保持机器输出模式 stdout 严格稳定、无 ANSI、无 pager、无 Markdown 装饰。
- 提供 CLI-authored `pinax config` 命令规划，避免手写结构化配置作为主流程。

**Non-Goals:**

- 不实现新的全屏 TUI 或默认 Bubble Tea pager。
- 不把输出模式本身写入配置作为默认行为；`--json`、`--agent`、`--events`、`--explain` 仍必须由命令行显式选择。
- 不保存 provider token、webhook、cookie、Authorization header 或任何 secret-like 字段。
- 不重构全部 Cobra 命令到 `internal/cli`；本变更只抽配置层并在当前命令入口接线。
- 不把主题配置扩展成任意 CSS/模板系统；只支持有限 role-based 颜色。

## Decisions

### 1. Viper 只封装在 `internal/config`

命令层不直接调用 Viper，不直接读取配置文件。新增包形状：

```text
internal/config/
  config.go       typed Config struct 和 SourceSet
  defaults.go     DefaultConfig()
  paths.go        XDG/user/project 路径解析
  loader.go       Load()
  env.go          PINAX_ env mapping
  overlay.go      project/user/env/flag merge
  validate.go     Validate()
  write.go        config set/unset 写入支持
```

原因：Viper 的全局状态和 pflag 绑定容易污染测试，Pinax 需要 typed config 和显式校验。把 Viper 限制在配置包内，可以让命令层和 app service 只依赖稳定结构体。

备选方案是让每个命令自行 `viper.GetString`。该方案短期快，但会让默认值、环境变量、错误处理和测试散落，不符合 Pinax 子项目边界。

### 2. 使用显式 flag overlay，而不是盲目 `BindPFlag`

配置优先级为：

```text
显式命令行 flag > 环境变量 > 项目级配置 > 用户级配置 > 内置默认值
```

实现上，Cobra flag 默认值不能直接作为覆盖值。配置层只读取 `cmd.Flags().Changed(name)` 和 `cmd.InheritedFlags().Changed(name)` 为 true 的 flag。这样 `--vault`、`--limit`、`--color` 等 flag 的默认值不会意外遮蔽项目配置。

复杂的 flag overlay 逻辑需要简短中文注释说明原因，因为这是防止配置优先级 bug 的关键边界。

### 3. Vault 选择先于项目配置读取

加载顺序：

1. 从内置默认值开始。
2. 读取用户级 config。
3. 使用显式 `--vault`、`PINAX_VAULT`、用户配置中的 vault 默认值或 `.` 确定 vault root。
4. 读取 `<vault>/.pinax/config.yaml`。
5. 应用环境变量 overlay。
6. 应用显式 flag overlay。
7. 执行 `Validate()`。

原因：项目级配置路径依赖 vault root，因此必须先确定 vault；但最终 vault 仍可被显式 flag 或环境变量解释为来源更高的选择。

### 4. 配置 schema 保持窄而稳定

第一阶段 typed config：

```yaml
schema_version: pinax.config.v1
title: ""
vault: ""
output:
  color: auto
  theme: pinax
  width: auto
  markdown:
    enabled: true
    style: auto
    pager: never
editor:
  command: ""
note_defaults:
  project: ""
  group: ""
  folder: ""
  kind: ""
  status: active
  tags: []
search:
  allow_stale: false
  limit: 20
storage:
  backend: local
  local:
    root: ""
  s3:
    bucket: ""
    region: ""
    prefix: ""
    endpoint: ""
    profile: ""
themes:
  custom: {}
```

不支持任意 provider secret 字段。写入配置时，如果 key 名或值疑似包含 secret、token、password、cookie、authorization、webhook 等敏感内容，必须拒绝或脱敏。

### 5. 环境变量使用稳定 `PINAX_` 前缀

嵌套 key 用 `_` 映射：

```text
PINAX_VAULT
PINAX_OUTPUT_COLOR
PINAX_OUTPUT_THEME
PINAX_OUTPUT_WIDTH
PINAX_OUTPUT_MARKDOWN_ENABLED
PINAX_OUTPUT_MARKDOWN_STYLE
PINAX_EDITOR_COMMAND
PINAX_SEARCH_LIMIT
PINAX_SEARCH_ALLOW_STALE
PINAX_STORAGE_BACKEND
PINAX_STORAGE_S3_PROFILE
```

兼容标准环境变量：`NO_COLOR` 影响默认 human color；`EDITOR` 只作为 editor.command 未配置时的 fallback；`PAGER` 暂不默认调用，保留给未来显式 pager 设计。

### 6. 输出层消费 `RenderOptions`

新增输出选项：

```go
type RenderOptions struct {
    ColorMode     string
    ThemeName     string
    Width         int
    Markdown      MarkdownOptions
    IsTerminal    bool
}
```

`output.Render` 保持兼容入口，新增 `RenderWithOptions`。当前硬编码颜色迁移为 role-based theme：accent、muted、rule、success、warning、danger、key、value、path、link、code、heading。

原因：renderer 不应该读取配置文件或环境变量，避免输出测试不稳定。配置层负责生成最终选项，输出层只渲染。

### 7. Markdown 使用 Glamour，不调用 Glow CLI

引入 `github.com/charmbracelet/glamour`。默认人类模式中，对 `note.show`、`note.read`、`daily.show`、`weekly.show`、`monthly.show`、`template.show`、`template.render` 的正文做 Markdown 渲染。

机器模式仍输出原始数据：JSON 里保留 `data.note.body` 或 `data.body`；agent/events/explain 不包含 Glamour 装饰。`output.markdown.enabled=false` 时回退到纯文本。

### 8. 配置命令由 app service 写入 structured assets

新增命令规划：

```text
pinax config path
pinax config get <key>
pinax config doctor
pinax config set <key> <value> --scope user|project
pinax config unset <key> --scope user|project
```

`set/unset` 必须显式 scope，且由 command 调 app service 写入用户级或项目级 config。普通只读命令不得隐式创建用户级配置或项目级配置。

## Risks / Trade-offs

- [Risk] Viper 与 Cobra flag 默认值结合容易导致配置被默认 flag 覆盖。 → Mitigation：不使用盲目 `BindPFlag` 作为最终来源，只对 `Changed` 的 flag 做显式 overlay，并加单元测试。
- [Risk] 用户自定义主题可能降低可读性。 → Mitigation：内置 `pinax`、`mono`、`high-contrast`，自定义缺失 role 回退到 `pinax`，并支持 `NO_COLOR`。
- [Risk] Glamour 依赖增加输出快照波动。 → Mitigation：测试分离 plain/ANSI；机器输出测试只断言无 ANSI 和 JSON/agent 合同，Markdown human 测试使用稳定片段而不是整屏 golden。
- [Risk] 配置写入可能误保存 secret。 → Mitigation：`Validate()` 和 config write path 同时检查 secret-like key/value，测试覆盖拒绝路径。
- [Risk] journal show 现有默认 TTY pager 行为与本设计目标冲突。 → Mitigation：本变更将默认行为收敛为 stdout Markdown 渲染，pager 行为后续通过显式配置或 flag 单独设计。

## Migration Plan

1. 先引入 `internal/config` 并保持现有命令默认行为不变。
2. 将 `PINAX_COLOR` 迁移到配置层解析，同时保留现有环境变量兼容。
3. 新增 global human output flags：`--color`、`--theme`、`--width`、`--markdown-style`。
4. 输出层新增 `RenderWithOptions`，旧 `Render` 继续使用默认选项。
5. 逐步将命令接线改为加载 typed config 并传入 render options。
6. 接入 Glamour Markdown 渲染。
7. 增加 config 命令和配置写入服务。
8. 运行 `task check` 或 fallback 门禁，并执行 `openspec validate --all`。

## Open Questions

- `output.markdown.pager=auto|always` 是否在本变更后续任务中实现，还是保留为未来 change。
- `--config <path>` 是否只用于测试/调试，还是作为正式用户级配置覆盖入口。
- 自定义 Glamour style 是否允许直接引用用户本地 JSON 样式文件，还是先限制为 `auto|dark|light|notty`。
