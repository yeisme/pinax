# MVP 范围

MVP 分四个阶段推进：

| Phase | 目标 | 验证 |
| --- | --- | --- |
| Local Vault Workbench | `init`、`doctor`、`note new/list/show`、`search`、Git snapshot plan | `go test ./...` 与 testscript e2e |
| CLI-backed Provider Pull | `ntn` / `lark-cli` capability probe、fake executable、`sync diff`、`sync pull --dry-run` | provider 和 sync fixture 测试 |
| Agent/MCP Read and Plan | `pinax mcp serve` 的只读 resources/tools、handoff、triage dry-run | MCP frame 和 output contract 测试 |
| Controlled Apply | action file apply、本地写入 approval、event evidence、Gateway handoff | dry-run/yes gate 和 redaction 测试 |

每日热点笔记 briefing 是后续 agent workflow 切片，必须基于本地 vault、research evidence ledger、review queue 和 delivery receipt，不应变成独立新闻 bot。

