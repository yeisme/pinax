# pinax-template-authoring-cli

## Why

Pinax 当前已有模板 MVP：`template init/list/show/render` 可以初始化内置模板、查看模板、渲染模板；`note new --template` 可以用模板生成笔记。但模板仍主要依赖内置文件，用户不能通过 CLI 创建、校验、删除或用自定义变量驱动模板。这会让 Pinax 在真实笔记管理中不够像 Obsidian/Logseq 类工具：会议、日报、项目、阅读摘录、任务复盘、YAML frontmatter、Mermaid 图和自定义工作流都需要可维护的模板体系。

本 change 设计 Pinax 模板作者能力：模板仍然是本地 Markdown 文本真源，存放在 `.pinax/templates/*.md`；CLI/service 负责创建、校验、渲染和删除模板；变量替换保持非执行式，不运行脚本、不读环境变量、不访问网络。

## What Changes

- 增加 `pinax template create <name>`，支持从文件、stdin 或 `--body` 创建模板。
- 增加 `pinax template delete <name> --yes`，删除自定义模板并保留审批门禁。
- 增加 `pinax template validate <name>`，检查模板名、路径安全、变量语法、frontmatter fence、Mermaid/YAML fence 和未知变量。
- 扩展 `pinax template render <name>`，支持多个 `--var key=value` 自定义变量。
- 扩展 `pinax note new <title> --template <name>`，支持 `--var key=value`，并把变量传给模板渲染。
- 明确模板 metadata 不单独落数据库；模板文件本身是用户可编辑真源，`.pinax/events.jsonl` 记录 CLI-authored 操作事件。
- 保留现有内置模板：`note`、`daily`、`project`、`yaml`、`mermaid`。

## Non-Goals

- 不引入脚本模板、Lua/JS 执行、shell 插值或环境变量读取。
- 不引入复杂模板语言；MVP 只支持 `{{name}}` 形式的安全变量替换。
- 不实现 Obsidian 插件生态、TUI 模板编辑器或 Web UI。
- 不把模板内容写入 SQLite；SQLite/GORM 仍用于本地索引投影，模板文件保持文本真源。
- 不允许 agent 手写 `.pinax` 机器状态；模板正文可由用户/agent 编辑，模板创建/删除事件由 CLI/service 写入。

## Impact

- CLI 影响：`cmd/pinax/main.go` 增加 `template create/delete/validate`，并扩展 `template render` 和 `note new` flags。
- App service 影响：`internal/app/service.go` 增加模板创建、删除、校验、变量解析和渲染上下文。
- 测试影响：补充 service tests 和 CLI tests，覆盖路径安全、变量替换、未知变量、approval gate、输出合同和从模板生成笔记。
- 文档影响：更新 `docs/README.md` 或 `docs/operations/local-development.md` 的模板工作流示例。

