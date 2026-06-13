# pinax-auth-token-management Specification

## Purpose
TBD - created by archiving change pinax-backend-auth-profile-cache. Update Purpose after archive.
## Requirements
### Requirement: 默认启动生成 temp token

`pinax api serve` 默认 SHALL 生成临时 token，进程退出后失效。

#### Scenario: 默认模式启动

- **GIVEN** 用户运行 `pinax api serve --vault ./my-notes`
- **WHEN** server 启动
- **THEN** stderr SHALL 输出 `Token: pt_<hex>` 行
- **AND** 该 token SHALL 只在进程内存中持有
- **AND** 进程退出后该 token SHALL 不可用

#### Scenario: temp token 验证

- **GIVEN** server 以默认模式运行，token 为 `pt_abc123`
- **WHEN** 请求携带 `Authorization: Bearer pt_abc123`
- **THEN** 请求 SHALL 通过认证
- **WHEN** 请求不携带 token
- **THEN** 响应 SHALL 为 401，projection error code 为 `token_required`

### Requirement: 长期 token 持久化管理

CLI SHALL 提供长期 token 的创建、列出、轮转和撤销。

#### Scenario: 创建长期 token

- **GIVEN** 一个本地 vault
- **WHEN** 用户运行 `pinax token create --label "agent-claude" --scope read --scope folders:write --expires 30d --vault ./my-notes`
- **THEN** stdout SHALL 输出一次 token 明文
- **AND** `.pinax/tokens/tokens.json` SHALL 存储 `TokenRecord`，包含 `secret_hash` 和 `salt`
- **AND** 文件权限 SHALL 为 0600
- **AND** 文件中 SHALL NOT 包含 token 明文

#### Scenario: 列出 token

- **WHEN** 用户运行 `pinax token list --vault ./my-notes`
- **THEN** 输出 SHALL 包含每个 token 的 `id`、`label`、`scope`、`created_at`、`expires_at`、`last_used_at`
- **AND** 输出 SHALL NOT 包含 token secret 或 secret_hash

#### Scenario: 轮转 token

- **GIVEN** 存在 token `pt_old123`
- **WHEN** 用户运行 `pinax token rotate pt_old123 --vault ./my-notes`
- **THEN** SHALL 创建新 token 并输出明文
- **AND** 旧 token SHALL 被标记为 `rotated_from` 状态并立即失效
- **AND** 审计日志 SHALL 记录轮转事件

#### Scenario: 撤销 token

- **GIVEN** 存在 token `pt_old123`
- **WHEN** 用户运行 `pinax token revoke pt_old123 --vault ./my-notes`
- **THEN** 该 token SHALL 从存储中移除
- **AND** 后续使用该 token 的请求 SHALL 返回 401

### Requirement: Token scope 控制路由访问

每个 API 路由 SHALL 根据注册的 scope 要求验证 token 权限。

#### Scenario: 只读 token 访问写入路由

- **GIVEN** token scope 只有 `read`
- **WHEN** 请求 POST `/v1/folders?path=new-folder`
- **THEN** 响应 SHALL 为 403，error code 为 `insufficient_scope`

#### Scenario: 限定 group 的写入

- **GIVEN** token scope 为 `write` + `Groups: ["inbox"]`
- **WHEN** 请求 POST `/v1/inbox:capture`
- **THEN** 请求 SHALL 通过
- **WHEN** 请求 POST `/v1/folders?path=new-folder`
- **THEN** 响应 SHALL 为 403

#### Scenario: 过期 token

- **GIVEN** token 的 `expires_at` 已过
- **WHEN** 请求使用该 token
- **THEN** 响应 SHALL 为 401，error code 为 `token_expired`

### Requirement: 无认证模式强制 loopback

`--no-auth` 模式 SHALL 只允许 loopback 连接。

#### Scenario: loopback 请求放行

- **GIVEN** server 以 `--no-auth` 模式运行
- **WHEN** 请求来自 `127.0.0.1` 或 `[::1]`
- **THEN** 请求 SHALL 通过，无需 token

#### Scenario: 非 loopback 请求拒绝

- **GIVEN** server 以 `--no-auth` 模式运行
- **WHEN** 请求来自非 loopback 地址
- **THEN** 响应 SHALL 为 403，error code 为 `non_loopback_rejected`

### Requirement: API 审计日志

所有 API 请求 SHALL 记录审计日志。

#### Scenario: 审计日志格式

- **GIVEN** server 以任意 auth 模式运行
- **WHEN** 收到任何 API 请求
- **THEN** `.pinax/events/api-audit.jsonl` SHALL 追加一行 JSON
- **AND** 该 JSON SHALL 包含 `ts`、`token_id`、`method`、`path`、`scope`、`group`、`status`
- **AND** 该 JSON SHALL NOT 包含 token secret、request body 或 response body

#### Scenario: 审计日志脱敏

- **GIVEN** 审计日志文件
- **WHEN** 检查文件内容
- **THEN** 日志 SHALL NOT 包含 token 明文、Authorization header 值、Cookie 值或 webhook URL

