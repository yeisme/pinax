## Why

当前文档容易把 `pinax api serve` 的中心化远程访问模式与 `pinax cloud` / `pinax sync --target cloud` 的分布式多端同步目标混在一起，导致用户误以为启动一个本地服务就已经具备 Obsidian Sync 类同步能力。

本变更明确 Pinax Cloud 的目标设计：每台设备都保留本地 Markdown vault，Cloud Sync 是一套本地优先同步协议，而不是必须依赖远程 Pinax Cloud 服务。协议可以通过 Pinax Cloud Server、S3-compatible direct backend、rclone/OneDrive direct backend 或本地 Go API/RPC transport 执行。无论 transport 是什么，CLI 都继续禁止把计划态、blob 上传态或未完成 CAS 的状态宣称为真实远端写入。

## What Changes

- 更新用户文档，明确区分：
  - 中心化 Local API：`pinax api serve` 暴露一个服务端本地 vault。
  - Cloud Sync Protocol：每台设备有自己的本地 vault，通过 transport 交换加密 revision/blob 并处理冲突。
  - Pinax Cloud Server：Cloud Sync Protocol 的一种 server transport，提供 auth/device/audit/policy。
  - Direct Backend：本地 Pinax 直接使用 S3/MinIO/R2 或 rclone/OneDrive，不启动远程 Pinax Cloud 服务。
- 新增 Cloud Sync 架构文档，说明 transport modes、object-store layout、CAS/lock 策略、当前可用能力、目标数据流和 CLI/Cloud owner 边界。
- 为 `pinax-cloud-sync` 增量规范补充分布式同步要求：本地 vault 所有权、transport 抽象、direct object-store layout、Cloud Server 后台职责、revision CAS、冲突保留、真实 `remote_write=true` 的准入条件。
- 建立实现任务清单，按协议核心、S3 direct、rclone/OneDrive direct、server backend、CLI sync engine、local RPC/Go API、双设备 E2E 和文档验收拆分。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `pinax-cloud-sync`: 明确 Cloud Sync 是本地优先的分布式同步协议；补充 transport 抽象、S3/rclone direct backend、Cloud Server 合同、真实远端写入 gate、冲突处理和双设备收敛验收要求。

## Impact

- 影响文档：`docs/architecture/cloud-sync-design.md`、`docs/commands/cloud.md`、`docs/commands/sync.md`、`docs/interfaces/remote-api-contract.md`、`docs/commands/README.md`、`docs/overview/product-positioning.md`、`docs/README.md`。
- 影响 OpenSpec：新增 `openspec/changes/pinax-cloud-distributed-sync/`，修改目标为 `pinax-cloud-sync` 能力。
- 后续实现影响代码：`internal/cloudsync/`、`internal/cloudclient/`、`internal/remote` S3/rclone adapters、`internal/app` cloud/sync service、`internal/sync` executor/planner、`internal/cli` cloud/sync commands、local RPC/Go API entrypoints、`cmd/pinax` contract tests、`tests/e2e` 双设备流程。
- 后端依赖：Pinax Cloud Server transport 需要后端提供 auth/device、vault revision、blob batch-check/upload/download、CAS commit、audit/health 接口；S3/rclone direct transport 不依赖远程 Pinax Cloud 服务，但不提供 server auth/audit/multi-tenant policy。
- 不改变当前 `pinax api serve` 的 loopback、本地 projection adapter、remote API mode 或现有 write gate 行为。
