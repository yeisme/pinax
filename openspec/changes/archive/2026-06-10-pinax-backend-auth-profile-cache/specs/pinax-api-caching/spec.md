# pinax-api-caching Delta Spec

## ADDED Requirements

### Requirement: 只读路由返回 Cache-Control header

API server SHALL 为只读 GET 路由设置 HTTP 缓存头。

#### Scenario: capabilities 路由缓存

- **GIVEN** server 以任意 auth 模式运行
- **WHEN** 请求 `GET /v1/capabilities`
- **THEN** 响应 SHALL 包含 `Cache-Control: max-age=300, scope=public`
- **AND** 响应 SHALL 包含 `ETag` header

#### Scenario: notes 路由缓存

- **WHEN** 请求 `GET /v1/notes/note-001`
- **THEN** 响应 SHALL 包含 `Cache-Control: max-age=30, scope=private` 和 `ETag`

#### Scenario: 写入路由不缓存

- **WHEN** 请求 `POST /v1/folders`
- **THEN** 响应 SHALL NOT 包含 `Cache-Control` 或 `ETag` header

### Requirement: ETag 验证返回 304

客户端 SHALL 能通过 `If-None-Match` 获取 304 Not Modified。

#### Scenario: 内容未变化

- **GIVEN** 上一次响应的 `ETag` 为 `"abc123"`
- **WHEN** 客户端发送 `GET /v1/capabilities` with `If-None-Match: "abc123"`
- **AND** projection 内容未变化
- **THEN** 响应 SHALL 为 HTTP 304
- **AND** 响应 body SHALL 为空

#### Scenario: 内容已变化

- **GIVEN** 上一次响应的 `ETag` 为 `"abc123"`
- **WHEN** 客户端发送 `GET /v1/capabilities` with `If-None-Match: "abc123"`
- **AND** projection 内容已变化（例如 vault 被修改）
- **THEN** 响应 SHALL 为 HTTP 200
- **AND** 响应 SHALL 包含新的 `ETag` 和完整 projection body

### Requirement: 缓存策略可配置

缓存策略 SHALL 可通过配置覆盖。

#### Scenario: 自定义缓存 TTL

- **GIVEN** vault 的 `.pinax/config.yaml` 设置 `api.cache.policies.notes.max_age: 60`
- **WHEN** 请求 `GET /v1/notes/note-001`
- **THEN** `Cache-Control` 的 `max-age` SHALL 为 60 而非默认 30

#### Scenario: 禁用特定路由缓存

- **GIVEN** 配置设置 `api.cache.policies.capabilities.enabled: false`
- **WHEN** 请求 `GET /v1/capabilities`
- **THEN** 响应 SHALL NOT 包含 `Cache-Control` 或 `ETag` header

### Requirement: RPC 请求不缓存

RPC dispatcher SHALL NOT 对任何 RPC 调用返回缓存头。

#### Scenario: RPC 请求无缓存

- **WHEN** 请求 RPC `Pinax.Note.Read`
- **THEN** 响应 SHALL NOT 包含 `Cache-Control` 或 `ETag` header
- **AND** 响应 SHALL NOT 检查 `If-None-Match`
