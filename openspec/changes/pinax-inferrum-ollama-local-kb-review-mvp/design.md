## Context

Pinax 当前已经能把 Markdown vault 分块、调用 gemini/openai/ollama/fake provider，并通过 `inferrum-lancedb-sidecar` 写入本地 LanceDB。Inferrum 独立子项目提供 `github.com/yeisme/inferrum`、统一 provider registry、`Record`/`Store`/`RetrievalPipeline`、域多路复用 sidecar 和冻结的 `inferrum.sidecar.v1`。本变更完成 Pinax 作为首个 personal-KB consumer 的迁移，并把真实检索评测和 Web 评审所需 projection 纳入同一边界；旧 Pinax Python sidecar 分发不再保留，只有历史投影只读解析继续存在。

当前服务器的 2026-08-01 基线：Go 1.26.4、Ollama 0.20.5、128 CPU、约 1 TiB 内存；根盘使用率 96%，swap 已满。Ollama 初始未运行、未安装模型，Python 3.14 环境未安装 LanceDB，统一 sidecar 不在 PATH。已完成的首个真实 canary 拉取 `qwen3-embedding:0.6b`，base digest 为 `ac6da0dfba84a81fdbfbaf330198c33cd77c4cdfc53e8bc50eb581914a15621d`；由它创建的 `pinax-qwen3-embedding:lowmem` resolved model-manifest digest 为 `d8554d8d831a97fca079c9f690550e51d00d5dffff9fe93fe516cbdfe7edd859`，磁盘大小约 639 MB，返回 1024 维向量。服务仅检测到 CPU；默认 128 推理线程的冷请求约 26 秒，而 CPU affinity=8 且 derived profile `num_thread=8` 的同模型冷请求约 0.74 秒。该数字只证明当前机器上的调优方向，不是发布性能承诺。

本设计同时受以下约束：

- Pinax 保留 Markdown/source version、chunk、权限、citation、评测集和 UI projection 所有权。
- Inferrum 不 import Pinax，不解释 note/status/kind/权限语义，sidecar metadata 始终不透明。
- `inferrum.sidecar.v1` 不变；发现协议缺口时停止 Pinax 实现并回到 Inferrum owner 建兼容迁移 change。
- Web 不直接读取 vault、`.pinax/**`、SQLite、LanceDB、provider 配置或绝对路径。
- 当前 MVP 只验证 retrieval/citation；embedding 模型不等于回答生成模型。
- 所有 integration/component/system/e2e 运行必须写入 Pinax 自有 `temp/integration-test-runs/<run-id>/`，并经过默认脱敏。

## Goals / Non-Goals

**Goals:**

- 让 `pinax kb rebuild/search/context/provider doctor` 保持原命令面，同时内部统一使用 Inferrum SDK 与 `inferrum.sidecar.v1`。
- 用真实 Ollama embedding、真实 LanceDB 和代表性 Markdown/纯文本资料证明可重建、可检索、可引用、可评测链路。
- 以 staged generation、原子激活和 previous generation 回滚保护现有本地投影。
- 对 allowed IDs、metadata、citation、evidence 和 Web projection 采用 fail-closed/allowlist 边界。
- 提供版本化评测集、逐题 replay、Recall@K/MRR/citation coverage/latency 和稳定 run receipt。
- 为 Workbench 冻结最小只读“知识库评审”页面所需的 API/capability projection、状态和 fixtures。
- 在当前共享服务器上使用单模型、单并发、短 keep-alive、受限 CPU、loopback 和按需 sidecar 的最低资源拓扑。

**Non-Goals:**

- 不引入 PostgreSQL、Redis、Kafka、Kubernetes、MinIO、Infinity、云向量数据库或 Gnosis runtime。
- 不在 Inferrum sidecar 中实现 Pinax 权限、source 解析、chunk、评测或 UI 语义。
- 不在 `cli/pinax` 新建 React/Electron 应用；完整 UI 由 `client/yeisme-workbench` 的独立 change 实现。
- 不在 P0 实现 PDF、网页、Git 仓库、飞书、图片 OCR acquire adapter；未实现的格式保持明确 `unsupported`/`not_configured`。
- 不生成最终问答，不接生成模型，不把 extractive context 标为模型答案。
- 不自动监听文件、不自动重建、不自动重试超时的 embedding/evaluation run。
- 不删除用户已有旧 projection；旧 Pinax sidecar binary、Python package、CI 与新写协议在本次一步切换中删除。
- 不将当前服务器调优数字写成通用 SLA。

