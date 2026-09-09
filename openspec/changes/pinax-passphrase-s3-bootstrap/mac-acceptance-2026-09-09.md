# 当前 Mac 与现有 vault 验收（2026-09-09）

## 验收结论

本轮从真实 `darwin/arm64`、macOS `26.6.2` 发起，使用既有 `yeisme-notes` 与 agent-dev 上的 vault。三个 active change 剩余任务为 9 项（memory 2、S3 bootstrap 5、continuity 2）。未将命令成功、fixture 测试或测试次数替代真实外部门禁，未勾选、归档或发布 stable claim。

## 平台与来源

- 源码基线：`bb18c9f3b276dc4da3876c7d8ecd14ab317eccf6`，通过 `git archive` 导出到 Mac 独立验收目录。
- 既有 Homebrew `0.1.5`：candidate runner 失败，`macos_support_stage_missing`，bootstrap contract 缺失。旧命令 `sync repo doctor` 被误路由至 `sync.all`，无 `--yes`，receipt 显示 `remote_write=false`、`local_write=false`；生成了本地诊断收据。
- 正式 release `v0.2.0`、archive channel、`pinax_0.2.0_darwin_aarch64.tar.gz`：SHA-256 `a2bda63ba47ba0bf459e8d41955a82f1b026c581fe2e456980c74dba0d9bc2bf` 与 release `checksums.txt` 一致。候选 contract probe PASS；仅达到 candidate，不是 supported。

## 真实运行

- Mac 既有 vault：`vault validate` 51 notes、0 issues；原有六个 skills 文件未提交修改保留。
- agent-dev 既有 vault：`vault validate` 69 notes、0 issues；`sync repo doctor` healthy；continuity binding ready、`scope=project:pinax`。
- Mac 既有 vault 的新版 doctor 报 `sync_repo_config_drift`，字段为 `backend.endpoint`；未修改原 vault 配置。
- 源码基线初次 `sync diff --target capsa --unlock keychain` 读取到真实 remote revision `rev_20260817110953.551097933`、`remote_checked=true`、`remote_write=false`。
- 从 agent-dev 配置仓库独立 clone 后，正式 release 执行 `sync repo bootstrap --unlock keychain --pull --yes` 失败：`keychain_store_unavailable`。没有指定 `--remember-keychain`，但 CLI 把 selected Keychain 同时传作写入目标，在成功解锁 envelope 后调用 Remember；runtime 尚未生成。
- 该失败后原 vault 的 diff/push dry-run 退化为 cached、`remote_checked=false`。Keychain 条目是否被改变不能仅凭错误码断言；停止任何 Remember/同步写入，保留故障证据。
- 独立不存在的 Keychain 引用进行 bootstrap：`sync_repo_unlock_required`，非交互快速 fail-closed。未删除、锁定或主动替换真实 Keychain 项。
- 本轮没有 COS push、远端 revision mutation、canary 笔记写入、冲突选择或原有笔记改写。

## OpenSpec 剩余门禁

| Change / task | 本轮证据 | 状态 |
| --- | --- | --- |
| agent-memory-runtime 8.5 | `agent status` 可用，proposals=0；无连续四周双 runtime 使用证据 | 未通过 |
| agent-memory-runtime 8.6 | 8.5 依赖未满足，保持 experimental | 未关闭 |
| passphrase-s3-bootstrap 7.5 | 正式 release candidate probe PASS，真实 bootstrap Keychain failure | 未通过 |
| passphrase-s3-bootstrap 7.6 | 7.5 失败，dry-run remote_checked=false，未 push | 未执行 |
| passphrase-s3-bootstrap 7.7 | missing Keychain 真实 fail-closed；完整恢复矩阵未完成 | 部分取证 |
| passphrase-s3-bootstrap 7.8 | 精确 tuple 仅 candidate；不声明 supported/minimum stable | 未关闭 |
| passphrase-s3-bootstrap 7.9 | round-trip、完整恢复与 durable release 修复均未完成 | 未关闭 |
| trusted-continuity-dogfood-v1 8.3 | total_runs=1、completed_loops=1、codex、release_operations；source/weekly review 未测量 | 1/30；窗口至 2026-10-10 |
| trusted-continuity-dogfood-v1 8.4 | go_ready=false；silent confirmed writes=0，cross_project_routing=unvalidated | 不能作最终 Go 决策 |

