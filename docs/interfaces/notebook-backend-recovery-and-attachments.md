# Notebook 后端恢复与附件合同

本文对应 `pinax-drivebridge-workbench-v1` 的 P03/P04。代码 owner 为 Pinax；DriveBridge 的生成合同、宿主 admission、renderer 交互和 Electron 验收仍由消费者实现。本轮在既有脏工作树中增量实现，使用临时 vault、文件型远端和临时端口验证，不访问主力 vault 或真实云端。

## 同步 operation

公开的 `sync.push`、`sync.pull`、`sync.conflicts.resolve` 进入同一 GORM operation ledger，并由 vault operation lock 串行化。REST 接受 `X-Pinax-Operation-ID` 和 `Idempotency-Key`；RPC 接受 `operation_id` 和 `idempotency_key`。两者必须成对提供。Go SDK 的 `SyncRequest.Identity`、`SyncConflictRequest.Identity` 使用 `MutationIdentity`，调用方在首次发送前保留它。

同一 identity、参数和授权范围只执行一次；参数或身份绑定变化返回 `idempotency_conflict`。旧客户端没有提供 identity 时由 owner 创建并返回 operation ID，保留旧调用兼容性；这类客户端在整个响应丢失后没有预先保留的查询键，因此新消费者必须显式提供 identity。CLI 的 sync 命令保持本地执行；远端消费者通过 REST/RPC 或带 Identity 的 Go SDK 调用，不改变 CLI 的本地／远端路由规则。

run receipt 写入 operation ID，并使用确定的 run ID。进程在 domain commit 后、ledger 收口前退出时，reconcile 只检查原 run receipt 或冲突解决 receipt。证据不足继续保持 `reconcile_required`，不重放同步。`remote_write` 沿用已有 durable commit/read-back 事实；不会因本地 ledger 成功而升级。组合 `sync all` 和后台调度仍沿用自己的 run 合同，本次公开 mutation 合同不新增后台调度能力。

恢复入口：

```bash
pinax operation show <operation-id> --vault <vault> --json
pinax operation reconcile <operation-id> --vault <vault> --json
```

缺少 write admission 或确认时，不创建 operation。dry-run 不创建 operation ledger。同步结果仅持久化有界事实，不保存正文、凭据或 Provider payload。

## 附件采用

沿用显式启用的 `pinax.input.prepare` → 临时 HTTP transfer → complete 暂存协议。没有安装 Pinax CLI 的 MCP 客户端通过 `tools/list` 和 `pinax://input/capabilities` 获取 schema 与上传说明。启用输入监听仍由 owner 在 Pinax 所在主机执行既有 `pinax mcp serve --input-listen ... --input-base-url ...`；普通只读连接不自动获得输入能力。

新增工具：

| 工具 | 输入与结果 |
| --- | --- |
| `pinax.input.attachment.preview` | `input_request_id`、`operation_id`、`object_id`、完整源码 `revision`、`placement`；返回精确 proposal、摘要与新引用。 |
| `pinax.input.attachment.apply` | 原样 `proposal` 和 `confirm=true`；返回 operation receipt。 |
| `pinax.input.attachment.status` | 原 `operation_id`；返回持久化状态，不依赖上传文件仍存在。 |
| `pinax.input.attachment.reconcile` | 原 `operation_id`；核验源码、附件摘要和原 record event，不重新采用。 |

公开 Go 类型和闭合 JSON Schema 由 `github.com/yeisme/pinax/pkg/inputcontract` 导出。MCP 工具直接复用这些 schema。本轮不新增附件 REST/RPC 端点，DriveBridge 后续消费可复用此既有 MCP input surface；不得把它表述为 Electron 或 HTTP writer 已接通。

`placement` 支持 `per-note`、`vault-folder`、`note-folder`。不提供 `replace_reference` 时新增引用；提供时必须精确匹配目标笔记中的唯一已有附件引用。替换会生成新的不可覆盖附件文件，原子替换原 Markdown 引用，保留旧附件字节供其它引用和恢复使用。客户端从不提交绝对路径或服务器源文件路径。

固定顺序为：验证原 input actor/project → 校验字节与元数据 → 固定 note identity/full-source revision → 比较精确 proposal → operation claim → snapshot → 独占发布新附件 → 再次检查源码 → 原子更新源码 → record/index → 文件与摘要 read-back → operation receipt。进程异常可能留下未被引用的 staging/blob；不会盲目重新采用或删除共享附件。receipt 不保存正文或上传内容。

旧 `input.preview/apply` 的附件分支复用同一服务；历史 adoption receipt 保持有效。MCP 和 typed 服务使用同一 input 幂等绑定，不能通过切换入口重复采用。

## 验证矩阵

| 场景 | 预期 |
| --- | --- |
| plain/Markdown、PNG/JPEG/WebP、PDF | MIME 与实际字节相符，SHA-256 与暂存 receipt 一致。 |
| 2 MiB / 超过 2 MiB | 上界可预览，超界拒绝；文本须为无 NUL 的有效 UTF-8。 |
| 三种 placement；新增和替换 | 新引用可读回；替换保留原附件字节。 |
| stale body/metadata revision、重复 note ID、未知 object ID | 在创建 mutation receipt／snapshot 前拒绝。 |
| 非法 MIME/摘要/placement、路径字段、symlink、不唯一替换引用 | 拒绝；笔记、引用和 revision 不变。 |
| 未确认、proposal 篡改、同 identity 重复调用 | 未确认／篡改拒绝；重复调用返回同一回执。 |
| 错误 actor/project、暂存文件不可用 | 无权者不能读取或采用；已完成 operation 可独立查询。 |
| sync 响应丢失、domain receipt 后进程退出 | 原 identity 恢复；响应丢失代理只转发一次 mutation，重启后回执仍可读。 |

运行本地、无付费／真实云写入的证据入口：

```bash
go run ./tools/testkit/integrationevidence --profile notebook-backend
go run ./tools/testkit/integrationevidence --profile notebook-backend-final
```

后者执行项目 `task check`。两者保存 summary、command、stdout、stderr、env 和 artifacts；均不执行 race。真实 Cloud Sync endpoint、跨设备、真实 Obsidian vault、长期 dogfood、Electron 三平台与签名证据继续 pending。
