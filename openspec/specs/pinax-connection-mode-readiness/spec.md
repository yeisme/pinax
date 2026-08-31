# pinax-connection-mode-readiness Specification

## Purpose
TBD - created by archiving change pinax-multi-access-contract-foundation-v1. Update Purpose after archive.
## Requirements
### Requirement: Pinax 必须区分连接模式与传输方式

Pinax MUST 使用 `local-vault`、`remote-service`、`self-hosted-service` 表达 owner 位置与运营关系，并使用独立 transport 字段表达 `embedded`、`stdio`、`loopback-http` 或 `https`；系统 MUST NOT 用一个 enum 同时推断 owner、网络位置、认证和 transport。

#### Scenario: 本地 loopback API 仍属于 local-vault

- **WHEN** 用户通过 `http://127.0.0.1:<port>` 调用当前机器上的 `pinax api serve`
- **THEN** connection mode MUST 为 `local-vault`
- **AND** transport MUST 为 `loopback-http`

#### Scenario: 自托管与托管服务共享 wire contract

- **WHEN** 两个非 loopback endpoint 分别被声明为 `remote-service` 和 `self-hosted-service`
- **THEN** 两者 MUST 使用同一 versioned API、projection、scope、redaction 和 mutation recovery contract
- **AND** self-hosted mode MUST NOT 放宽 TLS 或 credential 安全要求

### Requirement: Connection resolver 必须保持既有优先级并拒绝危险推断

Pinax SHALL 按 explicit flag、environment、project config、user config、empty/local 的顺序解析 endpoint，并 SHALL 允许 non-loopback endpoint 显式声明 `remote-service` 或 `self-hosted-service`。系统 MUST NOT 根据 hostname、域名后缀或 token 类型猜测 self-hosted operator。为保持既有消费者兼容，缺少 mode 的 non-loopback endpoint SHALL 暂按 `remote-service` compatibility default 解析至少两个 minor release。

#### Scenario: 既有配置未声明远程模式

- **WHEN** `--api-url` 指向非 loopback endpoint 且没有 `--connection-mode` 或 `remote.mode`
- **THEN** Pinax SHALL 解析为 `remote-service` 且 `mode_source=legacy_default`
- **AND** readiness SHALL 为 `degraded` 并提供设置 `remote.mode` 或 `--connection-mode` 的 next action
- **AND** MUST NOT 回退到本地 vault或放宽 HTTPS/credential policy

#### Scenario: 没有 endpoint 时使用本地模式

- **WHEN** flag、environment、project config 和 user config 都没有 remote endpoint
- **THEN** Pinax MUST 解析为 `local-vault + embedded`
- **AND** 现有 local CLI 行为 MUST 保持不变

### Requirement: 非 loopback endpoint 必须使用 HTTPS 且拒绝 redirect

Pinax remote client MUST 只允许 absolute HTTP(S) URL；HTTP MUST 仅允许 loopback host。URL MUST NOT 含 userinfo、query 或 fragment，HTTP redirect MUST NOT 被自动跟随。

#### Scenario: 明文远程 endpoint 被拒绝

- **WHEN** 用户配置 `http://notes.example.com`
- **THEN** client MUST 在发送 Authorization header 或 request body 前返回 `tls_required`
- **AND** server MUST NOT 收到请求

#### Scenario: Owner 返回 redirect

- **WHEN** approved endpoint 返回 301、302、307 或 308
- **THEN** client MUST 返回 `redirect_rejected`
- **AND** MUST NOT 把 credential 转发到 redirect target

### Requirement: 远程 credential 必须来自受控本地 secret source

非 loopback remote invocation SHALL 使用 owner-only token file 或批准的用户级 secret ref。Token MUST 只进入 Authorization header，并且 MUST NOT 出现在 project config、stdout、stderr、events、logs、receipts、test fixtures 或 integration evidence。

#### Scenario: 安全 token file

- **WHEN** client 从 token file 加载 bearer secret
- **THEN** file MUST 是 absolute、regular、非 symlink、当前用户拥有且 owner-only
- **AND** client MUST 在打开后重新验证文件 identity 再读取有界内容

#### Scenario: 不安全 token file

- **WHEN** token file 是 symlink、权限宽于 owner-only、owner 不匹配或打开期间被替换
- **THEN** client MUST 返回稳定安全错误
- **AND** MUST NOT 发送任何远程请求或回显文件内容

### Requirement: 服务 token store 与客户端 token file 必须使用不同 canonical contract

`pinax api serve --token-store` SHALL 表示 hashed token registry；`--api-token-file` SHALL 表示单个 plaintext bearer secret。服务端旧 `--token-file` MUST 保留为 deprecated compatibility alias 至少两个 minor release，且 MUST NOT 改变客户端 flag 语义。

