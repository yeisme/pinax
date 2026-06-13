## 1. 配置层基础

- [x] 1.1 新增 `internal/config` 包，定义 typed `Config`、`OutputConfig`、`MarkdownConfig`、`EditorConfig`、`SearchConfig`、`StorageConfig` 和 `SourceSet`。
- [x] 1.2 实现 `DefaultConfig()`，覆盖 vault、output、markdown、editor、note defaults、search、storage 和 themes 默认值。
- [x] 1.3 添加 Viper 依赖，并确保 Viper 只在 `internal/config` 内部使用。
- [x] 1.4 实现用户级配置路径解析：`$XDG_CONFIG_HOME/pinax/config.yaml` 和 `~/.config/pinax/config.yaml` fallback。
- [x] 1.5 实现项目级配置路径解析：`<vault>/.pinax/config.yaml`，并拒绝越界配置路径。
- [x] 1.6 实现用户级、项目级 YAML 读取和 typed config merge，缺失配置文件时不报错。
- [x] 1.7 实现配置来源记录，至少包含用户配置路径、项目配置路径、使用的 env keys 和显式 flag keys。

## 2. 环境变量和显式 Flag Overlay

- [x] 2.1 实现 `PINAX_` 环境变量映射：`PINAX_VAULT`、`PINAX_OUTPUT_COLOR`、`PINAX_OUTPUT_THEME`、`PINAX_OUTPUT_WIDTH`、`PINAX_OUTPUT_MARKDOWN_ENABLED`、`PINAX_OUTPUT_MARKDOWN_STYLE`、`PINAX_EDITOR_COMMAND`、`PINAX_SEARCH_LIMIT`、`PINAX_SEARCH_ALLOW_STALE`。
- [x] 2.2 实现标准环境变量兼容：`NO_COLOR`、`EDITOR`，并记录 `NO_COLOR` 不影响机器输出合同。
- [x] 2.3 实现显式 flag overlay，只读取 `cmd.Flags().Changed` / `cmd.InheritedFlags().Changed` 为 true 的值。
- [x] 2.4 为避免 flag 默认值覆盖配置文件的合并逻辑添加中文注释，说明为什么不能盲目使用 `BindPFlag`。
- [x] 2.5 实现 `LoadOptions`，支持从命令层传入 vault flag、显式 flag accessor、环境变量读取函数和测试用 config path。
- [x] 2.6 增加配置优先级单元测试：显式 flag > env > project > user > defaults。
- [x] 2.7 增加 flag 默认值不覆盖用户级/项目级配置的回归测试。

## 3. 配置校验和安全边界

- [x] 3.1 实现 `Config.Validate()`，校验 `output.color`、`output.theme`、`output.width`、`output.markdown.style`、`output.markdown.pager`、`search.limit` 和 `storage.backend`。
- [x] 3.2 实现颜色值校验，支持 hex、ANSI color 名称或项目确认的 lipgloss color 格式。
- [x] 3.3 实现 secret-like key/value 拒绝或脱敏策略，覆盖 token、secret、password、cookie、authorization、webhook 等关键词。
- [x] 3.4 保持 S3 storage 只允许 bucket、region、prefix、endpoint、profile 等非 secret 字段。
- [x] 3.5 将配置校验错误映射为稳定 `domain.CommandError` code，并支持 default、`--json`、`--agent` 输出。
- [x] 3.6 增加非法 enum、非法颜色、secret-like 配置和 S3 缺失 bucket/region 的测试。

## 4. Cobra 接线和配置命令

- [x] 4.1 在 root command 增加 human output 全局 flags：`--color`、`--theme`、`--width`、`--markdown-style`。
- [x] 4.2 保持 `--json`、`--agent`、`--events`、`--explain` 只由显式命令行选择，不写入配置默认值。
- [x] 4.3 在命令执行前加载 typed config，并把 app request 和 output render options 从最终配置派生出来。
- [x] 4.4 实现 `pinax config path`，显示用户级和项目级配置路径，支持机器输出合同。
- [x] 4.5 实现 `pinax config get <key>`，读取合并后的有效配置值，支持 default、`--json`、`--agent`。
- [x] 4.6 实现 `pinax config doctor`，输出配置来源、覆盖关系、校验问题和下一步，支持 default、`--json`、`--agent`。
- [x] 4.7 实现 `pinax config set <key> <value> --scope user|project`，通过 service 写入配置文件，缺少 scope 时失败且不写文件。
- [x] 4.8 实现 `pinax config unset <key> --scope user|project`，通过 service 删除配置项，保留其他字段。
- [x] 4.9 增加 config 命令 e2e/testscript，验证 structured assets 只由 CLI/service 写入。

