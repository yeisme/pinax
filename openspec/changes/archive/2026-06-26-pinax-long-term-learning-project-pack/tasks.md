## 任务

- [x] 1. 写红灯测试：自定义 board 列必须影响 `show/plan/export` 和 `project item add`。
- [x] 2. 实现 board config 读取、动态列排序、动态计数、human/JSON additive 输出和 item 列校验。
- [x] 3. 写红灯测试：`pinax project learning init` 创建长期学习项目包且可重复运行。
- [x] 4. 实现 `ProjectLearningInit` app service 和 `pinax project learning init` Cobra 命令。
- [x] 5. 新增通用学习模板和 `stock-learning` 预设模板，并补模板推荐测试。
- [x] 6. 补 testscript e2e，覆盖 learning init、board show、search、template recommend 和 machine output cleanliness。
- [x] 7. 更新 `docs/commands/project.md`、`docs/commands/template.md`、`docs/commands/README.md`，示例使用真实可运行命令。
- [x] 8. 运行验证并记录结果：`go test ./internal/app ./internal/domain -run 'ProjectBoard|Learning|Template' -count=1`、`go test ./cmd/pinax -run 'Project|Template' -count=1`、`go test ./tests/e2e -run 'ProjectBoardWorkspace|JournalIndexTemplate' -count=1`、`go run ./internal/testkit/integrationevidence`、`openspec validate --all`。

## 验证记录

- `go test ./internal/app ./internal/domain -run 'ProjectBoard|Learning|Template' -count=1` 通过。
- `go test ./cmd/pinax -run 'Project|Template' -count=1` 通过。
- `go test ./tests/e2e -run 'ProjectBoardWorkspace|JournalIndexTemplate' -count=1` 通过。
- `go run ./internal/testkit/integrationevidence` 通过，证据目录：`temp/integration-test-runs/20260624T161422Z-3141632`。
- `openspec validate --all` 通过。
