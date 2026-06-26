# pinax-template-workflow-catalog 任务

## 任务原则

- Owner：`cli/pinax`。
- 所有 CLI/API/JSON/agent 变更必须 additive；不得删除、重命名或改义既有字段。
- 结构化 assets、receipt、events、catalog registry 必须由 CLI/application service 写入，不允许 agent 手写 `.pinax/**`。
- 新增复杂 metadata 解析、生命周期判断、proof gate、路径边界和脱敏逻辑时，代码注释使用中文说明非显然约束；CLI help/output/JSON field 保持英文。

## 1. Catalog metadata 模型和兼容解析

- [ ] 1.1 写失败测试：workflow metadata 被解析但旧模板继续可用。
  - Owner: `cli/pinax`
  - Lane: A
  - Scope: `internal/app/builtin_templates_test.go`、template metadata parser tests。
  - Depends on: none
  - Acceptance: 先运行 `go test ./internal/app -run 'TestTemplateWorkflowMetadata|TestBuiltinTemplates' -count=1`，应因缺少 workflow metadata support 或字段断言失败；实现后通过。
  - Expected result: 支持 `scenario_id`、`intents`、`maturity`、`pack`、`lifecycle`、`proof_gate`、`after_create_actions`，旧模板缺字段时按兼容默认值返回。
  - Failure re-check: 如果旧模板 preview/render 失败，先恢复缺省值兼容，不改模板调用方。

- [ ] 1.2 实现 workflow metadata 解析和默认值。
  - Owner: `cli/pinax`
  - Lane: A
  - Scope: `internal/app/builtin_templates.go`、template metadata/domain 类型、metadata parser。
  - Depends on: 1.1
  - Acceptance: `go test ./internal/app -run 'TestTemplateWorkflowMetadata|TestBuiltinTemplates' -count=1` 通过。
  - Expected result: 内置模板可声明 workflow starter metadata；legacy/simple/design draft 模板能被 inspect，但不可误作为 primary executable create recommendation。
  - Failure re-check: 如果 parser 把未知 optional field 当 fatal，改成 warning/issue，避免破坏 vault-local 模板。

## 2. Intent recommendation 升级为 workflow recommendation

- [ ] 2.1 写 CLI contract 失败测试：recommend 返回 workflow recommendation。
  - Owner: `cli/pinax`
  - Lane: B
  - Scope: `cmd/pinax/template_command_test.go`。
  - Depends on: none
  - Acceptance: 先运行 `go test ./cmd/pinax -run TestTemplateRecommend -count=1`，应缺少 workflow fields；实现后通过。
  - Expected result: `pinax template recommend --intent "meeting" --vault <fixture> --json` 保留既有 envelope/facts/actions，并在 `data.recommendations` 中新增 optional `scenario_id`、`maturity`、`pack`、`fit_reason`、`preview_command`、`create_command`、`proof_gate`、`after_create_actions`。
  - Failure re-check: 如果旧断言失败，先恢复旧字段，再加新字段。

- [ ] 2.2 实现本地 metadata-only recommendation scoring。
  - Owner: `cli/pinax`
  - Lane: B
  - Scope: template recommend service、`internal/cli/template_cmd.go`、output projection。
  - Depends on: 1.2, 2.1
  - Acceptance: `go test ./cmd/pinax -run TestTemplateRecommend -count=1` 通过。
  - Expected result: 推荐只读、local-only，不执行模板、不执行 SQL、不写 Markdown/`.pinax`/Git/provider/network；primary + 最多三个 alternatives。
  - Failure re-check: 如果 recommendation 需要调用 provider 或 query service，拆回纯 metadata 匹配。

## 3. Inspect/preview 暴露 workflow starter 和写入影响

- [ ] 3.1 写 inspect/preview 输出合同失败测试。
  - Owner: `cli/pinax`
  - Lane: C
  - Scope: `cmd/pinax/template_command_test.go`、`internal/output` contract tests。
  - Depends on: none
  - Acceptance: 先运行 `go test ./cmd/pinax -run 'TestTemplateInspect|TestTemplatePreview' -count=1`，应缺少 workflow/proof/evidence 字段；实现后通过。
  - Expected result: `template inspect` 和 `template preview` 输出 workflow fields、变量 schema、output policy、proof gate、next command；read-only 行为不变。
  - Failure re-check: 如果 preview 写入 index.sqlite、receipt、Markdown 或 Git 状态，修正 service 边界并加断言。

- [ ] 3.2 实现 inspect/preview projection。
  - Owner: `cli/pinax`
  - Lane: C
  - Scope: template inspect/preview service、CLI command、output renderer。
  - Depends on: 1.2, 3.1
  - Acceptance: `go test ./cmd/pinax -run 'TestTemplateInspect|TestTemplatePreview' -count=1` 通过。
  - Expected result: human output 中文摘要，`--json` 单对象，`--agent` 稳定 key=value；不输出 raw prompt、provider payload、secret 或 full chain-of-thought。
  - Failure re-check: 如果 `--agent` 出现中文 prose 或不稳定 key，按 `ai-native-cli-output-contract` 修正 projection。

## 4. Template use evidence 和 receipt/proof handoff

- [ ] 4.1 写模板创建使用证据失败测试。
  - Owner: `cli/pinax`
  - Lane: D
  - Scope: `cmd/pinax/template_command_test.go`、`cmd/pinax/note_record_command_test.go` 或相关 note add tests。
  - Depends on: none
  - Acceptance: 先运行 `go test ./cmd/pinax -run 'TestTemplateUseEvidence|TestNoteAddTemplate' -count=1`，应缺少 evidence 字段；实现后通过。
  - Expected result: `pinax note add "Client Meeting" --template meeting.notes --dir index --vault <fixture> --json` 输出 optional `template_use_id`、`template`、`template_pack`、`scenario_id`、`effective_path`、`proof_gate.status`、`next_actions[]`。
  - Failure re-check: 如果新增字段改变旧 envelope 顶层或 status enum，回退为 data/facts optional fields。

