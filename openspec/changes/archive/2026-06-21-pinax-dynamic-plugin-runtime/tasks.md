# pinax-dynamic-plugin-runtime 任务

## 0. 任务约束

- Owner: `cli/pinax`。
- 兼容性：只做 additive change；不得改变既有命令输出语义、API route metadata 语义或 `.pinax` 既有 schema。
- 构建：Pinax 默认 Go 构建路径必须保持纯 Go 和 `CGO_ENABLED=0`；JS/Python 通过外部 runner，不内嵌 VM。
- 注释：新增复杂 manifest validation、permission evaluation、runner protocol、redaction、action plan boundary 和 sandbox fallback 必须写中文注释说明不变量。
- 安全：插件不得直接写 vault；写入必须回到 Pinax app service、plan、snapshot、approval 和 record/index evidence。

## 1. Plugin manifest schema 和 domain model

- [x] Owner: `cli/pinax`; Lane: A; Depends on: none
- Scope: `internal/domain/plugin.go`、`internal/plugin/manifest.go`、`internal/plugin/manifest_test.go`
- Work: 定义 `pinax.plugin.v1` manifest、runtime kind、capability kind、permissions、budgets、hooks 和 digest 计算。
- Acceptance: `go test ./internal/plugin ./internal/domain -run 'Plugin|Manifest' -count=1` 通过；包含 secret/webhook/Authorization 拒绝测试。
- Failure re-check: 若 schema 过宽导致未知 runtime/capability 被接受，收紧 allowlist，不用字符串透传绕过。
- Evidence: 2026-06-20 新增 `internal/plugin/manifest.go` 和 `internal/plugin/manifest_test.go`，覆盖 `pinax.plugin.v1` manifest、runtime/capability allowlist、permissions、budgets、hooks/checksum 字段、manifest digest，以及 Authorization/Cookie/webhook/token secret-like 内容拒绝。运行 `go test ./internal/plugin ./internal/domain -run 'Plugin|Manifest' -count=1` 通过；同时新增 `pinax plugin validate` 最小命令入口并运行 `go test ./cmd/pinax -run TestPluginValidate -count=1` 通过，确认 validate 不写 registry/lock 且 machine stdout 不泄漏本地 root 或 raw secret。

## 2. Registry、lock 和 CLI-authored structured assets

- [x] Owner: `cli/pinax`; Lane: A; Depends on: 1
- Scope: `internal/plugin/registry.go`、`internal/app/plugin.go`、`cmd/pinax/plugin_command_test.go`
- Work: 实现 `.pinax/plugins/registry.json`、`.pinax/plugins/plugin-lock.json`、audit event 写入；install 只注册，不自动启用。
- Acceptance: `go test ./internal/plugin ./internal/app ./cmd/pinax -run 'Plugin.*Install|Plugin.*Registry|Plugin.*Lock' -count=1` 通过。
- Failure re-check: 确认 `plugin validate` 不写任何文件；install 只能通过 service 写 registry/lock。
- Evidence: 2026-06-20 新增 `internal/plugin/registry.go`、`internal/plugin/registry_test.go`、`internal/app/plugin.go` 的 install/list/inspect/enable/disable service 路径，以及 `internal/cli/plugin_cmd.go` 对应命令。`pinax plugin install` 通过 service 写 `.pinax/plugins/registry.json`、`.pinax/plugins/plugin-lock.json` 和 `.pinax/events/plugin-audit.jsonl`，默认 `enabled=false`；enable/disable 需要 `--yes`，无 `--yes` 返回 `approval_required`。运行 `go test ./internal/plugin ./internal/app ./cmd/pinax -run 'Plugin.*Install|Plugin.*Registry|Plugin.*Lock' -count=1` 通过；测试确认 registry/lock/audit 和 stdout 不泄漏本地 root 或插件 entrypoint bytes，且 `plugin validate` 仍不写 registry/lock。

## 3. CLI 命令族和输出合同