## Decisions

### 1. Pinax 是 consumer owner，Inferrum 保持域无关

Pinax 的 `internal/semantic` 收敛为域适配层：扫描 canonical notes、生成稳定 chunk ID、构造 safe metadata、预计算 allowed IDs、实现 `inferrum.Domain`，并调用 Inferrum provider/store/pipeline。通用 provider HTTP、sidecar subprocess、vector store 和 retrieval stage 不再复制。

备选方案是在 Inferrum 增加 Pinax importer。该方案会让 Inferrum 解释 Markdown、note status、vault path 和权限，违反产品中立边界，因此不采用。另一个备选是由 Pinax shell-out 到 `inferrum` operator CLI；该 CLI 是调试面，当前 `rebuild` 不拥有 Pinax 真源和权限，也不适合作为应用内部 RPC，因此不采用。

### 2. Inferrum v1 新写入，历史 Pinax v1 仅作显式只读迁移

迁移后的 active write path 只发 `inferrum.sidecar.v1`，记录映射为：

```text
Pinax Chunk
  chunk_id       -> Record.id
  vector         -> Record.vector
  model/dim      -> Record.embedding_model / embedding_dim
  indexed_at     -> Record.indexed_at
  safe citation  -> Record.metadata (opaque to Inferrum)
```

metadata allowlist 首版只包含 `note_id`、安全的 vault-relative source ref、title、heading path、page/span、bounded preview、content/chunk digest、tags、kind、status、source type/version/digest。未知字段默认丢弃；全文、绝对路径、向量、raw prompt、provider payload、Authorization、cookie、secret 不进入 sidecar。

历史 Pinax v1 投影只通过稳定 profile `legacy_v1_readonly` 暴露。首次发布 Inferrum v1 默认写路径的 Pinax 版本记为 N；N 与紧随其后的 N+1 必须保留历史 search/context/doctor，最早 N+2 才可通过独立兼容迁移 OpenSpec 删除。profile 输出必须包含 `compatibility_introduced_release=N`、`compatible_through_release=N+1`、`removal_eligible_release=N+2`；未发布的 dev build 使用 `unreleased` 且不得自行到期。任何 import/rebuild/refresh/activate/rollback 或其他 mutation 在该 profile 下返回稳定 read-only 错误，正常新写入只走 Inferrum v1。

### 3. generation 状态机保护唯一 active projection

投影目录采用 generation，而不是直接覆盖当前 store：

```text
<vault>/.pinax/kb/
  activation.json
  activation.lock
  generations/<generation_id>/
    lancedb/
    generation.json
    source-manifest.json
    evaluation-summary.json
```

`activation.json` 是 active/previous 的唯一权威描述符，一次性保存 `sequence`、active generation、previous generation、protocol、provider、exact model tag、resolved model-manifest digest、dimension、source snapshot/digest、activated_at、generation manifest hash 和通过的 evaluation receipt hash，不保存正文或向量。`previous.json` 不存在，避免两个指针分步更新。

所有 activate/rollback/prune mutation 必须先获得 vault-scoped exclusive lock，并携带读取时的 expected `sequence` 做 compare-and-swap；并发冲突返回稳定错误，不能 last-writer-wins。提交顺序为：验证 immutable generation 与绑定 receipt → 在同目录写完整临时描述符 → flush/fsync 文件 → atomic rename 为 `activation.json` → 在平台支持时 fsync 父目录 → 释放锁。崩溃发生在 rename 前时旧描述符仍权威，发生在 rename 后时新描述符完整权威。search/context 一次读取并固定一个 descriptor snapshot，不能在单次请求中跨 generation。实现必须对每个提交边界、stale sequence、并发 activate/rollback 和进程中断做故障测试；不依赖 symlink。

状态机：

