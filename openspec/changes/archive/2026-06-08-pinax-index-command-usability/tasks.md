## 1. 合同测试先行

- [x] 1.1 增加 `pinax index` 默认摘要测试，验证 readonly、中文摘要、推荐下一步和 `--json`/`--agent` 稳定 key。
- [x] 1.2 增加 `pinax index --help` 测试，验证 `status -> refresh -> doctor -> rebuild/sync/repair` 工作流顺序和中文示例。
- [x] 1.3 增加 `index refresh` contract tests，覆盖 missing、fresh、stale、partial failure 和 unmanaged Markdown 排除。
- [x] 1.4 增加 `index doctor` contract tests，覆盖 missing、stale、schema mismatch、unreadable/corrupt projection 的 issue code、evidence 和 next action。
- [x] 1.5 增加 `index repair` contract tests，覆盖 `--dry-run`、缺少 `--yes` 的 `approval_required`、`--yes` recreate 后 projection fresh。
- [x] 1.6 增加 machine output tests，验证 `--json` stdout 只有 JSON、`--agent` 无中文 prose、`--explain` 是中文证据摘要。

## 2. Index 诊断和维护模型

- [x] 2.1 在 `internal/index` 增加 `DoctorReport`、`Issue`、`RefreshResult`、`RepairPlan/RepairResult` 等窄结构，字段保持 JSON/agent 友好的英文 key。
- [x] 2.2 扩展 Inspect/diagnose 逻辑，识别 missing、fresh、stale、partial、unreadable、schema mismatch、row consistency issue。
- [x] 2.3 实现 `Refresh` 底层逻辑，复用现有 GORM projection，跳过未变更笔记并补齐缺失/过期行。
- [x] 2.4 实现 refresh partial failure 聚合，记录失败路径、红acted evidence、失败计数和最终状态。
- [x] 2.5 实现 projection-safe repair 计划生成，限制为 recreate/backup/remove stale projection rows 等索引层操作。
- [x] 2.6 实现 repair apply，默认备份旧 index projection，重建 registered Pinax notes，并保证不写 Markdown、record ledger、Git 或 provider 状态。

## 3. Application service 和输出投影

- [x] 3.1 增加 `IndexSummary` service，裸 `pinax index` 使用它生成状态摘要、影响范围和 recommended action。
- [x] 3.2 增加 `IndexRefresh` service，投影 refresh facts、evidence、actions 和 partial/failed 错误。
- [x] 3.3 增加 `IndexDoctor` service，投影 issue summary、issue data、影响说明和 next action。
- [x] 3.4 增加 `IndexRepair` service，处理 dry-run、approval_required、repair apply、event evidence 和 final status。
- [x] 3.5 调整 `IndexStatus` next action，能安全 refresh 时优先推荐 `pinax index refresh`，结构性问题推荐 doctor/rebuild。
- [x] 3.6 如默认 summary 表格不足，补充 index 专用 human summary renderer；不得影响 JSON/agent/events 输出。

## 4. CLI 命令树和帮助体验

- [x] 4.1 更新 `internal/cli/index_cmd.go`，给 `index` root 增加默认 RunE，接入 `IndexSummary`。
- [x] 4.2 增加 `index refresh`、`index doctor`、`index repair` 子命令和 flags：`--dry-run`、`--yes`、`--kind`、必要的 batch/limit 选项。
- [x] 4.3 更新 index help 的中文 Short/Long/Example，明确 refresh/rebuild/doctor/repair 的使用边界。
- [x] 4.4 更新错误 hint 和 actions，禁止建议用户直接编辑或删除 `.pinax/index.sqlite`。

## 5. E2E 和边界验证

- [x] 5.1 使用 testscript 增加 index usability e2e，覆盖真实进程、fixture vault、stdout/stderr 分离和 machine modes。
- [x] 5.2 增加 corrupt/unreadable index fixture，不依赖真实用户权限或平台特定错误文本。
- [x] 5.3 增加大 vault 或 benchmark 风险的聚焦测试/benchmark，确认 refresh 能跳过未变更笔记。
- [x] 5.4 验证 `search`、`query run --lazy-index`、`note list`、`organize plan` 仍能消费 fresh/stale index facts。

## 6. 文档、规格和门禁

- [x] 6.1 更新 README/docs 中 index 相关示例，推荐 `pinax index`、`refresh`、`doctor`、`rebuild` 主路径。
- [x] 6.2 运行聚焦测试：`go test ./cmd/pinax ./internal/cli ./internal/index ./internal/app -run 'Index|Search|Query' -count=1`。
- [x] 6.3 运行 e2e：`go test ./tests/e2e -run Index -count=1`。
- [x] 6.4 运行 `openspec validate pinax-index-command-usability` 和 `openspec validate --all`。
- [x] 6.5 运行 `task check`，并把验证证据记录到本文件。

## Verification Evidence