- [x] Owner: `cli/pinax`; Lane: B; Depends on: 1,2
- Scope: `internal/cli/plugin_cmd.go`、`internal/cli/root.go`、`cmd/pinax/plugin_command_test.go`、`cmd/pinax/cli_output_contract_test.go`
- Work: 新增 `plugin validate/install/list/inspect/enable/disable/permissions/doctor/uninstall/run` 命令，全部使用 projection renderer。
- Acceptance: `go test ./cmd/pinax -run 'Plugin|CLIOutput|Help|Completion' -count=1` 通过；`--json` 单 envelope；`--agent` 低 token key=value。
- Failure re-check: 插件错误不得把 raw stderr、host path、secret、stack trace 直接输出到 machine stdout。
- Evidence: 2026-06-20 扩展 `cmd/pinax/plugin_command_test.go` 覆盖 `plugin --help` 暴露 validate/install/list/inspect/enable/disable/permissions/doctor/uninstall/run，`plugin list --agent` 输出 `spec_version/mode/command/status/fact.*`，`plugin doctor --events` 输出 NDJSON start/end，`plugin permissions list` 输出 JSON envelope，`plugin uninstall` 需要 `--yes`，`plugin run` 对 disabled 插件返回 `plugin_disabled`，启用后在 runner 尚未接入时返回 `plugin_runner_unavailable` 且不泄漏本地 root 或 entrypoint bytes。实现对应 app service 和 Cobra wiring 后运行 `go test ./cmd/pinax -run 'Plugin|CLIOutput|Help|Completion' -count=1` 通过；`openspec validate pinax-dynamic-plugin-runtime --strict` 通过。注意：本任务只完成 CLI 合同，真实 runner 执行仍由任务 4/5 负责。

## 4. WASM runtime contract adapter

- [x] Owner: `cli/pinax`; Lane: C; Depends on: 1,3
- Scope: `internal/plugin/runner.go`、`internal/plugin/runner_test.go`、fake WASM adapter
- Work: 固定 WASM call/result/budget/sandbox 合同，默认无网络、无宿主 FS、无 env；未配置真实 engine 时 fail closed，返回 `plugin_runner_unavailable`。真实 WASM engine 延后到独立 change。
- Acceptance: `CGO_ENABLED=0 go test ./internal/plugin -run 'WASM|Runner|Budget|Sandbox' -count=1` 通过；空 adapter 不启动 runtime 且稳定返回 `plugin_runner_unavailable`。
- Failure re-check: 如果后续接入真实 WASM engine，必须新增独立 OpenSpec、真实 WASM smoke、`CGO_ENABLED=0` 验证和 redaction/evidence 覆盖。
- Evidence: 2026-06-20 新增 `internal/plugin/runner.go` 和 `internal/plugin/runner_test.go`，固定 `pinax.plugin.call.v1` / `pinax.plugin.result.v1` runner envelope、WASM 默认权限 `network=false`、空 env、`filesystem_read/write=none`、timeout/input/output/memory budgets、敏感 input key 清理、result schema/status 校验和 `plugin_budget_exceeded` / `plugin_runner_unavailable` 稳定错误码。当前未引入真实 WASM engine，使用 fake `Invoke` adapter 固定合同；默认 adapter 仍返回 `plugin_runner_unavailable`。运行 `CGO_ENABLED=0 go test ./internal/plugin -run 'WASM|Runner|Budget|Sandbox' -count=1` 通过。

## 5. JS/Python/process 外部 runner

- [x] Owner: `cli/pinax`; Lane: C; Depends on: 1,3
- Scope: `internal/plugin/runner_process.go`、`internal/plugin/runner_test.go`、`tests/fixtures/plugins/{js,python,process}`
- Work: 通过外部 runner 执行 JS/Python/process 插件，控制 cwd、env allowlist、timeout、stdout/stderr bytes、exit code 和 JSON-RPC schema。
- Acceptance: `go test ./internal/plugin ./cmd/pinax -run 'JavaScript|Python|Process|PluginRun' -count=1` 通过；无 runner 时返回 `plugin_runner_unavailable`。
- Failure re-check: 不把本机 shell profile、完整 env 或 vault absolute path 传给插件。
- Evidence: 2026-06-20 新增 `internal/plugin/runner_process.go` 和 `internal/plugin/runner_process_test.go`，实现 JS/Python/process 外部 runner 合同：`python3`/`node`/process entrypoint 通过 `exec.CommandContext` 直接执行，不走 shell；cwd 使用 Pinax 管理的临时目录；stdin 传 `pinax.plugin.call.v1` JSON envelope；stdout 解析 `pinax.plugin.result.v1`；只传有限 env（PATH 和测试诊断变量），不继承 `SECRET_TOKEN`、`SHELL` 等宿主环境；timeout/output budget 映射 `plugin_budget_exceeded`，runner 缺失映射 `plugin_runner_unavailable`。新增 `TestPluginRunUnavailableContract` 覆盖当前 CLI `plugin run` 对已启用但未接入默认 runtime adapter 时返回稳定错误且不泄漏 root/entrypoint bytes。运行 `go test ./internal/plugin ./cmd/pinax -run 'JavaScript|Python|Process|PluginRun' -count=1` 通过。
- Review fix evidence: 2026-06-20 新增红测确认 Python runner 在临时 cwd 下必须收到绝对 entrypoint；修复 `ExternalRunner` 对 Python/Node 传入 packaged entrypoint 绝对路径，同时 cwd 仍为 Pinax 临时目录、stdin 仍为 bounded envelope。运行 `go test ./internal/plugin -run TestPythonRunnerUsesStdinEnvelopeAndLimitedEnvironment -count=1` 通过。

