# Pinax Memory Recall Ranking 任务

## 分组说明

- **Lane A: Scorer 抽取**，先从现有 `Store.Recall` 中分离可测试 ranking。
- **Lane B: Ranking 行为**，新增复杂信号、collapse 和稳定排序。
- **Lane C: CLI 输出合同**，新增 `signals` 和 agent facts，保持兼容。
- **Lane D: 文档、证据和门禁**，更新文档并跑 integration evidence。

## Lane A: Scorer 抽取

- [x] **A1. 为现有 recall 行为写失败优先测试**
  - Owner: `cli/pinax`
  - Files: `internal/memory/store_test.go`, `internal/memory/scorer_test.go`
  - Scope: 固定现有 entity/type/FTS/source/recency 行为，保证重构不改变默认 recall 基线。
  - Depends on: none
  - Parallel lane: A
  - Acceptance: 测试覆盖 confirmed 默认召回、draft/superseded/expired/rejected 默认排除、`recall_reason` 存在。
  - Validation command: `go test ./internal/memory -run 'Recall|Scorer' -count=1`
  - Expected result: 测试能描述现有行为并在重构后通过。
  - Failure re-check: 不依赖真实时间；使用固定 clock 或可控 created_at。

- [x] **A2. 抽出 scorer 数据结构**
  - Owner: `cli/pinax`
  - Files: `internal/memory/recall.go`, `internal/memory/scorer.go`, `internal/memory/store.go`
  - Scope: 新增 `Candidate`、`SignalBreakdown`、`ScoredCandidate`、`Scorer`；`Store.Recall` 只负责加载候选并调用 scorer。
  - Depends on: A1
  - Parallel lane: A
  - Acceptance: `RecallHit` 保留 `Record`、`RecallReason`、`Score`，新增 `Signals` 可选字段。
  - Validation command: `go test ./internal/memory -run 'Recall|Scorer' -count=1`
  - Expected result: memory 单测通过，现有 app/CLI 编译通过。
  - Failure re-check: 不把 GORM query 逻辑移动进 scorer；scorer 只处理内存候选。

- [x] **A3. 固定 deterministic tie-break**
  - Owner: `cli/pinax`
  - Files: `internal/memory/scorer.go`, `internal/memory/scorer_test.go`
  - Scope: 排序顺序固定为 score desc、source authority desc、created_at desc、id asc。
  - Depends on: A2
  - Parallel lane: A
  - Acceptance: 同分候选在多次运行中顺序一致。
  - Validation command: `go test ./internal/memory -run 'TieBreak|Scorer' -count=1`
  - Expected result: tie-break 测试稳定通过。
  - Failure re-check: 不使用 map iteration 顺序作为排序依据。

## Lane B: Ranking 行为

- [x] **B1. 实现字段级 keyword signals**
  - Owner: `cli/pinax`
  - Files: `internal/memory/scorer.go`, `internal/memory/scorer_test.go`, `internal/memory/store.go`
  - Scope: 区分 FTS、subject、predicate、object、body fallback 的命中，reason 中包含 `keyword:fts` 或 `field:<name>`。
  - Depends on: A2
  - Parallel lane: B
  - Acceptance: predicate/object 精确命中高于 body fallback；FTS 命中保留加分。
  - Validation command: `go test ./internal/memory -run 'Keyword|Field|Scorer' -count=1`
  - Expected result: keyword ranking 测试通过。
  - Failure re-check: SQLite FTS raw SQL 仍集中在 store，不散落到 scorer。

- [x] **B2. 实现 source authority 和 confidence signals**
  - Owner: `cli/pinax`
  - Files: `internal/memory/scorer.go`, `internal/memory/scorer_test.go`
  - Scope: `openspec`、`docs`、`github_actions`、`file` 映射为 source 权重；confidence label 映射为权重。
  - Depends on: A2
  - Parallel lane: B
  - Acceptance: 同等关键词下 OpenSpec confirmed decision 排在普通 file 之前。
  - Validation command: `go test ./internal/memory -run 'Source|Confidence|Scorer' -count=1`
  - Expected result: source/confidence 测试通过。
  - Failure re-check: 未知 confidence 不报错，只给低权重并在 reason 标明。

