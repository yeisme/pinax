# Pinax Electron Canonical Renderer 任务

## 1. 当前文档清理

- [x] 1.1 更新 `docs/product/web-open-design.md`，只保留 Electron + Go sidecar + canonical renderer 路线。
  - Owner: `cli/pinax`
  - Lane: A
  - Acceptance: 文档只描述 Electron + Go sidecar + canonical renderer 主线。
  - Validation: `rg -n "Electron|pinax-web|canonical renderer" docs/product/web-open-design.md` 能看到主线定义。
  - Failure re-check: 如果搜索仍命中旧路线，确认是否为明确否定语境；否则继续清理。

- [x] 1.2 更新 `docs/commands/publish.md` 和 README 发布段落，改为 `pinax-web` canonical publish renderer 目标合同。
  - Owner: `cli/pinax`
  - Lane: A
  - Acceptance: 当前 publish 文档只描述 canonical renderer 输出 HTML，不再保留旧发布路径。
  - Validation: `rg -n "pinax-web|canonical renderer" README.md docs/commands/publish.md` 能看到发布主线定义。
  - Failure re-check: 若 README 仍没有 `pinax-web`，继续同步入口文档。

## 2. Live specs 更新

- [x] 2.1 重写 `openspec/specs/pinax-web-client-contracts/spec.md`，明确 Electron 客户端和 sidecar 边界。
  - Owner: `cli/pinax`
  - Lane: B
  - Acceptance: spec 声明未来客户端源码属于独立 Electron 子项目，`cli/pinax` 只负责 API/projection/gate。
  - Validation: `openspec validate pinax-electron-canonical-renderer --strict`。
  - Failure re-check: 如果 OpenSpec 失败，补齐 requirement/scenario 格式。

- [x] 2.2 重写 `openspec/specs/static-site-publishing/spec.md`，将 `pinax-web` 设为唯一 canonical publish renderer。
  - Owner: `cli/pinax`
  - Lane: B
  - Acceptance: spec 要求发布输出来自 Pinax projection bundle + static renderer。
  - Validation: `openspec validate --all --strict`。
  - Failure re-check: 如果其他 spec 仍描述旧发布合同，更新为 canonical renderer。

## 3. 后续实现任务

- [x] 3.1 新增 `renderer: pinax-web` profile enum 和 profile migration plan。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 1.1、1.2、2.1、2.2
  - Acceptance: `pinax publish profile init public --target github-pages --renderer pinax-web --vault ./my-notes --json` 成功，旧 renderer 不再出现在新文档示例。
  - Validation: `go test ./internal/domain ./internal/app ./cmd/pinax -run 'Publish.*Renderer|PublishProfile' -count=1`。
  - Failure re-check: 如果旧 profile 读取失败，先实现 profile service 迁移提示，不允许用户手写 `.pinax/publish/profiles/*.yaml`。
  - Implementation: 已 additive 新增 `PublishRendererPinaxWeb`，保留 `hugo`/`none` legacy 值；`publish profile init` 默认 renderer 改为 `pinax-web`；`publish profile validate` 对 legacy GitHub Pages/Hugo profile 输出 `migration_plan` 和 `migration.recommended=true`，但不自动改写旧 profile 文件。
  - Evidence: 先写 focused failing tests 后实现；`go test ./internal/domain ./internal/app ./cmd/pinax -run 'Publish.*Renderer|PublishProfile' -count=1` 通过。

- [x] 3.2 建立 renderer package contract 和 static export fixture。**Deferred / future-owner recorded**。
  - Owner: future Electron/client subproject plus `cli/pinax` API contract
  - Lane: sequential
  - Depends on: 3.1
  - Acceptance: 同一 fixture 的 Electron preview 和 static export 使用同一 AST semantics，wikilink、frontmatter、managed block 和 dataview placeholder 输出一致。
  - Validation: `task check` in owning client subproject and `openspec validate --all --strict` in `cli/pinax`。
  - Failure re-check: 如果 renderer 需要直接读 vault，退回 projection contract 设计，禁止绕过 Go sidecar。
  - Deferred reason: 当前 `cli/pinax` 尚未拥有 Electron/client renderer package，也没有可执行的 renderer package contract fixture；该项应由未来 Electron/client 子项目实现，Pinax 侧仅保留 bounded projection/API contract。
