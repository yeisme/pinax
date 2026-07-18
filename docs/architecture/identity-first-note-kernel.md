# Identity-first 笔记内核

Pinax 的核心不是文件浏览器，而是以稳定对象身份驱动的本地笔记内核。Markdown 文件仍是真源，但 `path` 只表示当前定位，不能代表对象身份。

## 四层合同

| 层 | 稳定字段 | 作用 |
| --- | --- | --- |
| 对象身份 | `object_id`（canonical UUIDv7） | 跨 rename、move、设备和恢复保持不变 |
| 内容版本 | `content_revision` / manifest `revision_id` | 判断内容是否变化、是否能安全 rebase |
| 当前定位 | `current_path` / manifest `path` | 找到当前文件；允许变化，不参与身份生成 |
| 审计状态 | ledger sequence、snapshot、apply receipt | 解释谁改了什么、是否可恢复、是否可同步 |

新建 note、journal、asset、project、subproject 和 managed task 时分配 canonical UUIDv7。旧 `note_*`、`asset_*` 等字段继续作为兼容 alias，但不能成为新对象的 canonical identity。普通同步文件通过 CLI-authored `.pinax/cloud/file-identities.json` 获得 UUID；禁止从 path、title 或 content hash 推导 canonical ID。

## Agent 写入门

Agent 不直接修改 Markdown。标准流程是：

```text
resolve object -> plan(object_id + expected revision + observed path)
               -> snapshot / approval
               -> apply
               -> receipt(before/after revision + ledger seq + changed paths)
               -> sync readiness
```

如果对象只发生 move 且内容 revision 未变，apply 可按 UUID 安全重定位；如果旧 path 已被另一个对象占用、内容已漂移、对象已删除或 ledger 不一致，返回 `plan_stale`，不能静默写入。

## 多端同步

manifest v2 以 `object_id` diff：同 ID 不同 path 是 move，不同 ID 同 path 是 collision，同 ID 从共同 base 分叉是 `revision_conflict`。tombstone 也以 UUID 表示删除对象。

升级必须显式执行：

```bash
pinax sync manifest audit --vault ./my-notes --json
pinax sync manifest plan --save --vault ./my-notes --json
pinax sync manifest promote --plan manifest-plan-<id> --remote-capability v2 --vault ./my-notes --yes --json
```

第二台设备生成 migration plan 时会继承远端 v2 manifest 中同路径普通文件的 `object_id`，避免每台设备各自分配形成双头。首次 v2 remote write 前可运行：

```bash
pinax sync manifest rollback --vault ./my-notes --yes --json
```

首次 v2 remote write 后 rollback 会返回 `manifest_rollback_unsafe`；此时应升级其他设备，而不是创建新的 v1 authoritative head。sync daemon 不会隐式 promotion，migration pending 或 identity unresolved 时只记录脱敏状态并拒绝远端写入。
