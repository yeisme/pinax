# 设计

## 1. Projection contract

`domain.Projection` 新增可选字段 `interaction`，其结构为：

```json
{
  "schema_version": "pinax.daily_interaction.v1",
  "phase": "capture|retrieve|review|decide|plan|apply|resume|close",
  "state": "ready|partial|blocked|approval_required|unknown",
  "read_only": true,
  "writes": false,
  "remote_effect": false,
  "scope": "project:pinax",
  "source_status": "fresh|stale|partial|missing|not_measured",
  "freshness_status": "fresh|stale|partial|not_measured",
  "recommended_next_action": {"name": "...", "command": "..."},
  "recovery_actions": [{"name": "...", "command": "..."}],
  "warning_codes": ["source_stale"]
}
```

字段是 additive 的；旧 `facts`、`actions`、`data`、`error` 和 command identity 保持不变。`scope` 只来自已有 bounded fact，不从路径或目录名推断。绝对 vault 路径在交互 action 中统一替换为 `<vault>`，避免把本机路径变成跨 runtime 合同。

## 2. Derivation

`internal/interaction` 只消费已生成的 `domain.Projection`，不读取文件、SQLite、registry 或 provider。决策顺序如下：

1. command 映射 phase；
2. `writes`/`local_write` 和 `remote_write`/`remote_writes` 派生安全事实；
3. error code、partial status、warning code、source/freshness facts 派生 state；
4. 从 owner `Actions` 选择一个 recommended action，最多保留三个 recovery actions；
5. 写入 `Projection.Interaction` 及 additive `interaction_*` facts。

特殊衔接：

- `inbox.capture` 推荐 `pinax inbox review --limit 5 --json`。
- `note.search` 优先使用首个 bounded note identity 推荐 `pinax note preview <id> --json`。
- `inbox.review` 优先选择已有 `organize_preview` action。
- unknown operation 优先选择 reconcile action。
- `continue` 复用其已有 `RecommendedNextAction`；checkpoint 进入 `close`。

## 3. Owner Skill

根仓 Skill 只做意图识别和命令编排：

- “记录/收进 inbox”调用 `pinax inbox capture`，确认返回 identity/readback 后再提示 review。
- “找回/阅读”调用 `pinax search` 或已有 `pinax continue`，只渲染 Pinax 返回的 action。
- “整理”先调用 `pinax inbox review`，再按用户选择调用限定 `pinax organize plan --note ...`。
- “应用/确认”保留现有 `--dry-run`、`--yes`、snapshot、revision、receipt 和 operation recovery。
- “暂停/切换 Agent”只调用 `pinax continue checkpoint`；durable candidate 继续进入 proposal/review。

Skill 不创建 store，不读取 `.pinax/**`/SQLite，不扫描所有 vault，不推断 trusted outcome，不把缺失或 stale 事实补成成功。

## 4. Verification and evidence

本地代码使用现有 Go testing、CLI contract tests 和 `tools/testkit/integrationevidence`。覆盖正常链、partial/stale、approval_required、unknown/reconcile、binding/handoff 状态、projection redaction 和多输出模式。所有 integration evidence 写入 `temp/integration-test-runs/<run-id>/`。

真实使用单独记录 capture 成本、review 时长、首次找回、再次使用和两周分母；没有真实分母时保持 `pending`/`unknown`。
