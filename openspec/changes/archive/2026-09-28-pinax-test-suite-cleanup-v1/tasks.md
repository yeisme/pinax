# 实施与验收任务

由 scripts/openspec-tasks.py 维护状态。

- [x] 1.1 核对测试消费者与替代覆盖，保留协议兼容、安全与恢复用例。 | evidence: 逐项消费者搜索完成；design.md 记录删除与替代覆盖映射，legacy MCP、权限、snapshot/restore 与前轮 daily-knowledge 测试未删除。
- [x] 2.1 合并证据工具测试，删除整页哈希和源码字符串断言，精简 KB 退役重复路径。 | evidence: 4 个 evidence 库测试迁回 tools/testkit/evidence；合并 1 个重复失败测试，删除整页 SHA 与源码子串断言，KB 4 条重复路径收敛为顶层退役检查；测试净减 83 行、3 个顶层函数；归属包测试清理前后均 PASS。
- [x] 3.1 运行归属测试与最终质量门禁，记录覆盖映射及证据。 | evidence: 归属包 go test ./tools/testkit/evidence -count=1 PASS；完整 task check PASS（全量 Go、fmt、lint 0 issues、build、OpenSpec 101/101、renderer 4/4 与 tsc）。证据：temp/integration-test-runs/20260928T162418Z-3958504/，exit_code=0、redaction scan PASS、六类文件齐全。未运行 race；删除项与保留断言映射已审阅，无运行时合同变化。
