## 为什么

跨子项目数据库审查发现 Pinax 的本地索引投影已经使用 GORM，但 `internal/index` 普通业务路径仍直接使用 GORM runtime API，例如 `Where`、`Find`、`Create`、`Save`、`Delete`、`Raw("PRAGMA schema_version")` 和手写条件字符串。新的治理要求是：数据库操作必须使用 GORM Gen 类型化 DAO，不能继续在业务 repository/index 逻辑中直接拼 ORM 条件或硬编码 SQL。

Pinax 的 `.pinax/index.sqlite` 是 note/link/tag/search/property/asset 投影的核心。这个索引会继续承载 notebook core、MCP readonly、remote API mode 和 agent-safe proof loop；如果不切到 GORM Gen，字段名漂移、条件字符串和 projection 写入会继续隐藏在大文件中，难以长期维护。

## 改动内容

- 为 `internal/index` 引入 GORM Gen 生成链路和 generated query package。
- 将索引重建、增量更新、lookup、doctor、diagnose、property projection 和测试 helper 的普通业务查询迁移到 GORM Gen DAO。
- 移除或集中处理 `db.Raw("PRAGMA schema_version")` 这类硬编码查询；需要 schema metadata 时优先用 GORM migrator 或集中 helper。
- 增加 source guard test，禁止普通 index 业务代码重新引入 direct SQL、`Raw`、`Exec` 或 direct GORM business query。
- 保持 vault Markdown、`.pinax/` CLI-authored assets、CLI output contract 和现有 index schema 不变。

## 非目标

- 不重做 Pinax vault 信息架构。
- 不把 Markdown note 真源迁入数据库。
- 不改变 MCP readonly、cloud sync、provider、Git 或 CLI 输出行为。
- 不新增外部数据库或远程服务依赖。

## 影响范围

- Owner: `cli/pinax`。
- 主要代码：`internal/index/**`，新增 `internal/index/gormgen` 和 `internal/index/query`。
- 验证：index focused tests、app/search/MCP paths where relevant、`task check` 或等价 Go/OpenSpec gate。
