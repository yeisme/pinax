## Why

Cloud Sync pull 当前按远端 manifest 直接写回本地，无法识别用户已经在本地移动或删除但尚未 push 的笔记，导致旧 `index/...` 路径被重新拉回。Pinax 需要把默认同步路径切到最新 Cloud Sync，并让本地优先的移动、删除在多设备间自动收敛。

## What Changes

- `pinax sync diff/push/pull` 默认 target 改为 `cloud`，显式 `--target git`、`--target s3` 仍保留。
- Cloud Sync 成功 push/pull 后缓存上次同步 manifest，下一次 pull/sync 基于 base/local/remote 三方计划执行。
- `pinax sync pull` 遇到本地未推移动/删除时返回 `LOCAL_UNPUSHED_CHANGES`，不恢复旧远端路径。
- `pinax sync` 作为双向入口自动 pull 安全远端变更并 push 本地移动/删除。
- note soft delete 生成 Cloud delete marker，远端 delete marker pull 到本地时进入 trash lifecycle。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `pinax-remote-sync-registry`: Cloud Sync 默认入口、manifest baseline 和三方收敛行为。
- `vault-trash-lifecycle`: note soft delete marker 与远端 note tombstone pull 行为。

## Impact

影响 `internal/app` Cloud Sync 执行、records/trash materialization、`internal/cli` sync 默认参数、CLI/output contract 回归测试和服务层两设备同步测试。不改变远端协议版本，不移除 git/s3 target。
