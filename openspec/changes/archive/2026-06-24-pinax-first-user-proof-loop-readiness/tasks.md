# Pinax 首次用户 Proof Loop Readiness 任务

## 分组说明

- **Lane A: 当前改动收口**，必须先完成，防止新主线压在未完成 TaskBridge/release 改动上。
- **Lane B: 黄金路径和 demo vault**，实现用户可感知主体验。
- **Lane C: 输出、脱敏和合同**，与 Lane B 可并行，但 D/E 依赖它。
- **Lane D: 集成证据**，依赖 B/C 的稳定命令和 redaction。
- **Lane E: 文档和 release smoke**，依赖 B/C 的最终命令形态。
- **Lane Z: Final Gate**，所有 lane 完成后执行。

## Lane A: 当前改动收口

- [x] **A1. 审计当前未提交改动边界**
  - Owner: `cli/pinax`
  - Scope: 只检查当前 dirty worktree，把改动按 `TaskBridge daily todolist`、`release docs`、`proof command wiring`、`cloud/backend plan test`、`unrelated` 分类。
  - Depends on: none
  - Parallel lane: A
  - Acceptance: 输出一段简短审计结论，列出每组文件和是否属于本 change 前置依赖。
  - Validation command: `git status --short && git diff --stat`
  - Expected result: 能明确哪些未提交文件必须先完成或排除。
  - Failure re-check: 如果发现用户未完成实现，不要回滚，先暂停并让 owner 决定是否纳入当前交付。

- [x] **A2. 校正 TaskBridge daily todolist OpenSpec 状态**
  - Owner: `cli/pinax`
  - Scope: 检查 `openspec/changes/archive/2026-06-22-pinax-taskbridge-daily-todolist/` 与 `openspec/specs/planning-workflows/spec.md` 是否一致，判断它是已完成归档还是误归档。
  - Depends on: A1
  - Parallel lane: A
  - Acceptance: 若已完成，记录验证证据并保持 archive；若未完成，恢复为 active change 或从本次主线中显式排除。
  - Validation command: `openspec validate --all --strict`
  - Expected result: OpenSpec 主 specs 与 change 状态没有“已归档但实现未完成”的矛盾。
  - Failure re-check: 不允许只修改 main spec 而没有对应实现、测试或归档证据。

- [x] **A3. 校验 release 版本文档真实性**
  - Owner: `cli/pinax`
  - Scope: 核对 README 和 quickstart 中 `v0.1.2` 下载链接、checksum 命令和 archive 名称是否与实际 release asset 命名一致。
  - Depends on: A1
  - Parallel lane: A
  - Acceptance: 文档只展示真实用户可运行命令，不展示 agent-only wrapper 或本地别名。
  - Validation command: `rg -n "v0.1.0|v0.1.2|checksums|pinax_" README.md README.zh-CN.md docs/quickstart.md`
  - Expected result: 旧版本示例被消除，新版本命令前后一致。
  - Failure re-check: 如果 release asset 不存在，不要继续宣传该版本，先改为已发布的真实 tag。

## Lane B: 黄金路径和 demo vault

- [x] **B1. 固定首次用户黄金路径命令表**
  - Owner: `cli/pinax`
  - Scope: 定义唯一黄金路径命令序列：`version`、`init`、`note add`、`proof loop run --json`、`repair plan --save --json`、`version snapshot`、`repair apply --yes`、`version restore --plan`、`version restore apply --yes`。
  - Depends on: A1
  - Parallel lane: B
  - Acceptance: 每条命令都有输入、期望输出字段、写入状态、失败 next action。
  - Validation command: `pinax --help && pinax proof loop --help && pinax version restore --help`
  - Expected result: 所有命令在 CLI 中真实存在，flag 名称与文档一致。
  - Failure re-check: 如果命令不存在，改任务为实现缺口，不允许在 docs 中保留假命令。

- [x] **B2. 构造 deterministic demo vault fixture**
  - Owner: `cli/pinax`
  - Scope: 更新 `examples/messy-vault` 或新增 fixture 生成器，使其稳定包含 broken link、missing tags、orphan note、manual review 项、低风险可 apply 项和可 restore 文件。
  - Depends on: B1
  - Parallel lane: B
  - Acceptance: fixture 内容 deterministic，不含真实用户路径、token、Authorization、provider payload 或 hidden prompt。
  - Validation command: `go test ./tests/e2e -run 'Proof|Fixture|MessyVault' -count=1`
  - Expected result: 测试能断言具体 issue code 和具体 fixture path。
  - Failure re-check: 不允许只检查命令成功，必须检查 seeded issue 被诊断出来。