- 2026-06-08: `go test ./cmd/pinax -run 'Test(IndexDefaultSummaryAndMachineContractsCLI|NotebookCoreOutputContractAndHelp)$' -count=1` 通过，覆盖 index 默认摘要、readonly、JSON/agent facts、help 工作流顺序和中文示例。
- 2026-06-08: `go test ./cmd/pinax -run 'Test(IndexDefaultSummaryAndMachineContractsCLI|IndexRefreshContractsCLI|NotebookCoreOutputContractAndHelp)$' -count=1` 通过，覆盖 index summary/help/refresh 的 missing、fresh、stale 和 unmanaged Markdown 排除；refresh partial failure 仍待补。
- 2026-06-08: `go test ./cmd/pinax -run 'Test(IndexDefaultSummaryAndMachineContractsCLI|IndexRefreshContractsCLI|NotebookCoreOutputContractAndHelp)$' -count=1` 通过，补齐 refresh partial failure：缺失 `note_id` 的 Pinax note 返回 `status=partial`、`failed=1`、failed path evidence、doctor/rebuild actions，且有效 notes 继续 refresh。
- 2026-06-08: `go test ./cmd/pinax -run 'Test(IndexDefaultSummaryAndMachineContractsCLI|IndexRefreshContractsCLI|IndexDoctorContractsCLI|NotebookCoreOutputContractAndHelp)$' -count=1` 通过，覆盖 index doctor missing、schema mismatch、changed-note stale、corrupt/unreadable、issue_codes、evidence、agent issue lines 和 safe next actions。
- 2026-06-08: `go test ./cmd/pinax -run 'Test(IndexDefaultSummaryAndMachineContractsCLI|IndexRefreshContractsCLI|IndexDoctorContractsCLI|IndexRepairContractsCLI|NotebookCoreOutputContractAndHelp)$' -count=1` 通过，覆盖 index repair dry-run no-write、approval_required no-write、recreate --yes 备份旧 projection 并重建为 fresh。
- 2026-06-08: `go test ./cmd/pinax ./internal/cli ./internal/index ./internal/app -run 'Index|Search|Query' -count=1` 通过。
- 2026-06-08: `go test ./cmd/pinax -run TestIndexMachineOutputContractsCLI -count=1` 通过，覆盖 index doctor `--json` stdout 纯 JSON/stderr 空、`--agent` 无中文 prose、repair `--events` NDJSON、index `--explain` 中文证据和 next action。
- 2026-06-08: `go test ./cmd/pinax -run 'Test(IndexDefaultSummaryAndMachineContractsCLI|IndexRefreshContractsCLI|IndexDoctorContractsCLI|IndexRepairContractsCLI|IndexMachineOutputContractsCLI|NotebookCoreOutputContractAndHelp)$' -count=1` 通过，覆盖 `index status` missing 优先推荐 refresh，且 index status/doctor/repair 不建议手动编辑或删除 `.pinax/index.sqlite`。
- 2026-06-08: `go test ./tests/e2e -run Index -count=1` 通过，新增 testscript 覆盖真实进程 index summary/doctor/refresh/repair、stdout/stderr 分离、agent/json/events modes、unmanaged Markdown 排除和 corrupt projection repair fixture。
- 2026-06-08: `go test ./cmd/pinax ./internal/cli ./internal/index ./internal/app -run 'Index|Search|Query' -count=1` 通过；新增 `internal/index.Refresh`/`RefreshResult` 并验证 missing refresh 创建 projection、fresh refresh 跳过未变更 notes。
- 2026-06-08: `go test ./cmd/pinax ./internal/app -run 'Index|Search|Query|NoteList|NotebookOrganization|Organize' -count=1` 通过，验证 search、query lazy-index、note list、organize plan 相关路径仍可工作。
- 2026-06-08: `go test ./internal/index -run '^$' -bench BenchmarkIndexRefreshSkipsUnchanged -benchtime=1x -count=1` 通过，benchmark 使用 200 notes 验证 refresh skip unchanged indexed=0/skipped=200/batches=4。
- 2026-06-08: `go test ./internal/index -run 'Inspect|Diagnose|Refresh|Index' -count=1` 通过；新增 `DoctorReport`/`Issue`/`RepairPlan`/`RepairResult` 和 `Diagnose`，覆盖 missing/fresh/stale/partial/unreadable/schema mismatch/row consistency。
- 2026-06-08: `go run ./cmd/pinax index --vault <tmp-vault>` 人工检查默认 summary 已包含状态、facts、证据和下一步；无需新增 index 专用 human renderer，机器输出路径未改动。
- 2026-06-08: 更新 `README.md`、`docs/README.md`、`docs/operations/local-development.md`、`docs/interfaces/cli-output-contract.md` 的 index 示例和 missing/stale 维护建议，主路径改为 `pinax index` -> `refresh` -> `doctor` -> 显式 `rebuild`。
- 2026-06-08: `openspec validate pinax-index-command-usability` 通过；`openspec validate --all` 通过，24 passed / 0 failed。
- 2026-06-08: `task check` 通过；包含 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`，lint 0 issues，OpenSpec 24 passed / 0 failed。
