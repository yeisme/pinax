## Why

Pinax 已有本地 Markdown 知识库与 Ollama provider，Inferrum 也已冻结共享 SDK 和 `inferrum.sidecar.v1`，但 Pinax 当前仍维护 `pinax.kb.sidecar.v1` 与重复 provider/sidecar 实现，无法证明“Pinax 真源 → Inferrum 投影 → Ollama embedding → 可引用评测”的统一真实链路。现在需要先用当前服务器和最小资源 profile 完成个人/本地 MVP，把检索质量、引用安全和运行证据验证清楚，再考虑 Gnosis 企业依赖。

本变更承接根级 [`docs/architecture/inferrum-vector-rag-platform.md`](../../../../../docs/architecture/inferrum-vector-rag-platform.md) 的 Phase 3 Pinax handoff，以及 [`openspec/changes/gnosis-enterprise-knowledge-platform-v1/proposal.md`](../../../../../openspec/changes/gnosis-enterprise-knowledge-platform-v1/proposal.md) 中“Inferrum 保留 local/embedded profile、Pinax 保留 canonical state”的边界。

## What Changes

- 将 Pinax personal KB 内部接入 `github.com/yeisme/inferrum`：复用共享 embedding provider、`Record`/`Store`/`RetrievalPipeline`、`KBDomain` 与 `inferrum-lancedb-sidecar`，新写入只使用冻结的 `inferrum.sidecar.v1`；现有 `pinax kb` 命令、flags 和机器输出保持兼容。
- 保留 `pinax.kb.sidecar.v1` 为显式 `legacy_v1_readonly` 兼容 profile：迁移发布记为 N，N 与紧随其后的 N+1 保留只读 search/context/doctor，最早 N+2 才能经独立 OpenSpec 删除；该 profile 的 import/rebuild/refresh/activate/rollback mutation 全部拒绝。不得把 v1 可运行误报为已完成 Inferrum v1 对接。
- 增加 staged rebuild 与单一 activation descriptor：只有所有 chunk embedding 成功、模型/维度一致、源版本未漂移，且存在绑定候选 generation、source snapshot、模型身份、suite version 与 manifest hash 的通过评测 receipt 后，才能在 vault 锁与 compare-and-swap 下原子激活；失败、崩溃或并发冲突时保留现有 active generation。
- 增加 Ollama 本地 embedding profile：首个服务器 canary 分别固定 `qwen3-embedding:0.6b` 的 base digest、`pinax-qwen3-embedding:lowmem` 的 resolved model-manifest digest、derived profile hash 与 1024 维事实，并支持有界 `num_ctx`、版本门控的 `num_thread`、`keep_alive`、单并发和 loopback endpoint；doctor 必须执行真实 embed canary，不能只检查端口或 `/api/tags`。
- 增加 fail-closed 权限与 safe citation：Pinax 在调用 Inferrum 前预计算 allowed IDs；空集合直接返回零结果。投影和响应只包含安全 source ref、heading/page/span、chunk ID、bounded preview 与 score，不包含全文、绝对私有路径、向量、凭据或 provider payload。
- 新增版本化评测集与 `pinax kb evaluate` 合同，支持 active 或显式 candidate generation、20–50 个真实问题、期望引用、逐题结果、Recall@K、MRR、citation coverage、延迟、失败分类和可重放的脱敏 evidence；Pinax 只持久化自有 allowlist receipt，不落盘 Inferrum 原始 query/permission manifest。
- 为 `client/yeisme-workbench` 冻结只读“知识库评审” projection：独立展示 Pinax source、Ollama、LanceDB sidecar、index freshness 和每次 run 的真实状态；未接生成模型时明确显示 `not_generated`，当前 identity rerank 显示为 passthrough。
- P0 只承诺 Markdown 与纯文本通过 Pinax 进入 canonical vault；PDF、网页、代码仓库、飞书和图片 OCR 在对应 acquire adapter 未实现前返回 `unsupported`/`not_configured`，不得显示已摄取。
- 当前服务器采用最小资源拓扑：一个 loopback Ollama daemon、一个按命令启动的 LanceDB sidecar、一个按需启动的 Pinax API/Dashboard；不引入 PostgreSQL、Redis、Kafka、Kubernetes、MinIO、Infinity、文件 watcher 或云向量数据库。
- 当前 Inferrum 目录 canary 只标记为 `baseline/component`，并显式列出 proven/not-proven claims；最终 Pinax→Inferrum v1 generation/evaluation 验收必须由 Pinax 自有 `temp/integration-test-runs/<run-id>/` 生成脱敏证据。端口监听、HTTP 200、sidecar doctor、模型存在、真实 embed、真实 rebuild/search/citation 分别记录，不互相替代。

## Capabilities

### New Capabilities

- `pinax-local-kb-evaluation`: 定义版本化问题集、真实检索 replay、期望引用、质量/延迟指标、run receipt、脱敏证据和只读评审 projection。

### Modified Capabilities

- `personal-kb`: 将本地语义投影从 Pinax 专用 provider/sidecar 写路径迁到 Inferrum SDK + `inferrum.sidecar.v1`，并增加 staged activation、Ollama embed readiness、fail-closed allowed IDs、safe citations 与模型/维度/新鲜度合同。
- `pinax-web-client-contracts`: 增加 Workbench 所需的 KB 状态、source inventory、query/context/evaluation 只读 projection 和页面边界；浏览器仍不得直接读取 vault、`.pinax/**` 或 LanceDB。

## Impact

- Pinax 代码：`internal/semantic/**`、`internal/app/kb.go`、KB CLI/API capability 与相关测试；`go.mod` 以本地开发 replace 接入 `github.com/yeisme/inferrum`，不得复制 Inferrum 实现。
- 本地投影：`.pinax/kb/` 增加 immutable generation、单一 active/previous activation descriptor、vault-scoped lock/CAS 与可回滚目录；每个 generation 的 LanceDB 位于其私有子目录。Markdown/Git/Cloud Sync 真源格式不变，矢量仍不进入同步 manifest。
- 外部本地依赖：Ollama `0.20.5`、固定 exact tag/base digest/resolved model-manifest digest/profile hash、Python 3.11/3.12 隔离环境、`inferrum-lancedb-sidecar` 与 LanceDB `0.33.x`；不新增 Go runtime 的 CGO 或 Python 绑定。
- Web handoff：Pinax 只实现稳定 bounded projections、fixtures 与 contract tests；完整 React 页面进入 `client/yeisme-workbench` 的独立实现 change，不在 `cli/pinax` 新建应用壳。
- Inferrum：不修改 Go module path、公共接口或 `inferrum.sidecar.v1`；若实作发现需要协议变化，必须停止并创建 Inferrum owner 的兼容迁移 OpenSpec。
- 部署：仅绑定 `127.0.0.1`，远程评审通过 SSH tunnel；LAN/公网暴露、认证代理和长期 systemd 安装不属于本切片。
