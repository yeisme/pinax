## 任务

- [x] 1. 增加 `pinax.sync.output.v1`、A/M/D/R/C 映射、统计、revision、bytes 和路径脱敏。
- [x] 2. 接入 `output.style`、`--output-style`、表格宽度和列表 shown/total 提示。
- [x] 3. 接入 sync preview、limit、metadata diff、bounded content diff，以及 JSON/agent/explain 展示。
- [x] 4. 接入 one-shot sync 的 stderr progress sink 和 events NDJSON；保持 daemon 原 event type 兼容。
- [x] 5. 扩展 receipt 计数、sync.file change_code/change_state，并聚合 sync.all。
- [x] 6. 补齐稳定输出 golden/e2e 与真实 S3/COS 双设备回归证据。（2026-08-22 完成：`internal/app/sync_output_view_test.go` wire-shape golden（A/M/D/R/C 分类、counts、bytes、排序、shown/total、hash/omit 路径脱敏、attach facts/data）；`cloud_direct_two_device.txt` 双设备 e2e 追加 sync_view 断言（push applied、pull A/path、收敛后二次 push `result=up_to_date` + `scope=remote-aware`）；真实 S3 双设备回归 `TestSyncOutputRealDualDeviceRegression` 经 `syncrealevidence` 在 MinIO 直连（manifest v2 + CAS 条件写）通过，证据 `temp/integration-test-runs/20260822T094645Z-1210789`。回归同时暴露并修复 manifest v2 冲突副本 identity 缺陷，见 `pinax-manifest-v2-conflict-copy-identity`。COS virtual-hosted addressing 未单独覆盖，发布评审时如需另跑。）
- [x] 7. 运行完整 `task check`、`go test ./tests/e2e` 和发布前兼容评审。（2026-08-23：`task check` exit 0（vet/lint/test 全树，含并发会话改动）、`go test ./tests/e2e -count=1` ok。兼容评审结论——`sync_view` 为 projection data 的 additive 新键，plan/receipt/旧 facts 全部保留；`sync.*` facts、receipt `upload_blobs`/`bytes_uploaded` 计数均为增量合并；daemon 事件类型未改（task 4 验证）；远端协议、manifest、加密格式、CAS 语义零变更；真实 S3（MinIO 直连）与 file:// 双设备证据均通过（`temp/integration-test-runs/20260822T094645Z-1210789`）。遗留披露：COS virtual-hosted addressing 未实测（非兼容性风险，`defaultS3PathStyle` 已按域名区分），发布后如需 COS 证据可经 `syncrealevidence` 流程补跑。）

每个未完成任务都必须保留脱敏失败证据；正文 diff 不得进入 receipt、事件日志或对象存储。
