## Phase 1: Local Briefing Dry Run

- [ ] P1.1 Owner: `cli/pinax`; Lane: A; Depends on: none; Scope: recipe service。实现 `internal/briefing` 包：recipe schema、CLI 命令 `pinax briefing recipe init/show/set`、结构化资产 validation；Acceptance: `go test ./internal/briefing ./cmd/pinax -run BriefingRecipe -count=1` 通过。
- [ ] P1.2 Owner: `cli/pinax`; Lane: B; Depends on: P1.1; Scope: evidence ledger。实现 evidence schema、dedupe、来源可信度计算；Acceptance: `go test ./internal/briefing -run Evidence -count=1` 通过。
- [ ] P1.3 Owner: `cli/pinax`; Lane: B; Depends on: P1.2; Scope: scorer。实现 vault 相关度、新颖度、综合评分；Acceptance: `go test ./internal/briefing -run Score -count=1` 通过。
- [ ] P1.4 Owner: `cli/pinax`; Lane: C; Depends on: P1.3; Scope: dry-run 输出。实现 `pinax briefing run --dry-run --json`，输出 top candidates 不写 vault；Acceptance: `go test ./tests/e2e -run BriefingDryRun -count=1` 通过。

## Phase 2: Candidate Notes Review Queue

- [ ] P2.1 Owner: `cli/pinax`; Lane: D; Depends on: P1.4; Scope: candidate note gen。生成 Markdown briefing_candidate、review queue、tags/backlinks；Acceptance: `go test ./internal/notes ./internal/briefing -count=1` 通过。
- [ ] P2.2 Owner: `cli/pinax`; Lane: D; Depends on: P2.1; Scope: vault write。`--yes` 模式写 review candidate notes 和 events；Acceptance: `go test ./tests/e2e -run BriefingCandidate -count=1` 通过。

## Phase 3: Hermes Research Integration

- [ ] P3.1 Owner: `cli/pinax`; Lane: E; Depends on: P1.1; Scope: research adapter 接口。定义 ResearchRequest/ResearchResponse 合同，Hermes adapter 和 fake fixture；Acceptance: `go test ./internal/research -count=1` 通过。
- [ ] P3.2 Owner: `cli/pinax`; Lane: E; Depends on: P3.1; Scope: Hermes 外部服务配置。在 Pinax 配置中登记 Hermes endpoint/capability，未配置时 fallback 到 fake fixture；Acceptance: `go test ./internal/research -run Hermes -count=1` 通过。

## Phase 4: Feishu Delivery and Feedback

- [ ] P4.1 Owner: `cli/pinax`; Lane: F; Depends on: P2.2; Scope: 飞书 webhook adapter。实现 HTTP POST delivery、message rendering、delivery receipt、secret redaction、fake sender；Acceptance: `go test ./internal/delivery ./cmd/pinax -run Feishu -count=1` 通过。
- [ ] P4.2 Owner: `cli/pinax`; Lane: F; Depends on: P4.1; Scope: feedback loop。实现 accept/archive/dismiss/follow_up/more_like_this/less_like_this feedback、偏好权重、事件证据回写；Acceptance: `go test ./internal/briefing -run Feedback -count=1` 通过。
- [ ] P4.3 Owner: `cli/pinax`; Lane: sequential; Depends on: P4.2; Scope: command e2e。fake Hermes + fake Feishu + temp vault + testscript，覆盖 dry-run/yes/json/agent/events；Acceptance: `go test ./tests/e2e -run Briefing -count=1` 通过。