- [ ] 4.2 实现 use evidence 和可选 receipt 写入。
  - Owner: `cli/pinax`
  - Lane: D
  - Scope: note/template app service、event/receipt service、redaction、output projection。
  - Depends on: 1.2, 4.1
  - Acceptance: `go test ./cmd/pinax -run 'TestTemplateUseEvidence|TestNoteAddTemplate' -count=1` 通过；触及 integration/e2e 时 evidence 写入 `temp/integration-test-runs/<run-id>/`。
  - Expected result: receipt/event 由 app service 写入；dry-run/preview 不写；失败保留原错误码；evidence 不含 raw provider payload、hidden prompt、secret 或 full chain-of-thought。
  - Failure re-check: 如果 receipt 需要 agent 手写 JSON/Markdown metadata，改为 service API。

## 5. Local template pack 和 lifecycle 门禁

- [ ] 5.1 写 pack/lifecycle 失败测试。
  - Owner: `cli/pinax`
  - Lane: E
  - Scope: `internal/app/builtin_templates_test.go`、`cmd/pinax/template_command_test.go`。
  - Depends on: none
  - Acceptance: 先运行 `go test ./internal/app ./cmd/pinax -run 'TestTemplatePack|TestTemplateLifecycle' -count=1`，应缺少 pack/lifecycle behavior；实现后通过。
  - Expected result: built-in 和 vault-local pack 可被 list/inspect/recommend；`draft_design` 不作为 primary create path；`deprecated` 推荐 replacement；`overridden` 暴露 source。
  - Failure re-check: 如果本地用户模板被隐藏或删除，恢复只读标记，不进行破坏性迁移。

- [ ] 5.2 实现 pack/lifecycle discovery。
  - Owner: `cli/pinax`
  - Lane: E
  - Scope: template registry/discovery、completion、inspect/recommend projection。
  - Depends on: 1.2, 5.1
  - Acceptance: `go test ./internal/app ./cmd/pinax -run 'TestTemplatePack|TestTemplateLifecycle' -count=1` 通过。
  - Expected result: 只支持 builtin/vault-local；不联网、不读远程 registry、不做 marketplace。
  - Failure re-check: 如果实现引入远程包读取，移出本变更并新建 OpenSpec。

## 6. Scenario matrix、docs 和操作说明

- [ ] 6.1 更新用户文档和本地开发说明。
  - Owner: `cli/pinax`
  - Lane: F
  - Scope: `docs/commands/template.md`、`docs/operations/local-development.md`、必要时 `docs/README.md`。
  - Depends on: 2.2, 3.2, 4.2, 5.2
  - Acceptance: `rg -n 'workflow catalog|template recommend --intent|template_use_id|proof_gate|template pack' docs openspec/changes/pinax-template-workflow-catalog` 命中 docs 和 OpenSpec。
  - Expected result: 文档展示真实可运行命令；human-facing 说明中文优先，CLI 输出字段/命令英文；不要求更新根仓库 docs。
  - Failure re-check: 如果文档出现 agent-only wrapper、本地别名或假命令，改成真实 `pinax`/`go test`/`openspec` 命令。

- [ ] 6.2 补充 scenario matrix evidence。
  - Owner: `cli/pinax`
  - Lane: F
  - Scope: `docs/commands/template.md` 或 `docs/operations/local-development.md` 中的 template workflow section。
  - Depends on: 6.1
  - Acceptance: `rg -n 'capture-sticky|idea-research-seed|meeting-decision|learning-pack|stock-learning|index-page' docs openspec/changes/pinax-template-workflow-catalog` 命中。
  - Expected result: 每个场景都有 target user、job、artifact、gate、evidence、handoff、validation command、readiness label。
  - Failure re-check: 如果把 exploratory 场景写成 production-ready，改回 readiness label。

## 7. 验证和收口

- [ ] 7.1 OpenSpec 验证。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Scope: OpenSpec change package。
  - Depends on: all implementation/doc tasks
  - Acceptance: `openspec validate pinax-template-workflow-catalog --strict` 和 `openspec validate --all --strict` 退出码为 0。
  - Expected result: proposal/design/tasks/spec 格式有效，delta spec 可应用。
  - Failure re-check: 根据 OpenSpec 错误修正 spec heading、requirement/scenario 格式或缺失 artifact。

- [ ] 7.2 代码质量门禁。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Scope: Go code、CLI contract、docs examples。
  - Depends on: all implementation/doc tasks
  - Acceptance: `task check` 退出码为 0；如果 `task` 不可用，运行 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`。
  - Expected result: Go tests、CLI contract tests、OpenSpec validate 全通过；构建产物不提交。
  - Failure re-check: 先修复最窄失败测试，不重写无关 dirty worktree。

## 兼容性记录

- Affected surfaces: CLI output (`--json`、`--agent`、human summaries)、template metadata schema、receipt/event structured assets、completion descriptions、docs examples。
- Change class: additive。
- Breaking surfaces: none planned。
- Deprecation window: 不删除旧字段/命令；模板 deprecated 只作为 metadata 标记，并保留至少一个 release 的替代推荐。
- Rollback: 禁用新的 workflow metadata/recommendation scoring，保留旧 template recommend/inspect/preview/render/note add behavior；已写入 Markdown notes 保持可读，新增 receipt/event optional fields 可被旧版本忽略。
