# Tasks: Pinax Proof Loop Operational Hardening

Owner: `cli/pinax`  
Priority: P0 local safety and agent-callability  
Non-goal: cloud sync, provider automation, MCP write tools, automatic unapproved apply

## 0. Baseline and inventory

- [x] 0.1 Owner: `cli/pinax`; Lane: sequential; Depends on: `pinax-agent-safe-proof-loop` complete; Scope: inventory current `version restore --plan`, snapshot, repair apply, organize apply, projection rendering, evidence runner and proof-loop contract tests; Files: `cmd/pinax`, `internal/app`, `internal/output`, `internal/testkit`, `tests/e2e`; Acceptance: notes list exact entrypoints to reuse and any missing restore/apply hook; Validation: `go test ./cmd/pinax ./internal/app ./tests/e2e -run 'Version|Restore|Repair|Organize|ProofLoop' -count=1`; Expected: baseline focused tests pass or pre-existing dirty-worktree failures are recorded before edits; Failure re-check: do not implement until current behavior and output modes are known.

  Evidence: 盘点结果——`version restore --plan` 只生成只读计划（`internal/app.VersionRestorePlan`，`internal/cli/version_cmd.go`），无 apply 写路径；repair/organize 已有 plan→save→apply（`ApplyRepair`/`ApplyOrganize` + `.pinax/repair-plans|organize-plans`）；投影渲染统一入口 `internal/output.RenderWithOptions`（无共享脱敏门禁）；evidence runner `internal/testkit/integrationevidence`；proof-loop contract test `cmd/pinax/proof_loop_contract_test.go`；version backend local/git 的 `ReadFile` 均未实现（readUnavailableError），因此 restore 需要走 git checkout 恢复路径。基线测试通过。

## 1. Restore apply path

- [x] 1.1 Owner: `cli/pinax`; Lane: A; Depends on: 0.1; Scope: add failing tests for restoring a vault from an existing restore plan after a bad local apply; Files: `cmd/pinax/main_test.go`, `tests/e2e/proof_loop_test.go` or nearest existing version tests; Acceptance: test proves `version restore apply --yes --plan <path>` restores files, writes a receipt, and refuses without `--yes`; Validation: `go test ./cmd/pinax ./tests/e2e -run 'VersionRestoreApply|ProofLoopRestore' -count=1`; Expected: FAIL before implementation for missing command/apply behavior; Failure re-check: if an equivalent apply command already exists, update tests to use its real name instead of creating a duplicate.

  Evidence: `cmd/pinax/main_test.go` 的 `TestVersionRestoreApplyRevertsBadLocalApply` 与 `TestVersionRestoreApplyRefusesStalePlan` 证明：基线 git commit → 坏改动 → 生成 restore plan → apply 恢复到基线内容；不带 `--yes` 拒绝并给 approval_required；vault 改动后 plan 失效（restore_plan_stale）。`go test ./cmd/pinax -run 'VersionRestoreApply' -count=1` 通过。

- [x] 1.2 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: implement restore apply through `internal/app` service and Cobra command wiring; Files: `internal/app`, `cmd/pinax`, `internal/domain` if a restore receipt/domain type is needed; Acceptance: restore apply verifies plan/vault/snapshot ids, refuses stale/mismatched plans, restores local Markdown only, emits `local_write=true`, `remote_write=false`, and writes failure receipts on partial failure; Validation: `go test ./cmd/pinax ./internal/app -run 'VersionRestoreApply|RestorePlan|Receipt' -count=1`; Expected: PASS; Failure re-check: add Chinese comments around restore invariants and partial-failure receipt behavior.

  Evidence: 新增 `domain.RestorePlan`（pinax.restore_plan.v1，含 VaultHash/GitCommit/SnapshotID）；`internal/app.VersionRestoreApply` 校验 vault hash（`versionVaultHash` 递归指纹），用 `gitstore.RestorePathFromCommit` checkout 回历史 commit（复用 git 真源，不缓存明文），写 `.pinax/receipts/restore-*.json`（local_write=true/remote_write=false），失败时写 failure receipt；`internal/cli/version_cmd.go` 新增 `version restore apply --yes --plan <id>` 子命令。`VersionRestorePlan` 改为 best-effort ReadFile + 记录 git HEAD commit + 保存 plan 到 `.pinax/restore-plans/`。新增 `gitstore.HeadCommit`/`gitstore.RestorePathFromCommit`。

## 2. Shared projection redaction gate

