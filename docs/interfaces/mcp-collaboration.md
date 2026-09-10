# MCP 人机笔记协作

本机单用户的增量交互面：结构化结果供 Agent 使用，Markdown 供人阅读，通过对话完成选择、修改审阅与保存。所有写入仍通过 Pinax application service。

## 启用位置与权限

命令在拥有 vault 的 MCP host 上运行，`./my-notes` 是该机器的目录：

```bash
# 维持旧只读接口
pinax mcp serve --vault ./my-notes

# 增强查询，仍不提供正文和写入
pinax mcp serve --vault ./my-notes --collaboration

# 指定正文读取及日常笔记闭环
pinax mcp serve --vault ./my-notes --collaboration --allow-note-body --allow-note-write
```

写入权限要求同时启用正文读取，以支持真实审阅。服务端 flags 才是权限来源，工具参数不能打开权限。新增开关不改变旧 `pinax.note.read` 的 bounded 行为。撤销开关不删除已经保存的笔记、快照或操作账本。

Codex 可注册本机服务器（将 vault 替换为 host 上的真实绝对路径）：

```bash
codex mcp add pinax -- pinax mcp serve --vault /path/to/my-notes --collaboration --allow-note-body --allow-note-write
```

这是接入示例，不代表已经在用户的 Codex 配置中注册。连接后 Agent 不需要在客户端再安装 Pinax、读取本地 vault 或运行 CLI doctor。

## Discovery 与工具

读取 `pinax://interaction/capabilities`，再以 `tools/list` 返回的完整 inputSchema 为准。工具名前缀在本机直连中如下；Gateway 改名前缀后必须重新发现。

| 工具 | 行为 |
| --- | --- |
| `pinax.interaction.search` | 摘要检索；query、offset、limit。单页最多 20 条，总浏览窗口 1000 条，结果明确报告截断。 |
| `pinax.interaction.read` | 默认 card；`display=body` 要求 owner 正文许可及指定 `intent=read|summarize|edit`。 |
| `pinax.interaction.preview` | 接受 change，返回 before/after、规范化 change、operation_id 和 preview_digest，不写正式笔记或操作账本。 |
| `pinax.interaction.apply` | 保存原样 change 与 preview_digest；authorization 为调用方声明，不是验证过的人工证明。 |
| `pinax.interaction.status` | 通过原 operation_id 查询持久结果；即使写入关闭，仍可查询已有操作。 |
| `pinax.interaction.version` | 通过 operation_id 读取对应保存前快照中的笔记；需要正文许可，不接受任意文件路径。 |

change 支持 `create|append|replace|tags|archive`。create 默认进入统一 intake 目录 `index`，允许明确指定 dir；target_path 由 preview 规范化并绑定，不能悄悄改变保存位置。已有笔记操作绑定 canonical note_ref 和 expected_revision；tags 使用 add/remove/set。预览和 apply 必须保留返回的完整 change，不自行重组。

正文以 UTF-8 文本传递，交互限额为 64 KiB；append 后总正文也受限。超限显式报错，不能截断后冒充完整保存。读取当前全文意味着正文对 Agent 可见。

## 对话流程与授权

- 搜索：返回摘要、标题、路径和分页信息，用户在对话中指定笔记。
- 阅读：默认摘要；明确要求阅读、总结或修改指定笔记时，通过工具获取正文。
- 差异：以 Markdown Before/After 和结构化提案审阅，用户接受后保存；取消即丢弃尚未提交的提案。
- 回执：返回保存状态、operation_id、版本及恢复引用，继续通过工具查询或读取保存前版本。

已明确授权目标与内容的指令使用 `authorization=explicit_instruction`。Agent 自拟修改先预览等待采纳，之后使用 `reviewed_proposal`；用户明确要求直接保存则按明确指令处理。两者都记录为 `caller_assertion`，不写成 `human_verified`。宿主自己的审批依然生效。

Markdown 路径返回相同语义结果，正文放入安全长度的 fenced text，避免宿主把笔记 HTML、图片或代码围栏当作主动内容。模型需要准确转述工具结果及确认状态，不应把未保存预览称为已完成。

## 资源

旧 JSON 资源保持原义。新增：

- `pinax://interaction/capabilities`：实例实际能力及输入入口。
- `pinax://interaction/note/{note_ref}`：Markdown card，参数按单个 URI path segment 编码；不通过资源绕过正文授权。
- `pinax://interaction/operation/{operation_id}`：已确认回执的 Markdown；未确认状态用 status 工具读取结构化恢复信息。

输入复用已有 `pinax.input.*` 与 `pinax://input/capabilities`。需要在 owner 显式配置受限 HTTP 输入服务；仅开启 collaboration 不会虚构上传能力。上传完成只表示 staged，之后仍需 preview/apply，不能把客户端路径当作服务端已有文件。参见 [MCP 输入](../mcp-input-intake.md)。

## 冲突与恢复

预览摘要绑定 vault、操作 ID、修改内容、目标和基础版本。保存前重读文件；新建目标被占用或原笔记变更时返回 revision_conflict，必须重新预览。不同内容复用旧操作 ID 返回 idempotency_conflict。

协作写入与现有同步操作共用 owner operation lock；成功回执写入现有 GORM operation ledger。修改已有笔记前创建版本快照，并在写笔记前持久化快照引用。重复提交已完成操作返回原结果。进程崩溃或结果未知时返回 applying/reconcile_required，禁止更换操作 ID 自动重试；status、version 和已有 owner 版本恢复流程用于调查与恢复。不能因为磁盘上的正文看起来相同就猜测整个操作成功。

协作路径拒绝 vault 内的符号链接。普通外部编辑器不遵守 Pinax 锁，最终版本检查与文件替换之间仍有文件系统竞态窗口；这不是任意编辑器之间的严格线性一致合同。断开客户端不会丢失已经持久化的操作结果，但未保存的提案只在当前会话中，不宣称跨会话持久草稿。

## 验证与当前成熟度

```bash
go run ./tools/testkit/integrationevidence --profile mcp-collaboration
go run ./tools/testkit/mcpevidence
```

所有 profile 写入 `temp/integration-test-runs/<run-id>/`，包括失败日志。协议测试使用合成 vault，验证工具契约和真实 stdio 行为；实际客户端对话仍需单独验收，当前为 `exploratory`。

2026-09-10 用户取消了 HTML/MCP Apps；该部分资源、实现及浏览器测试已移除。旧客户端应重连刷新资源目录，缓存的 UI URI 返回 resource_not_found。无需修改现有普通 MCP 连接参数。