- [x] **B3. 增加 proof loop preview 黄金路径 e2e**
  - Owner: `cli/pinax`
  - Scope: 用 testscript 覆盖 demo vault 的 `pinax proof loop run --vault <fixture> --json`，断言 `proof_loop_run_id`、阶段事实、next action 和 `local_write=false`。
  - Depends on: B2
  - Parallel lane: B
  - Acceptance: preview 不写 Markdown、`.pinax` planning/apply assets、Git state、remote state。
  - Validation command: `go test ./tests/e2e -run 'ProofLoopPreview' -count=1`
  - Expected result: preview 测试通过，且失败时能定位具体缺失阶段。
  - Failure re-check: 如果 preview 写入任何 apply/receipt 状态，回到 app service 修复写入 gate。

- [x] **B4. 增加 plan、snapshot、apply 黄金路径 e2e**
  - Owner: `cli/pinax`
  - Scope: 覆盖 `repair plan --save --json`、`version snapshot`、`repair apply --plan <id> --yes --json`，断言 plan id、snapshot fact、apply receipt 和 changed path。
  - Depends on: B2
  - Parallel lane: B
  - Acceptance: apply 只执行低风险修复，manual review 项不会自动修改正文或删除文件。
  - Validation command: `go test ./tests/e2e -run 'ProofLoopApply' -count=1`
  - Expected result: apply 路径通过，receipt 可定位到 plan id 和 snapshot。
  - Failure re-check: 如果 apply 依赖最新扫描状态，测试必须先创建新鲜 plan，不能复用 stale plan。

- [x] **B5. 增加 restore 黄金路径 e2e**
  - Owner: `cli/pinax`
  - Scope: 覆盖 `version restore <path> --revision HEAD --plan --json` 和 `version restore apply --plan <restore_id> --yes --json`。
  - Depends on: B4
  - Parallel lane: B
  - Acceptance: restore 使用 CLI service 写回，输出包含 `local_write=true`、`remote_write=false`，并拒绝 stale restore plan。
  - Validation command: `go test ./tests/e2e -run 'ProofLoopRestore' -count=1`
  - Expected result: restore 可以恢复指定文件，且不会触发 remote write。
  - Failure re-check: 如果 version backend 不可用，错误必须是稳定 code，并包含可运行 next action。

## Lane C: 输出、脱敏和合同

- [x] **C1. 扩展 JSON envelope 合同断言**
  - Owner: `cli/pinax`
  - Scope: 为黄金路径所有命令增加 JSON envelope 断言，要求顶层包含 `spec_version`、`mode`、`command`、`status`，错误输出保持单一 envelope。
  - Depends on: B1
  - Parallel lane: C
  - Acceptance: stdout 是合法 JSON，不混入 human prose；stderr 只放诊断或外部命令输出。
  - Validation command: `go test ./cmd/pinax ./tests/e2e -run 'JSON|Envelope|ProofLoop' -count=1`
  - Expected result: 所有黄金路径 JSON 输出合同通过。
  - Failure re-check: 不允许在命令层手拼 JSON，修复应回到 projection/rendering。

- [x] **C2. 扩展 agent key=value 合同断言**
  - Owner: `cli/pinax`
  - Scope: 为 preview、plan、apply、restore 增加 `--agent` 输出断言，要求稳定 key、可脚本解析、值中空格正确引用。
  - Depends on: B1
  - Parallel lane: C
  - Acceptance: agent 输出不含中文 prose、raw body、secret、provider payload 或 ANSI 控制码。
  - Validation command: `go test ./cmd/pinax ./tests/e2e -run 'Agent|ProofLoop' -count=1`
  - Expected result: `--agent` 输出每行都是 `key=value`。
  - Failure re-check: 如果 human summary 泄漏到 agent stdout，修复输出模式分离。

- [x] **C3. 递归 body-leak 和 secret-leak 扫描**
  - Owner: `cli/pinax`
  - Scope: 对 stdout、stderr、events、saved plan、receipt、snapshot evidence 和 restore evidence 做递归扫描。
  - Depends on: C1, C2
  - Parallel lane: C
  - Acceptance: 禁止非显式 body display 中出现 `body`、`note_body`、`raw_body` 非空字段，以及 Authorization、Bearer、api_key、secret、raw prompt、provider payload sentinel。
  - Validation command: `go test ./cmd/pinax ./tests/e2e -run 'Redaction|BodyLeak|ProofLoop' -count=1`
  - Expected result: 所有受保护 surface 无敏感值。
  - Failure re-check: 修复 redaction/projection 源头，不允许只改 golden fixture。