- [x] **B3. 实现 freshness 和 task fitness**
  - Owner: `cli/pinax`
  - Files: `internal/memory/scorer.go`, `internal/memory/scorer_test.go`
  - Scope: event/task 记录按近期加分；query 词与 release/test/provider/cloud/kb/memory 等主题匹配时加 task fitness。
  - Depends on: A2
  - Parallel lane: B
  - Acceptance: 新 event/task 在同等质量下优先，但不能压过 explicit entity mismatch。
  - Validation command: `go test ./internal/memory -run 'Freshness|TaskFitness|Scorer' -count=1`
  - Expected result: freshness/task fitness 测试通过。
  - Failure re-check: 使用固定 now，避免测试随日期漂移。

- [x] **B4. 实现 supersession collapse 和 duplicate collapse**
  - Owner: `cli/pinax`
  - Files: `internal/memory/scorer.go`, `internal/memory/scorer_test.go`, `internal/memory/store_test.go`
  - Scope: 默认隐藏被 `supersedes_id` 指向的旧记录；同 `subject+predicate` 只保留最高分 confirmed record。
  - Depends on: B1, B2
  - Parallel lane: B
  - Acceptance: superseded old record 可被 `list --include-superseded` 审计，但不会进入默认 context。
  - Validation command: `go test ./internal/memory -run 'Supersede|Duplicate|Recall' -count=1`
  - Expected result: collapse 测试通过。
  - Failure re-check: 不删除旧记录，只在 recall ranking 阶段隐藏。

## Lane C: CLI 输出合同

- [x] **C1. 在 JSON data 中输出 optional `signals`**
  - Owner: `cli/pinax`
  - Files: `internal/app/memory.go`, `cmd/pinax/memory_command_test.go`
  - Scope: `memoryHitsData` 为每条 hit 增加可选 `signals`；保留 `score` 和 `recall_reason`。
  - Depends on: A2, B1, B2
  - Parallel lane: C
  - Acceptance: `pinax memory recall "release workflow" --entity pinax --vault <vault> --json` 输出 valid envelope，旧字段仍存在。
  - Validation command: `go test ./cmd/pinax -run 'TestMemory.*Recall|TestMemory.*JSON' -count=1`
  - Expected result: CLI JSON 测试通过。
  - Failure re-check: 不删除或重命名现有 JSON 字段。

- [x] **C2. 增加 agent ranking facts**
  - Owner: `cli/pinax`
  - Files: `internal/app/memory.go`, `internal/output/render.go`, `cmd/pinax/memory_command_test.go`
  - Scope: 增加 `fact.memory.top_score` 和有限数量 `fact.memory.reason.N`；保持低 token key=value。
  - Depends on: C1
  - Parallel lane: C
  - Acceptance: `--agent` 不含中文 prose、完整 body、raw prompt、provider payload、secret 或 ANSI。
  - Validation command: `go test ./cmd/pinax -run 'TestMemory.*Agent|TestMemory.*Context' -count=1`
  - Expected result: agent contract 测试通过。
  - Failure re-check: 如果 renderer 不支持 repeated reason key，在 app projection 中提供稳定 facts，不手写 stdout。

- [x] **C3. 增加 redaction 和 body-leak tests**
  - Owner: `cli/pinax`
  - Files: `cmd/pinax/memory_command_test.go`, `internal/output/projection_redaction_test.go`
  - Scope: 使用 sentinel body、Authorization、Bearer、provider-payload、raw-prompt，断言 recall/context machine outputs 不泄漏。
  - Depends on: C1, C2
  - Parallel lane: C
  - Acceptance: `--json` 可包含 bounded memory object，但 `--agent` 不输出完整私有正文；受保护 surfaces 无敏感 sentinel。
  - Validation command: `go test ./cmd/pinax ./internal/output -run 'Memory|Redaction|BodyLeak' -count=1`
  - Expected result: redaction 测试通过。
  - Failure re-check: 修 projection/redaction 源头，不只改 fixture 文本。

## Lane D: 文档、证据和门禁

