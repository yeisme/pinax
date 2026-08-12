# Pinax 个人本地知识库 MVP

当前 P0 只把 Markdown/纯文本作为知识源，Pinax 保留真源和引用，Inferrum 负责通用向量存储/RAG sidecar，Ollama 只提供 embedding。PDF、网页、代码仓库、飞书和图片 OCR 当前显示为 `unsupported` 或 `not_configured`，不会伪造已接入。

## 当前服务器最小拓扑

```text
pinax CLI / loopback API
        ├─ Ollama 127.0.0.1:11434
        └─ inferrum-lancedb-sidecar（按命令启动的 Python 子进程）
```

当前 canary 使用 `pinax-qwen3-embedding:lowmem`，返回 1024 维向量；建议单模型、单并发、`keep_alive=0`，并按当前机器实际资源限制 CPU affinity/nice。模型 manifest digest、profile hash 和运行证据必须由 canary 记录，不要只拿 base model digest 代替 derived tag 身份。

## 当前服务器低资源 runbook

先确认现有 daemon，不要在 systemd 或其他 supervisor 已经托管 Ollama 时再启动第二个实例：

```bash
ollama --version                 # 当前 canary 记录为 0.20.5
OLLAMA_HOST=http://127.0.0.1:11434 curl -fsS http://127.0.0.1:11434/api/version
OLLAMA_HOST=http://127.0.0.1:11434 ollama list
OLLAMA_HOST=http://127.0.0.1:11434 ollama show pinax-qwen3-embedding:lowmem --modelfile
```

当前服务器的最低资源 profile 是：一个 exact derived model、一个并发、队列上限 4、`keep_alive=0`、`num_ctx=512` 起步、8 个 CPU affinity、nice 10。`num_thread=8` 只作为 Ollama 0.20.5 implementation canary，不能写成跨版本稳定环境变量；如果升级或 canary 失败，去掉它并重新测量。Ollama 已由 Pinax embed 请求固定 `keep_alive=0`，用于请求完成后卸载模型；并发/队列/CPU/nice 属于 daemon 或 supervisor 的运行策略，必须在实际服务配置中确认，不能从端口可达推导出来。

若当前机器没有 supervisor 托管且需要临时启动单实例，可用受限进程；否则只复用已有服务：

```bash
taskset -c 0-7 nice -n 10 env OLLAMA_HOST=127.0.0.1:11434 ollama serve
```

启动后必须执行真实 canary；它会记录 exact tag、base digest、resolved model-manifest digest、profile hash、daemon 版本、维度、cold/warm 时延、RSS、Ollama 模型占用和卸载状态，不保存向量或原始问题：

```bash
cd /home/yeshugen/workplace/yeisme-agent/cli/pinax
OLLAMA_HOST=http://127.0.0.1:11434 \
PINAX_KB_SIDECAR=/home/yeshugen/workplace/yeisme-agent/cli/inferrum/temp/local-kb-mvp/venv/bin/inferrum-lancedb-sidecar \
CGO_ENABLED=0 go run ./internal/testkit/kblocalevidence --allow-disk-high-water
```

只有 `summary.json` 的 `real_ollama_embed=true`、`inferrum_sidecar_v1=true`、`redacted=true` 且 artifact 的 `quality_verdict` 被正确标为 `not_proven_without_real_corpus`，才可把这次运行当作组件链路证据；它不能替代真实资料评测。

## MacBook Air M4 真实语料候选评测

M4 Air 的 first-support baseline 是 `qwen3-embedding:0.6b`。不要复用当前服务器的 `pinax-qwen3-embedding:lowmem` 标签，除非该 exact derived tag 已在目标 Mac 上重新创建、doctor 通过且 artifact 记录了对应的 model manifest identity。

### 必经兼容性预检

从仓库根目录开始。Ollama 的 M 系列 macOS 支持要求 macOS 14+；当前 Inferrum sidecar 要求 Python 3.10+，并把 LanceDB 限制为 `>=0.33,<0.34`。先在 M4 上完成二进制 + 原生 embedded LanceDB 检查，不使用 vault，也不需要容器：

