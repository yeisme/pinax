# Pinax 发布版 Agent 交互面收敛

## CEO 判断

Pinax 现在的问题不是能力不够，而是能力太多、叙事太散。CLI、MCP、Local API、Workbench、Memory、KB、publish、sync、cloud、plugin 都已经出现，但用户第一次接触时很难判断：Pinax 到底是笔记 CLI、agent brain、同步工具、发布工具，还是本地 API server。

发布版必须把判断收回到一个清晰承诺：**Pinax 是面向 Markdown vault 的 agent-safe knowledge control plane；CLI 是唯一产品真源，HTTP/RPC/MCP/Workbench 都是 CLI 应用服务的派生交互面。** 用户先通过 CLI 完成本地 proof loop；agent 再通过 MCP 或 Local API 读取 bounded context、生成计划，并把任何写入带回 CLI proof loop。

## 要解决的问题

- 首屏价值需要从“很多命令”收敛到“agent 可以安全使用你的真实 Markdown vault”。
- 核心需求功能需要落回 CLI：capture、retrieve、diagnose、plan、snapshot、apply、restore、memory/context、API route discovery、MCP read surface 都必须有 CLI 等价入口。
- HTTP/RPC、MCP 和未来 Workbench 不能变成第二套业务模型；它们只能公开 CLI application service projection、capability registry、write gate 和 evidence。
- Agent 体验需要从“能调用一些工具”升级成完整 journey：discover capability -> inspect bounded context -> plan change -> require approval/snapshot -> apply through CLI -> inspect receipt -> restore when needed。
- 发布版需要可验证，不依赖真实用户 vault、真实 provider token、云服务、daemon、桌面客户端或联网。

## Why

Pinax 已经具备大量 CLI、API、MCP、memory、KB、publish、sync 和 workbench building blocks，但发布版需要一个更强的收敛门：用户和 agent 必须先理解并验证同一条 CLI-first proof loop。没有这个门，继续扩展 HTTP、MCP 或客户端只会制造第二套业务模型和第二套安全边界，削弱 Pinax 最有价值的差异化：让 agent 安全操作真实 Markdown vault。

## 目标

- 定义 Pinax 发布版的最小可发布闭环：本地 Markdown vault + proof loop + agent brain bounded context + Local API route discovery + read-only MCP。
- 建立 CLI-first contract：每个发布版核心能力必须有 CLI 命令、projection envelope、`--json`/`--agent` 输出、docs 示例和测试证据。
- 建立 API/MCP 派生规则：Local API、RPC、MCP、Workbench 不直接读写 vault，不绕过 `internal/app`，不维护独立 schema 或业务状态。
- 明确 agent 交互体验规格：capability discovery、bounded body exposure、read/write 分层、approval/snapshot gate、redacted audit、receipts、restore。
- 给后续实现者提供可拆分、可并行、可验收的任务包。

## 非目标

- 不把 Pinax 发布版扩展成 hosted note app、团队知识库、HTTP MCP SaaS、Notion 替代品、通用发布平台或桌面客户端。
- 不要求发布版实现 provider-backed answer synthesis；`pinax brain answer` 仍可保持 extractive preview。
- 不要求 MCP 直接写 vault；发布版 MCP 默认只读，写入只返回 plan/next command 或通过 CLI proof loop 完成。
- 不要求 Cloud Sync、publish、plugin runtime 或 realtime daemon 进入首发必须路径；这些能力可以保留为高级或预览路径。
- 不新增第二套 OpenAPI 手写表；schema 必须从 route registry 派生。

## What Changes

- 新增发布版收敛 spec，定义 CLI proof loop、release core capability registry、Local REST/RPC projection adapter、read-only MCP、agent write gate、redacted evidence 和 release quality gate。
- 新增设计文档，规定 CLI、API/RPC、MCP、Workbench/agent integrations 的所有权边界和 Mermaid 架构图。
- 新增任务拆分，按文档收敛、capability/output contract、Local API/RPC、MCP、proof loop/evidence、最终发布门禁并行推进。

## Impact

- 影响 `cli/pinax` 的 README、quickstart、product positioning、MVP scope、command docs、capability registry、Local API/RPC、MCP adapter、CLI output contract tests 和 release evidence runner。
- 不改变根仓库治理规则，不把 Pinax 产品文档复制到根 `docs/**`。
- 不要求本变更立即实现所有任务；本变更是发布版 durable delivery handoff，后续实现必须在本 OpenSpec 下更新任务状态和验证记录。

## 成功标准

- 新用户可以只用安装后的 `pinax` 二进制完成从 `pinax init` 到 `pinax proof loop run`、`repair plan`、`version snapshot`、`repair apply`、`version restore` 的五分钟 proof loop。
- Agent 可以通过 `pinax api routes --json` 和 `pinax mcp serve` 发现可用能力，并且所有返回默认是 bounded projection，不泄露完整正文、token、provider payload 或私有工具参数。
- HTTP/RPC 对同一能力返回与 CLI JSON 相同的 projection envelope；不支持的命令明确返回 `remote_command_unsupported` 或 registry 中的 `local_only_reason`。
- 所有写入必须满足 `dry-run/plan -> approval -> snapshot when required -> apply -> receipt`；readonly API/MCP 不能写本地 Markdown、`.pinax/**`、Git、provider 或 remote state。
- `task check` 和 `openspec validate --all` 能作为最终发布版质量门禁；集成/e2e 证据写入 `temp/integration-test-runs/<run-id>/`。

## OpenSpec owner

本变更归属 `cli/pinax`。根仓库只保留跨项目规则；具体实现、测试、发布文档和 closeout 记录必须留在 `cli/pinax/openspec/changes/pinax-release-agent-interface-convergence/`。
