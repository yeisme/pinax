## Phase 1: Local Briefing Dry Run

- [x] P1.1 Owner: `cli/pinax`; Lane: A; Depends on: none; Scope: recipe service。实现 `internal/briefing` 包：recipe schema、CLI 命令 `pinax briefing recipe init/show/set`、结构化资产 validation；Acceptance: `go test ./internal/briefing ./cmd/pinax -run BriefingRecipe -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/briefing/recipe_test.go` 和 `cmd/pinax/main_test.go` 的 `TestBriefingRecipeCLI`，先运行 `go test ./internal/briefing ./cmd/pinax -run BriefingRecipe -count=1`，退出码 1，失败于缺少 recipe API 和 `briefing` 命令。实现 `internal/briefing` recipe schema/validation/save/load/set、app service `BriefingRecipeInit/Show/Set` 和 `pinax briefing recipe init/show/set`；重跑同一验收命令，退出码 0。
- [x] P1.2 Owner: `cli/pinax`; Lane: B; Depends on: P1.1; Scope: evidence ledger。实现 evidence schema、dedupe、来源可信度计算；Acceptance: `go test ./internal/briefing -run Evidence -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/briefing/evidence_test.go`，先运行 `go test ./internal/briefing -run Evidence -count=1`，退出码 1，失败于缺少 `EvidenceItem`、`WriteEvidence`、`EvidenceLedgerSchemaVersion` 和 `SourceTrust`。实现 evidence ledger、URL canonicalize、dedupe、source trust 计算和 `.pinax/briefing/evidence.jsonl` 写入；首次实现误用 cloud PathHash 并按 hash 排序导致测试失败，改为本包 sha256 evidence id 和 canonical URL 排序；重跑同一验收命令，退出码 0。
- [x] P1.3 Owner: `cli/pinax`; Lane: B; Depends on: P1.2; Scope: scorer。实现 vault 相关度、新颖度、综合评分；Acceptance: `go test ./internal/briefing -run Score -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/briefing/scorer_test.go`，先运行 `go test ./internal/briefing -run Score -count=1`，退出码 1，失败于缺少 `ScoreEvidence` 和 `CandidateScore`。实现 deterministic scorer：从 vault 文本提取词元，计算 relevance/novelty/trust 和加权 total，按 recipe limit 截断；首次实现暴露已存在同题候选 relevance 过高，修正为低 novelty 同步折减 relevance；重跑同一验收命令，退出码 0。
- [x] P1.4 Owner: `cli/pinax`; Lane: C; Depends on: P1.3; Scope: dry-run 输出。实现 `pinax briefing run --dry-run --json`，输出 top candidates 不写 vault；Acceptance: `go test ./tests/e2e -run BriefingDryRun -count=1` 通过。
  - Evidence: 2026-06-08 新增 `tests/e2e/briefing_dry_run_test.go` 和 testscript，先运行 `go test ./tests/e2e -run BriefingDryRun -count=1`，退出码 1，失败于 `briefing run` 缺少 `--dry-run` flag。实现 `briefing.FakeEvidence`、app service `BriefingRun` 和 `pinax briefing run --dry-run`，dry-run 读取 recipe、扫描 vault 文本、评分并只输出 candidates，不写 review queue 或 notes；重跑同一验收命令，退出码 0。

## Phase 2: Candidate Notes Review Queue

- [x] P2.1 Owner: `cli/pinax`; Lane: D; Depends on: P1.4; Scope: candidate note gen。生成 Markdown briefing_candidate、review queue、tags/backlinks；Acceptance: `go test ./internal/notes ./internal/briefing -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/notes/candidate_test.go` 和 `internal/briefing/candidate_test.go`，先运行 `go test ./internal/notes ./internal/briefing -count=1`，退出码 1，失败于缺少 `RenderBriefingCandidateMarkdown`、`BriefingCandidate`、`BuildCandidateNotes` 和 `ReviewQueueSchemaVersion`。实现 `internal/notes` candidate Markdown renderer 和 `internal/briefing` review queue/candidate generator，生成 `kind: briefing_candidate`、review status、tags 和 backlinks；重跑同一验收命令，退出码 0。
- [x] P2.2 Owner: `cli/pinax`; Lane: D; Depends on: P2.1; Scope: vault write。`--yes` 模式写 review candidate notes 和 events；Acceptance: `go test ./tests/e2e -run BriefingCandidate -count=1` 通过。
  - Evidence: 2026-06-08 新增 `tests/e2e/briefing_candidate_test.go` 和 testscript，先运行 `go test ./tests/e2e -run BriefingCandidate -count=1`，退出码 1，失败于 `briefing run --yes` 输出 `writes=false` 且未写 review queue。随后扩展 `BriefingRun`：`--yes` 生成 candidate Markdown、写 `notes/briefing/*.md`、`.pinax/briefing/review-queue.json` 和 `.pinax/events.jsonl`，stdout 只返回 scores/queue 不输出完整 Markdown body；重跑同一验收命令，退出码 0。

