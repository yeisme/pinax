## Why

可复用提示词现在分散在笔记正文、聊天记录、Eikona workflow、Auctra 创作 brief 和人工经验里。Pinax 是本地知识 vault 和索引投影的 owner，应该负责把提示词沉淀成可搜索、可引用、可版本演进的 prompt asset，而不是让 Eikona 或 Auctra 直接保存长期提示词资产。

这个 change 建立 Pinax 的 prompt asset vault：保存 `yeisme.prompt_asset.v1` 资产、维护 lifecycle、绑定 note/source refs、提供 `pinax://prompt/<id>` URI 解析、记录 usage feedback，并通过稳定 `--json` / `--agent` 输出供 Auctra、Eikona、Cohors 和脚本消费。

## What Changes

- 新增 prompt asset vault 能力：创建、导入、搜索、展示、更新 lifecycle、解析 `pinax://prompt/<id>`。
- 新增 prompt usage feedback 导入/链接能力：接收 Eikona 的 review usage record，但由 Pinax 自己决定 lifecycle 迁移。
- 新增本地索引投影和 GORM/GORM Gen repository 规划，禁止业务逻辑硬编码 SQL。
- 新增 fixture-first integration evidence，默认不依赖真实 Eikona、Auctra 或网络。
- 保持现有 note vault、index、CLI 输出合同兼容；所有新增字段和命令都是 additive。

## Capabilities

### New Capabilities

- `prompt-asset-vault`: Pinax SHALL persist, search, resolve, and evolve reusable prompt assets as local-first knowledge assets.

### Modified Capabilities

- `agent-safe-proof-loop`: prompt asset output SHALL follow existing bounded/redacted `--json` and `--agent` projection discipline when consumed by agents.

## Compatibility

This change is intended to be additive. It may add new CLI commands, optional JSON fields, GORM tables, and index projection rows. It SHALL NOT rename or remove existing Pinax commands, output fields, database columns, or config keys.

If implementation discovers an existing surface must change, pause coding and update this OpenSpec with migration, deprecation window, consumer update list, and rollback steps before continuing.

### Affected surfaces

- CLI commands: additive `pinax prompt create/import/search/show/resolve/lifecycle/feedback import`; no existing command names or flags are removed or repurposed.
- CLI output: additive prompt-specific `data` and `facts` fields under the existing Pinax JSON envelope; additive `--agent` keys for prompt asset ID, lifecycle, permission, and next action.
- Database and migrations: additive GORM tables for prompt assets, versions, source refs, and usage feedback; no existing table or column is dropped, renamed, narrowed, or made newly required.
- Config and structured assets: no new config keys are required in the first validator slice; later structured assets must be created or modified through Pinax app services and CLI commands.
- Vault schema: additive `yeisme.prompt_asset.v1` import schema; existing `pinax.note.v1` and `pinax.template.v2` documents are unchanged.

### Migration, rollback, and deprecation

- Migration: use expand-only migrations for prompt asset tables. Existing vaults remain valid when the prompt asset feature is unused.
- Rollback: because the current plan is additive, rollback is to disable or revert prompt commands and leave unused prompt asset tables intact until a later cleanup change; no existing note/index data requires rewrite.
- Deprecation: no deprecation window is needed for the current additive scope. If a later implementation needs to rename or remove an existing surface, this change must be updated with a release-length deprecation window before coding continues.

## Impact

- CLI: new `pinax prompt ...` command group and machine output projections.
- Storage: new prompt asset and usage feedback models via GORM/GORM Gen, plus migrations.
- Index/search: prompt assets become searchable by title, domain, tags, lifecycle, source refs, and text snippets.
- Docs: update `docs/` with prompt asset vault usage, cross-project URI boundary, and integration examples.
- Tests: unit, repository, CLI contract, testscript fixture flow, and integration evidence under `temp/integration-test-runs/<run-id>/`.

## Non-Goals

- No hosted prompt marketplace.
- No direct reads from Auctra or Eikona databases.
- No provider execution or image generation.
- No automatic lifecycle promotion without explicit imported feedback and Pinax-owned decision logic.
- No hand-written `.pinax/*.json` or SQLite rows by agents.