```text
planned -> embedding -> indexing -> validating -> ready -> active
                    \-> failed                 \-> rejected
active -> previous -> pruned
```

只有以下条件全部满足才能从 `ready` 激活：所有 chunk 有非空且同维向量；source digest 在构建期间未变化；sidecar row count 与计划一致；真实 smoke query 返回可解析 citation；无权限泄漏；存在 `passed` evaluation receipt，且它精确绑定 candidate generation ID、source snapshot/digest、provider、exact model tag、resolved model-manifest digest、profile hash、dimension、suite ID/version、gate config hash 与 generation manifest hash。缺少或不匹配的 receipt 一律拒绝激活。失败 generation 保留 bounded manifest/evidence 供诊断，但不得替换 active。

### 4. 权限为空必须在 Pinax 侧短路

Inferrum v1 的 `allowed_ids=[]` 当前表示不限制搜索，因此 Pinax 必须在调用 Store/Search 前区分：

- `nil`：调用方尚未提供，必须由 Pinax 从真源候选解析；解析失败返回稳定错误。
- 非空：传给 Inferrum sidecar。
- 已解析为空：直接返回零结果和 `permission_empty` receipt，绝不调用 sidecar。

`KBDomain.ResolvePermission` 只认识 Pinax status/kind/private 规则；Inferrum 和 sidecar 不加入任何规则。该分支必须有确定性单元和真实 sidecar component 测试。

### 5. Ollama 使用 exact model profile 和分层 readiness

默认产品模型不从 Inferrum 的 `nomic-embed-text` 常量静默推导。每个 generation 固定 provider、exact model tag、`base_model_digest`（若适用）、exact tag 解析出的 `model_manifest_digest`、normalized derived Modelfile/config 的 `profile_hash`、Ollama daemon version 和 dimension；查询默认读取 active generation 的这些事实。兼容门以 exact tag 的 `model_manifest_digest`、profile hash 和 dimension 为准，不能拿 base digest 代替 derived identity。显式 override 或当前 `/api/tags` 解析身份与 active generation 不一致时失败，不自动混用向量空间。

当前服务器的最低资源 canary 使用：

```text
base model: qwen3-embedding:0.6b
base model digest: ac6da0dfba84a81fdbfbaf330198c33cd77c4cdfc53e8bc50eb581914a15621d
derived local tag: pinax-qwen3-embedding:lowmem
resolved model-manifest digest: d8554d8d831a97fca079c9f690550e51d00d5dffff9fe93fe516cbdfe7edd859
profile hash: sha256(normalized Modelfile + admitted runtime profile)
num_ctx: 512 起步
num_thread: 8（Ollama 0.20.5 implementation canary，非跨版本承诺）
keep_alive: 0
max loaded models: 1
parallel: 1
queue: 4
CPU affinity: 8 cores
nice: 10
```

`num_ctx` 是官方稳定 Modelfile 参数；`num_thread` 在当前 Ollama source 可用但未进入公开参数表，因此部署启动时必须通过版本、derived Modelfile 和真实 embed 延迟/RSS canary 验证。若 canary 不通过，回退为不写 `num_thread` 的模型并仅使用 CPU affinity；不得假定存在 `OLLAMA_NUM_THREADS` 环境变量。

readiness 分层：

1. process: `127.0.0.1:11434` 可连接。
2. daemon: `/api/version` 返回允许版本。
3. model: `/api/tags` 含 exact tag，且 resolved model-manifest digest 与 generation 一致。
4. embed: 非敏感 canary 返回非空固定维向量。
5. projection: sidecar doctor + active generation 可读。
6. retrieval: 真实 query 命中 citation。

只有第 4 层可声明 embedding available，只有第 6 层可声明 local KB 可检索。

### 6. P0 source acquisition 先落 canonical Markdown

P0 仅接收 Markdown/纯文本目录或文件，继续使用 `pinax kb import` 的 dry-run/confirm/service 边界，将输入规范化为 Pinax Markdown 后再进入 chunk/index。source lineage 至少记录 `source_type`、safe source ref、source digest、source version、acquired_at 和 importer version。

