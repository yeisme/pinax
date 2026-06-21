## Why

Upma 提示词库验证了 Pinax 可以承载高价值内容资产，但流程还依赖临时脚本把采集结果拆成 Markdown、prompt asset 和 Eikona bundle。Pinax 需要一个可复用的内容库生产线入口，让下一个内容源能通过稳定 bundle 契约完成导入、质检、导出和图谱化。

## What Changes

- 新增 `pinax.content_bundle.v1` 导入契约。
- 新增 `pinax collection import/diff/doctor/export`，把内容 bundle 资产化为 vault notes、prompt assets、receipt 和 Eikona prompt bundle。
- 新增 `pinax graph rebuild/query` 的 prompt graph v1，本地可重建投影覆盖 source/category/technique/style/subject 关系。
- 维持 Pinax 边界：不做网页采集、不执行 provider、不把 graph 当真源。

## Compatibility

This change is additive. It adds CLI commands, a bundle input schema, docs, and a rebuildable `.pinax/graph/prompt_graph.json` asset. It does not remove or rename existing commands, flags, JSON envelope fields, database columns, or config keys.

Rollback is to stop using the new commands and delete generated collection notes, prompt assets, receipts, or graph projections through normal Pinax maintenance workflows. Existing vault notes and prompt assets remain readable.
