# Pinax 客户端 CLI 覆盖与实时同步设计

## 架构

客户端全 CLI 覆盖以 capability registry 为中心，不使用通用远程命令执行。

```mermaid
flowchart TD
    CLI[pinax CLI] --> LOCAL{local or remote mode}
    LOCAL -- local --> SVC[application service]
    LOCAL -- --api-url --> RPC[/POST /v1/rpc/]
    REST[local app / SDK / dashboard] --> API[pinax api serve]
    API --> REG[RemoteCapabilities / RemoteRoutes]
    RPC --> REG
    REG --> SVC
    SVC --> VAULT[(server-side local vault)]

    DEV1[(device A vault)] --> D1[pinax sync daemon]
    DEV2[(device B vault)] --> D2[pinax sync daemon]
    D1 -->|encrypted revision/blob/manifest| CLOUD[Cloud Sync transport]
    D2 -->|encrypted revision/blob/manifest| CLOUD
```

## 能力分层

1. `RemoteCapabilities()` 是客户端可发现能力的唯一清单，`pinax api routes --json` 和 OpenAPI 都从这里派生。
2. 每个客户端能力映射到一个 application service 方法；REST/RPC handler 只做参数解析、状态码映射和 projection 序列化。
3. CLI Remote API Mode 只转发 registry 支持的业务命令；不支持的命令返回 `remote_command_unsupported`。
4. 本地控制命令保持本地执行：`config`、`api`、`token`、`profile`、`vault`、`cloud`、`sync daemon`、completion、foreground server、editor 类命令不得被持久化 `remote.api_url` 劫持。
5. 写操作默认受 `--readonly` / `--allow-write`、`yes=true`、`dry_run=true`、snapshot 和 receipt 门禁约束。

## 覆盖路线

- Phase 1：补 note/search/kb/index/query/dataview/database/view 的只读和 view 管理能力。
- Phase 2：补 template/asset/prompt/collection/graph/import/export 的 dry-run/yes 写入能力。
- Phase 3：补 repair/metadata/organize/proof/version 的 plan/snapshot/apply/restore 闭环。
- Phase 4：补 publish/plugin/mcp/backend/storage/cloud 的集成面，危险操作默认 plan 或 dry-run。
- Phase 5：生成 CLI tree 与 capability registry 的覆盖审计，所有 local-only 命令有显式拒绝策略。

## 实时同步边界

`pinax sync daemon` 是设备本地进程。它启动时立即执行 pull-before-push，然后监听本地文件变化并轮询远端 head。它复用 `SyncPull` / `SyncPush` 和 Cloud Sync transport，不新增协议。

客户端可以通过显式 RPC 触发 `sync.push` / `sync.pull`，但实时多设备同步的推荐路径是每台设备运行：

```bash
pinax sync daemon run --target cloud --vault ./my-notes --yes
```

## 兼容性

- 新 route、RPC method、capability、optional schema field 都是 additive。
- 不删除现有 `remote_command_unsupported` 行为；它是客户端安全边界。
- 不改现有 HTTP path/method/status 语义；新增能力使用新 route 或新 RPC method。
- 不改变 `remote.api_url` 对本地控制命令的例外规则。

## 回滚

如果新增客户端能力导致风险，回滚该能力的 registry entry、handler 和 remote CLI mapper；保留现有 API server、sync daemon 和已支持能力不变。文档中新增覆盖矩阵可以回退到前一阶段说明。