PDF、网页、Git 仓库、飞书、OCR 后续都必须先由 Pinax acquire adapter 生成/更新 canonical note 与版本 receipt；Inferrum 不直接抓取 URL、仓库或飞书 API。评审页可展示这些 source type，但未实现时只能显示 `unsupported`/`not_configured` 和真实下一步，不能提供假上传按钮。

### 7. 评测是 retrieval/citation 评测，不伪装答案生成

评测集由 Pinax 管理，建议保存在 vault 内受版本控制的安全位置，包含稳定 `suite_id/version/question_id`、问题文本或本地引用、期望 citation ref、可选期望要点和 tags。运行证据只保存 question ID/query digest，不复制原始问题或期望答案。

`pinax kb evaluate` 默认评测当前 active generation；激活候选时必须显式使用 `--generation <candidate-id>`（或等价内部服务参数）读取 immutable candidate manifest/store，不修改 `activation.json`。逐题执行与普通 query 相同的 permission、embedding、search、context 路径，计算：

- Recall@K：期望 citation 是否出现在 top K。
- MRR@K：首个期望 citation 的倒数排名。
- citation coverage：有期望 citation 的问题中至少命中一个的比例。
- no-hit/permission-empty/error/timeout/model-mismatch/source-drift 分类。
- total、embed、search、assemble 可观察阶段延迟；缺失阶段不得伪造为 0。

评测完成后写 immutable receipt，并绑定 candidate generation ID、source snapshot/digest、provider、exact model tag、resolved model-manifest digest、profile hash、dimension、suite ID/version、gate config hash 与 generation manifest hash。激活服务只接受该候选的 matching passed receipt，不能复用旧 active generation 或另一套 suite 的结果。

Inferrum `RetrievalManifest` 只作为进程内执行事实，Pinax 不原样持久化它。Pinax 必须转换为自有 allowlist `PersistedRetrievalReceipt`：原始 query 替换为 question ID/query digest；permission filter 替换为 policy/version、resolution status、allowed count 与 allowed-set digest；只保留 safe stage/score/citation/latency facts。integration log、receipt、fixture、API 和 Web projection 对该 DTO 做递归 sentinel 测试，任何 raw query、permission ID list、全文、绝对路径或 secret 为发布 veto。receipt 中的 manifest ref/hash只能指向该脱敏 Pinax DTO。

初始评审门是可配置基线而非全局 SLA。首个 20–50 问题 canary 建议 Recall@5 ≥0.80、MRR@10 ≥0.65、citation leakage=0；真实数据不足时 run 保持 `insufficient_dataset`，不得判定模型已通过。未接生成模型时 `answer_mode=not_generated`，UI 只展示检索 context 和 citations。

### 8. 评审 Web 只消费 bounded projection

Pinax Local API/capability registry 增加只读 projection，建议路径：

```text
GET /v1/kb/review/overview
GET /v1/kb/review/sources
GET /v1/kb/review/evaluation-suites
GET /v1/kb/review/evaluation-suites/{suite_id}/questions
GET /v1/kb/review/runs/{run_id}
```

P0 不提供浏览器 rebuild/evaluate mutation；页面显示禁用操作和可复制的真实 CLI 命令。后续若加入运行按钮，必须在新的 contract 中定义幂等 run admission、服务端 receipt、权限和 unknown-outcome 恢复，不能让前端执行 shell。

页面归 `client/yeisme-workbench`，挂载现有 Pinax workspace/module。布局采用紧凑页头、独立健康状态条、`问题评测`/`知识源` 两个一级页签、左侧问题摘要列表和右侧单一详情流；不嵌套第二套侧栏或页面内 workbench。

```text
[知识库评审] [仅本机]                         [刷新状态] [复制运行命令]
[3 sources] [6 chunks] [Ollama embed ready] [LanceDB ready] [index fresh]

[问题评测] [知识源]

┌─ 问题列表 320px ──────┬──────────────── 当前问题详情 ────────────────┐
│ 搜索 / 状态过滤        │ Q-017 · partial · 0.82s                    │
│ ○ unrun               │ 期望引用            观测结果                │
│ ● current / partial   │ source + heading    not_generated          │
│ ✓ passed              │                                             │
│ ! failed              │ 引用：rank / score / title / heading / ref  │
│                       │ 运行：model / generation / stages / tokens   │
└───────────────────────┴─────────────────────────────────────────────┘
```

