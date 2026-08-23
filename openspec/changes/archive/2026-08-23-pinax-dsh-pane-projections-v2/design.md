# 设计

## 数据来源（全部进程内投影，避免二次扫描）

| 面 | 来源投影/服务 | 有界化 |
| --- | --- | --- |
| Backlinks | note links/backlinks 投影（`QueryBacklinks`） | 每笔记 ≤50 条，仅 ref/title/kind/status |
| Graph 摘要 | 链接图投影（orphans/graph 查询共用 `noteGraphNoteSummary`） | 计数 + top-k(k≤20) 度数，仅 ref+度数 |
| History | record ledger 投影（`records` 事件） | ≤100 条修订事件，仅 op/object_id/revision_id/时间 |

三者输入均为既有 domain 投影，组装器为纯函数（与 `AssemblePaneSnapshot` 同构），不落盘、不访问网络。

## 红线复用

- ref 一律过 `paneUnsafe`（白名单+黑名单）；不安全项 backlinks 跳过、graph top-k 剔除、history 事件丢弃，并在各自投影中暴露 `dropped_unsafe` 计数（有界，供 Host 观测而不是静默丢失）。
- 递归序列化扫描断言：无 `/` 开头路径、无 `token/authorization/cookie/secret/password/api_key/bearer` 子串、无 note body 字段。
- `payload.timeline` 从占位 `[]any{}` 变为 history 面的事件数组（≤100，超出标记 `truncated:true`）。

## envelope 复用

三个面共享 `newPaneSnapshotEnvelope`；`status` 映射与 note list 面一致（`failed/error/offline→offline`、`permission_denied` 保持），`freshness` 快照阶段 `fresh`。graph/backlinks 的空结果（无链接笔记）是合法 `ready` 状态，不映射 offline。

## 验证

- 单元：三组装器的有界性（>50 backlinks 截断、top-k、>100 events 截断）、红线递归扫描、空输入合法 ready。
- 证据：`dsh-pane` profile 扩展后经 `integrationevidence` 运行归档。
- 交叉核对：与 harness `dsh-pinax-pane-v1` 的 Host 消费面比对（该 change 实现落地时复核一次，作为归档门）。