```bash
cd cli/inferrum
sw_vers -productVersion  # Expected: 14 or later
uname -m                 # Expected: arm64
python3 --version        # Expected: 3.10 or later
ollama --version

python3 -m venv ./temp/embedded-venv
./temp/embedded-venv/bin/python -m pip install ./tools/inferrum-lancedb-sidecar
CGO_ENABLED=0 go build -trimpath -o ./temp/bin/inferrum ./cmd/inferrum
./temp/bin/inferrum validate embedded \
  --root ./temp/m4-preflight \
  --sidecar ./temp/embedded-venv/bin/inferrum-lancedb-sidecar \
  --json
```

如果版本/架构不符合、LanceDB 未以 macOS-arm64 兼容包安装，或 `validate embedded` 失败，停止，不要运行真实语料 runner，也不要把该主机标为 M4 性能证据。通过后回到 Pinax 构建二进制并拉取 baseline：

```bash
cd ../pinax
mkdir -p ./temp/bin
CGO_ENABLED=0 go build -trimpath -o ./temp/bin/pinax ./cmd/pinax
CGO_ENABLED=0 go build -trimpath -o ./temp/bin/pinax-kb-evidence ./internal/testkit/kblocalevidence
ollama pull qwen3-embedding:0.6b

./temp/bin/pinax kb provider doctor ollama \
  --model qwen3-embedding:0.6b --vault <vault> --json
```

也可在准备 vault 或评测集之前，用 Pinax 的 standalone gate 生成标准脱敏兼容性 evidence。它不读取 vault、不连接 Ollama、不创建 candidate 或 activation；`m4-preflight.json` 必须显示 `scope=compatibility_only`，通过只证明目标 M4 的编译 Inferrum + sidecar LanceDB 资格，不能当作质量、容量或 first-support 发布证据：

```bash
PINAX_KB_SIDECAR=../inferrum/temp/embedded-venv/bin/inferrum-lancedb-sidecar \
PINAX_KB_INFERRUM_BINARY=../inferrum/temp/bin/inferrum \
task integration:kb-m4-preflight
```

准备一个包含 50–200 份真实或脱敏 Markdown/纯文本的现有 vault，以及 20–50 个带期望 citation 的版本化 suite。将 `PINAX_KB_SIDECAR` 指向已安装的 `inferrum-lancedb-sidecar` 后，使用 runner 创建候选、评测候选并保存脱敏证据：

```bash
PINAX_KB_SIDECAR=../inferrum/temp/embedded-venv/bin/inferrum-lancedb-sidecar \
./temp/bin/pinax-kb-evidence \
  --real-corpus \
  --m4-preflight \
  --vault <vault> \
  --suite .pinax/kb/evaluation-suites/m4-baseline.json \
  --provider ollama \
  --model qwen3-embedding:0.6b \
  --pinax-binary ./temp/bin/pinax \
  --inferrum-binary ../inferrum/temp/bin/inferrum
```