#### Scenario: 同时提供两个服务端 flag

- **WHEN** 用户同时传入 `api serve --token-store` 与服务端兼容 `--token-file`
- **THEN** Pinax MUST 返回 `auth_mode_conflict`
- **AND** MUST NOT 启动 server

#### Scenario: 兼容 alias 迁移提示

- **WHEN** 用户仅使用服务端 `--token-file`
- **THEN** server MUST 继续读取 hashed token store
- **AND** human stderr MUST 输出一次不含 secret 的迁移命令
- **AND** machine stdout MUST 保持既有合同

### Requirement: Pinax 必须提供只读 connection inspect 与 doctor

Pinax SHALL 提供 `pinax connection inspect` 和 `pinax connection doctor`。`inspect` SHALL 输出 resolved descriptor；`doctor` SHALL 探测 manifest、transport、auth 和 readiness，但两个命令 MUST NOT 执行 domain mutation、remote mutation 或 credential rotation。

#### Scenario: Inspect 输出非敏感 descriptor

- **WHEN** 用户运行 `pinax connection inspect --json`
- **THEN** projection SHALL 包含 mode、transport、endpoint source、credential source type、TLS policy 和 manifest ref
- **AND** SHALL NOT 包含 bearer token、token file 内容或完整 secret ref value

#### Scenario: Doctor 遇到认证失败

- **WHEN** endpoint 可达但 credential 被拒绝
- **THEN** doctor SHALL 将 transport 报告为 `ready` 且 auth 报告为 `blocked`
- **AND** SHALL NOT 执行任何 write probe

### Requirement: Readiness 必须按层级报告

Pinax SHALL 分别报告 `contract`、`transport`、`auth`、`owner`、`mutation_recovery`、`production` readiness。每层 MUST 包含 status、maturity、blockers、next actions 和 evidence refs；系统 MUST NOT 用较低层成功推导 production ready。

#### Scenario: 本地合同通过但 production 不适用

- **WHEN** local stdio MCP 的 contract 与 process e2e 通过，但没有 hosted deployment
- **THEN** contract 和 transport SHALL 为 `ready`
- **AND** production MUST 为 `not_applicable` 或 `not_configured`
- **AND** overall summary MUST NOT 声称 hosted production ready

#### Scenario: Route 存在但 mutation recovery 未覆盖

- **WHEN** 某个远程 write route 已注册但尚未接入 operation ledger
- **THEN** owner readiness SHALL 按真实 application-service backing 报告 `ready`
- **AND** mutation recovery MUST 为 `blocked` 或 `degraded`

### Requirement: Readiness 状态和成熟度必须使用稳定词汇

Layer status MUST 使用 `ready|degraded|blocked|not_configured|not_applicable`；maturity MUST 使用 `exploratory|first-support|mature`。未知值 MUST 被客户端保留为 unknown extension，而不是误判为 ready。

#### Scenario: 新 server 返回未知 readiness 状态

- **WHEN** 旧 client 收到不认识的 readiness status
- **THEN** client MUST 将该层视为 non-ready unknown
- **AND** MUST 保留原值用于诊断而不是 silently coerce 为 `ready`

### Requirement: 现有 backend profile 不得被 repurpose

现有 `pinax profile` SHALL 继续表示 backend/storage connection alias，并 MUST NOT 被 Remote API Mode 静默解释为 API endpoint profile。本 change 的 connection descriptor SHALL 从既有 flags/env/config 解析。

#### Scenario: Backend profile 与 remote config 同时存在

- **WHEN** 用户已有 `pinax profile add` 创建的 storage profile，同时配置 `remote.api_url`
- **THEN** remote client MUST 只使用 remote connection resolver 的 endpoint/credential
- **AND** MUST NOT 把 storage `secret_ref` 当成 API bearer token

### Requirement: Future remote MCP 与 hosted service 必须经过 split-owner handoff

`cli/pinax` MUST NOT 在本 change 内实现公网 Streamable HTTP MCP、多用户 hosted state、组织 RBAC 或 rate limit。Future MCP adapter MUST 由 `mcp/` owner 持有并只消费 stable owner API/SDK；hosted service MUST 先经过根级 owner-fit admission。

#### Scenario: 用户请求网络 MCP

- **WHEN** 当前 binary 只有 local stdio MCP 和 loopback API
- **THEN** readiness SHALL 报告 remote MCP 为 `future_owner`/`not_configured`
- **AND** MUST NOT 把 loopback REST bridge 宣称为 Streamable HTTP MCP
