## Context

Pinax 现在有三条容易被混淆的远程路径：

1. `pinax api serve`：本地 REST/RPC projection adapter。它把一个正在运行进程绑定的 vault 暴露给 dashboard、agent 或另一条 CLI，属于中心化访问。
2. `pinax cloud` + `pinax sync --target cloud`：目标是 Obsidian Sync 类分布式同步。每台设备保留自己的本地 vault，通过 Cloud Sync Protocol 交换加密对象和 revision。
3. direct/embedded transport：本地 Pinax 可以不经过远程 Pinax Cloud 服务，直接使用 S3/MinIO/R2、rclone/OneDrive 或本地 Go API/RPC 执行同一套 Cloud Sync Protocol。

当前 CLI 已具备 cloud state、manifest 构建、sync plan、部分冲突辅助命令和 guarded sync 输出；`internal/remote` 已有 `BlobStore`、`file://` 和 `s3://` registry；`domain.BackendKind` 已预留 `onedrive` / `pinax-cloud`；后端服务已有 S3-compatible storage adapter。设计应把这些能力统一到 `cloudsync.Transport`，而不是把 Cloud Sync 绑定到远程 HTTP 服务。

## Goals / Non-Goals

**Goals:**

- 明确文档和规格中“中心化 Local API”、“Cloud Sync Protocol”和“具体 transport”的边界。
- 定义 transport 合同：server、s3-direct、rclone-direct/OneDrive、embedded Go API 都必须暴露同一组 revision/blob/manifest/CAS 操作。
- 定义 Cloud Server 后台必须提供的最小同步合同：auth/device、vault revision、blob batch-check、encrypted blob upload/download、CAS revision commit、audit/health。
- 定义 Direct Backend 路径：本地 Pinax 可直接配置 S3/MinIO/R2 或 rclone/OneDrive，不启动远程 Pinax Cloud 服务。
- 定义 CLI 端执行路径：加载 cloud state、扫描本地 vault、构建 manifest、端侧加密、检查/上传缺失 blob、提交 revision、拉取远端 revision、应用变更和保留冲突副本。
- 建立两设备 E2E 验收：两个独立本地 vault 通过 fake/local transport、S3 direct 或 server transport 同步并收敛。
- 保持本地优先：无网络时普通 vault 命令继续可用，Cloud/Direct backend 失败不破坏本地 Markdown。

**Non-Goals:**

- 不把 Pinax Cloud 设计成中心化 plaintext note editor 或托管笔记真源。
- 不在 `cmd/pinax` 或 CLI command handler 内实现长期运行的 Cloud 后台；但允许 CLI 调用本地 Go API/embedded transport 来操作对象存储。
- 不在第一版实现 native OneDrive Microsoft Graph OAuth；OneDrive 先通过 rclone transport 支持。
- 不在本阶段实现实时 WebSocket、CRDT、全文搜索、在线编辑器、计费、多租户商业控制台。
- 不在 Cloud 输出、日志、事件、fixture、对象 metadata 或审计中暴露 plaintext note body、raw token、Authorization header、Cookie 或 provider payload。

## Decisions

### 1. 采用分布式本地 vault 模型，而不是把 Local API 扩展成公网同步服务

推荐方案是保留 `pinax api serve` 的中心化本地 projection adapter 定位，把 Cloud Sync 作为独立后台协议实现。

替代方案 A：把 `api serve` 直接开放成公网服务。拒绝原因：它绑定一个 vault，语义是远程控制同一份本地数据，不提供多设备 base revision、blob 缺失检查、冲突收敛或离线本地可用语义。

替代方案 B：使用 Git 作为唯一同步协议。拒绝原因：移动端适配、冲突 UX 和对象级加密控制成本过高，且此前设计已决定 Cloud Sync 使用盲存储和 manifest/revision。

### 2. Cloud 后台是 revision coordinator + encrypted object store

Cloud 后台只存储加密 manifest、加密 blob、revision 元数据、设备 session 和审计记录。note path、note body 和 provider payload 在服务端日志与持久化中不可明文出现。

后台必须使用 CAS 提交：客户端带 `base_revision` 提交新 manifest；如果当前 revision 已变化，后台返回稳定 conflict error，不接受半提交。复杂并发、状态机、CAS 边界、错误码映射和非显然测试夹具需要中文注释说明。

### 3. Cloud Sync Protocol 与 transport 解耦

Cloud Sync Protocol 由 encrypted manifest、encrypted blob、revision metadata、head pointer、CAS commit 和 conflict handling 组成。Transport 只负责把这些逻辑对象放到某个远端或本地后端：

