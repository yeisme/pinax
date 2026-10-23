# Agent 安全边界

Pinax 允许 Agent 操作真实 Markdown vault，同时限制明文暴露、静默写入和云端权限。本页描述当前边界，不把规划中的团队能力当作已实现。

<a id="plaintext-boundary-cli-default-does-not-leak-full-note-bodies"></a>
## 默认读取不返回完整正文

所有读取共用 NoteDisplay。card/detail/context 只返回标题、路径、标签、状态、摘要、链接与下一步；只有显式本地 `--display body` 才将正文放入 JSON 投影。

```bash
pinax note read "Research Log" --display card --vault ./my-notes --json
pinax note read "Research Log" --display detail --vault ./my-notes --json
pinax note read "Research Log" --display context --vault ./my-notes --json
pinax note read "Research Log" --display body --vault ./my-notes --json
```

note read/show、项目 board、本地 dashboard、MCP、REST 和 RPC 共用该边界。TestProofLoopJSONProjections 验证有界投影任意嵌套的 body/Body/note_body/raw_body 不含非空正文，也不泄漏正文 sentinel。

<a id="mcp-bounded-context-agents-read-projections-not-raw-files"></a>
## MCP 使用有界上下文

默认 MCP 工具只读并复用 CLI 投影。未指定 display 或请求 body 时降为 card，不返回完整正文，不写 Markdown、.pinax、Git、Provider 或远端状态：

- `pinax.note.links`：出链，支持 broken-only、kind、limit。
- `pinax.note.backlinks`：反链，支持 include-broken、limit。
- `pinax.note.context`：链接与反链构成的有界上下文。
- `pinax.vault.graph_summary`：vault 链接健康摘要。

显式 `--collaboration` 添加独立的 body/write 权限工具、绑定 revision 的 preview 和持久 receipt，不扩大原工具。见 [MCP 协作](../interfaces/mcp-collaboration.md)。调用者声明意图或确认不等于已验证的人类批准。

<a id="answer-synthesis-boundary-answers-need-citations-not-raw-dumps"></a>
## 回答综合必须引用证据

例如“见 Alice 前需要了解什么”应从 memory、search、graph、database views 和 proof receipts 组合有界投影，给出带引用的摘要、未知项与下一步，不倾倒私密笔记正文。旧 KB 命令的当前可用性以 CLI 为准，不因历史描述自动恢复。

```bash
pinax memory context "prepare for Alice meeting" --entity alice --limit 12 --vault ./my-notes --agent
pinax search "Alice" --vault ./my-notes --json
pinax note backlinks "Alice" --vault ./my-notes --json
pinax graph query --kind technique --match storyboard --vault ./my-notes --json
pinax query run 'SELECT title, status FROM notes WHERE status = "active" LIMIT 20' --lazy-index --vault ./my-notes --json
```

`pinax.agent_brain.context_bundle.v1` 保留 task、entities、memory_refs、graph_refs、query_refs、receipts、freshness、body_exposure、next_actions。外部 RAG 投影归集成 owner；Pinax 不装配或保存其原文。bundle 不复制正文、原始 snippet、证据原文、Provider payload、prompt、凭据或私有工具参数。

未来 answer/synthesis 也必须遵守：每项结论有 note/memory/graph/query/receipt 或安全来源引用；过期索引、被替代记忆和未知 Provider 状态明确标注；只引用有界片段；付费/网络调用显式显示来源类型、Provider 和模型；实体合并、矛盾处理、记忆裁剪和正文重写产生计划或回执，不后台静默执行。下一步是可直接运行的命令，例如 `pinax proof loop run --vault ./my-notes --json`。

<a id="teamcompany-kb-permission-boundary"></a>
## 团队知识权限

当前 Agent Brain 是单用户本地模式。团队/企业 KB、HTTP MCP、OAuth、组织 ACL 和限流属于未来 owner 表面，不是当前 CLI backend 已交付能力。

未来跨用户投影必须携带 principal、workspace、source_acl、visibility、redaction_policy、audit_ref。缺少 ACL 证明或 owner 未配置时，返回 permission_unknown、scope_required、insufficient_scope 或本地-only 边界，不把私人笔记拼成团队知识。S3/rclone 等直接同步传输不提供组织授权证明。

<a id="cloud-no-exec--no-plaintext-invariant"></a>
## 云端不保存明文、不执行本地工具

Cloud Sync 是分布式同步协调者，不是托管 vault。客户端上传前加密正文；capsa 只协调密文 blob、manifest、revision 和冲突元数据，不执行本地工具、Provider 调用或 CLI。

`pinax.cloud.envelope.v1` 包含 alg=AES-256-GCM、key_id（PBKDF2，100000 次迭代）、每 blob 随机 nonce、Base64 ciphertext、完整性摘要 plain_sha256。缺失 ciphertext/key_id/nonce/plain_sha256 或 metadata 含明文 token 时拒绝。manifest/revision metadata 也使用加密 envelope，服务器只见密文与摘要。

只有 transport 完成持久 revision 提交（服务端 CAS 或持久对象写入），并写入本地 sync-state 证据后，remote_write 才能为 true。dry-run、计划、单独 blob 上传、失败、不支持的 transport 或 pull 始终保持 false。

<a id="proof-loop-write-control-chain"></a>
## 写入保护链

```mermaid
flowchart LR
  Plan[可审阅计划] --> Snapshot[版本快照]
  Snapshot --> Apply[批准的低风险操作]
  Apply --> Receipt[审计回执]
  Receipt --> Restore[需要时生成恢复计划]
```

1. `pinax repair plan --save` / `pinax organize plan --save` 在 .pinax 保存计划；未 apply 前只作为证据。
2. `pinax version snapshot` 生成保护快照；apply 可要求快照，或通过 snapshot-message 自动创建。
3. repair/organize apply --yes 只执行批准的低风险 metadata、标签、索引重建、归档状态和移动。重名、断链、歧义、正文重写与删除只进入人工审阅，不自动应用。
4. apply 写持久 receipt，方便追踪与审阅。
5. `pinax version restore <path> --revision <rev> --plan` 先生成恢复计划，再以 `pinax version restore apply --plan <id> --yes` 经 CLI 服务恢复，保持 local_write=true、remote_write=false。

`pinax proof loop run` 编排这条链：preview 只读，--apply --yes 创建新快照并应用批准的操作。

<a id="related"></a>
## 相关说明

- [产品定位](product-positioning.md)
- [CLI 输出合同](../interfaces/cli-output-contract.md)
- [同步架构](../architecture/cloud-sync-design.md)
- [架构边界](../architecture/architecture-boundaries.md)
