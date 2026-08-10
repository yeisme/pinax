## 背景

Pinax 现有命令已经共享 JSON、agent 和人类摘要，但同步命令的变更含义分散在 plan、receipt 和日志中；默认输出也无法快速回答“哪些文件发生了什么”。本变更把这些来源投影为一个 additive 的同步视图，并让人类、agent、JSON、events、explain 共用同一份结果。

## 目标

- 默认人类输出使用表格，支持 `--output-style table|compact`、配置文件 `output.style` 和 `PINAX_OUTPUT_STYLE`。
- 为 `sync`、`sync.diff`、`sync.push`、`sync.pull`、`sync.all`、`sync.logs.show` 增加 `pinax.sync.output.v1`。
- 提供 Git 状态式 A/M/D/R/C、统计、revision、bytes 和 `shown/total/truncated`。
- 提供 `--preview status|diff|none`、`--content-diff`、`--limit N`，并对正文 diff 设置有界脱敏规则。
- 让默认人类模式在 stderr 显示阶段进度，让 `--events` 输出 NDJSON start/progress/end/error；JSON/agent 保持单一机器投影。
- 增加 receipt 计数和 sync.file 可选变更字段，同时保留旧字段、远端协议、加密格式和 CAS 语义。

## 非目标

- 不改变对象存储布局、manifest 加密、revision CAS 或 `remote_write` 判定。
- 不把正文 diff 写入 receipt、事件日志或远端对象。
- 不引入全屏 TUI、pager 或新的 progress 依赖。
