## 背景

Pinax 已经能用 project、subproject workspace、board、template、search、link/backlink 和 proof loop 管理长期知识项目，但“长期学习项目”现在仍需要用户手动串命令。以《学习炒股的全部笔记》为例，用户需要自己创建 vault、project、workspace、board、学习笔记模板和 starter tasks。

本变更把这个真实用例沉淀为 Pinax 产品能力：通用长期学习项目场景包，加一个 `stock-learning` 预设。Pinax 只管理学习资料、复盘、术语、来源和风险原则，不做荐股、买卖建议、收益承诺或自动交易决策。

## 目标

- 新增 `pinax project learning init`，一条命令初始化长期学习项目 workspace、board、starter notes 和 starter items。
- 让 `project board configure` 的自定义列真正影响 `board show/plan/export` 和 `project item add/move`。
- 新增通用学习模板和股票学习预设模板，用于术语、资料来源、练习记录、案例复盘、交易日志、风险规则和周复盘。
- 保持 CLI JSON envelope、`--agent` key、默认 board 字段和现有命令向后兼容，只新增可选字段。
- 覆盖 Go unit、CLI contract、testscript e2e 和 integration evidence。

## 非目标

- 不实现独立学习 App、Web UI、TUI 或移动端。
- 不接入行情、券商、交易 API 或荐股模型。
- 不把股票学习模板包装成金融建议或投资顾问能力。
- 不删除、重命名、重定义既有 `project.board.*`、`project.item.*` 或 `template.*` 输出字段。
- 不新增远程写 API 或 MCP 写工具。

## 兼容性

- CLI/API 输出为 additive：保留 `next`、`doing`、`blocked`、`review`、`done` 等事实字段，新增 `column.<id>` facts 和 `data.board.facts.column_counts`。
- `project item add/move` 继续接受默认列；当 project/subproject 配置了自定义列时，再接受配置列。
- 自定义完成列不替代 `done` 的受保护完成语义；`done` 仍需要 approval/snapshot。
- 结构化资产仍由 Pinax app service 写入 `.pinax/project-boards/**`、`.pinax/project-workspaces/**` 和 `.pinax/events.jsonl`。
