# pinax-capsa-sdk-source-migration Tasks

## 1. Source migration

- [x] 1.1 Owner: Pinax maintainer；Scope: `go.mod` 的 Capsa local replace；Dependency: 无；Parallel lane: A；将 source 指向 `../../backend-server/capsa/sdk`，仅修改 replace；Verification command: `grep 'replace github.com/yeisme/capsa' go.mod`；Expected result: 输出指向 `../../backend-server/capsa/sdk` 的 replace；Failure recheck: 确认 `../../backend-server/capsa/sdk/go.mod` 存在且 module path 为 `github.com/yeisme/capsa`。Evidence: `replace github.com/yeisme/capsa => ../../backend-server/capsa/sdk` confirmed in `cli/pinax/go.mod`; SDK module path verified.
- [x] 1.2 Owner: Pinax maintainer；Scope: SDK import 与运行时不变量；Dependency: 1.1；Parallel lane: A；确认不修改 import、wire/schema、crypto salt、state paths 或 daemon naming；Verification command: `rg -n 'github.com/yeisme/capsa|shared/capsa' --glob '*.go' --glob 'go.mod'`；Expected result: import 保持公开 module path，且无业务源码路径迁移；Failure recheck: 如发现业务变更，拆出独立 change，不在本 change 扩大范围。Evidence: no `shared/capsa` imports found; public module path `github.com/yeisme/capsa` preserved.

## 2. Documentation and validation

- [x] 2.1 Owner: Pinax maintainer；Scope: 本 change artifacts；Dependency: 无；Parallel lane: B；记录方案 B、保留 shared/capsa、Scaena 独立 change 和 dirty worktree 保护；Verification command: `openspec validate pinax-capsa-sdk-source-migration --strict`；Expected result: 严格校验通过；Failure recheck: 按 validate 输出修正 artifact 或 Scenario 格式。Evidence: `openspec validate pinax-capsa-sdk-source-migration --strict` → "Change is valid".
- [x] 2.2 Owner: Pinax maintainer；Scope: migration regression；Dependency: 1.1；Parallel lane: B；运行 owner 项目中既有的最窄 Capsa 相关测试；Verification command: `go test ./...`；Expected result: 现有测试通过且没有协议、状态或 daemon 回归；Failure recheck: 先确认子模块初始化和 replace 解析，不回滚当前 dirty worktree。Evidence: `go test ./...` all packages pass (0 failures, 0 protocol/state/daemon regressions).

## 3. Deferred cleanup

- [x] 3.1 Owner: root migration；Scope: `shared/capsa` 删除与 Scaena consumer；Dependency: 2.1, 2.2；Parallel lane: C；留待独立 cleanup gate 和独立 Scaena change；Verification command: `openspec validate pinax-capsa-sdk-source-migration --strict`；Expected result: 当前 change 未删除 shared/capsa，未实施 Scaena；Failure recheck: 将超出范围工作迁入后续独立 change。Evidence: `shared/capsa` does not exist in repo; no Scaena consumer changes in this change.

## 4. Pinax legacy crypto

- [x] 4.1 Owner: Pinax maintainer；Scope: legacy salt/re-push 操作说明与验证；Dependency: 1.1；Parallel lane: D；确认 `pinax-cloud-sync-salt-v1` 旧对象需要完整本地 vault 的显式 re-push，且目标使用 `capsa-sync-salt-v1`；Verification command: `openspec validate pinax-capsa-sdk-source-migration --strict`；Expected result: change 明确记录 legacy 风险且严格校验通过；Failure recheck: 不删除用户状态、不静默重加密，改以隔离 fixture 重现并另开行为变更。Evidence: proposal.md documents legacy `pinax-cloud-sync-salt-v1` re-push requirement and `capsa-sync-salt-v1` target; strict validation passes.
