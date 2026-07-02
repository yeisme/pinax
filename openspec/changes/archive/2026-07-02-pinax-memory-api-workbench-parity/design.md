# 设计：Memory API 与 Workbench 对齐

## 契约面

新增能力使用现有 `RemoteCapabilities` 与 `RemoteRoutes` 注册表，OpenAPI、RPC gate、Workbench capability explorer 都从同一份注册表读取。

```mermaid
flowchart LR
  CLI[pinax memory CLI] --> App[app.Service Memory*]
  REST[/v1/memory* REST] --> App
  RPC[Pinax.Memory.* RPC] --> App
  Caps[/v1/capabilities] --> Registry[Remote capability registry]
  Workbench[API Workbench] --> REST
  Workbench --> Caps
  App --> Ledger[(.pinax/memory/ledger.sqlite)]
```

## REST 路由

- `GET /v1/memory` -> `memory.list`
- `POST /v1/memory:capture` -> `memory.capture`
- `GET /v1/memory:recall?query=...` -> `memory.recall`
- `GET /v1/memory:context?task=...` -> `memory.context`
- `GET /v1/memory:stats` -> `memory.stats`

## RPC 方法

- `Pinax.Memory.List`
- `Pinax.Memory.Capture`
- `Pinax.Memory.Recall`
- `Pinax.Memory.Context`
- `Pinax.Memory.Stats`

## 写入策略

`memory.capture` 是写命令。`dry_run=true` 只构造预览记录，不落盘，可在只读服务上使用。真实写入必须满足：

- `pinax api serve --allow-write` 已开启；
- 请求带 `yes=true`；
- payload 满足 `body` 或 `subject` + `object` 的 capture 约束。

## Workbench

`pinax api serve` 暴露 `/workbench` 本地页面：

- Memory tabs：Capture、Records、Recall、Context、Stats。
- Capability Explorer：展示 capability id、command、REST/RPC、read/write gate、copy command。
- 写入按钮在服务端未开启 `allow_write` 时禁用；dry-run 仍可预览。

`pinax vault dashboard` 继续只读，不新增写入入口。

## 验证

- `go test ./internal/app ./internal/api ./cmd/pinax -run 'Memory|RPC|Remote|Workbench' -count=1`
- `go test ./internal/api ./internal/app ./cmd/pinax -count=1`
- `openspec validate pinax-memory-api-workbench-parity --strict`
