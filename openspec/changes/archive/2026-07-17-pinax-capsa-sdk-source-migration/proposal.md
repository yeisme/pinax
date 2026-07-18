# pinax-capsa-sdk-source-migration Proposal

## 背景

Pinax 通过公开 module path `github.com/yeisme/capsa` 使用 Capsa SDK。方案 B 已将 canonical SDK source 设为 `backend-server/capsa/sdk`；继续指向 `shared/capsa` 会使本地开发脱离 Capsa 的同仓库版本 pin 与 review 边界。

## 变更内容

本 change 的实施范围仅为把 `go.mod` 中 `github.com/yeisme/capsa` 的本地 `replace` 切换至 `../../backend-server/capsa/sdk`。public module path、Go import、SDK API 与 vault 同步、设备和加密 合同均不改变。

## 兼容与非目标

- wire/schema、crypto salt、envelope、state paths 与 daemon naming 保持不变。
- `shared/capsa` 暂时保留为回滚来源；删除需要后续独立 cleanup gate。
- 不修改业务代码、命令行为、持久化数据或 OpenSpec baseline；Scaena 另有独立 consumer change。
- 当前 dirty worktree 不得回滚、清理或覆盖。

## 验收

Pinax 的 change 使用严格 OpenSpec 校验；后续实现只能修改本地 `replace`，并以既有测试验证 source migration 不改变运行时合同。

## Pinax legacy crypto 风险

既有由 Pinax-local crypto path 写入、使用 `pinax-cloud-sync-salt-v1` 的远端对象不会因 source migration 自动改写。迁移后 Capsa SDK 的派生 salt 为 `capsa-sync-salt-v1`；从拥有完整本地 vault 的设备执行显式 `pinax sync push --target capsa --yes` 才能重新推送。不得删除本地状态、静默重加密或把 re-push 当作自动回滚步骤。
