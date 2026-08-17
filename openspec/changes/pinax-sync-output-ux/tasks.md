## 任务

- [x] 1. 增加 `pinax.sync.output.v1`、A/M/D/R/C 映射、统计、revision、bytes 和路径脱敏。
- [x] 2. 接入 `output.style`、`--output-style`、表格宽度和列表 shown/total 提示。
- [x] 3. 接入 sync preview、limit、metadata diff、bounded content diff，以及 JSON/agent/explain 展示。
- [x] 4. 接入 one-shot sync 的 stderr progress sink 和 events NDJSON；保持 daemon 原 event type 兼容。
- [x] 5. 扩展 receipt 计数、sync.file change_code/change_state，并聚合 sync.all。
- [ ] 6. 补齐稳定输出 golden/e2e 与真实 S3/COS 双设备回归证据。
- [ ] 7. 运行完整 `task check`、`go test ./tests/e2e` 和发布前兼容评审。

每个未完成任务都必须保留脱敏失败证据；正文 diff 不得进入 receipt、事件日志或对象存储。
