# pinax-capsa-sdk-source-migration Design

## Context

Capsa SDK 经历两个阶段：

1. **plain directory 阶段**：SDK 代码抽取到 monorepo 内 `shared/capsa/`，消费方通过 `replace github.com/yeisme/capsa => ../../shared/capsa` 本地开发。这是 `capsa-sdk-extraction` B1 的过渡形态。
2. **submodule 阶段**：SDK promote 到 `backend-server/capsa`（git submodule `github.com/yeisme/backend-server-capsa`），SDK Go module 位于 `backend-server/capsa/sdk/`，公共 module path 保持 `github.com/yeisme/capsa`。Capsa backend server 与 SDK 同仓库共存，共享 CI、review 和 version pin。

Pinax 当前 replace 指向 plain directory，脱离了 Capsa 的版本化发布链路。

## Goals

- Pinax 编译时消费的 Capsa SDK 代码与 Capsa backend server 完全一致。
- 通过 submodule pointer 而非 plain directory copy 锁定 SDK 版本。
- Module path、import、wire schema、加密格式零变更。

## Non-Goals

- 删除 `shared/capsa`（留作 rollback，后续 cleanup change 处理）。
- 引入新的 Capsa SDK API 或 Pinax sync 行为变更。
- 改变加密 salt、envelope schema 或远端对象格式。

## Decisions

### Decision 1: replace 指向 submodule 内 SDK 目录

选择 `../../backend-server/capsa/sdk` 而非 `../../backend-server/capsa`，因为 Capsa submodule 的 Go module root 在 `sdk/` 子目录，`go.mod` 位于 `backend-server/capsa/sdk/go.mod`。指向 submodule 根目录会让 Go 找不到 module 定义。

### Decision 2: 保留 shared/capsa 作为回滚源

`shared/capsa` 的文件内容当前与 `backend-server/capsa/sdk` 一致（`client.go` MD5 相同）。保留它意味着：

- 迁移失败时改一行 replace 即可回滚。
- 不依赖本 change 同步删除 plain directory 副本。
- 后续 cleanup change 在确认 submodule 链路稳定后再删除 `shared/capsa`。

### Decision 3: 不在本 change 引入 go.sum 锁定变化

`go mod tidy` 可能更新 `go.sum` 中与 Capsa indirect dependencies 相关的 checksum。这是 Go toolchain 正常行为，只要 `go build ./...` 和 `go test ./...` 通过即可。

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| 新 contributor 未初始化 submodule | 编译失败 | 文档提示 `git submodule update --init --recursive` |
| `shared/capsa` 与 `backend-server/capsa/sdk` 后续分叉 | rollback 读到旧代码 | 后续 cleanup change 删除 `shared/capsa`；当前 MD5 一致 |
| go.sum checksum 漂移 | CI 失败 | `go mod tidy` + `task check` 验证 |

## Verification Plan

- `go build ./...` 通过。
- `go test ./internal/remote ./internal/cloudsync ./internal/app -run 'Sync\|Cloud\|Capsa\|Crypto' -count=1` 通过。
- `go mod tidy` 后 `git diff go.mod` 只保留 replace target 切换。
- `openspec validate pinax-capsa-sdk-source-migration --strict` 通过。