该命令在 `temp/integration-test-runs/<run-id>/` 写入标准 evidence、`artifacts/m4-preflight.json`、`artifacts/provider-benchmark.json` 与 `artifacts/real-corpus.json`。真实语料预检会在读取 vault 前从 sidecar shebang 解析实际 interpreter，并验证已解析 Pinax、Inferrum 两个 Mach-O binary 均为原生 `arm64`；随后以同一 Inferrum binary 对 exact model 执行一次固定的安全 provider benchmark（64 samples、batch 8、warmup 1），再开始 candidate。`provider-benchmark.json` 和 `real-corpus.json.provider_benchmark` 只保留 provider/model、batch、实测维度、warmup/elapsed/P50/P95、items/s、scope 与 not-measured facts；它不读取 vault，不能代替真实 citation/质量或内存压力结论。benchmark 失败或输出不符合合同会在读取 vault 前停止。预检只记录 sidecar Python、macOS 主次版本、M4/M4 Air host 状态、sidecar Python/Pinax/Inferrum binary 的安全架构值，以及 Inferrum embedded-LanceDB 的 backend/dependency/dimension/row/store-state facts，会在实际 LanceDB 验证前拒绝 Rosetta/x86。compatibility-only 预检故意不解析或检查 Pinax binary，也不连接 Ollama 或运行 benchmark。artifact 不保存二进制路径、sidecar 路径、vault 路径、正文、问题、期望答案、向量、token 或 provider payload。`target_m4_status` 保持“观察到 Apple M4”的既有语义；新增的 `target_m4_air_status` 只有在 `arm64` Apple M4 且 `hw.model` 属于当前支持的 M4 Air 型号时才为 `confirmed`，且不会持久化原始 model identifier。`pinax_binary_architecture`、`provider_benchmark` 与已有 M4 架构字段都是可选 additive facts；真实语料成功时同时出现在 `real-corpus.json` 顶层或嵌套投影，旧 consumer 可忽略它们。只有 `m4-preflight.json` 的 `status=passed`、两个 status 都为 `confirmed`、`sidecar_python_architecture=arm64`、`inferrum_binary_architecture=arm64`、`pinax_binary_architecture=arm64`、`provider-benchmark.json.status=passed`，且 `real-corpus.json` 的同名 `m4_preflight` 与 `provider_benchmark` 为 passed，才可将本次结果作为 M4 Air baseline 证据。还必须检查可选 `first_support_gate.status=passed`：它独立要求 Recall@5 `>=0.80`、MRR@10 `>=0.65`、citation coverage `=1.00`、failure count `=0` 和 active generation 未变化。它不会改写通用 `kb evaluate` receipt；若 top-10 的通用评测为 passed、但 top-K citation coverage 不完整，runner 会保留 `real-corpus.json`、写失败 artifact 并非零退出。`unconfirmed` 或 `not_macos` 不能当作 M4 性能证据。

runner 只会写 candidate generation 并执行 `kb evaluate`，不调用 `kb activate` 或 `kb rollback`。它会在运行前后读取 Pinax 的权威 activation descriptor；无论评测通过与否，artifact 都会写 `activation_status=not_attempted`，并记录前后 sequence/active generation 的脱敏身份。两者不同会以 `activation_state_changed` 失败，不能作为可激活的候选证据。`real-corpus.json.resources` 还会记录 `provider_doctor_duration_ms`、`candidate_rebuild_duration_ms` 与 `candidate_evaluation_duration_ms`，它们是实际子命令时长而非未经证明的 cold/warm 标签；现有 consumer 可忽略这些新增字段。比较 `qwen3-embedding:4b` 或 `qwen3-embedding:8b` 时，保持同一 vault、suite、chunk profile 和批处理条件，只替换 `--model`，并先比较 Recall@5、MRR@10、citation coverage、Inferrum synthetic provider benchmark 的 P95、上述 candidate 时长、RSS 与资源压力；不要因单次 provider benchmark 自动激活更大的模型。

也可以使用会先构建两个 CGO-disabled 二进制的任务入口；调用它本身是一次对指定 vault 的 candidate rebuild：

```bash
PINAX_KB_REAL_CORPUS_VAULT=<vault> \
PINAX_KB_REAL_CORPUS_SUITE=.pinax/kb/evaluation-suites/m4-baseline.json \
PINAX_KB_REAL_CORPUS_MODEL=qwen3-embedding:0.6b \
PINAX_KB_SIDECAR=../inferrum/temp/embedded-venv/bin/inferrum-lancedb-sidecar \
PINAX_KB_INFERRUM_BINARY=../inferrum/temp/bin/inferrum \
task integration:kb-real-corpus
```

`task integration:kb-real-corpus` 始终加上 `--m4-preflight`，因此缺少 `PINAX_KB_INFERRUM_BINARY`、Pinax/Inferrum binary 不是原生 `arm64`、预检失败或 host 不符合时会在读取 vault 前失败并保留脱敏 failure artifact。直接调用 runner 但省略 `--m4-preflight` 仍兼容此前的 candidate-only 行为，却不能被当作 M4 baseline、容量或发布验收证据。

只有真实语料评测通过既定质量门、人工检查 receipt/evidence，且操作员明确决定后，才单独运行 `pinax kb activate`。失败候选、M4 identity 未确认、citation coverage 不完整、权限/秘密泄漏或持续内存压力都应保留当前 active generation，并按已有 `kb rollback` 路径恢复。

## 真实服务检查

