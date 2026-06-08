# MVP 范围

MVP 分四个阶段推进：

| Phase | 目标 | 验证 |
| --- | --- | --- |
| Local Vault Workbench | `init`、`validate`、daily/inbox、`note list/show`、links/backlinks/orphans、attachments、saved views、index/search、Markdown import/export、`metadata plan/apply`、`repair plan/apply`、`organize suggest/list/apply`、`git snapshot` | `go test ./...` 与命令级测试 |
| CLI-backed Provider Pull | `ntn` / `lark-cli` capability probe、fake executable、`sync diff`、`sync pull --dry-run` | provider 和 sync fixture 测试 |
| Agent/MCP Read and Plan | `pinax mcp serve` 的只读 resources/tools、handoff、triage dry-run | MCP frame 和 output contract 测试 |
| Controlled Apply | action file apply、本地写入 approval、event evidence、Gateway handoff | dry-run/yes gate 和 redaction 测试 |

每日热点笔记 briefing 是后续 agent workflow 切片，必须基于本地 vault、research evidence ledger、review queue 和 delivery receipt，不应变成独立新闻 bot。

当前 MVP 的第一条自用闭环优先服务真实 Markdown vault：先让用户能安全接入、捕获 daily/inbox、建立 SQLite/GORM 本地索引、按 tags/group/folder/kind/status 检索和浏览、保存常用视图、检查链接/附件、导入导出 Markdown bundle、补 metadata、生成 repair/organize 计划，再在显式 Git snapshot 保护后执行本地改动。MCP 在 MVP 中只读，负责让 agent 查询 vault、读取笔记和查看整理计划，不直接写文件或远端 provider。
