# pinax-dynamic-plugin-runtime 提案

## 问题

Pinax 已经有本地 vault、索引、Dataview 设计、KB、publish、sync、briefing、MCP/API 等能力，但扩展方式仍是“把能力写进 Go 主程序”。这会带来几个问题：

- 高级用户无法用 JavaScript、Python 或 WASM 快速补自己的导入器、查询源、模板函数、导出渲染器或审计规则。
- 把所有 provider/transformer 都编进 Pinax 会膨胀 CLI，并把不稳定生态依赖带进纯 Go 发布路径。
- 直接支持脚本执行又有明显安全风险：插件可能读取整个 vault、偷取环境变量、联网、绕过 snapshot/approval 写文件。
- Dataview、publish、KB、provider adapter 等后续能力都需要同一套“可扩展但可审计”的执行边界。

## 目标

设计 Pinax 动态插件机制，支持 JS、Python、WASM 和通用外部进程插件提供扩展能力，同时保持：

- Markdown vault 仍是真源。
- `.pinax` 结构化资产仍由 Pinax CLI/service 写入。
- 插件不能直接绕过 app service 写 vault。
- CLI 输出、事件、审计、证据和 redaction 仍遵守 Pinax 合同。
- 默认发布路径保持 Go CLI `CGO_ENABLED=0` 可构建。

## MVP 范围

1. 插件 manifest v1：声明 runtime、entrypoint、capabilities、permissions、hooks、schemas、checksum、resource budgets。
2. 插件注册表：`pinax plugin install/list/inspect/enable/disable/doctor/uninstall` 通过 CLI/service 管理 `.pinax/plugins/registry.json` 和 lock 文件。
3. 动态执行 runtime：
   - `wasm`：首版固定 call/result/budget/sandbox 合同和 fail-closed adapter boundary；未配置真实 engine 时返回 `plugin_runner_unavailable`。真实纯 Go WASM/WASI engine 延后到独立 change。
   - `javascript`：通过外部 runner（优先 `node`/`deno`/`bun` 显式配置）走 stdio JSON-RPC，默认禁用未信任安装。
   - `python`：通过外部 `python3` runner 走 stdio JSON-RPC，默认禁用未信任安装。
   - `process`：通用外部命令 runner，用于兼容 CLI-backed provider，但权限更严格。
4. 插件能力模型：首版支持只读 query source、import/export transformer、template safe function、publish renderer、note action planner、diagnostic rule。
5. 插件输出模型：插件只能返回 bounded projection、rows、rendered artifact、diagnostic finding 或 action plan；真实写入必须由 Pinax 审核并通过现有 service 执行。
6. 审计和证据：每次插件执行记录 plugin id、version、runtime、capability、permissions、input hash、output hash、duration、exit status、redaction status，不记录 raw note body、secret 或 provider payload。

## 非目标

- 不做插件 marketplace、远端自动更新或社区发布服务。
- 不执行 DataviewJS 作为内联插件；Dataview 仍是受限查询语言。
- 不允许插件拿到无限制 vault path、完整环境变量、用户 shell profile 或 Pinax API token。
- 不保证 JS/Python 外部进程在所有平台都达到强沙箱；首版把它们定义为 permissioned/trusted runner，真正未信任执行优先 WASM。
- 不让插件直接注册新的 HTTP/RPC 写接口；API/MCP 暴露仍由 Pinax route registry 管理。

## 兼容性

本变更为 additive：新增 `pinax plugin` 命令、manifest schema、registry schema、runtime adapter 和可选 plugin capability metadata。不得改变既有 note/query/database/publish/sync 命令输出字段语义。插件能力暴露给 API/MCP 时只能新增 optional metadata。

## 首版用户流程

```bash
pinax plugin validate ./plugins/project-dashboard --json
pinax plugin install ./plugins/project-dashboard --scope vault --vault ./my-notes --json
pinax plugin inspect project-dashboard --vault ./my-notes --json
pinax plugin enable project-dashboard --vault ./my-notes --yes --json
pinax plugin run project-dashboard render --vault ./my-notes --dry-run --json
pinax plugin permissions grant project-dashboard projection.read --vault ./my-notes --yes --json
pinax plugin doctor --vault ./my-notes --json
```

## 验收摘要

- WASM 插件 manifest、权限和 runner envelope 可被校验；未配置真实 engine 时 fail closed，并返回稳定 `plugin_runner_unavailable`，不泄漏 bounded input。
- JS/Python 插件必须通过显式安装、启用和权限授权后才能执行，且只通过 stdio JSON-RPC 收发 bounded input/output。
- 插件写入只能返回 action plan；`--dry-run` 不写 vault、`.pinax`、Git、provider 或远端状态。
- 插件 registry/lock/evidence 只能由 CLI/service 写入，不能由 agent 手写。
- 所有 `--json`、`--agent`、`--events` 输出保持现有 Pinax envelope/redaction 合同。
