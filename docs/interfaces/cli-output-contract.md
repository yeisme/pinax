# CLI 输出合同

Pinax 命令必须从同一个 command projection 渲染所有输出模式：

- 默认输出：简洁中文摘要，包含事实、风险和下一步。
- `--agent`：低 token `key=value`，稳定字段名，适合 agent 消费。
- `--json`：单一 JSON envelope，stdout 只包含 JSON。
- `--events`：NDJSON 事件流。
- `--explain`：说明输入、决策、风险和可复验命令。

stdout/stderr 规则：

- 机器输出模式下 stdout 只包含所选机器格式。
- progress、diagnostics、provider stderr、日志和非结构化错误写 stderr。
- 所有错误必须有稳定 status 和 error code。
- token、webhook URL、cookies、Authorization header、外部 CLI 配置内容和 raw payload 必须脱敏。