## 5. 主题和 Summary 渲染

- [x] 5.1 新增 `internal/output` theme role 模型：accent、muted、rule、success、warning、danger、key、value、path、link、code、heading。
- [x] 5.2 提供内置主题：`pinax`、`mono`、`high-contrast`，并支持 `custom` fallback。
- [x] 5.3 新增 `RenderOptions` 和 `RenderWithOptions`，保留现有 `Render` 兼容入口。
- [x] 5.4 将 `newSummaryTheme` 从硬编码颜色迁移到 theme role。
- [x] 5.5 实现 `NO_COLOR`、`TERM=dumb`、非 TTY、`--color always|auto|never` 的最终颜色决策。
- [x] 5.6 保证 `--json`、`--agent`、`--events` 在任何 color/theme 配置下 stdout 都没有 ANSI。
- [x] 5.7 增加主题选择、custom fallback、NO_COLOR、PINAX_OUTPUT_COLOR 和机器输出无 ANSI 测试。

## 6. Glamour Markdown 渲染

- [x] 6.1 添加 `github.com/charmbracelet/glamour` 依赖，不调用外部 `glow` 或 `gum` 二进制。
- [x] 6.2 新增 `internal/output/markdown.go`，实现 Markdown renderer，支持 enabled、style、width、color mode。
- [x] 6.3 在 default summary 模式下为 `note.show`、`note.read` 渲染 metadata summary + Markdown 正文。
- [x] 6.4 在 default summary 模式下为 `daily.show`、`weekly.show`、`monthly.show` 渲染 metadata summary + Markdown 正文。
- [x] 6.5 在 default summary 模式下为 `template.show` 和 `template.render` 渲染 Markdown 正文。
- [x] 6.6 当 `output.markdown.enabled=false` 时回退到纯文本正文输出。
- [x] 6.7 保持 `--json` 下原始正文仍在 `data.note.body` 或 `data.body`，不包含 ANSI 或 Glamour 装饰。
- [x] 6.8 调整 journal show 默认 TTY 行为，不隐式进入 Bubble Tea pager；如保留 pager，必须由显式 flag/config 触发并另行测试。
- [x] 6.9 增加 Markdown 渲染测试，覆盖 heading、list、code block、blockquote、禁用 Markdown 和非 TTY 无 ANSI。

## 7. 文档、验证和回归

- [x] 7.1 更新 `docs/interfaces/cli-output-contract.md`，说明主题和 Markdown 渲染只作用于默认人类输出。
- [x] 7.2 更新 Pinax 运行文档，说明用户级配置、项目级配置、环境变量和显式 flag 优先级。
- [x] 7.3 更新 CLI help 文案，新增配置命令和输出主题 flags 的中文说明。
- [x] 7.4 运行 `gofmt -w <changed-go-files>`。
- [x] 7.5 运行 `go test ./...`。
- [x] 7.6 运行 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` 或 `task check`。
- [x] 7.7 运行 `openspec validate --all`。
- [x] 7.8 手动 smoke：创建临时 vault，分别验证 user config、project config、env、flag 覆盖和 `NO_COLOR` 行为。
- [x] 7.9 手动 smoke：验证 `pinax note show --json`、`--agent`、`--events` 在强制颜色配置下仍保持机器 stdout 干净。


## Evidence

- `go test ./internal/config -count=1` passed after adding layered config, key presence merge, path containment, secret/S3/custom color validation tests.
- `go test ./internal/config ./internal/output ./internal/cli -count=1` passed after Cobra config wiring, `RenderOptions`, theme roles and Markdown rendering integration.
- `go test ./tests/e2e -run TestConfigRendering -count=1` passed with testscript coverage for `pinax config set/get/doctor` and missing `--scope` failure.
- Manual smoke passed with `dist/pinax`: user config, project config, env, explicit `--theme`, `NO_COLOR`, and forced-color `note show --json/--agent/--events` ANSI checks.
- `task check` passed: `openspec validate --all`, `go test ./...`, `golangci-lint fmt --diff`, `golangci-lint run`, and release-style build.