首屏必须分别显示 Pinax source、Ollama daemon/model、LanceDB sidecar、active index freshness。问题详情显示 expected citation、observed retrieval、rank/score/title/heading/bounded preview/chunk ID、run/model/index/stages/context tokens；identity rerank 明确标记 passthrough，`not_generated` 不显示模型答案。50 个问题只加载摘要和一个详情，不批量返回全部 chunks。

### 9. 当前服务器使用按需进程和 loopback

开发拓扑：

```text
Pinax CLI / Local API (按需, loopback)
       | import github.com/yeisme/inferrum
       +--> Ollama 127.0.0.1:11434 (唯一常驻、单模型/单并发)
       +--> inferrum-lancedb-sidecar (每次命令启动的 Python 子进程，无端口)
       +--> <vault>/.pinax/kb/generations/*/lancedb
Workbench --SSH tunnel--> Pinax bounded GET projections
```

Pinax API/Dashboard 开发优先使用 `--port 0`；需要固定端口时每次部署重新探测，当前候选 `127.0.0.1:18786` 不写成永久默认。禁止绑定 `0.0.0.0`；远程评审通过 SSH tunnel。根盘达到 90% 以上时 rebuild admission 默认拒绝或要求显式 override，模型缓存目标 ≤4 GiB，首批 50–200 文档的 generation+manifest+evidence 目标 ≤2 GiB，日志按大小/时间轮转。

### 10. 验收证据分层且默认脱敏

当前尚未完成 Pinax→Inferrum v1 的 runner 只能标记为 `baseline/component`，summary 必须列出 `proven` 与 `not_proven`；不得因为独立 Inferrum v1 与 Pinax v1 均通过就声明统一链路成功。最终 component/system run 由 Pinax 项目 runner 生成：

```text
temp/integration-test-runs/<run-id>/
  summary.json
  command.txt
  stdout.log
  stderr.log
  env.json
  artifacts/
    provider.json
    generation.json
    evaluation-summary.json
    citations.json
```

`env.json` 只记录允许的版本、端口类别、CPU/内存/disk 摘要和 env key 名，不记录 env value。日志/summary 不保存原始问题、全文、绝对 vault 路径、向量、Authorization、token、provider payload、hidden prompt 或 chain-of-thought。端口监听、HTTP 200、doctor、model tag、embed、row count、search 和 citation 各自有独立字段，不能用上游较弱证据推导下游成功。最终 acceptance 的 `project` 必须是 `cli/pinax`，并真实走一条 Pinax candidate generation → Inferrum v1 → evaluation receipt → activation decision 链路。

### 11. M4 Air 真实语料 runner 只生成候选证据，不隐式激活

`internal/testkit/kblocalevidence` 保留原有合成 canary，新增显式 `--real-corpus --vault <vault> --suite <suite>` 模式。该模式只允许 Ollama provider，并要求操作员显式提供既有 vault 与版本化评测集；没有确认 flag、vault 或 suite 时，在任何 Pinax rebuild 前失败。它不创建测试 note、不复制语料、问题或 vault 路径到 artifact，也不执行 `kb activate` 或 `kb rollback`。

runner 优先使用 `--pinax-binary` 指定的已编译 Pinax 二进制；未指定时只在临时目录以 `CGO_ENABLED=0` 编译一次再调用。artifact 只记录 `provided` 或 `compiled_temp` 的 binary mode，绝不记录本机二进制绝对路径。真实语料运行需要先做 `kb provider doctor`，再 rebuild candidate、evaluate candidate，并写入项目标准 evidence 目录。它会在 rebuild 前后从 Pinax-owned activation descriptor 读取 sequence 与 active generation identity；若二者改变，保留 `real-corpus.json` 与 failure artifact、返回 `activation_state_changed`，不会把这次并发状态下的结果标为可激活。它记录 host OS/arch、CPU 核数、可获得的 CPU 品牌/统一内存事实、Ollama 占用、RSS，以及 `resources.provider_doctor_duration_ms`、`resources.candidate_rebuild_duration_ms`、`resources.candidate_evaluation_duration_ms` 三个实际子命令时长；不会将一次运行标注为未验证的 cold/warm/P95。后面三项是 `real-corpus.json` 的 optional additive evidence fields，旧 consumer 可忽略，回滚只需停止写入或回退 runner，不影响 activation descriptor 或既有评测 receipt。既有 `target_m4_status` 继续标明是否观察到 Apple M4；新的 optional `target_m4_air_status` 只在观察到 `arm64` Apple M4 与受支持的 M4 Air `hw.model` 时为 `confirmed`，并作为 `real-corpus.json` 的同级事实与嵌套 host/preflight 投影输出。原始硬件 model identifier 不持久化。不能从用户标签、未识别 M4 设备或 Linux 运行推导 M4 Air 结论。

