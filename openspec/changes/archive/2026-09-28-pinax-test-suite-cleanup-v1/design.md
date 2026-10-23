# 测试清理依据

## 删除与替代覆盖映射

| 旧项 | 处理 | 保留覆盖 |
| --- | --- | --- |
| tests/e2e/integration_evidence_test.go 四个测试 | 移至 tools/testkit/evidence/evidence_contract_test.go，保持外部包视角 | Success、FailurePreservesExitCode、ArgvRedaction、RedactUnit；失败层级与原始 prompt sentinel 合并到同一权威失败用例 |
| TestRunWritesCompleteRedactedFailureEvidenceWithOriginalExitCode | 与上面的失败用例合并 | exit=7、status=failed、component layer、stderr、六类证据文件、递归扫描与 redaction_scan_passed；成功 component 标签和负向扫描器测试独立保留 |
| 自制 contains、未使用 JSON import 的占位引用 | 删除 | 标准 strings.Contains 与既有断言；无生产依赖 |
| TestExplorePageGoldenDigest 与时间戳正则 | 删除不可读的全 HTML SHA 固定值 | TestExplorePageContract、TestExplorePageExternalRefScanZeroHits、TestExplorePageRendersEmbedAndNoEmbed、Explore bundle/body/path/auth 测试；整页字节完全相同不再作为验收承诺 |
| TestSyncConflictsCobraLayerDoesNotResolveFilesDirectlyCLI | 删除四个源码子串扫描 | TestSyncConflictsCommandsUseProjectionOutputModesAndReceiptsCLI 的真实权限、文件保留/处理、输出与回执；架构 import guards 保留 |
| TestKBCommandsAreRemoved 的 init + 三个后代路径 | 只测试顶层退役入口 | unknown command 与禁止 kb_decoupled stub；顶层缺失即不能注册其后代命令 |

## 边界

fit：测试实现归 cli/pinax。未触及任何 stable CLI/API、数据库、输出字段或生产行为；无迁移、弃用周期或发布动作。回退本 change 的测试修改即可恢复原测试组织。不能按 legacy/retired 关键字批量删除兼容测试，也不能删除权限、恢复、脱敏门槛来使检查变绿。

## 验证

归属包测试清理前后均运行 `go test ./tools/testkit/evidence -count=1`。最终复用现有 `daily-knowledge-quality` evidence profile 执行完整 `task check`，不新增一次性 runner/profile、不运行 race。该 profile 的名称来自前一轮工作，实际命令是整个 Pinax 的 check，覆盖全部保留测试与新位置测试。证据记录在本 change tasks 中；历史归档引用不改写。
