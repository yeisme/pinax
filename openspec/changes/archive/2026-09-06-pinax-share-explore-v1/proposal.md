## Why

OKF 参考实现的旗舰体验是 `visualize` 模式：把整个知识 bundle 渲染为**自包含单文件 HTML**——客户端搜索框、类型过滤、backlinks 面板、力导向图/列表双布局切换，全部在本地页面内完成、零外部依赖。Pinax 已有 graph 投影、share 只读端点与 web renderer，但缺少一个"打开浏览器就能探索整个 vault"的交互面：`search` 是行式 CLI、`publish` 面向外部发布、`share start` 目前只服务 published 静态产物。结合本波 `pinax-okf-trust-discovery-v1` 落地的信任/新鲜度信号，一个本地优先的 vault explore 页能把 facet 收窄、信任徽标、链接图浏览统一到一个界面，显著改善"找东西"与"看结构"两个高频体验。

## What Changes

- 新增有界 explore 数据投影 `pinax.explore_bundle.v1`：节点（id/title/kind/tags/trust/fresh/摘要，无正文）、边（含 broken link 标记），由 graph + index 投影合成，容量上限（节点/边/摘要长度）fail-safe 截断。
- `pinax share start` 新增 `--view explore`（配合 `--scope vault-readonly`）：在现有只读 share 端点上服务 `/explore` 单页——自包含 HTML（内联 JS/CSS，**零 CDN、零外部请求**），客户端搜索、kind/trust/fresh 过滤、backlinks 面板、图/列表双布局、点击节点加载有界 note 卡（经 loopback/LAN 认证端点，走既有脱敏与有界投影）。
- 页面数据内嵌或经 `/explore/data.json` 提供；trust/fresh 徽标复用 `pinax-okf-trust-discovery-v1` 派生信号。
- 非目标：不做可写交互（explore 严格只读）、不发布到外部（非 publish 面）、不引入任何前端框架或外部运行时依赖、不做向量/语义搜索。

## Capabilities

### New Capabilities
- `spec:share-explore-surface` — explore bundle 投影边界、自包含页面合同、share 接线与只读/认证约束、离线与测试要求。

## Impact

- 代码：`internal/app`（explore bundle 组装）、`internal/output` 或 `web/pinax-web-renderer`（内联模板）、`internal/cli`（share flag 接线）、`cmd/pinax`（测试）。
- 兼容：`share start` 默认行为零变更（`--view` 缺省仍为 published 面）；依赖 change `pinax-okf-trust-discovery-v1` 的派生信号（未实施时徽标字段缺省为 unverified/fresh，页面不崩溃）。
- 安全面：explore 数据与 note 卡全部走既有 redaction/有界投影；`--once` 冒烟覆盖；页面禁止外联。