当真实语料调用选择 `--m4-preflight` 时，runner 必须先解析本次实际将执行的 Pinax binary（指定文件或临时 `CGO_ENABLED=0` 构建），并在读取 vault 前以 Go Mach-O 解析器确认它原生包含 `arm64`。`x86_64`/Rosetta、非 Mach-O 或未知架构必须以稳定失败码终止；成功 evidence 只增加可选 `pinax_binary_architecture` 值，绝不保存 binary path。该检查不适用于 `--m4-preflight-only`，该模式必须继续不解析、不构建或执行 Pinax binary。

目标 M4 Air 运行的操作员门禁独立于上述 host-fact 采集：运行前必须确认 macOS 14+、`arm64`、`target_m4_status=confirmed` 与 `target_m4_air_status=confirmed`、sidecar 实际 interpreter 为 Python 3.10+ 且原生 `arm64`、实际 Pinax 与编译 Inferrum Mach-O binary 均为 `arm64`、在隔离 venv 安装当前 sidecar 的 `lancedb>=0.33,<0.34`，并让该二进制对无敏感临时 root 的 `validate embedded` 成功。`--m4-preflight --inferrum-binary <binary>` 是 real-corpus runner 的 additive gate：它在读取 vault 前从 sidecar shebang 解析并验证实际 Python/architecture，以 Go Mach-O 解析器验证 Pinax 与 Inferrum binary 架构，随后执行上述 compiled binary 验证，将仅含版本/M4 Air host/native architecture/embedded-LanceDB 安全事实的 `m4-preflight.json` 写入 Pinax evidence，并把同一投影作为可选 `m4_preflight` 字段写入 `real-corpus.json`。`pinax_binary_architecture` 是 real-corpus artifact 的可选 additive 顶层与嵌套字段；不适用于 compatibility-only scope。它不保存 binary、sidecar、vault、LanceDB root 或硬件 model identifier。`task integration:kb-real-corpus` 强制该 gate；不带 flag 的旧 candidate-only runner 调用保持兼容但不能用于 M4 Air baseline 接受。`target_m4_status=confirmed` 单独只说明观测到 M4，不等于目标 Air 身份、native Python/binaries 或 LanceDB 合约已通过。任一预检失败时，操作员不得启动 candidate rebuild 或把结果用于 baseline 决策。

同一 real-corpus gate 在 compiled embedded-LanceDB 验证通过后、读取 vault 前，以已验证的 Inferrum binary 精确执行一次 `provider benchmark ollama --model <exact-model> --samples 64 --batch-size 8 --warmup 1 --json`。Pinax 只 allowlist 该 JSON envelope 的 provider/model、sample/batch、observed dimension、batch API、warmup/elapsed/P50/P95、items/sec、固定 scope 与 not-measured facts，写入 `provider-benchmark.json`，并将它作为 optional `provider_benchmark` 投影写入 `real-corpus.json`。这不是检索质量或内存压力结论，不能替代随后的 candidate rebuild/evaluate；但它把同一台原生 M4 Air 的 provider 性能证据与 model identity 绑定到候选运行。任一输出不符合预期、Inferrum benchmark 失败或 metrics 无效时在触碰 vault 前失败。compatibility-only 不调用 Ollama 或 benchmark，也没有该 artifact。新增字段只追加，不改变既有 `real-corpus.json`、activation descriptor 或 receipt；旧 consumer 可忽略，回滚为停止调用该新增 benchmark/projection 或回退 runner。