## 6. Permission engine 和 action plan boundary

- [x] Owner: `cli/pinax`; Lane: D; Depends on: 2,4,5
- Scope: `internal/plugin/permissions.go`、`internal/app/plugin.go`、`internal/domain/plugin.go`
- Work: 实现 permission deny-by-default、grant/revoke/list、capability-scope 评估、action plan 输出校验；插件写入只能返回 plan。
- Acceptance: `go test ./internal/plugin ./internal/app ./cmd/pinax -run 'Permission|ActionPlan|Plugin' -count=1` 通过。
- Failure re-check: 对未授权 body read、network、env、filesystem、direct write 都返回稳定错误码且不执行 runner。
- Evidence: 2026-06-20 新增 `internal/plugin/permissions.go` 和 `internal/plugin/permissions_test.go`，实现 permission allowlist、deny-by-default、capability-scoped `projection.read` 检查、`action_plan.write` 边界校验和稳定 `plugin_permission_denied` / `plugin_permission_invalid` 错误码。扩展 registry/service/CLI，新增 `plugin permissions grant|revoke`，必须 `--yes`，写回 registry 并追加 audit；`plugin run` 对 enabled 但未授权 capability 返回 `plugin_permission_denied`，授权 `projection.read` 后才进入 runner 前置检查并返回当前默认 `plugin_runner_unavailable`。运行 `go test ./internal/plugin ./internal/app ./cmd/pinax -run 'Permission|ActionPlan|Plugin' -count=1` 通过。

## 7. Capability hook registry

- [x] Owner: `cli/pinax`; Lane: E; Depends on: 6
- Scope: `internal/plugin/hooks.go`、`internal/app/searchops`、`internal/app/templateops`、`internal/app/publishops` 按首批 hook 接入
- Work: 首批只接只读 hook：`query.source.read`、`template.function`、`export.render`、`diagnostic.rule`；写入类 hook 只产 plan。
- Acceptance: `go test ./internal/plugin ./internal/app ./cmd/pinax -run 'PluginHook|QuerySource|TemplateFunction|Diagnostic' -count=1` 通过。
- Failure re-check: hook 不得覆盖内置 source/command 语义；同名冲突必须返回 `plugin_capability_conflict`。
- Evidence: 2026-06-20 新增 `internal/plugin/hooks.go` 和 `internal/plugin/hooks_test.go`，实现只读 hook registry 合同：允许 `query.source.read`、`template.function`、`export.render`、`diagnostic.rule` 和 `note.action_plan` 注册；拒绝 disabled plugin、plugin id 不匹配、未知 hook kind、重复 target，以及覆盖内置 query source `notes/tasks/links/backlinks/assets`，冲突返回 `plugin_capability_conflict`；写入类 hook 若声明 direct write 返回 `plugin_direct_write_denied`，只能作为 action plan hook 注册。当前内置 search/template/diagnostic 命令语义保持不变，插件 hook 只作为后续 dispatch registry，不覆盖内置 source。运行 `go test ./internal/plugin ./internal/app ./cmd/pinax -run 'PluginHook|QuerySource|TemplateFunction|Diagnostic' -count=1` 通过。

## 8. Audit、redaction 和 integration evidence

