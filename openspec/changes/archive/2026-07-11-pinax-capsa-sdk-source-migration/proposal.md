# pinax-capsa-sdk-source-migration Proposal

## Why

Pinax 通过 `replace github.com/yeisme/capsa => ../../shared/capsa` 引入 Capsa 加密同步 SDK 完成第一轮 sync hardening。`shared/capsa` 当时是 monorepo 内的 plain directory + replace 占位，用于在 SDK 独立仓库化之前提供本地开发路径。

Capsa SDK 现已 promote 为同仓库双 Go module：SDK canonical source 位于 `backend-server/capsa/sdk`（由 git submodule `github.com/yeisme/backend-server-capsa` 承载），公共 module path 保持 `github.com/yeisme/capsa`。`shared/capsa` 退化为迁移回滚源，新功能和修复以 `backend-server/capsa/sdk` 为准。

继续指向 `shared/capsa` 会让 Pinax 脱离 Capsa 的版本化发布、code review、CI 和 submodule pin，存在分叉漂移风险。本变更把 Pinax 的本地 replace 切到 canonical SDK source，让 Pinax 和 Capsa backend server 共享同一份版本化 SDK 代码。

## What Changes

- `go.mod` replace 指令从 `../../shared/capsa` 切换到 `../../backend-server/capsa/sdk`。
- 公共 module path `github.com/yeisme/capsa` 保持不变；`internal/remote/crypto.go` wrapper、import path 和所有调用方代码零改动。
- `shared/capsa` 目录保留原样作为迁移回滚源；不在本 change 删除。
- 不引入新的 SDK API、行为变更或 wire schema 变更；加密 salt 仍是 `capsa-sync-salt-v1`，envelope 仍是 `pinax.cloud.envelope.v1`。

## Migration

本变更对已部署 vault 无运行时影响：

- Module path 不变，Go import 仍是 `github.com/yeisme/capsa`。
- 远端 COS/S3 对象的加密格式不变（`capsa-sync-salt-v1` + `pinax.cloud.envelope.v1`）。
- 开发者本地只需确保 `backend-server/capsa` submodule 已初始化：`git submodule update --init --recursive`（在仓库根执行）。

回滚步骤：把 `go.mod` 的 replace 改回 `../../shared/capsa` 即可，因为 `shared/capsa` 保持兼容。

## Capabilities

### Modified Capabilities

- `pinax-cloud-sync`: SDK source 从 plain directory 切到版本化 submodule，加密 API 来源被固化。

## Impact

- Pinax 开发依赖从 `shared/capsa` plain directory 切换到 `backend-server/capsa/sdk` submodule。
- 新贡献者 clone 仓库后需初始化 `backend-server/capsa` submodule 才能编译 sync 相关代码。
- `shared/capsa` 暂不删除，保留为 rollback path；后续 cleanup change 再处理。