M4 first-support 不能只依赖通用 `kb evaluate` 的 `status=passed`：该 status 的现有语义允许期望 citation 在 top-10 内完整命中，而 pilot 的 release gate 需要 `citation_coverage` 在评测 K（默认 K=5）为 `1.00`。因此仅当 `--real-corpus --m4-preflight` 运行时，runner 额外计算只读、可选的 `first_support_gate` 投影。它要求 M4 预检和 provider benchmark 均为 passed、activation 未变化、evaluation status 为 passed、Recall@5 `>=0.80`、MRR@10 `>=0.65`、citation coverage `=1.00`、failure count `=0`；缺失、非有限或范围外的 metric 也 fail closed。runner 先写完整 `real-corpus.json`，然后在 gate 失败时写 bounded `failure.json` 并以非零状态结束，绝不 activation。

这是 M4 pilot 的新增证据/执行门，而不是对通用 `pinax kb evaluate`、既有 receipt、activation eligibility 或其 `gate_config_hash` 的重定义：candidate-only 调用保持原行为，历史 receipt 继续按照其原有 gate 被读取和激活。`first_support_gate` 仅为新增 optional artifact 字段，旧 consumer 可忽略；当前未存在通过的 M4 real-corpus release artifact，因此无数据迁移或 deprecation window。若该门导致误拒绝，回滚为停止使用 M4 runner 的新增 gate 或回退该 runner 版本；它不写 activation descriptor、评测 receipt 或 canonical vault 内容。

为在尚未接触任何 vault 前验证目标机环境，runner 还提供 additive `--m4-preflight-only --inferrum-binary <binary>`。该模式拒绝 `--real-corpus`、vault 与 suite 输入，不调用 Pinax binary、Inferrum provider benchmark、provider doctor、Ollama、rebuild、evaluate、activate 或 rollback，只写标准 evidence 和 `scope=compatibility_only` 的 `m4-preflight.json`；`task integration:kb-m4-preflight` 强制 sidecar 与 Inferrum binary 输入。`scope` 是现有 preflight artifact 的 optional additive field，旧 consumer 可忽略；回滚只需停止使用新 task/flag 或回退 runner，不影响既有 real-corpus preflight、descriptor、receipt 或 generation。compatibility-only passed 只证明可开始真实语料评测，不是 M4 quality/capacity/baseline 或发布结论。

通过评测只表示候选满足当前 Pinax gate，`activation_status=not_attempted` 始终保留。模型比较、人工审阅和显式 `pinax kb activate`/`pinax kb rollback` 是随后独立的操作员决定。

## Risks / Trade-offs

- [Pinax dirty worktree 与其他变更重叠] → 本 change 先限制在 `internal/semantic`、KB app/CLI/API、相关 tests/specs；实施前冻结 path lease，遇到重叠停止写入并由 root 协调。
- [空 allowed IDs 造成全库泄漏] → Pinax fail-closed 短路、safe-field allowlist、单元/component/Web redaction 三层测试；该项为发布 veto。
- [直接 overwrite 或并发指针竞争破坏唯一投影] → immutable generation、单一 activation descriptor、vault lock、sequence CAS、fsync/rename 故障测试和 previous 回滚。
- [历史投影与 Inferrum v1 双轨增加复杂度] → 历史投影仅 `legacy_v1_readonly`，N/N+1 保留读取、最早 N+2 另开 change 删除；所有 mutation 确定性拒绝，新写入只走 Inferrum v1，不做双写。
- [Inferrum manifest 持久化泄漏原始问题/权限] → 只保留进程内 raw manifest，落盘前转换为 Pinax allowlist receipt 并递归扫描。
- [Ollama `num_thread` 未公开文档化] → 固定 0.20.5 canary、记录 derived Modelfile、真实 embed/RSS 测试；失败时回退 CPU affinity，不把 implementation detail 写入稳定跨版本合同。
- [CPU-only embedding 影响延迟] → 单模型、单并发、batch rebuild、低优先级/CPU affinity；用真实评测决定是否扩大 CPU 或启用可用 GPU，而不是预先常驻更多模型。
- [根盘 96% 与 swap 满] → admission disk threshold、generation/evidence/model quota、失败清理和 previous generation 有界保留；vault lock 强制串行 mutation。
- [Python 3.14 与 LanceDB wheel 兼容未知] → 使用独立 Python 3.11/3.12 venv 与固定 `lancedb>=0.33,<0.34`；不污染系统 Python。
- [只读 Web 无法一键运行问题] → P0 显示真实 CLI 命令和最新 receipts；获得稳定 run admission contract 后再加按钮，避免 shell 执行和未知结果自动重试。
- [首批数据不足导致虚假模型结论] → 少于约定问题数时输出 `insufficient_dataset`；所有阈值保留 suite/version 和 corpus 适用范围。
- [PDF/网页/仓库/飞书期望被误认为完成] → source inventory 逐项显示支持状态；P0 只验 Markdown/text。

