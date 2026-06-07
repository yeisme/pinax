## 1. Domain and Application Services

- [x] 1.1 梳理现有 vault scan、frontmatter、index、event 和 output projection 代码路径，确认 stats/doctor 可复用的 repository/service 边界。
- [x] 1.2 新增 vault analytics domain model，覆盖 note count、tag count、目录分布、frontmatter 覆盖率、最近更新、scan duration 和 index status。
- [x] 1.3 新增 `VaultAnalyticsService`，从 Markdown vault、文件 stat、现有 index 投影和 event evidence 计算 stats，索引缺失或过期时降级为 Markdown scan。
- [x] 1.4 新增 vault health issue model，定义 `issue_code`、`severity`、affected path/note id、evidence 和 next actions。
- [x] 1.5 新增 `VaultHealthService`，检测 missing title、missing tags、missing Pinax metadata、duplicate title、empty note、stale note、orphan note、path anomaly 和 stale index。
- [x] 1.6 为复杂 issue 判定、路径边界和 index freshness 判断补充中文注释，说明判定意图和失败降级行为。

## 2. CLI Commands and Output Contract

- [x] 2.1 新增 `pinax stats` Cobra 命令，支持 `--vault`、默认 human 输出、`--json` 和 `--agent`。
- [x] 2.2 新增 `pinax doctor` Cobra 命令，支持 `--vault`、`--stale-after`、默认 human 输出、`--json` 和 `--agent`。
- [x] 2.3 在 `internal/output` 新增 stats projection renderer，确保 JSON envelope、agent 输出和中文 human 摘要来自同一 projection。
- [x] 2.4 在 `internal/output` 新增 doctor projection renderer，确保 issue code、severity 分布、next actions 和 diagnostics stdout/stderr 分离。
- [x] 2.5 更新 root command help，使 `pinax --help` 暴露 `stats`、`doctor` 和 `dashboard`，并使用本地 Markdown vault 管理语义。

## 3. Readonly Local Dashboard

- [x] 3.1 新增 dashboard application/server 边界，HTTP handler 只调用 stats 和 doctor service，不在 handler 中重复扫描 vault。
- [x] 3.2 新增 `pinax dashboard --vault <vault> --port <port>` 命令，默认绑定 `127.0.0.1`，`--port 0` 使用系统分配端口。
- [x] 3.3 实现 dashboard 静态 UI 或服务端 HTML，展示统计摘要、health issues、recent activity 和 index status。
- [x] 3.4 实现只读 JSON data endpoint，并确保 dashboard URL、启动日志和 diagnostics 写 stderr。
- [x] 3.5 验证 dashboard 不提供 Markdown、`.pinax/`、Git、provider 或 remote 写入路由。
- [x] 3.6 对 dashboard 输出经过 redaction projection，避免展示 token、webhook URL、cookies、Authorization header、raw payload 或未脱敏 trace。

## 4. Tests and Fixtures

- [x] 4.1 新增 fixture vault，覆盖有 metadata、缺 metadata、重复标题、空笔记、孤立笔记、stale note、缺 index 和 stale index 场景。
- [x] 4.2 为 `pinax stats --json` 增加 contract tests，验证单一 JSON envelope、指标字段、index missing 降级和 stdout/stderr 分离。
- [x] 4.3 为 `pinax stats` 默认输出增加 golden test，验证中文摘要不包含机器 diagnostics。
- [x] 4.4 为 `pinax doctor --json` 增加 contract tests，验证 issue code、severity、evidence、next actions 和只读行为。
- [x] 4.5 为 `pinax doctor --agent` 增加 contract tests，验证 agent 输出稳定且不混入 human prose。
- [x] 4.6 为 path boundary 增加测试，验证 vault 外路径被拒绝且不会读取或渲染 vault 外文件。
- [x] 4.7 为 dashboard handler 增加测试，验证 localhost 绑定、只读 endpoints、service 复用和敏感信息脱敏。
- [x] 4.8 增加 process e2e 或 testscript 覆盖 `pinax --help`、`pinax stats`、`pinax doctor` 和 `pinax dashboard --port 0` 的基本用户流程。

## 5. Documentation and Verification

- [x] 5.1 更新 Pinax 本子项目 README 或 docs 中的 note CLI 使用说明，新增 stats、doctor、dashboard 示例，不新增独立执行 checklist。
- [x] 5.2 运行 `gofmt -w` 覆盖变更 Go 文件。
- [x] 5.3 运行 `go test ./...` 并记录结果。
- [x] 5.4 运行 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` 并记录结果。
- [x] 5.5 运行 `openspec validate --all` 并记录结果。
- [x] 5.6 如果本机安装 `task`，运行 `task check` 并记录结果；否则记录 fallback 命令结果。


## Verification Evidence

- [x] RED: `go test ./internal/app ./cmd/pinax ./internal/dashboard` 先失败，缺 `VaultStats`、`VaultDoctor`、`stats` 命令和 dashboard `NewServer`。
- [x] GREEN: `go test ./internal/app ./cmd/pinax ./internal/dashboard` 通过。
- [x] 全量测试: `go test ./...` 通过。
- [x] 构建: `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` 通过。
- [x] OpenSpec: `openspec validate --all` 通过。
- [x] 质量门禁: `task check` 通过。