- [x] Owner: `cli/pinax`; Lane: F; Depends on: 3-7
- Scope: `internal/plugin/audit.go`、`internal/redaction`、`tests/e2e/testdata/plugin_runtime/scripts/*`
- Work: 插件安装、启用、授权、执行、失败写 audit；集成测试保存脱敏 evidence；redaction 扫描 stdout/stderr/argv/env/audit/registry/lock。
- Acceptance: `task test:integration` 通过并生成 `temp/integration-test-runs/<run-id>/`；evidence 不含 secret/body/provider payload。
- Failure re-check: 任何 redaction sentinel 命中必须修 redaction 或 projection，不允许删测试。
- Evidence: 2026-06-20 新增 `tests/e2e/plugin_runtime_test.go` 和 `tests/e2e/testdata/plugin_runtime/scripts/plugin_runtime.txt`，覆盖 plugin validate/install/list/enable/permissions grant/run/doctor/uninstall 以及 unsafe manifest secret 拒绝；脚本验证 registry/lock/audit 由 CLI 写入，`plugin run` disabled/runner unavailable 错误合同稳定，stdout 和 `.pinax` 资产不含 `PLUGIN_RUNTIME_SECRET_SENTINEL` 或 entrypoint bytes。更新 `internal/testkit/integrationevidence/main.go` 将 `TestPluginRuntime` 纳入 `task test:integration` evidence run。运行 `go test ./tests/e2e -run TestPluginRuntime -count=1` 通过；运行 `task test:integration` 通过并生成 `temp/integration-test-runs/20260620T163110Z-4194187/{summary.json,command.txt,stdout.log,stderr.log,env.json,artifacts/README.txt}`。随后运行 `rg -n 'PLUGIN_RUNTIME_SECRET_SENTINEL|fake wasm bytes|Authorization: Bearer|provider payload|raw prompt|hidden system prompt|private tool arguments|chain-of-thought' temp/integration-test-runs/20260620T163110Z-4194187 || true`，无命中。
- Review fix evidence: 2026-06-20 `plugin run` 已从占位 `plugin_runner_unavailable` 接入真实 runtime dispatch：外部 runtime 安装时打包到 vault-relative `.pinax/plugins/runners/<plugin-id>`，registry/lock 不保存本机绝对路径；run 成功或失败追加 `.pinax/events/plugin-audit.jsonl`，事件含 plugin id、runtime、capability、status 和 error code，不含 raw input/output/root。新增 `TestPluginRunPythonExternalRunnerContract` 覆盖 Python 插件 install/enable/grant/run 成功、audit 写入和 root/entrypoint body 不泄漏。运行 `go test ./cmd/pinax -run TestPluginRunPythonExternalRunnerContract -count=1` 通过；运行 `go test ./internal/plugin ./internal/app/searchops ./cmd/pinax -count=1` 通过。
- Review fix evidence: 2026-06-21 refreshed integration evidence with `task test:integration`; current run is `temp/integration-test-runs/20260621T073517Z-965454/`. Redaction scan for plugin, dataview, secret/body, provider payload and hidden prompt sentinels had no matches.

## 9. 文档、OpenSpec 和完整门禁

- [x] Owner: `cli/pinax`; Lane: sequential; Depends on: 1-8
- Scope: `README.md`、`README.zh-CN.md`、`docs/commands/plugin.md`、`docs/architecture/plugin-runtime.md`、`docs/operations/local-development.md`
- Work: 记录插件信任模型、runner 安装要求、真实命令示例、权限说明、WASM 合同边界、JS/Python 风险边界。
- Acceptance: `task check` 通过；`openspec validate pinax-dynamic-plugin-runtime --strict` 和 `openspec validate --all --strict` 通过。
- Failure re-check: 文档不得推荐手写 `.pinax/plugins/*.json`，不得声称 JS/Python 是强沙箱。
- Evidence: 2026-06-20 新增 `docs/commands/plugin.md` 和 `docs/architecture/plugin-runtime.md`，更新 `docs/commands/README.md`、`README.md`、`README.zh-CN.md`、`docs/operations/local-development.md`。文档包含真实 `pinax plugin ...` 命令示例、manifest shape、permission grant/revoke、WASM 合同边界、JS/Python/process trusted runner 风险边界、CLI-authored registry/lock/audit 规则，并明确不要 hand-edit/手写 `.pinax/plugins/*.json` 或 `.pinax/events/plugin-audit.jsonl`，不声称 JS/Python 是 strong sandbox/强沙箱。运行 `rg -n "pinax plugin|Plugin Runtime|plugin-runtime|\.pinax/plugins|strong sandbox|强沙箱|hand-edit|手写" README.md README.zh-CN.md docs/commands/plugin.md docs/commands/README.md docs/architecture/plugin-runtime.md docs/operations/local-development.md` 命中文档入口和安全边界。运行 `task check` 通过，覆盖 OpenSpec validate、fmt-check、lint、go test ./...、kb sidecar tests 和 build；运行 `openspec validate --all --strict` 通过，44/44 items。
- Review fix evidence: 2026-06-21 将 WASM 范围修正为 contract adapter + fail-closed boundary，真实 engine 后续独立交付；新增 `task kb:sidecar:protocol` 作为默认本地门禁，真实 LanceDB sidecar 测试保留在 `task kb:sidecar:test`。