## Migration Plan

1. 冻结现有 `pinax kb` human/JSON/agent/events、v1 sidecar request、projection metadata 和真实/假 provider 测试基线。
2. 在 Pinax 增加 Inferrum module 本地 replace，仅实现 `KBDomain`、Chunk→Record、safe metadata allowlist 和 fail-closed allowed-ID 单元测试；不切 active 写路径。
3. 安装隔离的 `inferrum-lancedb-sidecar`，对 deterministic fixtures 跑 Inferrum v1 shadow generation，与历史投影的 document/chunk/citation facts 对照。
4. 实现 immutable generation、单一 activation descriptor、vault lock/sequence CAS、fsync/rename 崩溃恢复、previous 回滚和 disk admission；使用 fake provider 先完成 RED/GREEN，但尚不允许无评测 receipt 的候选激活。
5. 在当前服务器启动 loopback Ollama，固定 exact tag、base digest、resolved model-manifest digest、profile hash 与 dimension，创建低资源 derived model canary，跑真实 embed 与 50–200 文档 shadow rebuild。
6. 实现 active/candidate evaluation suite/run/metrics、Pinax allowlist receipt 与 candidate-bound gate；用 20–50 题真实问题冻结初始 baseline。未达到门槛或 receipt 身份不匹配时保持 shadow，不切 active。
7. 将 `pinax kb` active write/read 切到 Inferrum v1；激活只接受 matching candidate receipt；保留显式 `legacy_v1_readonly` 读取兼容并拒绝其全部 mutation。验证无命令/机器输出 breaking change。
8. 增加 Local API bounded GET projections、capability metadata、fixtures 与 contract tests；把 Workbench 实现 handoff 到其独立 change。
9. 运行 Go/sidecar/OpenSpec、Pinax focused/full checks、真实 component/system/e2e 和故障注入；证据写入 project temp。
10. 连续 canary 通过且 N+1 兼容发布完成后，最早在 N+2 另开 change 决定 v1 shim 删除；本 change 不删除。

回滚顺序：停止新 run admission → 在 vault lock 下以 expected sequence 将 `activation.json` 的 active/previous 一次性 CAS 交换 → 仅在兼容窗口且确有必要时切换 `legacy_v1_readonly` 读取 profile → 保留失败 generation 的脱敏 manifest → 停止/卸载 derived Ollama model，不修改 Markdown/Git/Cloud Sync 真源。

## Open Questions

- 用户最终提供的 50–200 份资料中，P0 Markdown/纯文本占比、平均长度和敏感等级是什么；哪些 source adapters 进入下一 change。
- 首批 20–50 个问题的 expected citation 如何标注：note+heading、page/span 还是稳定 source fragment ID。
- 当前任务只评 retrieval/citation；若需要生成答案，应选择哪个独立生成模型和资源上限。
- Workbench 评审是只通过 SSH/loopback，还是未来需要 LAN/公网；后者必须另做认证、反向代理和安全评审。
- active generation 的默认 previous 保留数量和 disk admission 阈值需要用首批真实 corpus 校准。
- `num_thread=8` 是否在未来 Ollama 版本继续可用；每次升级必须重新跑 model-only client canary。
