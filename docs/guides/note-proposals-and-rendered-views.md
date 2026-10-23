# 笔记提案、渲染预览与稳定引用

这些能力保持 **experimental**，由本机 vault owner 的 CLI 和 MCP 消费同一应用服务。既有 `note preview`、`note show --view rendered`、memory proposal 与远端 RPC 合同保留；新增渲染合同不会通过旧 RPC 静默降级执行。

## 提出与审阅

```bash
pinax agent note propose --kind note_draft --title "Research draft" --stdin --vault ./my-notes --json < draft.md
pinax agent note proposal list --vault ./my-notes --agent
pinax agent note proposal describe <proposal-id> --vault ./my-notes --json
pinax agent note proposal approve <proposal-id> --vault ./my-notes --json
pinax agent note proposal approve <proposal-id> --yes --vault ./my-notes --json
pinax agent note proposal reject <proposal-id> --yes --vault ./my-notes --json
pinax agent note proposal stats --vault ./my-notes --json
```

`inbox_capture`、`journal_append`、`note_draft` 是硬编码低风险白名单，校验通过后经快照、应用、回读与 receipt 自动保存。新 inbox/draft 使用 proposal 派生名字避免覆盖；draft 保留 draft 生命周期。`journal_append` 支持 `--date YYYY-MM-DD`，既有 journal 目标必须是 daily note；不是任意正文追加入口。

编辑使用稳定 object ID 和当前文件的 SHA-256 revision。先读取引用获取 revision，再提出正文替换；不能借 payload 修改路径、身份或手写 frontmatter。

```bash
pinax note ref <object-id> --vault ./my-notes --json
pinax agent note propose --kind note_edit --target <object-id> --expected-revision <sha256-revision> --rationale "Update the conclusion" --stdin --vault ./my-notes --json < replacement.md
```

编辑等待审阅；approve/reject 不加 `--yes` 只预览。批准前再次验证 revision，漂移返回 `plan_stale`，应重读卡片再提出新提案。列表、详情、receipt 和计数不包含 payload 全文；receipt 的 diff 仅为行数与 hunk 数。organize/repair 不进入自动应用白名单。

默认读取使用服务维护的脱敏文件投影，不打开 SQLite、不创建 WAL/SHM。发生失败或超时，先查询原 proposal ID；`applying`、`reconcile_required` 和已消费状态不能重新应用。若文件投影丢失，可由 owner 显式刷新原 ID 的元数据：

```bash
pinax agent note proposal refresh <proposal-id> --yes --vault ./my-notes --json
pinax operation describe <proposal-id> --vault ./my-notes --json
```

刷新只从受控 store 重建脱敏投影，不写笔记、不重试业务操作。主操作的原 operation、snapshot 和 receipt 应一起核对；无证据时不改写状态为成功。

## 提案队列续阅（experimental）

```bash
pinax agent note proposal list --status approval_required --limit 5 --vault ./my-notes --json
pinax agent note proposal list --status approval_required --limit 5 --cursor <cursor> --vault ./my-notes --json
pinax agent note proposal list --kind note_edit --vault ./my-notes --agent
```

`status`、`kind` 精确筛选；limit 为 1–50，省略参数仍返回最新 50 条及原 data 数组。排序为 created_at 降序、proposal_id 升序。新增 total/remaining/has_more/next_cursor，超过 50 条的旧待审项可继续找到。游标最大 2 KiB，绑定 vault、筛选、页大小和匹配集；`proposal_cursor_invalid` 表示参数不匹配，`proposal_cursor_stale` 表示匹配集已变。批准/拒绝后刷新第一页，不持久保存游标作为审阅进度。

返回的 CLI 动作包含原 vault，换工作目录也不会误选另一库。show 只读原 ID 投影，不受无关坏文件阻塞；列表遇到损坏投影仍明确报错，不静默丢项。所有读取不打开 SQLite、不创建 WAL/SHM、不返回 payload 或正文 diff。

