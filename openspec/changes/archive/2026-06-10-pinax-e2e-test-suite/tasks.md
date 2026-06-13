## 1. 测试基础架构与共享构建

- [x] 1.1 在通用测试包的 `TestMain` 中，实现统一的编译缓存，仅在测试启动时编译一次 `pinax` CLI 及 Fake CLI 依赖至共享临时目录
- [x] 1.2 在 testscript 启动参数的 `Setup` 阶段，将编译好的临时共享二进制目录注入 `PATH` 最前端，以覆盖系统原生命令

## 2. CLI 输出契约校验 (Contract Verification)

- [x] 2.1 编写 JSON Envelope 契约自动验证工具，支持对 `--json` 标准信封格式（检查 `spec_version`、`mode`、`command` 和 `status` 字段）的测试断言
- [x] 2.2 新建 `tests/e2e/testdata/config/scripts/config_contract.txt` 脚本，测试配置类命令的多模式（Summary/Agent/JSON）输出标准契约
- [x] 2.3 扩充 existing testscript，增加对 stdout 中空值、非 ANSI 等格式污染的安全拦截校验

## 3. 本地 Markdown 与 SQLite 索引一致性测试 (State Projection)

- [x] 3.1 新建 `tests/e2e/testdata/links/scripts/link_projection.txt` 测试脚本，验证物理 Markdown 存在双向链接后，能通过 `pinax query` 精准查询入链投影
- [x] 3.2 编写 `tests/e2e/testdata/records/scripts/records_metadata_sync.txt` 脚本，验证笔记增加/修改元数据时，索引缓存的更新和自愈逻辑符合一致性审计

## 4. 外部 Provider 离线模拟与流事件测试 (Provider Mocking & Events)

- [x] 4.1 编写伪造的 `fake-lark-cli` 和 `fake-ntn` Go 代码，能够基于参数返回 mock 响应及流式 NDJSON
- [x] 4.2 新建 `tests/e2e/testdata/sync/scripts/sync_offline.txt` 脚本，注入 Fake CLI，离线闭环断言 `pinax sync --events` 的流事件（start/end 契约）
- [x] 4.3 编写 `tests/e2e/testdata/sync/scripts/sync_redaction.txt` 脚本，验证同步故障/错误时，在默认中文摘要、标准输出和错误流中敏感 Token 均已成功脱敏（验证 `[REDACTED]` 替代）