| Transport | 入口 | 是否需要远程 Pinax Cloud 服务 | 说明 |
| --- | --- | --- | --- |
| `server` | `https://cloud.example.test` | 是 | 通过 `internal/cloudclient` 调 HTTP API，获得服务端 auth、audit、policy。 |
| `s3-direct` | `s3://bucket/prefix` | 否 | 本地 CLI 使用 AWS/S3-compatible SDK 写 encrypted objects 和 `head.json`。 |
| `rclone-direct` | `rclone://onedrive/PinaxSync` | 否 | 本地 CLI 通过 rclone 适配 OneDrive/Dropbox/WebDAV 等 provider。 |
| `embedded` | Go API / local RPC | 否 | 本地 agent、桌面 app、MCP bridge 直接调用同一个 app service。 |

Direct transport 的安全边界是 provider credential reference 和本地 Pinax approval flow；它不提供 Pinax Cloud Server 的账号权限、服务端审计、配额或多租户隔离。

### 4. Direct object store 使用统一 object layout

S3/rclone direct transport 在配置 prefix 下写入：

```text
protocol.json
workspaces/{workspace_id}/vaults/{vault_id}/head.json
workspaces/{workspace_id}/vaults/{vault_id}/locks/commit.lock
workspaces/{workspace_id}/vaults/{vault_id}/revisions/{revision_id}.json
workspaces/{workspace_id}/vaults/{vault_id}/manifests/sha256/{first2}/{next2}/{manifest_blob_id}.json
workspaces/{workspace_id}/vaults/{vault_id}/blobs/sha256/{first2}/{next2}/{blob_id}.json
```

`head.json` 是 trunk pointer；`revisions/*.json` 只保存 revision metadata；manifest 与 note blob 都是 encrypted envelope。plaintext path 只允许出现在客户端解密后的 manifest 内存对象或本地 vault 中。

### 5. CAS 策略按 transport 分层

Server transport 使用 DB transaction 做 head CAS。S3 direct 优先使用 `If-Match` / ETag conditional write；如果 provider 不支持可靠 conditional write，则使用带 TTL 的 `locks/commit.lock`。Rclone/OneDrive direct 必须默认使用 lock object，因为 provider 条件写语义不一致；锁过期后允许其他设备抢占并重新计算 conflict。

### 6. `remote_write=true` 只能由 durable revision commit 触发
CLI 可以 dry-run、生成 plan、写本地事件、上传 encrypted blobs 或返回 partial，但只有 transport 成功完成 durable revision commit，并且 CLI 写入本地 sync-state receipt 后，Projection facts/data 才能出现 `remote_write=true`。

这避免“计划已生成”或“blob 已上传”被误读为“多端同步已完成”。


### 7. 冲突策略先做无损保留，不做自动合并

当本地与远端在同一 base revision 之后修改同一路径，CLI 拉取远端 trunk，并把本地版本写成同目录冲突副本，例如 `note.20260611123045.conflict.md`。用户或 agent 通过 `pinax sync conflicts list/diff/show/resolve` 完成后续合并。

自动三方 merge、CRDT 和语义合并延期；当前优先保证不丢数据和可解释。

### 8. 后端、direct transport 和 CLI 用 fake/local transport 做第一条验收线

真实部署前，必须先有进程内 fake transport 或本地 direct backend 组件测试：两个临时 vault、两个 device id、一个 workspace，执行 A push -> B pull -> B edit push -> A pull -> conflict 场景。测试不得依赖公网、真实 token 或用户 vault。

### 9. Sync logs 分三层存储，避免状态、历史和事件混杂

Cloud Sync 需要可审计的同步日志，但不能把所有诊断塞进 `.pinax/events.jsonl`。设计采用三层：

1. `.pinax/sync-state.json` 保存当前可信同步状态，例如 target、backend kind、workspace、device、last synced revision、last sync run id、last status 和 updated_at。它服务于 `sync status` 和默认 base revision，不保存长历史。
2. `.pinax/sync-runs/YYYY/MM/<run_id>.json` 保存每次 `sync diff/push/pull/all` 的完整 run receipt。成功、partial、failed、approval_required 都写入，包含 command、direction、status、remote_write/local_write、transport、request_id、base/current revision、manifest id、counts、timings、error、actions 和 redaction policy。
3. `.pinax/events.jsonl` 只保留轻量 timeline summary，例如 run_id、status、revision_id、remote_write、error_code 和 conflict count。它不承载完整 diagnostics。

新增 `pinax sync logs list/show/tail/prune` 读取 run receipts，全部复用 Projection 输出合同。`sync logs show <run-id> --json` 是 agent 和调试工具读取完整同步证据的主入口；`sync logs tail` 是短生命周期读取，不引入 daemon。