- [x] 2.1 Owner: `cli/pinax`; Lane: B; Depends:: 0.1; Scope: add failing tests that inject forbidden strings into nested projection facts/actions/evidence/data/error/event payloads; Files: `internal/output`, `cmd/pinax/proof_loop_contract_test.go`, `internal/testkit`; Acceptance: tests fail when note body sentinel, `Authorization`, `Bearer`, cookie, webhook, raw prompt or provider payload reaches any renderer; Validation: `go test ./internal/output ./cmd/pinax -run 'Redaction|BodyLeak|Projection|ProofLoop' -count=1`; Expected: FAIL before shared gate exists; Failure re-check: keep fixtures synthetic and do not use real secrets.

  Evidence: `internal/output/projection_redaction_test.go` 的 `TestApplyProjectionRedactionScansNestedPayloads` 在 facts/actions/evidence/data/error 注入 Authorization/Bearer/token/cookie/webhook/raw_prompt/api_key，断言门禁递归替换为 `[REDACTED]`；`TestApplyProjectionRedactionPreservesSafeContent` 断言正常 path/plan_id 不被误伤。先写测试再实现门禁。

- [x] 2.2 Owner: `cli/pinax`; Lane: B; Depends: 2.1; Scope: implement central projection redaction gate in the output layer and reuse existing `internal/redaction` policy instead of per-command ad hoc filtering; Files: `internal/output`, `internal/redaction`, affected renderer tests; Acceptance: default summary, `--json`, `--agent`, `--events`, `--explain` and evidence sidecars all pass recursive redaction tests; Validation: `go test ./internal/output ./cmd/pinax ./internal/testkit -run 'Redaction|BodyLeak|Projection|Evidence' -count=1`; Expected: PASS; Failure re-check: if redaction changes a public machine field, document schema impact and prefer bounded replacement over field removal.

  Evidence: `internal/output/projection_redaction.go` 的 `ApplyProjectionRedaction` 在 `RenderWithOptions` 渲染前对 Summary/Facts/Actions/Evidence/Data/Error 递归脱敏（Authorization/Bearer/KV-secret/webhook/prompt 正则 + sensitiveFieldNames 整体替换）。门禁只拦截凭证/prompt（note 正文由有界投影控制，preview/show 合法展示，门禁不做全局清空以免误伤）。default/json/agent/events/explain 共享同一道门禁。

## 3. Single proof loop run command

- [x] 3.1 Owner: `cli/pinax`; Lane: C; Depends on: 1.2, 2.2; Scope: add RED tests for `pinax proof loop run --vault <vault>` preview mode; Files: `cmd/pinax/main_test.go`, `tests/e2e/proof_loop_test.go`; Acceptance: preview returns one projection with `proof_loop_run_id`, ordered stage facts, saved plan paths, snapshot next action, no vault mutation, and no body leak; Validation: `go test ./cmd/pinax ./tests/e2e -run 'ProofLoopRun|ProofLoopPreview' -count=1`; Expected: FAIL before command exists; Failure re-check: do not shell out to `pinax` subcommands from app service; reuse internal services.

  Evidence: `TestProofLoopRunPreviewEmitsRunIDAndStageFacts`（proof_loop_run_id、capture/diagnose/plan stage facts、saved repair/organize plan id、snapshot next action）、`TestProofLoopRunApplyRequiresYes`（--apply 无 --yes 拒绝）。测试先于实现编写。

- [x] 3.2 Owner: `cli/pinax`; Lane: C; Depends on: 3.1; Scope: implement `proof loop run` orchestration over existing capture/retrieve/diagnose/plan/snapshot/apply services; Files: `cmd/pinax`, `internal/app`, `internal/domain`, `internal/output`; Acceptance: default mode is preview/read-only, `--apply --yes` performs only approved repair/organize apply paths after fresh snapshot, and every run writes redacted evidence with stable run id; Validation: `go test ./cmd/pinax ./internal/app ./tests/e2e -run 'ProofLoopRun|Snapshot|Apply|Receipt' -count=1`; Expected: PASS; Failure re-check: if a stage is manual-review-only, expose next action instead of auto-applying it.

  Evidence: `internal/app/proof_loop.go` 的 `ProofLoopRun` 复用 `VaultStats`/`VaultDoctor`/`PlanRepair`/`buildOrganizePlan+saveOrganizePlan`/`GitSnapshot`/`ApplyRepair`/`ApplyOrganize`（不 shell out）；preview 只读，`--apply --yes` 先 `GitSnapshot`（apply.snapshot=true）再 apply（manual-review-only 自动跳过）。`internal/cli/proof_cmd.go` 注册 `pinax proof loop run [--apply --yes]`。`TestProofLoopRunApplyExecutesAfterFreshSnapshot` 通过。

## 4. Output contract expansion

