# pinax-cli-sync-unification Proposal

## Why

pinax 当前有多个重叠的远程/后台命令组，与 capsa 统一同步服务方向冲突：
- `pinax cloud`：capsa 的旧版别名，完全重复
- `pinax storage`：set-s3 等 Hidden 遗留命令
- `pinax backend`：push/pull/diff 与 sync 重叠
- `pinax git snapshot`：隐藏遗留命令

## Summary

移除 pinax CLI 中冗余的远程/后台命令入口，统一到 `pinax sync`（同步操作）和 `pinax capsa`（设备/会话配置），消除多个 S3 配置入口和重复的 sync 路径。

## Motivation

**问题现状**：

pinax 当前有多个重叠的远程/后台命令组，与 capsa 统一同步服务方向冲突：

1. **`pinax cloud`**：capsa 的旧版别名，完全重复 `pinax capsa` 的功能（login/logout/doctor/backend set/status）
2. **`pinax storage`**：`set-local`/`set-s3` 已 Hidden，但遗留命令组存在；主存储 vs 同步目标边界模糊
3. **`pinax backend`**：`push/pull/diff` 与 `pinax sync push/pull/diff` 完全重叠
4. **`pinax git snapshot`**：隐藏遗留命令，已被组织工作流替代

**为什么现在需要改**：

- capsa 后端已生产就绪（wire 协议冻结、143 测试全绿）
- 统一同步服务方向明确：sync 协议（全量快照 + 增量 changes）本身就能满足 backup 需求
- CLI 聚焦本地创作，所有远程/后台/持久化应统一走 `sync`（capsa SDK → capsa 后端）
- 多入口造成用户困惑（cloud vs capsa、backend push vs sync push、storage set-s3 vs backend add s3）

**不做什么**：

- 不改 sync 引擎（`internal/app/cloud_sync.go` 保持不变）
- 不改 capsa SDK（`shared/capsa/` 保持不变）
- 不改 wire 协议（capsa 后端协议保持冻结）
- 不改 `pinax backend` 的 blob 浏览功能（`object list|stat`、`notes list|stat` 保留）

## Proposed Changes

### 1. 移除 `pinax cloud`（完全重复 capsa）

删除 `internal/cli/ops_integration_cmd.go` 的 `addCloudCommands` 函数（L57-123），包括：
- `cloud login` → 迁移到 `pinax capsa login`
- `cloud backend set s3|rclone` → 迁移到 `pinax capsa backend set s3|rclone`
- `cloud status` → 迁移到 `pinax capsa status`
- `cloud logout` → 迁移到 `pinax capsa logout`
- `cloud doctor` → 迁移到 `pinax capsa doctor`

### 2. 移除 `pinax git snapshot`（遗留）

删除 `internal/cli/git_cmd.go` 整个文件：
- `git snapshot` 已被组织工作流替代（`pinax organize` 要求先 snapshot）

### 3. 移除 `pinax storage` 遗留命令

删除 `internal/cli/storage_cmd.go` 的 Hidden 命令：
- `storage set-local`（Hidden）→ 删除
- `storage set-s3`（Hidden）→ 删除
- 保留 `storage set local|s3`/`status`/`doctor`（非 Hidden，明确"主存储"配置）

**决策**：storage 是"vault 主存储后端"配置（与 sync 目标不同），保留 `storage set local|s3`/`status`/`doctor`。
- **主存储**：vault 本地/远程内容存储（`storage set local|s3`）
- **同步目标**：capsa 同步目标（`pinax sync` 通过 capsa 配置）

### 4. 移除 `pinax backend` 的 sync 操作

修改 `internal/cli/ops_integration_cmd.go` 的 `addBackendCommands` 函数（L240），移除：
- `backend push` → 迁移到 `pinax sync push`
- `backend pull` → 迁移到 `pinax sync pull`
- `backend diff` → 迁移到 `pinax sync diff`（如果存在）

保留 `backend` 的 blob 浏览功能：
- `backend list|add|show|doctor|capabilities|remove`
- `backend object list|stat`
- `backend notes summary|list|stat`

### 5. 保留 `pinax capsa`

保留 `addCapsaCommands`（L125），包括：
- `capsa login`：配置 capsa 后端状态
- `capsa logout`：登出本地 capsa 设备会话
- `capsa status`：显示 capsa 状态
- `capsa doctor`：诊断 capsa 状态
- `capsa backend set s3|rclone`：配置 capsa 同步传输后端

**边界明确**：
- `capsa` = 设备/会话配置（login/logout/doctor/设备配对/backend set）
- `sync` = 同步操作（init/status/diff/push/pull/logs/daemon/conflicts）

### 6. 清理引用

- `internal/cli/root.go`：移除 `addCloudCommands` 和 `addGitCommands` 调用
- docs：更新/删除引用 `cloud`/`git snapshot`/`storage set-s3`/`backend push` 的文档
- tests：清理相关测试条目

## Impact Analysis

**用户影响**：

- `pinax cloud login` → 需迁移到 `pinax capsa login`
- `pinax backend push` → 需迁移到 `pinax sync push`
- `pinax backend pull` → 需迁移到 `pinax sync pull`
- `pinax backend diff` → 需迁移到 `pinax sync diff`（或删除，如果 sync 无 diff）

**向后兼容**：

- 直接删除，不加 deprecation hint（plan 明确说移除）
- 文档说明迁移路径（cloud→capsa, backend push→sync push）

**代码影响**：

- 删除 `addCloudCommands` 函数（~70 行）
- 删除 `addGitCommands` 函数（~15 行）
- 删除 `git_cmd.go` 文件（~23 行）
- 修改 `addBackendCommands` 移除 push/pull/diff（~10 行）
- 修改 `root.go` 移除调用（~2 行）

**测试影响**：

- 清理 `operation_catalog_test.go` 中的 backup/cloud 测试条目
- 清理 `output_contract_test.go` 中的 backup/cloud 用例

## Alternatives Considered

**A. 加 deprecation hint 而非直接删除**

优点：向后兼容，用户有时间迁移  
缺点：代码冗余持续，维护成本高，与 plan 方向不符  
**决策**：不采纳，plan 明确说移除

**B. 合并 `capsa` 到 `sync config`**

优点：进一步统一入口  
缺点：capsa 和 sync 职责不同（配置 vs 操作），合并会造成混淆  
**决策**：不采纳，本阶段保留 capsa，合并留后续

**C. 保留 `storage` 整个删除**

优点：彻底消除"主存储"vs"同步目标"混淆  
缺点：storage 是 vault 核心功能，不应删除  
**决策**：不采纳，storage 保留作为主存储配置

## OpenSpec References

- **baseline spec**：`pinax`（`openspec/specs/pinax/spec.md`）
- **related spec**：`pinax-cli-remote-api-mode`（`openspec/specs/pinax-cli-remote-api-mode/spec.md`）
- **related change**：`pinax-cloud-sync`（如存在）

## Success Criteria

1. `pinax --help` 命令树无 `cloud`/`git snapshot` 冗余
2. `pinax sync push/pull` 仍工作
3. `pinax backend object list`（blob 浏览）仍工作
4. `pinax capsa` 保留（login/logout/doctor/backend set/status）
5. `go build ./...` + `go test ./...` 全绿
6. `openspec validate pinax-cli-sync-unification --strict` 通过
7. `task check`（pinax）全绿

## Timeline

- 立项：立即（独立于 auctra 阶段 1/2）
- 实施：1-2 小时（删除 + 修改 + 验证）
- 验证：30 分钟（build/test/openspec validate/smoke）
- 总计：2-3 小时