路径脱敏策略采用可配置模型：默认记录 vault 内相对路径，便于用户和 agent 定位冲突；敏感 vault 可切换为 path hash 或 omitted。无论哪种策略，note body、raw token、Authorization header、Cookie、provider payload、secret-ref 原文和 provider stderr 原文都不得进入 receipt、events、stdout、stderr 或测试证据。

日志保留策略采用数量加时间双条件：默认保留最近 200 次 sync run 且最多 90 天；`pinax sync logs prune --before 90d --yes` 或配置策略可清理历史 run receipt，但不得删除当前 `.pinax/sync-state.json`。


## Implementation Shape

### Pinax Cloud server owner

- Auth/device：注册设备、签发/验证会话、保存 device/workspace 关系。
- Vault/revision：读取 current revision，CAS commit revision，返回 `revision_conflict`。
- Blob：batch-check 缺失 blob，上传/download encrypted envelope。
- Audit/diagnostics：记录 workspace、device、operation、revision、status、duration、error_code；不记录 plaintext 或 token。
- Idempotency：相同 request id 或 idempotency key 重试不得重复产生不一致 revision。

### Pinax CLI / protocol engine owner

- `internal/cloudsync`：同步协议核心，包含 manifest、envelope、head、revision、conflict 和 `Transport` interface。
- `internal/cloudclient`：server transport 的 HTTP request building、bearer/device headers、stable error decode，不做 CLI rendering，不直接改 vault。
- `internal/remote`：复用 `BlobStore`、S3/file/rclone adapter 的 object IO 能力。
- `internal/app`：在 application service 编排 push/pull/diff，保持命令层只做参数校验和输出选择。
- `internal/sync`：plan/executor、冲突应用、receipt 更新。
- `internal/output` / CLI tests：保持 `--json`、`--agent`、summary、events 的输出合同。
- Local RPC / Go API：调用同一个 application service，不绕过 approval、dry-run、snapshot、events 和 redaction。

## Migration Plan

1. 文档先落地，避免用户把本地 API 中心化访问、Pinax Cloud Server 和 Direct Backend 混为一谈。
2. 先抽象 `cloudsync.Transport` 与 object layout，使用 fake/local transport 验证协议。
3. S3 direct transport 先落地，因为当前仓库已有 S3 profile、`internal/remote.S3Backend` 和 backend S3-compatible adapter。
4. rclone direct transport 第二步落地，用于 OneDrive/Dropbox/WebDAV；native OneDrive Graph adapter 推迟。
5. Server transport 通过 `internal/cloudclient` 接 Pinax Cloud backend；Cloud backend 后续实现 auth/blob/revision/CAS。
6. 任一 transport 的两设备 E2E 通过后，才允许 `sync push --target cloud --yes` 在真实提交成功时输出 `remote_write=true`。
7. 如果 transport 不可用或返回未知错误，CLI 保持本地 vault 不变，返回结构化错误并提示 dry-run/status/doctor。

## Risks / Trade-offs

- Revision CAS 实现错误导致覆盖写入 → server 用 DB transaction；S3 用 conditional write 或 TTL lock；rclone/OneDrive 用 lock object；全部用并发提交测试覆盖。
- 加密元数据泄露路径或正文 → manifest/note blob 必须是 encrypted envelope；object key 不含 plaintext path；redaction tests 和 fixture 扫描覆盖。
- Direct backend 权限误解 → 文档和 `cloud doctor` 必须说明 direct backend 没有 Pinax server auth/audit/multi-tenant policy。
- 后端/CLI/transport 合同漂移 → 生成 shared contract fixtures，server fake、S3 fake、rclone fake 共用成功/错误样例。
- 移动端网络不稳定导致重复提交 → 所有 mutation 需要 request id / idempotency key，重复请求返回同一结果或当前终态。
- 冲突副本过多影响用户理解 → `sync conflicts` 必须提供 list/diff/show/resolve，并在 sync 输出中给出 next action。

## Open Questions

- S3 direct 是否必须依赖 provider `If-Match`？设计允许 fallback 到 TTL lock；实现时按 provider 能力选择。
- Native OneDrive 何时实现？建议先通过 rclone 支持，等 OAuth/token/keychain 和 eTag 语义有单独设计后再做 native adapter。
- 生产 Pinax Cloud Server 数据库由当前设计继续抽象，PostgreSQL 切换不在本变更内。
- Cloud Secret 的最终来源可以是 `env://`、系统 keychain、1Password、AWS profile、rclone config 或其他 provider；本变更只要求 CLI 保存 secret reference，不保存 raw secret。
- 移动端 UI 如何呈现 conflict queue 属于后续客户端产品设计，不阻塞 CLI/Cloud sync 合同。
