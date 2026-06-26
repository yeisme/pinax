# pinax-sticky-note-template-pack Proposal

## 背景

Pinax 已有 `idea.*` 和内容笔记模板，但用户还需要更轻量的“便签”短文档来暂存摘录、链接、问题、术语、人名线索和项目线索。这类内容应该进入 inbox 等待分拣，不应自动变成 todo 或受控 project board item。

## 目标

- 新增内置 `sticky.*` note template 包，默认写入 inbox 短文档区域。
- 默认使用 `kind: sticky`、`status: inbox`，并提供可推荐的中文 metadata。
- 提供 `sticky.project_signal` 记录项目/子项目/看板上下文，但不写 `board_column` 或 `kind: task`。

## 非目标

- 不新增 `pinax sticky` 子命令。
- 不新增 `note add --subproject` 或 `note add --board-column`。
- 不绕过 `pinax project item add` 创建或移动受控看板项。