MCP `pinax.agent.note_proposals` 使用相同 status/kind/limit/cursor 参数；proposal_id 与分页参数互斥。客户端可直接执行返回的 `tool_actions`，不需要安装 CLI。`owner_action.execution_location=vault_owner` 表示审批/刷新必须在 vault owner 机器完成，不是客户端文件访问权限。MCP 没有审批工具，写 proposal 仍默认关闭。

## 渲染与引用

```bash
pinax note preview <object-id> --view rendered --vault ./my-notes
pinax note preview <object-id> --view rendered --display context --json --vault ./my-notes
pinax note preview <object-id> --view rendered --display body --format html --vault ./my-notes > preview.html
pinax note ref <object-id> --copy --vault ./my-notes
pinax note ref "Same title" --pick 1 --vault ./my-notes --json
pinax note ref <object-id> --floating --vault ./my-notes --json
pinax note ref resolve '<pinax-uri>' --resolved-revision <first-resolved-revision> --vault ./my-notes --json
```

只有显式 `--view rendered` 才启用新树合同；旧 preview 默认行为不变。默认 card 保留骨架与 fence 摘要，context 给有界正文片段，body 是显式完整深度；树源内容仍以 64 KiB、512 个节点为边界，截断必须显示。`--format json` 使用标准 CLI envelope；HTML 仅导出转义后的文本树，不能与其他机器模式混用。

固定 `pinax://note/<id>?v=<sha256-revision>` 精确读取当前内容或既有快照中的该内容，不可达显示 unresolved。floating 首次返回 `resolved_revision`；调用方后续传回原值，输出分别报告原 revision、current revision 和 stale，解析不会持久记忆或静默换版。只读解析不会为缺失历史创建快照。版本徽标取自既有 record version；无 ledger 记录时 0 表示未知，不能据此宣称版本已验收。相关图、看板和反链来自当前 vault，其来源在 `relations_source` 中明确。

复制输出 Markdown 与带 URI 的 wikilink 两种格式，均绑定同一对象及 revision。剪贴板不可用时显示双格式并标记 unavailable；标题多命中先输出最多 20 个候选。编辑器的 URI handler 属于后续工作，本次不宣称所有编辑器可直接导航版本化 URI。

支持 `pinax-database-view`、`pinax-graph hops=1|2`、`pinax-board <project>/<subproject>`、`pinax-timeline`。database table ≤20 行、graph ≤50 节点/100 边、backlinks ≤50、timeline ≤100。失败 fence 明确显示错误码和下一步；未知 fence 在正文深度内保留为代码。附件仅解析 manifest 中的 `pinax-asset://` 与 mime；`drivebridge://file/` 仅为状态引用，不读取外部字节或发起网络请求。

## MCP

```bash
pinax mcp serve --vault ./my-notes
pinax mcp serve --allow-note-proposals --vault ./my-notes
pinax mcp serve --collaboration --allow-note-body --vault ./my-notes
```

默认可发现只读 `pinax.agent.note_proposals` 与 `pinax.render_note`；两个 propose 工具默认不注册，直接调用也拒绝。owner 显式开启 proposal capability 后，draft 可以自动应用，edit 仍需本地人工批准；没有 MCP 自我批准工具，也不接受调用方提供 principal/权限字段。render 工具默认 card；body 需 owner 正文权限与显式 display，读取深度记录在结果 facts 中，不产生持久审计写入。

## 本地验证与真实门禁

```bash
go run ./tools/testkit/integrationevidence --profile agent-note-proposal
go run ./tools/testkit/integrationevidence --profile rendered-view-parity
task check
openspec validate --all
```

测试只使用隔离 vault、testscript 和本地 MCP/SDK，无模型调用或远端写入。证据进入 `temp/integration-test-runs/`。工程通过不替代两周日常使用、四周 memory runtime、30 个真实 continuity loops、真实判断案例或 Mac 同步验收；这些 change 的外部门保持未完成。不运行 race。
