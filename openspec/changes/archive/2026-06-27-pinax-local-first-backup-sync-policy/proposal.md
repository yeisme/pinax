## Why

Pinax 同时有 S3 direct object-store transport、rclone/remote concepts、Cloud Sync client 和 sync daemon 相关设计。需要在 Pinax 子项目内同步 `local-first-backup-sync-policy`，明确 direct storage、Cloud Sync server transport 和 realtime daemon 的边界。

## What Changes

- 明确 S3 direct 是 CLI 侧 provider-credential transport，不是 Pinax Cloud server-side storage。
- 明确 Cloud Sync server transport 走 Pinax Cloud auth/audit/object lifecycle。
- 明确 realtime daemon 和 conflict resolution 不属于普通 backup mirror。

## Capabilities

### New Capabilities
- 无。

### Modified Capabilities
- `pinax-cloud-sync`: 增加 local-first backup/sync policy 边界要求。

## Impact

- 影响 `cli/pinax/openspec/specs/pinax-cloud-sync/spec.md` delta。
- 可能同步 `docs/architecture/cloud-sync-design.md` 和 `docs/commands/sync.md`。
- 不修改 Cloud Sync protocol fields、CLI output、daemon behavior 或 S3 direct implementation。