- [x] **C4. 错误路径命名和 next action 合同**
  - Owner: `cli/pinax`
  - Scope: 为 empty vault、missing vault、stale plan、missing snapshot、restore stale、invalid note ref 增加稳定错误码和 next action 断言。
  - Depends on: C1
  - Parallel lane: C
  - Acceptance: 错误不沉默、不 panic、不输出 stack trace 到 machine stdout。
  - Validation command: `go test ./cmd/pinax ./tests/e2e -run 'ProofLoop|Error|Stale|Missing' -count=1`
  - Expected result: 错误路径都有稳定 code 和可执行 next action。
  - Failure re-check: 不允许 catch-all 只打印 generic failure。

## Lane D: 集成证据

- [x] **D1. 固定 `task test:integration` proof loop 入口**
  - Owner: `cli/pinax`
  - Scope: 确保 `task test:integration` 覆盖 proof loop readiness，并调用项目 runner 生成证据。
  - Depends on: B3, B4, C3
  - Parallel lane: D
  - Acceptance: 每次运行写入 `temp/integration-test-runs/<run-id>/`。
  - Validation command: `task test:integration`
  - Expected result: 命令通过并生成最新 evidence 目录。
  - Failure re-check: 不要新增平行测试框架，复用现有 Go/testscript/Taskfile。