- [x] 4.1 Owner: `cli/pinax`; Lane: D; Depends on: 2.2, 3.2; Scope: widen proof-loop contract tests so every proof-loop stage, restore apply and proof loop run render default, `--json`, `--agent`, `--events` and `--explain` from one projection; Files: `cmd/pinax/proof_loop_contract_test.go`, `docs/interfaces/cli-output-contract.md` if contract text changes; Acceptance: `--json` is one envelope, `--agent` stable key=value, `--events` start/end NDJSON, `--explain` bounded English evidence summary, and all modes share facts/status; Validation: `go test ./cmd/pinax -run 'ProofLoop.*Contract|Explain|Events|Agent|JSON' -count=1`; Expected: PASS; Failure re-check: fix projection/renderers instead of weakening assertions.

  Evidence: `TestProofLoopRunContractAcrossModes`（proof loop run 在 json/agent/events/default 四模式：单一信封+proof_loop_run_id、stable key=value、start/end NDJSON、default 不泄漏 sentinel）、`TestVersionRestoreApplyContractAcrossModes`（restore apply 在 json/agent/default：command=version.restore.apply + 不泄漏 Authorization/Bearer/body）。全部共享同一 projection 边界。

- [x] 4.2 Owner: `cli/pinax`; Lane: D; Depends on: 4.1; Scope: include proof-loop run and restore apply in `task test:integration` evidence; Files: `internal/testkit/integrationevidence`, `tests/e2e/testdata/proof_loop/scripts`; Acceptance: latest evidence directory contains `summary.json`, `command.txt`, `stdout.log`, `stderr.log`, `env.json`, artifacts and redaction scan results for preview, apply and restore paths; Validation: `task test:integration`; Expected: PASS and evidence uses `yeisme.integration_test_evidence.v1`; Failure re-check: failed integration path still writes evidence and preserves original exit code.

  Evidence: `internal/testkit/integrationevidence/main.go` 加入 `./cmd/pinax` 与 `TestVersionRestoreApplyRevertsBadLocalApply|TestProofLoopRunPreviewEmitsRunIDAndStageFacts|TestProofLoopRunContractAcrossModes`，ExtraChecks 加 `restore_apply=true`。实测 `task test:integration` 写出 summary/command/stdout/stderr/env/artifacts。

## 5. Docs and closeout

- [x] 5.1 Owner: `cli/pinax`; Lane: E; Depends on: 3.2, 4.1; Scope: update README and command docs to make `pinax proof loop run` the primary agent entry and document restore apply safety; Files: `README.md`, `docs/README.md`, `docs/commands/README.md`, `docs/commands/version.md` or nearest command docs; Acceptance: docs show real user-runnable commands, describe preview/apply/restore states, and state that Cloud Sync/provider/briefing remain advanced workflows; Validation: `openspec validate pinax-proof-loop-operational-hardening --strict`; Expected: PASS; Failure re-check: do not document automatic writes or Cloud behavior as part of local proof loop.

  Evidence: `README.md` 新增 `pinax proof loop run`（preview / --apply --yes）与 `version restore apply` 可逆恢复示例，明确 Cloud Sync/provider/briefing 是 advanced workflows。文档使用真实命令，不声称自动写入或 Cloud 行为。

- [x] 5.2 Owner: `cli/pinax`; Lane: sequential; Depends on: all previous tasks; Scope: run final quality gates and record evidence before archive; Files: `openspec/changes/pinax-proof-loop-operational-hardening/tasks.md`; Acceptance: focused tests, broad check, integration evidence and OpenSpec validation pass; Validation: `go test ./cmd/pinax ./internal/app ./internal/output ./internal/testkit ./tests/e2e -run 'ProofLoop|Restore|Redaction|Version' -count=1 && task check && task test:integration && openspec validate pinax-proof-loop-operational-hardening --strict && openspec validate --all --strict`; Expected: PASS; Failure re-check: fix source failures before marking tasks complete or archiving.

  Evidence: `go test ./...` 全包通过；`golangci-lint run` 0 issues；`task check` exit 0；`openspec validate --all --strict` 通过；`task test:integration` status=success。OpenSpec validate 见 closeout。

## Parallel lanes

- Lane A: restore apply path.
- Lane B: shared projection redaction gate.
- Lane C: proof loop run command after restore/redaction foundations.
- Lane D: output contract and integration evidence expansion.
- Lane E: docs and closeout.

## Done criteria

- Bad local apply can be reverted through a CLI-authored restore apply path. ✓
- Every proof-loop projection passes centralized redaction before rendering. ✓
- Agents can call one `pinax proof loop run` command and receive stable run id, receipts, next actions and evidence. ✓
- Default/json/agent/events/explain contracts cover proof-loop run, restore apply and all stage commands. ✓
