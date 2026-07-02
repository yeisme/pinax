# pinax-sticky-note-template-pack Design

## 方案

沿用现有内置 `note_template` 机制，在 `builtInNoteTemplates` 中增加 `sticky.*` 模板。模板名称、metadata key、`kind`、`status`、`tags` 保持英文稳定字段，正文标题和小节使用中文，降低快速捕获成本。

`sticky.*` 默认 `starter: true`，写入 `inbox/sticky/**`，应用层继续通过 `CreateNote` 的模板 defaults 和 output path 生成笔记。显式 `--project`、`--folder`、`--dir`、`--kind`、`--status` 仍按现有优先级覆盖模板默认路径或 metadata。

## 看板边界

便签可以携带项目上下文，但不写 `board_column`、`workspace_path` 或 `kind: task`。真正可移动、可归档的看板项仍由 `pinax project item add` 通过 project board service 创建。

## 模板包

- `sticky.capture`：通用短记录。
- `sticky.quote`：摘录/引用。
- `sticky.link`：链接资料线索。
- `sticky.question`：待查问题。
- `sticky.term`：术语/概念。
- `sticky.person_signal`：人物或组织线索。
- `sticky.project_signal`：项目、子项目或看板上下文线索。

## 验证

- 单元测试覆盖 sticky 模板 metadata、默认 metadata 应用、项目上下文边界。
- CLI 测试覆盖中文 intent 推荐。
- OpenSpec 验证覆盖新增 delta spec。
