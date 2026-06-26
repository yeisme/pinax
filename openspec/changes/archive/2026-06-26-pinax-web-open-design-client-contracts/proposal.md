# Pinax Web 开放设计客户端合同

## 背景

Pinax 已经形成 local-first CLI、Local REST/RPC、MCP/dashboard projection、project board、database view、知识图谱、KB provider 和 proof loop 等能力。新的 [Pinax Web 开放设计](../../../docs/product/web-open-design.md) 明确了未来 Web 工作台方向：看板、知识图谱、搜索、无限画布、右侧 Agent 侧栏、BYOK/local provider 和 Pinax Editor。

当前风险是：如果直接从 UI 开始实现，未来客户端容易绕过 Pinax 的 application service、直接读取 `.pinax/**` 或 SQLite、让 Agent 变成自由执行 shell，或者让 BYOK 凭据进入 Web 表单和日志。需要先把 Web/Editor/Agent 依赖的 CLI/API/projection 合同补齐成 OpenSpec 任务，确保未来客户端只消费稳定、可审查、可回滚的 Pinax 能力。

## 目标

- 固化 Pinax Web 工作台的信息架构和服务边界：Web UI 只消费 Local REST/RPC、CLI JSON 和 bounded projection，不直接读写 vault internals。
- 为 Settings 设置中心建立配置来源、主题、keymap、Cloud Sync、Publish、安全和 Advanced diagnostics 的 UI-facing 合同。
- 为右侧 Agent 侧栏建立可发现 capability、上下文包、provider 状态、命令预览、plan/diff/apply gate 和 redaction 合同。
- 为 BYOK/local provider 建立 UI 可展示但不泄密的 provider status、credential source 和 doctor/rebuild 下一步命令。
- 为 Pinax Editor 建立 preview/source/split/diff/managed block 需要的 note、link、attachment、version、index 和 proof projection。
- 为 Kanban、知识图谱、搜索和无限画布建立 P0 客户端数据合同，明确哪些已有命令可复用，哪些 capability 需要补齐。
- 形成未来独立客户端子项目的 handoff 标准，不把跨平台客户端源码塞进 `cli/pinax`。

## 非目标

- 不在 `cli/pinax` 中实现 React/Web/桌面客户端源码。
- 不新增 public hosted API、多用户 SaaS、浏览器直接文件系统访问、WebSocket 协作或 CRDT 编辑。
- 不新增通用远程 shell、任意命令执行 RPC 或让 Web 直接运行未注册命令。
- 不让 Web、Agent 或客户端手写 `.pinax/**`、SQLite index、provider config、token 文件、sync state、receipt 或 structured assets。
- 不把真实 provider key、Bearer token、raw provider payload、raw prompt、完整 note body 或完整 chain-of-thought 写入 docs、fixtures、stdout、stderr、日志、截图或运行证据。

## 稳定合同影响

- CLI/API 输出继续使用现有 `pinax.projection.v1` envelope；新增字段必须是 optional facts/data/actions/evidence。
- Local REST/RPC capability 只做 additive 扩展；不删除、重命名或改变现有 route、RPC method、error code、JSON key、`--agent` key 或 CLI flag 语义。
- Web 客户端相关能力必须能通过 `pinax api routes --vault <vault> --json` 和 `pinax api schema export --format openapi --vault <vault> --json` 发现。
- 写操作继续复用 `--readonly` / `--allow-write`、`yes=true`、dry-run、snapshot、receipt、restore hint 和 redaction gate。

## 交付边界

本变更先交付 Pinax 侧合同、测试和文档任务。未来如果要实现真正的跨平台 Web/桌面客户端，需要在独立客户端子项目中创建新的 OpenSpec，并引用本变更输出的 capability matrix、API schema 和 Web 开放设计文档。
