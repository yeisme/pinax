# 实施任务

- [x] 1.1 创建 change skeleton，冻结 owner、边界、兼容与 rollback；验证：`openspec validate pinax-daily-interaction-orchestration-v1 --strict`。
- [x] 1.2 新增 `domain.DailyInteraction` 与 `Projection.Interaction` additive 类型；验证：`go test ./internal/domain`。
- [x] 1.3 新增无状态 `internal/interaction` composer，覆盖 phase/state、安全布尔值、source/freshness、推荐动作、恢复动作和路径脱敏；验证：`go test ./internal/interaction`。
- [x] 1.4 在统一 CLI renderer 接缝调用 composer，保证既有命令的多输出模式共享同一 projection；验证：`go test ./internal/cli`。
- [x] 1.5 为完整 `--json`/`--agent`/`--events`/`--explain` 与 remote-adapted projection 增加合同回归；验证：`go test ./internal/output ./internal/remoteapi ./pkg/pinaxclient`。
- [x] 1.6 新建并同步根仓 `pinax-daily-interaction-operator` Skill；验证：`scripts/skills.sh validate-custom`、`scripts/skills.sh validate-runtime`、`scripts/skills.sh sync-root`、`scripts/skills.sh sync-target cli/pinax`。
- [x] 1.7 更新 daily knowledge、continuity/operator 和命令路由文档；验证：`openspec validate pinax-daily-interaction-orchestration-v1 --strict` 与 Skill quick validator。
- [x] 1.8 运行 focused integration evidence、`task check` 和 `openspec validate --all`；验证：`go run ./tools/testkit/integrationevidence --profile daily-knowledge`（`temp/integration-test-runs/20261031T163759Z-3546866`）与 `task check`（OpenSpec 112/112、Go 全量、lint 0 issues、build、renderer 4/4、tsc）；未运行 race。
- [ ] 2.1 在真实主力 vault 观察两周 capture 成本、review 时长、首次找回与再次使用；本地 fixture 不得替代。
- [ ] 2.2 依据真实观察生成独立 evidence report；工程门通过不自动关闭 adoption/continuity gate。
