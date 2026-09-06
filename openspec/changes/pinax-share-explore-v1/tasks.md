# Tasks

## 1. 投影合同冻结

- [ ] 1.1 冻结 `pinax.explore_bundle.v1`（字段、容量上限与 truncated 语义、无正文/无绝对路径红线、trust/fresh 信号缺省兼容）。Validation: `openspec validate pinax-share-explore-v1 --strict --no-interactive`。

## 2. 投影实现

- [ ] 2.1 实现 explore bundle 组装（graph+index 合成、fail-safe 截断、broken 标记）与单测（含递归无正文扫描）。Validation: `go test ./internal/app -run Explore -count=1`。

## 3. 页面与接线

- [ ] 3.1 实现自包含 explore HTML 模板（内联 JS/CSS、搜索/kind/trust/fresh 过滤、graph/list 切换、backlinks 面板、键盘可达、reduced-motion）；外联扫描测试。Validation: `go test ./cmd/pinax -run ExplorePage -count=1`。
- [ ] 3.2 `share start --view explore` 接线（vault-readonly 门控、`/explore*` 路由、token 认证、note 有界预览端点、默认 `--view` 零变更回归）。Validation: `go test ./internal/cli -run Share -count=1`。

## 4. e2e 与收口

- [ ] 4.1 testscript e2e：`--once` 三端点冒烟 + 401 认证缺失 + 默认 share golden 回归。Validation: `task test:integration`。
- [ ] 4.2 docs（`docs/commands/share.md` 增 explore 用法与安全边界）+ golden 固化 + `task check` 全绿。Validation: `task check`。
