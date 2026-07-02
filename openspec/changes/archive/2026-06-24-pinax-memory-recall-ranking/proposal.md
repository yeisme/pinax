# Pinax Memory Recall Ranking

## 为什么

Pinax 已经有 `pinax memory` 非向量 ledger：它能 capture/list/recall/context，能记录事实、决策、事件、任务和 source citation，也能通过 FTS、entity、type、status 做基础召回。当前 ranking 仍偏简单：多个候选只按少量信号加分，缺少 source authority、confidence、freshness、supersession、diversity 和可解释信号拆分。随着用户把更多项目事实、决策和 release 证据写入 memory，简单排序会让旧决策、低可信来源或重复记录挤掉真正应该进入 agent context 的记录。

这个变更把 recall 排名升级为可解释的 deterministic scorer pipeline。它保持 memory 的定位：不依赖 embeddings，不使用 LanceDB，不和 KB semantic search 混合；memory 只负责短、准、可引用、可审计的 agent context。

## 做什么

1. 在 `internal/memory` 中抽出 recall candidate selection、signal extraction、weighted scoring、dedupe/diversity 和 explanation 生成。
2. 扩展 recall 信号：FTS/字段命中、entity/type/status、source authority、confidence、freshness、supersession、task fitness。
3. 在 `--json data.matches[]` 中新增可选 `signals` breakdown，不改变现有 `score` 和 `recall_reason` 字段。
4. 在 `--agent` 中只新增低 token `fact.memory.*` key，不输出完整 body。
5. 为 ranking 增加行为测试、CLI contract 测试和 integration evidence redaction 测试。

## 不做什么

- 不引入 vector recall、LanceDB、embedding provider 或 semantic reranker。
- 不把 memory ledger 当作 Cloud Sync 权威数据。
- 不默认召回 `draft`、`superseded`、`expired`、`rejected`。
- 不删除或重命名现有 memory CLI 命令、JSON envelope、agent key 或 SQLite 表字段。
- 不实现 LLM 自动确认 memory；自动提取仍应先进入 draft 或明确 plan。

## 用户结果

用户运行同样的命令，会得到更稳定、更可解释的排序：

```bash
pinax memory recall "release workflow" --entity pinax --vault ./my-notes --json
pinax memory context "prepare next release" --entity pinax --limit 12 --vault ./my-notes --agent
```

JSON 中每条 match 会继续包含 `score` 和 `recall_reason`，并新增可选 `signals`：

```json
{
  "score": 87,
  "recall_reason": "status:confirmed + entity_match:pinax + field:predicate + source:openspec + confidence:high",
  "signals": {
    "keyword": 24,
    "entity": 30,
    "source": 12,
    "confidence": 10,
    "freshness": 4,
    "lifecycle": 7
  }
}
```

## 成功标准

- `openspec validate pinax-memory-recall-ranking --strict` 通过。
- `openspec validate --all --strict` 通过。
- `go test ./internal/memory ./internal/app -run 'Memory|Recall|Ranking' -count=1` 通过。
- `go test ./cmd/pinax -run 'TestMemory' -count=1` 通过。
- `task test:integration` 通过，并生成 redacted evidence。
- `task check` 通过。

## 合同和兼容性

- CLI commands: 不新增必需命令，不删除现有 `memory capture/list/recall/context/stats/link/prune`。
- CLI output: 保留现有 `score`、`recall_reason`、`data.matches[]`；只新增可选 `signals` 和可选 `fact.memory.*` key。
- Database: 只允许新增 nullable columns、new tables 或 indexes；不 drop/rename/narrow 现有表字段。
- Config: 如果需要权重配置，只能新增可选 `memory.recall.*`，默认值必须与代码内置一致。
- Rollback: 可关闭新 scorer，回退到现有 recall 分支；ledger SQLite 和 Markdown 真源不需要迁移。