- [x] **D2. 完成 evidence 最小文件集断言**
  - Owner: `cli/pinax`
  - Scope: 测试 latest run directory 必须包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json`、`artifacts/`。
  - Depends on: D1
  - Parallel lane: D
  - Acceptance: `summary.json` 由 runner 生成，包含 schema、project、run_id、layer、command、status、exit_code、started_at、finished_at、duration_ms、evidence、redaction。
  - Validation command: `go test ./tests/e2e -run 'IntegrationEvidence' -count=1`
  - Expected result: evidence schema 与 `yeisme.integration_test_evidence.v1` 一致。
  - Failure re-check: agent 不得手写 official `summary.json`。

- [x] **D3. 完成失败仍保留 evidence 断言**
  - Owner: `cli/pinax`
  - Scope: 注入一个可控失败命令，断言 runner 保留原始非零 exit code 且仍写 redacted evidence。
  - Depends on: D2
  - Parallel lane: D
  - Acceptance: 失败 evidence 不吞 stderr、不覆盖 exit code、不泄漏 sentinel。
  - Validation command: `go test ./tests/e2e -run 'IntegrationEvidenceFailure' -count=1`
  - Expected result: 失败路径测试通过。
  - Failure re-check: 如果 failure path 无 evidence，修复 runner defer/finalize 逻辑。

- [x] **D4. 增加 evidence 人工检查命令文档**
  - Owner: `cli/pinax`
  - Scope: 在开发/测试文档中记录如何查看最新 integration evidence。
  - Depends on: D1
  - Parallel lane: D
  - Acceptance: 文档展示真实命令，不展示本地 wrapper。
  - Validation command: `find temp/integration-test-runs -maxdepth 2 -type f | sort | tail -50`
  - Expected result: 用户能用命令定位 latest evidence 文件。
  - Failure re-check: 不承诺提交 temp evidence，`temp/` 仍不应进入 Git。

## Lane E: 文档和 release smoke

- [x] **E1. README 首屏收束为 Proof Loop 主线**
  - Owner: `cli/pinax`
  - Scope: 调整 `README.md` 和 `README.zh-CN.md`，第一屏只承载定位、三概念、aha moment、安装和 Proof Loop 黄金路径。
  - Depends on: B1, C1
  - Parallel lane: E
  - Acceptance: Cloud、Plugin、Publish、KB、Planning 只作为高级入口，不抢第一路径。
  - Validation command: `rg -n "Proof Loop|Cloud Sync|Dynamic plugins|publish|kb|memory" README.md README.zh-CN.md`
  - Expected result: 首屏主线清晰，高级能力下沉。
  - Failure re-check: 不删除高级能力文档，只调整信息层级。

- [x] **E2. Quickstart 改成 5 分钟可复制脚本**
  - Owner: `cli/pinax`
  - Scope: 更新 `docs/quickstart.md`，确保每一步命令可直接复制运行，并标注 preview/write/restore 的写入边界。
  - Depends on: B5, C4
  - Parallel lane: E
  - Acceptance: quickstart 覆盖安装、init、note add、proof preview、plan、snapshot、apply、restore。
  - Validation command: `rg -n "pinax (version|init|note add|proof loop run|repair plan|version snapshot|repair apply|version restore)" docs/quickstart.md`
  - Expected result: quickstart 包含完整黄金路径命令。
  - Failure re-check: 文档不能引用不存在的 `<plan_id>` 来源，必须说明如何从上一步输出获取。

- [x] **E3. 增加 release archive smoke 测试入口**
  - Owner: `cli/pinax`
  - Scope: 增加本地 release smoke 任务或脚本，下载/使用 archive 二进制，在临时目录运行 `version`、`init`、`note add`、`proof loop run --json`。
  - Depends on: A3, B3, C1
  - Parallel lane: E
  - Acceptance: smoke 不依赖源码路径、不写用户 vault、不需要真实 token。
  - Validation command: `task release:smoke`
  - Expected result: release-installed binary 能跑通最小 proof loop preview。
  - Failure re-check: 如果项目没有 `task release:smoke`，先添加 Taskfile 入口并用本地 dist binary dry-run 验证。

- [x] **E4. 增加 release smoke 失败诊断**
  - Owner: `cli/pinax`
  - Scope: 对 checksum mismatch、archive missing、binary not executable、unsupported OS/arch、proof loop command failure 增加稳定 stderr 诊断和文档说明。
  - Depends on: E3
  - Parallel lane: E
  - Acceptance: release smoke 失败能告诉用户下一步检查 release asset、checksum 或 PATH。
  - Validation command: `task release:smoke`
  - Expected result: 正常路径通过，失败路径可诊断。
  - Failure re-check: 不把 token、完整本地路径或临时目录细节写入 machine stdout。

## Lane Z: Final Gate

- [x] **Z1. OpenSpec 严格验证**
  - Owner: `cli/pinax`
  - Scope: 验证本 change 和全量 specs。
  - Depends on: A2, all B/C/D/E tasks
  - Parallel lane: sequential
  - Acceptance: 本 change 和全量 OpenSpec 均通过 strict validate。
  - Validation command: `openspec validate pinax-first-user-proof-loop-readiness --strict && openspec validate --all --strict`
  - Expected result: 两条命令均通过。
  - Failure re-check: 如果 delta spec 无法匹配主 spec，修正 spec header，不绕过 validate。

- [x] **Z2. 全量质量门禁**
  - Owner: `cli/pinax`
  - Scope: 运行 Pinax 标准质量门禁。
  - Depends on: Z1
  - Parallel lane: sequential
  - Acceptance: format、lint、unit、e2e、build、OpenSpec 全部通过。
  - Validation command: `task check`
  - Expected result: `task check` exit 0。
  - Failure re-check: 先修源头失败，不降低测试断言或跳过子任务。

- [x] **Z3. 集成证据终检**
  - Owner: `cli/pinax`
  - Scope: 运行集成测试并人工检查 latest evidence。
  - Depends on: Z2
  - Parallel lane: sequential
  - Acceptance: latest evidence summary 记录 `project=cli/pinax`、`redaction.applied=true`、`status=passed`。
  - Validation command: `task test:integration && find temp/integration-test-runs -maxdepth 2 -type f | sort | tail -50`
  - Expected result: 证据目录完整且无 forbidden sentinel。
  - Failure re-check: 失败也必须保留 evidence，并带原始 exit code。

- [x] **Z4. 归档前审查**
  - Owner: `cli/pinax`
  - Scope: 确认 tasks 全部完成、specs 已同步、docs 不含假命令、release smoke 通过。
  - Depends on: Z3
  - Parallel lane: sequential
  - Acceptance: 具备归档条件，但归档必须在实现和验证完成后单独执行。
  - Validation command: `openspec validate pinax-first-user-proof-loop-readiness --strict`
  - Expected result: change 可归档。
  - Failure re-check: 如果还有 skipped/deferred 任务，写明原因并不要 archive。

## 验证记录

- 2026-06-24：RED `task release:smoke` 失败，错误为 `Task "release:smoke" does not exist`。
- 2026-06-24：新增 `Taskfile.yml` 的 `release:smoke`，依赖 `build`，使用 `dist/pinax` 在临时 vault 运行 `version`、`init`、`note add`、`proof loop run --json`，并校验 `proof_loop_run_id` 与 preview 不写入。
- 2026-06-24：GREEN `task release:smoke` 通过，输出 `release smoke passed: dist/pinax`。
- 2026-06-24：`go test ./cmd/pinax ./tests/e2e -run 'TestProofLoop|TestProofLoopRun|TestVersionRestoreApply|TestDemo|TestDemoPlanSnapshotApply|TestDemoRestore|TestIntegrationEvidence' -count=1` 通过。
- 2026-06-24：`openspec validate pinax-first-user-proof-loop-readiness --strict` 通过。
- 2026-06-24：`task test:integration` 通过，生成 `temp/integration-test-runs/20260624T085623Z-2637531/`，`summary.json` 记录 `project=cli/pinax`、`exit_code=0`、`checks.proof_loop=true`、`redaction.applied=true`。
- 2026-06-24：`task check` 通过，覆盖 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build`、`openspec validate --all` 和 LanceDB sidecar protocol。