```bash
cd /home/yeshugen/workplace/yeisme-agent/cli/pinax
OLLAMA_HOST=http://127.0.0.1:11434 \
go run ./cmd/pinax kb provider doctor ollama \
  --model pinax-qwen3-embedding:lowmem \
  --vault <vault> --json
```

`embed_ready=true` 才表示真实 `/api/embed` canary 通过；端口或 `/api/tags` 单独成功不代表 embedding 可用。

将 sidecar 路径配置到本地环境后运行：

```bash
export PINAX_KB_SIDECAR=/path/to/inferrum-lancedb-sidecar
export OLLAMA_HOST=http://127.0.0.1:11434
go run ./cmd/pinax kb rebuild \
  --vault <vault> --backend lancedb \
  --provider ollama --model pinax-qwen3-embedding:lowmem --json
```

当前服务器已验证的最小 sidecar 环境为 `/home/yeshugen/workplace/yeisme-agent/cli/inferrum/temp/local-kb-mvp/venv`（Python 3.12.13、`lancedb==0.33.0`）；sidecar 是按命令启动的子进程，不监听常驻端口。

服务器根盘达到 90% 高水位时，rebuild/refresh 默认在 provider 和 sidecar 之前拒绝，避免在 active 旁边开启无法安全完成的 generation。确认已有足够空间后才显式使用 `--allow-disk-high-water`；当前默认预算是 generation/evidence 2 GiB、模型缓存 4 GiB，后续用真实语料校准。

磁盘预算会统计现有 generation、evaluation/activation evidence；设置 `OLLAMA_MODELS` 后也会统计本地 Ollama 模型缓存。超过任一预算会 fail closed。旧候选必须先 dry-run 评审，再显式确认删除；active 和 previous 永远保留：

```bash
go run ./cmd/pinax kb generations prune \
  --keep 1 --vault <vault> --json
go run ./cmd/pinax kb generations prune \
  --keep 1 --dry-run=false --yes --vault <vault> --json
```

第一个命令只输出删除候选；第二个才会删除非 active/previous 的 generation 及其 evaluation receipts。未知或损坏的 generation manifest 会被保留，不会被 prune 静默删除。

`rebuild` 只写 `.pinax/kb/generations/<generation-id>/` 候选目录，不会隐式替换 active。用评测套件通过 gate 后再显式激活：

```bash
go run ./cmd/pinax kb evaluate \
  --suite .pinax/kb/evaluation-suites/local-canary.json \
  --generation <candidate-id> --vault <vault> --json
go run ./cmd/pinax kb activate \
  --generation <candidate-id> \
  --suite .pinax/kb/evaluation-suites/local-canary.json \
  --run-id <passed-run-id> --vault <vault> --json
go run ./cmd/pinax kb search "要验证的问题" \
  --vault <vault> --backend lancedb --json
go run ./cmd/pinax kb rollback --vault <vault> --json
```

搜索前 Pinax 会从自己的 Markdown 元数据解析个人 vault 的 allowed IDs；解析为空时返回零结果，不调用 sidecar。候选评测不会修改 `activation.json`，激活使用 expected-sequence CAS；返回内容只包含 bounded preview 和相对 source ref，不包含正文、向量或 provider payload。

## 只读评审 API

```bash
go run ./cmd/pinax api serve --no-auth --readonly --port 18090 --vault <vault>
```

`--no-auth` 只允许 loopback；远程 Workbench 通过 SSH tunnel，不开放公网端口：

```bash
# 在本地工作站执行，<server-user>@<server-host> 只替换为你的 SSH 目标
ssh -N -L 18090:127.0.0.1:18090 <server-user>@<server-host>
```

随后 Workbench 只读取 `http://127.0.0.1:18090` 的 GET projection：

```text
GET /v1/kb/review/overview
GET /v1/kb/review/sources?limit=200
GET /v1/kb/review/evaluation-suites
GET /v1/kb/review/evaluation-suites/{suite_id}/questions?limit=50
GET /v1/kb/review/runs/{run_id}
```

当前页面投影明确显示 `not_generated`、`candidate_only`、`fresh`、`stale`、`failed`、`present_unversioned`、`not_checked`、`unavailable`、`not_configured`、`partial`、`no_hits` 和 `insufficient_dataset` 等真实状态；active generation 只证明绑定身份，不会把 provider 显示为已 ready，必须由 provider doctor 另行证明。API 不执行 rebuild/evaluate，也不让浏览器直接读取 vault、`.pinax/**` 或 LanceDB。评测套件的问题摘要可供页面按需加载，run detail 只返回脱敏 immutable receipt。

