# pinax-transport-contract-parity Specification

## Purpose
TBD - created by archiving change pinax-multi-access-contract-foundation-v1. Update Purpose after archive.
## Requirements
### Requirement: Pinax 必须提供 authoritative transport manifest

Pinax SHALL 提供 versioned `pinax.transport_manifest.v1`，将领域 capability 与真实 CLI remote binding、REST route、RPC method、MCP tool/resource 绑定。Manifest MUST 是新消费者判断 transport availability 的权威来源。

#### Scenario: 查询 manifest

- **WHEN** 用户运行 `pinax api manifest --json`、读取 `GET /v1/manifest` 或调用 `Pinax.Transport.Manifest`
- **THEN** 三个入口 SHALL 返回同一 capability/binding 集合和 schema version
- **AND** 每个 available binding SHALL 包含可解析的 backing identity

### Requirement: Available binding 必须对应真实注册对象

Capability 只有在对应 adapter 注册了可调用 handler、tool 或 resource 时，才可进入 `available_surfaces`。Planned、blocked、local-only 或 future-owner 项 MUST 显式分类，并 MUST NOT 被报告为 available。

#### Scenario: Legacy capability 声明 MCP mutation 但没有 tool

- **WHEN** legacy capability 的 `surfaces` 含 `mcp`，但 MCP registry 没有对应 tool/resource
- **THEN** authoritative manifest MUST NOT 把 MCP binding 标为 available
- **AND** MUST 说明 legacy declaration 与实际 binding 的差异

#### Scenario: Adapter 注册了未绑定 tool

- **WHEN** MCP registry 出现没有 capability id 的 tool
- **THEN** conformance validation MUST 失败
- **AND** build/test MUST NOT 把该 tool 静默发布到 production discovery

### Requirement: Legacy capability projection 必须 additive 迁移

现有 `RemoteCapabilities()`、`pinax api routes` 与 legacy `surfaces` 字段 SHALL 保持可读。新 manifest MUST 以新字段/新 endpoint 增加，不得在本 change 重解释或删除 legacy 字段。

#### Scenario: 旧消费者只读取 surfaces

- **WHEN** 旧消费者升级到包含 transport manifest 的 Pinax 版本
- **THEN** legacy capability projection SHALL 继续包含原有字段类型
- **AND** 新 deprecation metadata SHALL 是 optional additive field

### Requirement: OpenAPI 3.1 必须包含可生成 client 的 typed contract

OpenAPI export SHALL 从真实 REST bindings 和 versioned schema registry 生成，并包含 typed parameters/request body、success/error responses、components schemas、Bearer security scheme、operationId 和 Pinax extension metadata。

#### Scenario: 导出一个 mutation route

- **WHEN** OpenAPI 导出包含 `inbox.capture`
- **THEN** operation SHALL 描述 request fields、confirmation/idempotency headers、projection response 和 stable error responses
- **AND** SHALL 包含 capability id、write gate、readiness、stability 与 body exposure metadata

#### Scenario: Schema 可被 semantic validator 读取

- **WHEN** contract test 用 OpenAPI 3.1 validator 加载导出结果
- **THEN** schema SHALL 无 dangling ref、重复 operationId 或无效 security reference

### Requirement: OpenAPI 不得为非 REST capability 伪造 path

CLI-only、RPC-only、MCP-only、planned 或 future-owner capability MUST NOT 仅因为存在 capability metadata 而生成 OpenAPI path/method。

#### Scenario: MCP-only brain tool

- **WHEN** `pinax.brain.*` tool 没有对应 REST route
- **THEN** OpenAPI paths MUST NOT 包含合成的 brain endpoint
- **AND** manifest SHALL 仍可报告其真实 MCP binding 或 local-only 状态

### Requirement: MCP stdio 必须支持 current/legacy 双时代版本合同

Local stdio MCP MUST 支持 current `2026-07-28` per-request metadata 与 `server/discover`，并对 allowlisted legacy protocol version 完成 initialize。Server capabilities MUST 与实际 tools/resources lifecycle 一致；不支持的 current 或 legacy version MUST 返回对应标准 protocol error。

#### Scenario: Current client discovery

- **WHEN** MCP client 以 `_meta.io.modelcontextprotocol/protocolVersion=2026-07-28` 调用 `server/discover`
- **THEN** server SHALL 返回 supported versions、server info 和真实 capabilities
- **AND** 后续 current requests SHALL 使用 per-request metadata，不依赖 initialize session

#### Scenario: 支持的 legacy 协议版本

- **WHEN** MCP client 使用 server allowlist 中的 protocol version 初始化
- **THEN** server SHALL 返回协商后的 version、server info 和真实 capabilities
- **AND** process SHALL 保持运行直到 stdin 关闭、session 结束或收到 signal

#### Scenario: 不支持的协议版本

- **WHEN** client 请求 server 不支持的 protocol version
- **THEN** current request MUST 返回 `UnsupportedProtocolVersionError`，legacy initialize MUST 返回 invalid-params protocol error
- **AND** MUST NOT 假装协商成功或执行 tools/call

