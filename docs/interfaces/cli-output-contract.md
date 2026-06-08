# CLI 输出合同

Pinax 命令必须从同一个 command projection 渲染所有输出模式：

- 默认输出：简洁中文摘要，包含事实、风险和下一步。
- `--agent`：低 token `key=value`，稳定字段名，适合 agent 消费。
- `--json`：单一 JSON envelope，stdout 只包含 JSON。
- `--events`：NDJSON 事件流。
- `--explain`：说明输入、决策、风险和可复验命令。
- 输出模式互斥：一次只能选择默认模式、`--agent`、`--json`、`--events` 或 `--explain` 中的一个；冲突时返回 `cli.output_mode` / `output_mode_conflict`。

stdout/stderr 规则：

- 机器输出模式下 stdout 只包含所选机器格式。
- `--events` 必须至少输出 `start` 和 `end` / `error` 事件；没有值的 `facts`、`actions`、`evidence` 和 `error` 字段不输出，避免 `null` 语义歧义。
- progress、diagnostics、provider stderr、日志和非结构化错误写 stderr。
- 所有错误必须有稳定 status 和 error code。
- notebook core 新增命令必须复用同一 projection：`daily`、`inbox`、`view`、`index`、`search`、`note links/backlinks/orphans/attach/attachments`、`import markdown`、`export markdown`、`organize suggest/list/apply` 的默认输出为中文摘要，`--json` 为单一 envelope，`--agent` 为低 token facts。
- 常用稳定 facts 包括但不限于：`path`、`note_id`、`group`、`folder`、`kind`、`status`、`index_status`、`engine`、`returned`、`links`、`backlinks`、`unresolved`、`attachments`、`missing`、`view`、`plan_id`、`operations`、`receipt_path`。
- 参数、flag 和用法错误也必须可操作：默认输出说明缺少或多出的参数、给出真实可运行示例、提供 `--help` 或下一条命令；不得只暴露 `accepts N arg(s)` 这类框架错误。
- `--json`、`--agent`、`--events` 和 `--explain` 下的参数错误必须从同一个 failed projection 渲染，包含稳定 `error.code`、中文 `error.message` 和可执行 `error.hint` / `actions`。
- token、webhook URL、cookies、Authorization header、外部 CLI 配置内容和 raw payload 必须脱敏。