## 故障恢复与 v1 rollback

恢复只沿着“诊断 → 候选 → 评测 → 激活/回滚”的顺序执行；不要删除 active/previous，也不要为了重试未知运行重复提交 Ollama 计算：

```bash
go run ./cmd/pinax kb doctor --vault <vault> --json
go run ./cmd/pinax kb provider doctor ollama --model pinax-qwen3-embedding:lowmem --vault <vault> --json
go run ./cmd/pinax kb generations prune --keep 1 --vault <vault> --json
go run ./cmd/pinax kb rebuild --backend lancedb --provider ollama --model pinax-qwen3-embedding:lowmem --vault <vault> --json
go run ./cmd/pinax kb evaluate --suite <suite> --generation <candidate-id> --vault <vault> --json
go run ./cmd/pinax kb activate --generation <candidate-id> --suite <suite> --run-id <passed-run-id> --vault <vault> --json
go run ./cmd/pinax kb rollback --expected-sequence <sequence> --vault <vault> --json
```

如果 `doctor` 发现只有 `pinax.kb.sidecar.v1` 历史投影，它会给出 `legacy_v1_readonly`、N/N+1/N+2 期限和 `rebuild_inferrum_v1` 下一步。兼容窗口内只能显式读取：

```bash
go run ./cmd/pinax kb search "迁移验证" --legacy-v1-readonly --vault <vault> --json
go run ./cmd/pinax kb context "迁移验证" --legacy-v1-readonly --vault <vault> --json
```

历史投影读取不启动旧 sidecar、不写旧 projection；要回到当前合同，先运行普通 `kb rebuild` 生成 Inferrum v1 candidate，再用 matching evaluation receipt 激活。不要手工删除历史投影，也不要把可读 JSONL fixture 当成任意旧二进制兼容证据。

## 当前实现边界

- 已切换：Pinax 新 semantic 写入、provider registry 和 sidecar 请求使用 `inferrum.sidecar.v1`。
- 已提供：safe metadata allowlist、稳定唯一 chunk ID、个人 vault fail-closed 权限三态、Ollama embed canary、staging generation、source lineage、disk admission/budget、显式 generation prune、`pinax kb evaluate`、immutable receipt、`kb activate`/`kb rollback` CAS 路径、失败也留证的 `task integration:kb-local`，以及只读 overview/sources/evaluation-suites/questions/run projection。
- `task integration:kb-local` 默认跑合成 3 文档/20 问题 canary；它能证明真实 Ollama→Inferrum v1→candidate evaluation→activation decision 链路，但 artifact 明确标记 `quality_verdict=not_proven_without_real_corpus`。根盘高水位服务器需要显式：`OLLAMA_HOST=http://127.0.0.1:11434 PINAX_KB_SIDECAR=/path/to/inferrum-lancedb-sidecar go run ./internal/testkit/kblocalevidence --allow-disk-high-water`。
- canary artifact 还记录 `cold_rebuild_duration_ms`、`warm_evaluation_duration_ms`、`warm_search_duration_ms`、`max_rss_bytes`、`cpu_user_ms`、`cpu_system_ms`、`ollama_peak_model_bytes`、`ollama_peak_vram_bytes`、`ollama_unload_status` 和 `ollama_models_after`。这些是当前服务器组件资源观察值，不是容量承诺；`measurement_scope=go_run_command_process` 包含 Go 命令进程开销，后续正式部署需用固定二进制再测。
- 尚未宣称完成：真实 50–200 份资料和 20–50 个期望引用的问题集质量结论、PDF/网页/Git/飞书/OCR acquire adapter、legacy v1 完整 N/N+1/N+2 兼容矩阵、Workbench React 页面和浏览器 smoke。当前已能诊断 `legacy_v1_readonly` 并对可读 JSONL 投影执行显式只读搜索，但不把任意旧 Inferrum 二进制投影宣称为已验证兼容。没有真实资料和期望引用前，不报告 Recall/MRR 质量结论。
