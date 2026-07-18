# pinax-capsa-sdk-source-migration Tasks

## 1. SDK source 切换

- [x] 1.1 Owner: migration；Scope: `go.mod`；Dependencies: none；Lane: A；将 `replace github.com/yeisme/capsa` 从 `../../shared/capsa` 切换到 `../../backend-server/capsa/sdk`；Verification: `grep 'replace github.com/yeisme/capsa' go.mod`；Expected: 输出 `replace github.com/yeisme/capsa => ../../backend-server/capsa/sdk`；Failure re-check: 确认 `backend-server/capsa/sdk/go.mod` 存在且 module path 为 `github.com/yeisme/capsa`。
- [x] 1.2 Owner: migration；Scope: `go.sum`、`go.mod` tidy；Dependencies: 1.1；Lane: A；运行 `go mod tidy` 确保依赖一致；Verification: `go mod tidy && git diff --stat go.mod go.sum`；Expected: diff 只涉及 Capsa 相关 dependency 变化，不引入无关 module；Failure re-check: 检查 indirect dependency 变化是否合理，不手动回退 go.sum。
- [x] 1.3 Owner: migration；Scope: `internal/remote/crypto.go` 及所有 `github.com/yeisme/capsa` import；Dependencies: 1.1；Lane: A；确认 import path 和调用代码无需改动；Verification: `rg -n 'github.com/yeisme/capsa' --type go`；Expected: 所有 import 保持 `github.com/yeisme/capsa`，无路径变更；Failure re-check: 确认没有残留 `shared/capsa` 硬编码路径。

## 2. 构建与测试验证

- [x] 2.1 Owner: verification；Scope: 全项目构建；Dependencies: 1.2；Lane: B；验证编译通过；Verification: `go build ./...`；Expected: exit 0，无编译错误；Failure re-check: 确认 submodule 已初始化，运行 `git submodule update --init --recursive` 后重试。
- [x] 2.2 Owner: verification；Scope: sync 相关测试；Dependencies: 2.1；Lane: B；运行 crypto/cloud/sync 测试；Verification: `go test ./internal/remote ./internal/cloudsync ./internal/app -run 'Sync|Cloud|Capsa|Crypto' -count=1`；Expected: 全部通过；Failure re-check: 区分测试自身 flakiness（已知 `TestObjectStoreTransportLockFallbackRejectsConcurrentFirstHeadCreation` 偶发失败）与迁移引入的回归。
- [x] 2.3 Owner: verification；Scope: 完整质量门禁；Dependencies: 2.2；Lane: B；运行 `task check`；Expected: exit 0；Failure re-check: 按 fmt/lint/test/build 分段定位。

## 3. 文档与 OpenSpec 收尾

- [x] 3.1 Owner: docs；Scope: 本 change proposal/design/tasks/specs；Dependencies: none；Lane: C；完成 change 文档；Verification: `openspec validate pinax-capsa-sdk-source-migration --strict`；Expected: validation 通过；Failure re-check: 按 openspec 错误信息修正 delta spec 格式。
- [x] 3.2 Owner: docs；Scope: `openspec validate --all --strict`；Dependencies: 3.1；Lane: C；全量验证；Expected: 全部通过，无 regression；Failure re-check: 定位具体失败的 spec 或 change。
