## Why

Pinax Cloud Sync 的产品价值只有在两台独立 local vault 通过真实 server transport 收敛后才成立。CLI 已经有 Cloud Sync protocol、direct transports 和 server transport 设计；Pinax Cloud MLP 会提供最小后端面。还需要一个明确的 CLI owner handoff，把 `cli/pinax` 与 `backend-server/pinax-cloud` 的 MLP 合同联调起来，防止 Cloud 只停在后端 API 或 CLI fake transport。

本变更必须排在 `pinax-agent-safe-proof-loop` 和 `backend-server/pinax-cloud/openspec/changes/pinax-cloud-sync-mlp` 之后。Proof loop 证明本地价值，Cloud MLP 提供可信 server CAS，CLI client handoff 再证明跨设备同步。

## What Changes

- 对接 Pinax Cloud MLP REST 合同：bootstrap/login state、vault create/link、changes cursor、blob batch-check/upload planning、revision CAS commit、audit/status facts。
- 增加 server transport 双设备 e2e：device A push -> device B pull 后收敛；并发编辑时 stale push 拒绝或 pull 产生 conflict copy。
- 强化 `remote_write=true` gate：只有 server CAS commit 成功且本地 sync-state evidence 写入后才允许为 true。
- 保持 Local API 与 Cloud Sync 文档分离：`pinax api serve` 不是 Cloud Sync transport。
- 只实现 server sync client handoff，不新增 hosted UI、native OneDrive、Cloud plaintext search、briefing delivery 或 provider SDK。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `pinax-cloud-sync`：补充 Pinax CLI 到 Pinax Cloud MLP server transport 的联调准入和双设备验收要求。

## Impact

- 影响 `internal/cloudclient`：与 Pinax Cloud MLP error envelope、auth、blob、revision、changes 合同保持一致。
- 影响 `internal/cloudsync` / `internal/app` sync orchestration：server transport 必须复用同一 sync engine 和冲突处理。
- 影响 `cmd/pinax` / `internal/cli`：cloud backend set/login、sync push/pull/status/doctor 输出需要呈现 server audit/auth boundary。
- 影响 `tests/e2e`：新增 server-backed two-device convergence、conflict 和 redaction evidence。
- 影响 docs：`docs/commands/cloud.md`、`docs/commands/sync.md`、`docs/architecture/cloud-sync-design.md` 需要记录联调路径和限制。
