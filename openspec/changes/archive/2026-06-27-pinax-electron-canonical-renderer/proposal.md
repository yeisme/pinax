# Pinax Electron Canonical Renderer 提案

## 背景

Pinax 未来桌面/网页体验需要稳定的编辑器、预览、图谱、看板、Agent 侧栏和发布预览。客户端预览和静态发布必须共享同一套 Markdown/AST/组件语义，避免“客户端看到的”和“部署后的 HTML”不一致。

本变更把 Pinax 客户端路线收敛为：Electron 作为一致的桌面运行时，Pinax Go sidecar 继续拥有 vault、index、proof gate、sync、publish plan 和 redaction；一个 TypeScript canonical renderer 同时服务 Electron 实时预览和静态 HTML 发布。

## 目标

- 将未来桌面客户端确定为独立 Electron 子项目，不把客户端源码放进 `cli/pinax`。
- 将 `pinax-web` 设为新的 canonical publish renderer：Electron 预览和静态发布复用同一套 Markdown/AST/React 渲染包。
- 将外部站点生成器从当前设计真源中移除，不再作为默认或推荐渲染路线。
- 更新 Pinax 当前产品文档和 live specs，只描述 Electron + canonical renderer 目标架构。
- 保持 `cli/pinax` 的职责为 Go sidecar、Local REST/RPC、projection、权限门禁、发布计划、扫描和 receipt。

## 非目标

- 本变更不在 `cli/pinax` 中实现 Electron 客户端源码。
- 本变更不在 Go 里实现 HTML 组件渲染器。
- 本变更不承诺保留旧渲染兼容路径作为长期产品路线。
- 本变更不让 Electron 直接读写 `.pinax/**`、SQLite、LanceDB、token 文件或 provider config。

## 影响面

- 文档：`docs/product/web-open-design.md`、`docs/commands/publish.md`、`README.md`、docs 索引。
- 规格：`openspec/specs/pinax-web-client-contracts/spec.md`、`openspec/specs/static-site-publishing/spec.md`、必要时更新 `openspec/specs/pinax/spec.md` 的发布边界。
- 稳定合同风险：`renderer` profile enum 当前实现仍含旧值。后续实现必须通过 OpenSpec 任务完成迁移，而不是只改文档。

## 验收标准

- 当前文档和 live specs 只推荐 Electron + Go sidecar + `pinax-web` canonical renderer 作为 Pinax 客户端/发布主线。
- specs 明确 `pinax-web` 是唯一 canonical renderer，静态发布从同一 renderer 输出 HTML 文件。
- specs 明确 Electron 客户端只通过 Local REST/RPC/projection 与 Go sidecar 交互。
- `openspec validate pinax-electron-canonical-renderer --strict` 和 `openspec validate --all --strict` 通过。