## Phase 3: Hermes Research Integration

- [x] P3.1 Owner: `cli/pinax`; Lane: E; Depends on: P1.1; Scope: research adapter 接口。定义 ResearchRequest/ResearchResponse 合同，Hermes adapter 和 fake fixture；Acceptance: `go test ./internal/research -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/research/research_test.go`，先运行 `go test ./internal/research -count=1`，退出码 1，失败于缺少 `ResearchRequest/ResearchResponse`、fake adapter 和 Hermes adapter。实现 `internal/research` adapter interface、fake fixture、HermesConfig/HermesAdapter fallback 和 request validation；重跑同一验收命令，退出码 0。
- [x] P3.2 Owner: `cli/pinax`; Lane: E; Depends on: P3.1; Scope: Hermes 外部服务配置。在 Pinax 配置中登记 Hermes endpoint/capability，未配置时 fallback 到 fake fixture；Acceptance: `go test ./internal/research -run Hermes -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/research/hermes_test.go`，先运行 `go test ./internal/research -run Hermes -count=1`，退出码 1，失败于缺少 external service config 和 resolver。实现 `.pinax/briefing/research.json` 配置 schema、Save/Load 和 `ResolveAdapter`，Hermes 未配置时返回 fake adapter；重跑同一验收命令，退出码 0。

## Phase 4: Feishu Delivery and Feedback

- [x] P4.1 Owner: `cli/pinax`; Lane: F; Depends on: P2.2; Scope: 飞书 webhook adapter。实现 HTTP POST delivery、message rendering、delivery receipt、secret redaction、fake sender；Acceptance: `go test ./internal/delivery ./cmd/pinax -run Feishu -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/delivery/feishu_test.go` 和 `cmd/pinax/main_test.go` 的 `TestFeishuDeliveryCLI`，先运行 `go test ./internal/delivery ./cmd/pinax -run Feishu -count=1`，退出码 1，失败于缺少 `DeliverFeishu`/`FeishuRequest` 和 CLI flags。实现 `internal/delivery` Feishu webhook POST、text message rendering、delivery receipt、webhook/secret_ref redaction 和 `pinax briefing deliver feishu` dry-run/yes 接口；重跑同一验收命令，退出码 0。
- [x] P4.2 Owner: `cli/pinax`; Lane: F; Depends on: P4.1; Scope: feedback loop。实现 accept/archive/dismiss/follow_up/more_like_this/less_like_this feedback、偏好权重、事件证据回写；Acceptance: `go test ./internal/briefing -run Feedback -count=1` 通过。
  - Evidence: 2026-06-08 新增 `internal/briefing/feedback_test.go`，先运行 `go test ./internal/briefing -run Feedback -count=1`，退出码 1，失败于缺少 feedback API。实现 feedback action 枚举、权重、`.pinax/briefing/feedback.jsonl` 和 `.pinax/events.jsonl` 回写；重跑同一验收命令，退出码 0。
- [x] P4.3 Owner: `cli/pinax`; Lane: sequential; Depends on: P4.2; Scope: command e2e。fake Hermes + fake Feishu + temp vault + testscript，覆盖 dry-run/yes/json/agent/events；Acceptance: `go test ./tests/e2e -run Briefing -count=1` 通过。
  - Evidence: 2026-06-08 新增 `tests/e2e/briefing_test.go`、`tests/e2e/fake_http_test.go` 和 testscript；`TestBriefing` 构建真实 CLI、启动 fake Feishu HTTP server，脚本使用 fake research source，覆盖 `briefing run --dry-run --json`、`--agent`、`--yes --events` 和 `briefing deliver feishu --yes --json`，断言写 review queue/candidate notes 且不泄漏 webhook token/secret_ref。运行 `go test ./tests/e2e -run Briefing -count=1`，退出码 0。