## 本地标准证据

`temp/integration-test-runs/` 中每个 run 的 summary 由现有 `tools/testkit/evidence` 自动生成；真实命令仅保存结构化摘要、facts 和错误码，不保存笔记正文或凭据。`summary.status=passed` 只表示被包装的命令/contract probe 成功，不能替代上表业务门禁。

- `macos-homebrew-015-20260909`
- `macos-release-020-20260909`
- `macos-release-bootstrap-20260909`
- `macos-release-existing-preflight-20260909`
- `macos-release-existing-diff-20260909`
- `macos-release-keychain-missing-20260909`
- `macos-source-existing-dryrun-20260909`
- `macos-vault-final-validate-20260909`
- `remote-vault-validate-20260909`
- `remote-memory-status-20260909`
- `remote-memory-proposals-20260909`
- `remote-continuity-report-20260909`

恢复前置：不在聊天、日志或证据中提供口令；使用用户持有的既有仓库口令和修复后的 CLI 在本机安全交互输入。不要使用有隐式 Remember 缺陷的 release 重试 bootstrap；不要重新生成 content key 或覆盖 COS revision。

## 本轮修复与验证

修复范围为 `internal/cli/sync_repo_cmd.go`、`internal/cli/sync_repo_migrate_cmd.go` 和回归测试：bootstrap/migrate 的 Keychain 读取来源与持久化目标分离，只有显式 `--remember-keychain` 才传入写入目标。显式 remember 的原行为保留，但其真实 macOS 写入可靠性未通过；不要用该路径恢复真实口令。

- Mac 修复版构建 PASS。
- Mac focused CLI regression PASS：`go test ./internal/cli -run 'TestResolveRememberKeychainRequiresExplicitFlag|TestSyncRepoBootstrapAdditiveFlagsSurfaceFacts|TestSyncRepoMigrateDeviceProfileCommand' -count=1`。
- Mac 现有 agent-memory integration：9 packages PASS（`20260909T095536Z-7159`）。
- Mac sync/bootstrap/recovery fixture focused suite：89 tests PASS；这些是隔离 fixture，不能代替真实恢复矩阵。
- 修复版再次使用已有 Keychain 做无 Remember 的 bootstrap，返回 `sync_repo_unlock_required`，当前 Keychain source 无法提供有效解锁值（`macos-fixed-bootstrap-20260909`）。没有执行 COS push 或生成新 content key。
- 安全审查发现 migrate 同类隐式 Remember 缺陷后，扩大限定修复至该 handler；两入口统一使用显式 persistence gate。底层 credentialctl 的显式 Remember 写后验证与无回滚风险仍未修复，不在真实 Keychain 上继续试写。

本轮修复未发布，不能把源码构建标成正式 release candidate；`v0.2.0` 的真实 bootstrap 失败记录保留，7.5-7.9 不关闭。下一步需要用户在本机安全输入原仓库口令，使用修复版 `--unlock prompt --pull` 且不加 `--remember-keychain` 恢复验证；随后仍需新的 release artifact、完整恢复矩阵与真实 round-trip。

独立审查：bootstrap 的 no-remember 路径已通过安全审查；底层显式 Remember 仍为写入后验证，存在失败后条目状态不明风险。不得从 error 推断 Keychain 内容未改变。第一轮 `task check` PASS（66 Go packages、lint 0 issues、renderer 4 tests、OpenSpec 88 items）；最终双入口修复继续执行独立总门禁。

最终双入口版本验证：独立 `task check` exit 0（`temp/integration-test-runs/20260909-final-check-v2/`），Mac 构建与 bootstrap/migrate focused regression exit 0，最终三文件安全审查 PASS，无 blocking diff findings。底层显式 Remember 的真实平台安全性、原条目恢复、正式修复版、COS round-trip 和长期 dogfood 门禁仍未通过。源码与证据已保留，未 commit/push/release。
