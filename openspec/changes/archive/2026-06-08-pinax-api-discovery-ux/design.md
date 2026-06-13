## Context

Pinax 已有 `pinax api routes`、`pinax api schema export` 和 `pinax api serve` 命令。API schema 导出能力存在，但用户从 root 命令尝试 `pinax schema` 时没有可发现路径；routes 默认人类摘要也缺少 endpoint 明细。

## Goals / Non-Goals

**Goals:**

- 保留主命令树的清晰度，同时让自然输入的 `pinax schema` 能进入 schema help/export 流程。
- 让 `pinax api routes` 默认输出能直接显示 REST path、RPC method 和 projection command。
- 继续通过同一个 projection 渲染默认、`--json`、`--agent` 等输出模式。

**Non-Goals:**

- 不新增公网 API、鉴权、CORS、TLS 或 long-running daemon 行为。
- 不改变 OpenAPI schema 字段、不新增 route registry 来源、不修改 handler 映射。
- 不把 root `schema` 作为主帮助入口公开。

## Decisions

- 在 `internal/cli/api_cmd.go` 提取 schema command builder，分别挂载到 `api schema` 和隐藏 root `schema`。这样两个入口复用同一 service 调用和输出渲染，避免分叉。
- 把 routes 人类摘要放入 projection `Evidence`，而不是定制 renderer。这样 `--json` 仍保留完整 `data.routes`，默认输出自动多出证据表格。
- routes projection 的下一步 action 使用现有 `shellQuote` 拼接 vault path，保持用户可复制执行。

## Risks / Trade-offs

- 隐藏 root `schema` 可能与未来数据库 schema root 入口命名冲突。缓解：root alias 隐藏且只承载 `export`，主路径仍是 `pinax api schema export`。
- Evidence 摘要不是完整机器合同。缓解：脚本和 agent 继续使用 `--json` 读取完整 route/capability 数据。