### Requirement: MCP tools discovery 必须提供闭合输入与输出合同

`tools/list` SHALL 只列真实 backed tool。每个 tool MUST 有非空、closed input schema；协议支持时 MUST 提供 output schema，否则 result MUST 包含 versioned structured content 和 schema ref。

#### Scenario: Tool 参数未知

- **WHEN** client 向 closed input schema 提交未知字段
- **THEN** tool call MUST 返回 stable invalid-params error
- **AND** application service MUST NOT 被调用

#### Scenario: Tool 返回 malformed projection

- **WHEN** handler 返回缺少 required schema/version/identity 的 projection
- **THEN** MCP adapter MUST 返回 bounded internal contract error
- **AND** MUST NOT 把 malformed data 当成 tool success

### Requirement: MCP advertised resource 必须支持 resources/read

`resources/list` 或 resource templates 中出现的每个可实例化 resource MUST 能通过 `resources/read` 返回 bounded projection；不能读取的 planned resource MUST NOT 出现在 available discovery。

#### Scenario: 读取 capability manifest resource

- **WHEN** client 对 discovery 中的 manifest resource 调用 `resources/read`
- **THEN** server SHALL 返回与 `pinax.transport_manifest.v1` 一致的 bounded content
- **AND** SHALL NOT 返回 vault path、token store 或 raw database content

### Requirement: MCP stdout 必须只包含 JSON-RPC

在 stdio lifecycle 中，stdout MUST 只包含 MCP JSON-RPC frames；startup、diagnostics、deprecation、panic recovery 和 shutdown messages MUST 写 stderr 或 redacted sidecar。

#### Scenario: Server 启动与关闭

- **WHEN** MCP host 启动 server、完成一次 tool call 并关闭 stdin
- **THEN** stdout 的每一帧 MUST 是有效 JSON-RPC
- **AND** human startup/shutdown 文本 MUST NOT 混入 stdout

### Requirement: Transport adapter 必须只调用 application service

REST/RPC/MCP handler MUST 通过 Pinax application service 执行 use case，MUST NOT 直接写 Markdown、`.pinax/**`、GORM/SQLite、Git、provider 或 remote state。

#### Scenario: MCP plan tool

- **WHEN** client 调用 organize/repair plan tool
- **THEN** MCP adapter SHALL 调用与 CLI 相同的 application service
- **AND** direct filesystem write spy SHALL 观察不到 adapter 层写入

### Requirement: Local MCP mutation 必须保持未发布

在独立 MCP write-surface OpenSpec 获批前，local MCP discovery MUST NOT 发布 direct create/edit/rename/delete/apply mutation tool。既有只读和 plan/dry-run tool SHALL 继续按真实 backing 发布。

#### Scenario: Folder mutation capability 存在于 domain registry

- **WHEN** client 请求 `tools/list`
- **THEN** folder create/rename/move/delete direct mutation tool MUST NOT 出现
- **AND** manifest MUST 将其 MCP binding 报告为 unavailable 或未绑定

### Requirement: Public Go client 必须复用同一 typed contract

`pkg/pinaxclient` SHALL 提供 manifest、readiness、operation 和首批业务调用的 versioned DTO/error。Remote CLI 与 future sibling MCP MUST 复用该 client 或同一 public contract，而不是复制 transport security/reconcile 逻辑。

#### Scenario: Remote CLI 与 SDK 读取同一 note projection

- **WHEN** remote CLI 和 Go client 对同一 fixture owner 调用相同 read capability
- **THEN** 两者 SHALL 返回相同 command、facts、data、error 与 schema version
- **AND** stdout rendering 差异只能来自 CLI output projection

### Requirement: Client 必须拒绝 malformed 2xx 和越权 identity

Client MUST 校验成功响应的 schema version、capability/operation/resource identity、body exposure 和 bounded size。Malformed 2xx MUST 返回 `upstream_invalid_response`，不得合成成功或 fallback 到本地执行。

#### Scenario: Operation identity 不匹配

- **WHEN** owner 对 operation status request 返回另一个 operation id
- **THEN** client MUST 返回 `upstream_invalid_response`
- **AND** MUST 标记原 operation 为 reconcile required

### Requirement: Transport conformance 必须双向验证

Tests SHALL 验证 manifest→adapter 与 adapter→manifest 两个方向：每个 available binding 可执行，每个公开 route/method/tool/resource 有 capability binding，且 OpenAPI/MCP discovery 与 runtime registry 一致。

#### Scenario: 新 route 未加入 manifest

- **WHEN** 开发者注册新的 public REST route 但没有 capability binding
- **THEN** conformance test MUST 失败
- **AND** failure SHALL 指出 route id 和缺失 binding

#### Scenario: Manifest 声明不存在的 route

- **WHEN** manifest 编译结果引用未注册 route id
- **THEN** conformance test MUST 失败
- **AND** OpenAPI export MUST NOT 生成该 path
