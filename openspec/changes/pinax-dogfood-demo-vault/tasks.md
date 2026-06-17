# Tasks: Pinax Dogfood Demo Vault

Owner: `cli/pinax`  
Priority: P1  
Execution rule: demo vault 必须是 synthetic fixture，不含真实数据。E2E 测试不依赖网络/credentials。

## 0. 基线

- [ ] 0.1 Owner: `cli/pinax`; Lane: sequential; Depends on: none; Scope: 确认 design.md 中的 vault 结构和问题清单。Validation: `openspec validate pinax-dogfood-demo-vault --strict`。Expected: 校验通过。Failure re-check: 不混合定位/preview-release 变更。

## 1. Demo Vault fixture

- [ ] 1.1 Owner: `cli/pinax`; Lane: A; Depends on: 0.1; Scope: 创建 `examples/messy-vault/` 目录结构，包含 6 类故意问题的 synthetic Markdown notes（broken link、orphan、missing metadata、duplicate title、empty body、stale note）。Acceptance: `pinax vault doctor --vault ./examples/messy-vault --json` 能发现全部 6 类问题。Validation: `go run ./cmd/pinax vault doctor --vault ./examples/messy-vault --json`。Expected: 6 类问题各至少 1 个。
- [ ] 1.2 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: 创建 `examples/messy-vault/.pinax/config.yaml` 最小配置和 `examples/messy-vault/README.md` 说明。Acceptance: config 可被 `pinax init`/`pinax vault validate` 接受。Validation: `go run ./cmd/pinax vault validate --vault ./examples/messy-vault --json`。Expected: validation pass。
- [ ] 1.3 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: 确认 demo vault 不含真实数据/credentials/tokens。Acceptance: grep 扫描无敏感模式。Validation: 人工检查 + 敏感模式扫描。Expected: 无敏感内容。

## 2. E2E Proof Loop 测试

- [ ] 2.1 Owner: `cli/pinax`; Lane: B; Depends on: 1.2; Scope: 创建 testscript `tests/e2e/testdata/demo/scripts/demo_diagnose.txt`：使用 messy-vault fixture 运行 `pinax vault doctor`，断言发现 6 类问题。Acceptance: testscript 通过。Validation: `go test ./tests/e2e -run TestDemo -count=1`。Expected: 诊断结果匹配。
- [ ] 2.2 Owner: `cli/pinax`; Lane: B; Depends on: 2.1; Scope: 创建 testscript `demo_plan_snapshot_apply.txt`：plan --save → snapshot → apply --yes，断言低风险 metadata 补全成功、manual review 项不变。Acceptance: apply 后 metadata 被修复，broken/orphan/duplicate 不变。Validation: `go test ./tests/e2e -run TestDemoPlanSnapshotApply -count=1`。Expected: 低风险操作成功。
- [ ] 2.3 Owner: `cli/pinax`; Lane: B; Depends on: 2.2; Scope: 创建 testscript `demo_restore.txt`：从 snapshot restore 一个文件，断言内容回到 apply 前状态。Acceptance: restore 后文件内容与 pre-apply 一致。Validation: `go test ./tests/e2e -run TestDemoRestore -count=1`。Expected: 内容一致。
- [ ] 2.4 Owner: `cli/pinax`; Lane: B; Depends on: 2.3; Scope: 创建 `tests/e2e/demo_proof_loop_test.go` 注册 demo testscript。Acceptance: test runner 正确发现和执行 demo scripts。Validation: `go test ./tests/e2e -run TestDemo -count=1`。Expected: 全部通过。

## 3. Demo 文档

- [ ] 3.1 Owner: `cli/pinax`; Lane: C; Depends: 1.1; Scope: 创建 `docs/demo-proof-loop.md`：场景设定、step-by-step 命令（可复制运行）、预期输出摘要、安全保证说明、讲解 tips。Acceptance: 命令使用 messy-vault fixture 可运行。Validation: 人工审阅 + 命令可运行性检查。Expected: 文档独立可完成。
- [ ] 3.2 Owner: `cli/pinax`; Lane: C; Depends on: 3.1; Scope: 更新 `docs/README.md` 链接到 demo-proof-loop.md。Acceptance: 文档索引包含 demo。Validation: 人工审阅。Expected: 索引完整。

## 4. 验证

- [ ] 4.1 Owner: `cli/pinax`; Lane: sequential; Depends on: all previous; Scope: 运行 `task check`（含 demo E2E）和 `openspec validate pinax-dogfood-demo-vault --strict`。Acceptance: 全绿。Validation: 同上。Expected: 36+ openspec, 0 lint issues, tests pass, build success。
- [ ] 4.2 Owner: `cli/pinax`; Lane: sequential; Depends on: 4.1; Scope: 运行 `openspec validate --all --strict`。Acceptance: 全绿。Validation: 同上。Expected: 全部通过。
- [ ] 4.3 Owner: `cli/pinax`; Lane: sequential; Depends on: 4.2; Scope: 检查 `git status --short`：demo vault fixture 被提交，但 `.pinax/` 运行产物（events/receipts/snapshots/plans）不被提交。Acceptance: fixture notes + config + README + docs + tests 被提交。Validation: `git status --short`。Expected: 只有预期文件。
