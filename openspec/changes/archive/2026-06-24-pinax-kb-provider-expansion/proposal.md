# Pinax KB Provider Expansion

## 为什么

Pinax 现有 `pinax kb` 已经证明了本地 Markdown vault 到 LanceDB projection 的闭环：Markdown 是真源，`.pinax/kb/lancedb/` 是本地可重建投影，Go CLI 通过 `pinax-lancedb-sidecar` 访问 Python LanceDB。当前问题是 provider 和 backend 逻辑仍集中在 `internal/semantic/semantic.go`，新增 embedding provider 会继续扩大单文件分支判断，也容易把 LanceDB sidecar、embedding provider、CLI 输出和凭据处理混在一起。

这个变更把 KB 扩展点收束成稳定、可测试、可回滚的 provider/backend 注册结构，让 Pinax 能安全增加 `openai` 和 `ollama` 这类 embedding provider，同时保持 LanceDB 为本地 sidecar projection，不把 provider token、raw payload 或完整 note body 泄漏到输出、sidecar、fixture 或 integration evidence。

## 做什么

1. 在 `internal/semantic` 内拆出 embedding provider registry 和 vector backend registry，保留 `gemini`、`fake`、`lancedb`、`fake` 的既有行为。
2. 新增 `openai` embedding provider，使用 `OPENAI_API_KEY` 或用户级配置引用，不把凭据写入项目资产。
3. 新增 `ollama` embedding provider，默认连接本地 `http://127.0.0.1:11434`，用于无云凭据的本地 KB 验证。
4. 为 provider 增加 `doctor` 和 `list` 可观测面，输出 provider/model/dimension/configured 状态，不输出 token 或 raw provider payload。
5. 保持 LanceDB sidecar 协议 `pinax.kb.sidecar.v1` 兼容，只添加可选 metadata 字段，不要求旧 sidecar 破坏性升级。
6. 更新 KB docs、CLI 合同测试、sidecar protocol 测试和 integration evidence 验证。

## 不做什么

- 不改变默认 provider：`--provider gemini` 继续是默认。
- 不改变默认 backend：`--backend lancedb` 继续是默认。
- 不把 `.pinax/kb/lancedb/` 纳入 Cloud Sync 权威数据。
- 不把 provider token 写入 `.pinax/config.yaml`、OpenSpec、fixture、stdout、stderr、events、receipts 或 evidence。
- 不在 Go CLI 中引入 Python/LanceDB native dependency；LanceDB 仍由 sidecar 拥有。
- 不做 Memory ledger 的 recall 排名，这由 `pinax-memory-recall-ranking` 负责。

## 用户结果

用户可以在同一个 KB 命令族下选择不同 embedding provider：

```bash
pinax kb provider list --vault ./my-notes --json
pinax kb provider doctor --provider openai --model text-embedding-3-small --vault ./my-notes --json
pinax kb rebuild --backend lancedb --provider openai --model text-embedding-3-small --vault ./my-notes --json
pinax kb rebuild --backend lancedb --provider ollama --model nomic-embed-text --vault ./my-notes --json
pinax kb search "release workflow" --vault ./my-notes --agent
```

## 成功标准

- `openspec validate pinax-kb-provider-expansion --strict` 通过。
- `openspec validate --all --strict` 通过。
- `go test ./internal/semantic ./internal/app ./cmd/pinax -run 'KB|Provider|Semantic' -count=1` 通过。
- `task kb:sidecar:protocol` 通过，证明 sidecar protocol additive 兼容。
- `task test:integration` 通过，并在 `temp/integration-test-runs/<run-id>/` 写入 redacted evidence。
- `task check` 通过。

## 合同和兼容性

- CLI commands: 新增 `pinax kb provider list` 和 `pinax kb provider doctor`，additive。
- CLI flags: `--provider` 新增可选值 `openai`、`ollama`，不移除 `gemini`、`fake`。
- CLI output: 只新增 `facts` 或 `data` 下可选字段；不删除或重命名 envelope 顶层字段、现有 `fact.*` key 或 command name。
- Config: 只新增 `kb.provider.default`、`kb.providers.<id>.*` 这类可选键；不改名 `kb.sidecar.*`。
- Sidecar protocol: `pinax.kb.sidecar.v1` 保持版本，新增字段为可选 metadata；旧字段语义不变。
- Rollback: 可隐藏新增 provider 命令和 provider id；已生成的 LanceDB projection 是本地可删除重建 artifact，不影响 Markdown 真源。