- [x] **D1. 更新 memory 命令文档**
  - Owner: `cli/pinax`
  - Files: `docs/commands/memory.md`, `docs/commands/README.md`
  - Scope: 说明 deterministic ranking、signals、与 KB semantic search 的边界、真实命令示例和安全边界。
  - Depends on: C1, C2
  - Parallel lane: D
  - Acceptance: 文档不推荐手写 `.pinax/memory/*.sqlite` 或直接编辑 structured assets。
  - Validation command: `rg -n "ranking|signals|recall_reason|memory recall|memory context|KB" docs/commands/memory.md docs/commands/README.md`
  - Expected result: 文档覆盖新 ranking。
  - Failure re-check: 如果 signals 尚未实现，文档标注为本 change 交付项，不声称已发布。

- [x] **D2. 增加 integration evidence 覆盖**
  - Owner: `cli/pinax`
  - Files: `internal/testkit/integrationevidence/**`, `tests/e2e/**`, `cmd/pinax/memory_command_test.go`
  - Scope: `task test:integration` 覆盖 memory recall/context 的 bounded output 和 redaction sentinel。
  - Depends on: C3
  - Parallel lane: D
  - Acceptance: 最新 `temp/integration-test-runs/<run-id>/` 包含 summary、command、stdout、stderr、env、artifacts，且 redaction applied。
  - Validation command: `task test:integration`
  - Expected result: integration evidence 生成并通过。
  - Failure re-check: 失败也必须保留 evidence 和原始 exit code。

- [x] **D3. OpenSpec 严格验证**
  - Owner: `cli/pinax`
  - Files: `openspec/changes/pinax-memory-recall-ranking/**`
  - Scope: 验证本 change 和全量 specs。
  - Depends on: all A/B/C/D1/D2 tasks
  - Parallel lane: sequential
  - Acceptance: 本 change 和全量 OpenSpec 均通过 strict validate。
  - Validation command: `openspec validate pinax-memory-recall-ranking --strict && openspec validate --all --strict`
  - Expected result: 两条命令 exit 0。
  - Failure re-check: 修正 delta spec header 或 requirement 格式，不绕过 validate。

- [x] **D4. 全量质量门禁**
  - Owner: `cli/pinax`
  - Files: project-wide
  - Scope: 跑 Pinax 标准门禁。
  - Depends on: D3
  - Parallel lane: sequential
  - Acceptance: format、lint、unit、build、sidecar protocol、OpenSpec 全部通过。
  - Validation command: `task check`
  - Expected result: `task check` exit 0。
  - Failure re-check: 修源头失败，不降低 ranking 或 redaction 断言。

## 验证记录

- RED: `go test ./internal/memory -run 'Recall|Scorer|TieBreak|Keyword|Field|Source|Confidence|Freshness|TaskFitness|Supersede|Duplicate' -count=1` 初始失败，确认缺少 scorer/signals 行为。
- RED: `go test ./cmd/pinax -run 'TestMemory.*Recall|TestMemory.*JSON|TestMemory.*Agent|TestMemory.*Context|TestMemory.*Redaction|TestMemoryRecallRankingSignalsAndRedaction' -count=1` 初始失败，确认 `signals`、`memory.top_score` 和 body redaction 尚未交付。
- GREEN: `go test ./internal/memory -run 'Recall|Scorer|TieBreak|Keyword|Field|Source|Confidence|Freshness|TaskFitness|Supersede|Duplicate' -count=1` 通过。
- GREEN: `go test ./cmd/pinax -run 'TestMemory.*Recall|TestMemory.*JSON|TestMemory.*Agent|TestMemory.*Context|TestMemory.*Redaction|TestMemoryRecallRankingSignalsAndRedaction' -count=1` 通过。
- GREEN: `go test ./cmd/pinax ./internal/memory ./internal/app -run 'Memory|Recall|Scorer' -count=1` 通过。
- 文档检查: `rg -n "ranking|signals|recall_reason|memory recall|memory context|KB" docs/commands/memory.md docs/commands/README.md` 覆盖 ranking、signals、KB 边界和安全说明。
- Integration evidence: `task test:integration` 通过，run id `20260624T090723Z-2669174`，`summary.json` 记录 `memory_recall_ranking: true` 和 `redacted: true`。
- OpenSpec: `openspec validate pinax-memory-recall-ranking --strict && openspec validate --all --strict` 通过，49 passed, 0 failed。
- Quality gate: `task check` 通过，包含 `go test ./...`、`golangci-lint run`、`golangci-lint fmt --diff`、sidecar protocol tests、`openspec validate --all` 和 `go build -trimpath`。
