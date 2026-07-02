# 任务

## 0. 基线与失败用例

- [x] 0.1 Owner: `cli/pinax`; Lane: sequential; Depends on: none; Scope: 记录当前 `publish build` 返回 `publish_not_implemented` 的失败基线，并补一个命令级失败测试锁定现状。
  - Acceptance: `go test ./cmd/pinax -run TestPublishBuildPinaxWebStaticSite -count=1` 先失败，失败原因指向 build 未实现或缺少期望输出。
  - Validation: `go test ./cmd/pinax -run TestPublishBuildPinaxWebStaticSite -count=1`。
  - Expected: 测试在实现前失败，实现后通过。
  - Failure re-check: 如果测试直接通过，确认是否已有 build 实现；重新检查输出是否包含 `index.html`、note page、manifest 和 scan facts。

- [x] 0.2 Owner: `cli/pinax`; Lane: sequential; Depends on: none; Scope: 为 publish integration run evidence 建立测试入口约定，不手写 evidence metadata。
  - Acceptance: 新的 integration/component/e2e publish 测试入口会把每次运行证据写到 `temp/integration-test-runs/<run-id>/`。
  - Validation: 后续任务的 integration smoke 命令生成 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json`、`artifacts/`。
  - Expected: 失败也保留证据，并用原始测试退出码退出。
  - Failure re-check: 如果 evidence 缺字段，先修 runner，不允许只在测试日志里解释。
  - Evidence: 2026-06-29 将 `TestPublishProfile` 纳入 `task test:integration` 的 `internal/testkit/integrationevidence` runner，并新增 runner 配置测试；`task test:integration` 通过，生成 `temp/integration-test-runs/20260629T022805Z-2357619/{summary.json,command.txt,stdout.log,stderr.log,env.json,artifacts/README.txt}`，summary `checks.publish_static_profile=true`，敏感扫描无命中。

## 1. Publish-safe bundle 与 Go 服务后端

- [x] 1.1 Owner: `cli/pinax`; Lane: A; Depends on: 0.1; Scope: 新增 `internal/app/publishops/bundle.go` 和 focused tests，生成 publish-safe bundle。
  - Acceptance: bundle 包含 `manifest.json`、`notes.json`、`graph.json`、`taxonomies.json`、`search-index.json`、`sources.json`、assets；不包含 private/draft/secret/unpublished note body。
  - Validation: `go test ./internal/app ./internal/app/publishops -run 'Publish.*Bundle|PublishPlan' -count=1`。
  - Expected: 选择 active/public notes，跳过 draft/private/secret/publish=false，并产出稳定 skip reasons。
  - Failure re-check: 如果 bundle 出现绝对路径、`.pinax` 内容或 private body sentinel，修 selection/body policy，不在 renderer 层掩盖。

- [x] 1.2 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: 新增 `internal/app/publishops/scan.go`，递归扫描 bundle 和 output。
  - Acceptance: scanner 拒绝 token-like value、Authorization/Cookie、webhook URL、provider raw payload、绝对本地路径、`.pinax` internals 和 private body sentinel。
  - Validation: `go test ./internal/app/publishops -run 'Publish.*Scan|Redaction' -count=1`。
  - Expected: 违规项返回 stable issue codes，并进入 `blocking_count`。
  - Failure re-check: 如果 scanner 只扫顶层 JSON，补递归 map/list/string 扫描和 HTML/text 扫描 fixture。

- [x] 1.3 Owner: `cli/pinax`; Lane: A; Depends on: 1.1, 1.2; Scope: 实现 `Service.PublishBuild` 状态机。
  - Acceptance: `pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes --json` 成功生成静态输出、manifest、receipt；blocking plan 会拒绝 build。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'PublishBuild|Publish.*StaticSite' -count=1`。
  - Expected: JSON facts 包含 `profile`、`target`、`renderer`、`selected_count`、`output_hash`、`scan_findings`、`receipt`。
  - Failure re-check: 如果 build 修改 Markdown、Git 或远端状态，回退到 plan/bundle/output-only 边界。

- [x] 1.4 Owner: `cli/pinax`; Lane: A; Depends on: 1.3; Scope: 扩展 `PublishRequest`、domain target enum 和 profile validation，加入 `local`、`vercel`、`cloudflare-pages`。
  - Acceptance: `publish profile init public --target local|github-pages|vercel|cloudflare-pages --renderer pinax-web --json` 均能创建合法 profile。
  - Validation: `go test ./internal/domain ./internal/app ./cmd/pinax -run 'Publish.*Target|PublishProfile' -count=1`。
  - Expected: 旧 `github-wiki`、`github-gist`、`http` profile 继续 read-compatible。
  - Failure re-check: 如果旧 profile validation 失败，改 additive migration，不删除旧 enum。

## 2. 前端 renderer 构建

- [x] 2.0 Owner: `cli/pinax`; Lane: B; Depends on: none; Scope: 以前端预览 Web UI PRD 固化交互、页面、状态和验收合同。
  - Acceptance: `openspec/changes/pinax-static-publish-renderer-platforms/preview-web-ui-prd.md` 覆盖 Overview、Notes、Diagnostics、Receipts、Share、preview approval、LAN share、数据合同、可访问性和验证命令；后续 renderer 实现必须对齐 PRD。
  - Validation: `rg -n "Overview|Diagnostics|Receipts|Share|preview approve|pinax-data|Playwright" openspec/changes/pinax-static-publish-renderer-platforms/preview-web-ui-prd.md`。
  - Expected: PRD 能直接指导前端信息架构、状态派生、静态数据合同和 smoke 验收。
  - Failure re-check: 如果 PRD 只描述视觉风格或缺少真实命令、数据合同、错误状态，先补 PRD 后再进入 renderer 实现。
  - Evidence: 2026-06-29 `rg -n "Overview|Diagnostics|Receipts|Share|preview approve|pinax-data|Playwright" openspec/changes/pinax-static-publish-renderer-platforms/preview-web-ui-prd.md` 命中关键页面、状态、数据合同和 Playwright smoke；`openspec validate pinax-static-publish-renderer-platforms --strict` 通过。

- [x] 2.1 Owner: `cli/pinax`; Lane: B; Depends on: none; Scope: 新建 `web/pinax-web-renderer/` TypeScript renderer 包。
  - Acceptance: `web/pinax-web-renderer/package.json` 提供 `test`、`build`、`render:static` scripts；Vitest 能运行空 fixture 测试。
  - Validation: `cd web/pinax-web-renderer && npm test` 或项目选定包管理器等价命令。
  - Expected: 测试通过，构建产物进入包内 build output，不提交临时缓存或 node_modules。
  - Failure re-check: 如果引入包管理器锁文件，确认 Pinax 子项目可复现安装，并更新 `.gitignore` 排除缓存。
  - Evidence: 2026-06-29 使用 Bun 初始化 `web/pinax-web-renderer/`，新增 TypeScript static renderer package、Vitest fixture 测试和 `render:static` 入口；2026-07-01 移除 React/Vite 页面 scaffold，`bun run test`、`bun run build`、`bun run render:static` 应保持通过。

- [x] 2.2 Owner: `cli/pinax`; Lane: B; Depends on: 2.1; Scope: 实现 Markdown pipeline 和 sanitize 规则。
  - Acceptance: 支持 GFM、frontmatter metadata、wikilink token、safe attachment placeholder、managed block placeholder、dataview/database result placeholder；禁止 MDX/script/import/network execution。
  - Validation: `cd web/pinax-web-renderer && npm test -- markdown`。
  - Expected: fixture HTML 不含 `<script>`、raw import、外部 fetch、未脱敏 secret sentinel。
  - Failure re-check: 如果 sanitize 依赖浏览器环境，补 Node/Vitest 环境 fixture，避免只在浏览器 smoke 中发现。
  - Evidence: 2026-06-29 新增 `src/markdown.ts` 和 `src/markdown.test.ts`，覆盖 GFM task list、frontmatter、wikilink、attachment、managed/dataview/database placeholder，以及 script/iframe/import/network URL 剥离；`bun run test -- markdown`、`bun run test`、`bun run build` 均通过。

- [x] 2.3 Owner: `cli/pinax`; Lane: B; Depends on: 2.1, 2.2; Scope: 实现 `render-static`，从 bundle 生成 `dist/site/`。
  - Acceptance: 输出 `index.html`、`notes/<slug>/index.html`、`tags/<tag>/index.html`、`assets/`、`pinax-data/manifest.json`、`pinax-data/graph.json`、`pinax-data/search-index.json`。
  - Validation: `cd web/pinax-web-renderer && npm test -- render-static && npm run build`。
  - Expected: 同一 fixture 的 semantic manifest 稳定，适合未来 Workbench module 通过 Pinax contracts 复用。
  - Failure re-check: 如果 renderer 需要读取 vault 路径，重构为只读取 bundle。
  - Evidence: 2026-06-29 扩展 `src/render-static.ts`，从 publish bundle fixture 写出 index、note、tag、assets 和 `pinax-data/{manifest,graph,search-index}.json`，并拒绝 unsafe relative paths；`bun run test -- render-static`、`bun run test`、`bun run build` 均通过。

- [x] 2.4 Owner: `cli/pinax`; Lane: B; Depends on: 2.3; Scope: 在 `Taskfile.yml` 增加 renderer 任务并接入 `task check`。
  - Acceptance: `task publish:renderer:test`、`task publish:renderer:build`、`task publish:smoke` 可运行；`task check` 覆盖 Go + renderer + OpenSpec。
  - Validation: `task publish:renderer:test && task publish:renderer:build`。
  - Expected: 本地无 renderer 缓存污染，失败日志指向具体 fixture。
  - Failure re-check: 如果没有安装 Node/Bun/npm，doctor 输出明确依赖缺失，不让 Go 单测误报通过。
  - Evidence: 2026-06-29 新增 `publish:renderer:test`、`publish:renderer:build`、`publish:smoke` 并将 `publish:smoke` 接入 `task check`；`task publish:renderer:test`、`task publish:renderer:build`、`task publish:smoke` 和完整 `task check` 均通过。

## 3. 渲染服务适配与本地预览

- [x] 3.1 Owner: `cli/pinax`; Lane: C; Depends on: 1.3, 2.3; Scope: 新增 `internal/app/publishops/renderer.go` adapter，Go build 调用 renderer。
  - Acceptance: adapter 能把 bundle path、out path、base URL、theme 和 renderer version 传给 renderer，并脱敏 stderr。
  - Validation: `go test ./internal/app/publishops ./internal/app -run 'RendererAdapter|PublishBuild' -count=1`。
  - Expected: renderer 失败返回 stable code，如 `publish_renderer_failed`，stdout 仍是单一 JSON envelope。
  - Failure re-check: 如果 renderer 日志混入 stdout，修 subprocess stdout/stderr 分离。
  - Evidence: 2026-06-29 新增 `publishops.RendererAdapter` 和 fake executable 测试，TS `render:static` 支持 `--bundle/--out`，Go `PublishBuild` 的 pinax-web local path 先生成 publish-safe bundle 再调用 renderer；`go test ./internal/app/publishops -run 'RendererAdapter|Publish.*Bundle' -count=1`、`go test ./internal/app ./cmd/pinax -run 'PublishBuild|PublishServe|Publish.*StaticSite' -count=1`、`task publish:smoke` 均通过。

- [x] 3.2 Owner: `cli/pinax`; Lane: C; Depends on: 1.3; Scope: 完成 `PublishServe` loopback preview。
  - Acceptance: `pinax publish serve --profile public --out ./dist/site --host 127.0.0.1 --port 0 --once --vault ./my-notes --json` 启动一次 loopback request 后退出。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'PublishServe|Loopback' -count=1`。
  - Expected: JSON facts 包含 `served=true`、`host`、`port`、`url`，不会暴露 vault root。
  - Failure re-check: 如果可绑定非 loopback host，加入 host validation gate。

- [x] 3.3 Owner: `cli/pinax`; Lane: C; Depends on: 3.1, 3.2; Scope: 新增 `publish dev` 命令。
  - Acceptance: `pinax publish dev --profile public --out ./dist/site --host 127.0.0.1 --port 4173 --vault ./my-notes` 执行 build + serve；`--once --json` 可做 CI smoke。
  - Validation: `go test ./cmd/pinax ./internal/app -run 'PublishDev' -count=1`。
  - Expected: dev 不写远端、不部署、不绕过 scan；watch 初版可先不实现或标记 unsupported。
  - Failure re-check: 如果 dev 变成长驻 daemon，确保 shutdown、日志和端口释放测试覆盖。
  - Evidence: 2026-06-29 新增 `publish dev` Cobra 命令和 `Service.PublishDev`，默认执行 local build 后复用 loopback `PublishServe`；`go test ./cmd/pinax -run TestPublishDevBuildsAndServesOnce -count=1`、`go test ./internal/app ./cmd/pinax -run 'PublishDev|PublishBuild|PublishServe' -count=1`、`go test ./cmd/pinax -run Publish -count=1` 均通过。

- [x] 3.4 Owner: `cli/pinax`; Lane: C; Depends on: 3.2; Scope: 新增 `publish preview approve`，把主动预览确认变成部署前 gate。
  - Acceptance: `pinax publish preview approve --profile public --out ./dist/site --vault ./my-notes --json` 校验 output manifest/hash/scan result 后写入 CLI-authored preview receipt。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'PublishPreviewApprove|PublishDeployRequiresPreview' -count=1`。
  - Expected: receipt 包含 profile、target、out hash、served URL 或 preview source、approved_at、selected/skipped/blocking counts；不包含私密正文或 token。
  - Failure re-check: 如果 approve 能在未 build 或 hash 不匹配时成功，修 receipt 校验状态机。

- [x] 3.5 Owner: `cli/pinax`; Lane: C; Depends on: 3.3; Scope: 增加可选 `publish dev --watch` debounce rebuild。
  - Acceptance: 修改 vault Markdown 或 renderer source 后触发 rebuild；失败保留上一版可用输出并显示错误。
  - Validation: `go test ./internal/app -run 'PublishDevWatch' -count=1`。
  - Expected: watch 只监听 vault 内 Markdown、profile 和 renderer source，不监听 `.pinax/**` secret/config internals。
  - Failure re-check: 如果 watch 跨出 vault boundary，修 path filter。
  - Evidence: 2026-06-29 新增 `publish dev --watch`；`--watch --once` 用于 CI，初始 build+serve 后等待一次允许路径变更，debounce rebuild 后 smoke 并退出；watch filter 只接受 vault Markdown、`.pinax/publish/profiles/*.yaml|*.yml` 和 renderer source 文件，忽略 `.pinax/**` 其他内部路径。`go test ./internal/app -run PublishDevWatch -count=1` 和 `go test ./internal/app ./cmd/pinax -run 'PublishDev|PublishServe|PublishBuildPinaxWebStaticSite' -count=1` 通过。

## 4. 内网 Web/API 分享服务

- [x] 4.1 Owner: `cli/pinax`; Lane: F; Depends on: 1.3, 3.2; Scope: 新增 `pinax share start` CLI surface 和 request/domain 类型。
  - Acceptance: `pinax share start --scope published --host 127.0.0.1 --port 0 --readonly --vault ./my-notes --json` 返回合法 JSON projection，并暴露 web/api URL facts。
  - Validation: `go test ./cmd/pinax ./internal/app -run 'ShareStart|ShareCommand' -count=1`。
  - Expected: 新命令是 additive surface，不改变 `api serve`、`publish serve` 和 `vault dashboard` 默认行为。
  - Failure re-check: 如果旧命令 help/output 改变，回退为兼容 shim，只新增 `share` 命令族。
  - Evidence: 2026-06-29 新增 `ShareRequest`、`Service.ShareStart` 和 `pinax share start` CLI surface；`go test ./cmd/pinax -run ShareStart -count=1`、`go test ./internal/app ./cmd/pinax -run 'ShareStart|Share.*Gate|Share.*Auth|Share.*Host' -count=1` 均通过。

- [x] 4.2 Owner: `cli/pinax`; Lane: F; Depends on: 4.1; Scope: 实现 host/auth/scope 安全 gate。
  - Acceptance: 非 loopback host 必须传 `--allow-lan`；`--scope vault-readonly` 必须有 token auth；`--no-auth` 只能用于 loopback；第一版内网分享只支持 `--readonly`。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'Share.*Gate|Share.*Auth|Share.*Host' -count=1`。
  - Expected: 错误码包括 `share_allow_lan_required`、`share_auth_required`、`share_readonly_required`，并提供真实 next action。
  - Failure re-check: 如果 `0.0.0.0` 可在无 auth/无 allow-lan 下启动，阻塞实现并补 gate 测试。
  - Evidence: 2026-06-29 `TestShareStartSecurityGates` 覆盖非 loopback 缺 `--allow-lan`、`vault-readonly` 缺 token auth、缺 `--readonly` 三个 gate；`go test ./internal/app ./cmd/pinax -run 'ShareStart|Share.*Gate|Share.*Auth|Share.*Host' -count=1` 通过。

- [x] 4.3 Owner: `cli/pinax`; Lane: F; Depends on: 4.1, 4.2; Scope: 实现 `published` scope 的 Web + bounded API 分享。
  - Acceptance: `pinax share start --profile public --out ./dist/site --scope published --host 0.0.0.0 --port 8787 --allow-lan --readonly --vault ./my-notes --json` 只挂载 publish-safe output 和公开 API projection。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'SharePublishedScope' -count=1`。
  - Expected: Web root 只能是 `dist/site`；API 不返回 draft/private/secret/unpublished note body。
  - Failure re-check: 如果 handler 可访问 vault root、`.pinax/**` 或未发布笔记，修 route root 和 selection policy。
  - Evidence: 2026-06-29 新增 `sharePublishedHandler` 和 `--once` smoke；`/api/share/notes` 从 publish-safe `pinax-data/search-index.json` 生成 bounded notes，过滤 body/raw/private 字段；`go test ./internal/app ./cmd/pinax -run 'SharePublishedScope' -count=1` 通过。

- [x] 4.4 Owner: `cli/pinax`; Lane: F; Depends on: 4.2; Scope: 实现 `vault-readonly` scope 的受控只读工作台/API。
  - Acceptance: `pinax share start --scope vault-readonly --host 0.0.0.0 --port 8787 --allow-lan --readonly --token-file ~/.config/pinax/share-token --vault ./my-notes --json` 只开放 read-only route group。
  - Validation: `go test ./internal/app ./internal/api ./cmd/pinax -run 'ShareVaultReadonlyScope|ReadOnlyRoutes' -count=1`。
  - Expected: 默认 body exposure 是 card/detail/context；完整 body route 需要显式授权并记录 facts，不暴露 provider config/token/sync state。
  - Failure re-check: 如果任何 mutation route 可用，修 route registry exposure 并补 deny 测试。
  - Evidence: 2026-06-29 新增 `shareVaultReadonlyHandler`、token-file auth wrapper 和 `--once` authenticated smoke；`/api/share/notes` 只返回 metadata/card projection，认证后 mutation route 返回 405，未认证请求返回 401；`go test ./internal/app ./cmd/pinax -run 'SharePublishedScope|ShareVaultReadonlyScope|ShareStartSecurityGates|ReadOnlyRoutes' -count=1` 通过。

- [x] 4.5 Owner: `cli/pinax`; Lane: F; Depends on: 4.3, 4.4; Scope: 增加内网 share component/e2e evidence。
  - Acceptance: `go test ./tests/e2e -run 'ShareLANReadOnly' -count=1` 使用 loopback/fake LAN host 模式验证 Web 和 API，并写入 `temp/integration-test-runs/<run-id>/`。
  - Validation: `go test ./tests/e2e -run 'ShareLANReadOnly' -count=1`。
  - Expected: evidence 包含 summary、command、stdout、stderr、env、artifacts，且脱敏 token、Authorization、Cookie。
  - Failure re-check: 如果测试依赖真实局域网或公网，改成 loopback + host gate 单测，不依赖外部网络。
  - Evidence: 2026-06-29 新增 `TestShareLANReadOnly` testscript，使用 `0.0.0.0 --allow-lan --once` 的本地 fake LAN smoke 覆盖 `published` 和 `vault-readonly` scopes、缺 token gate、token-file auth 和输出脱敏；`go test ./tests/e2e -run TestShareLANReadOnly -count=1` 通过。`task test:integration` 通过并生成 `temp/integration-test-runs/20260629T030859Z-2461892/`，包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json`、`artifacts/README.txt`，summary `checks.share_lan_readonly=true`；精确泄漏扫描 `share-token|Authorization:|Bearer [A-Za-z0-9._-]+|Cookie:|VAULT_BODY_SENTINEL|PUBLISHED_BODY_SENTINEL|raw prompt|provider payload|hidden system prompt|private tool arguments|chain-of-thought` 无命中。

## 5. 部署后端适配

- [x] 5.1 Owner: `cli/pinax`; Lane: D; Depends on: 1.3, 3.4; Scope: 完成 GitHub Pages deploy adapter。
  - Acceptance: `pinax publish deploy --profile public --target github-pages --out ./dist/site --repo ../kb-pages --branch gh-pages --yes --vault ./my-notes --json` 只提交生成产物到目标 repo/branch。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'PublishDeployGitHubPages' -count=1`。
  - Expected: 无 `--yes` 返回 `approval_required`；缺少 preview approval receipt 返回 `preview_required`；target repo 不能是 vault root 或 `.pinax/**`。
  - Failure re-check: 如果 git stderr 泄露 credential-like 字符串，修 redaction。

- [x] 5.2 Owner: `cli/pinax`; Lane: D; Depends on: 1.4, 3.4; Scope: 实现 Vercel deploy adapter。
  - Acceptance: `pinax publish deploy --profile public --target vercel --out ./dist/site --project my-notes --yes --vault ./my-notes --json` 调用 fake `vercel` CLI 测试通过。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'PublishDeployVercel' -count=1`。
  - Expected: 未安装 CLI 返回 `publish_vercel_cli_missing`；不读取或输出 raw token。
  - Failure re-check: 如果要求 token 参数，改为依赖 vercel 本地配置或环境变量名，不接收 token 值。
  - Evidence: 2026-06-29 `go test ./cmd/pinax -run 'PublishDeployVercel|PublishDeployCloudflarePages' -count=1`；2026-06-29 `go test ./internal/app ./cmd/pinax -run 'PublishDeployVercel|PublishDeployCloudflarePages|PublishDeployGitHubPages|PublishPreviewApprove' -count=1`。

- [x] 5.3 Owner: `cli/pinax`; Lane: D; Depends on: 1.4, 3.4; Scope: 实现 Cloudflare Pages deploy adapter。
  - Acceptance: `pinax publish deploy --profile public --target cloudflare-pages --out ./dist/site --project my-notes --yes --vault ./my-notes --json` 调用 fake `wrangler pages deploy` 测试通过。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'PublishDeployCloudflarePages' -count=1`。
  - Expected: 未安装 CLI 返回 `publish_wrangler_cli_missing`；stdout/stderr/receipt 不含 token、Authorization、Cookie。
  - Failure re-check: 如果 wrangler 输出含 token-like 内容，统一走 redaction filter。
  - Evidence: 2026-06-29 `go test ./cmd/pinax -run 'PublishDeployVercel|PublishDeployCloudflarePages' -count=1`；2026-06-29 `go test ./internal/app ./cmd/pinax -run 'PublishDeployVercel|PublishDeployCloudflarePages|PublishDeployGitHubPages|PublishPreviewApprove' -count=1`。

- [x] 5.4 Owner: `cli/pinax`; Lane: D; Depends on: 5.1, 5.2, 5.3; Scope: 扩展 `publish doctor` 对 local/GitHub/Vercel/Cloudflare 的诊断。
  - Acceptance: doctor 输出 renderer、git、vercel、wrangler、out safety、latest receipt、scan status 和 next actions。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'PublishDoctor' -count=1`。
  - Expected: 缺失外部 CLI 是 warning 或 actionable error，不阻塞本地 build/serve。
  - Failure re-check: 如果 doctor 把云 CLI 缺失当成本地 build 失败，拆分 target-specific readiness。
  - Evidence: 2026-06-29 `go test ./cmd/pinax -run TestPublishDoctorReportsPlatformReadinessReceiptAndScan -count=1`；2026-06-29 `go test ./cmd/pinax -run TestPublishDoctorDetectsFakeHugoAndProfile -count=1`。

## 6. 文档、规格和端到端证据

- [x] 6.1 Owner: `cli/pinax`; Lane: E; Depends on: 3.3, 4.3, 5.1; Scope: 更新 `docs/commands/publish.md`、`docs/commands/api.md`、新增/更新 `docs/commands/share.md` 和 README 相关段落。
  - Acceptance: 文档优先展示 `build`、`serve`、`dev`、`share start`，再展示 GitHub Pages/Vercel/Cloudflare deploy；所有命令真实可运行。
  - Validation: `rg -n "publish dev|share start|allow-lan|cloudflare-pages|vercel|github-pages|publish serve" README.md docs/commands`。
  - Expected: 不推荐未实现命令；未完成 target 标注 planned 或落到对应任务后再公开。
  - Failure re-check: 如果文档出现 agent-only wrapper 或本地别名，改成真实 `pinax`/`task` 命令。
  - Evidence: 2026-06-29 更新 `docs/commands/publish.md`、`docs/commands/api.md`、新增 `docs/commands/share.md`，并同步 `README.md`、`README.zh-CN.md`、`docs/README.md`、`docs/commands/README.md`；`rg -n "publish dev --profile public --out ./dist/site --host 127\\.0\\.0\\.1 --port 4173 --watch|publish preview approve|target vercel|target cloudflare-pages|pinax share start|share_allow_lan_required|vault-readonly|share_lan_readonly" README.md README.zh-CN.md docs/README.md docs/commands/README.md docs/commands/publish.md docs/commands/api.md docs/commands/share.md openspec/changes/pinax-static-publish-renderer-platforms/specs/static-site-publishing/spec.md` 找到当前命令和边界说明。

- [x] 6.2 Owner: `cli/pinax`; Lane: E; Depends on: all implementation tasks; Scope: 增加 testscript 或 process e2e，覆盖完整本地发布和内网只读 share flow。
  - Acceptance: fixture vault 执行 `profile init -> plan -> build -> serve --once`，生成 integration evidence。
  - Validation: `go test ./tests/e2e -run 'PublishStaticSite' -count=1`。
  - Expected: evidence 写入 `temp/integration-test-runs/<run-id>/`，包含 redacted stdout/stderr 和 output artifacts hash。
  - Failure re-check: 如果测试依赖公网或真实 GitHub/Vercel/Cloudflare，改成 fake executable 和临时 repo。
  - Evidence: 2026-06-29 新增 `TestPublishStaticSite` testscript，覆盖 profile init、plan、local pinax-web build、serve `--once`、preview approve、published share 和 vault-readonly share；`go test ./tests/e2e -run TestPublishStaticSite -count=1` 通过。`task test:integration` 通过并生成 `temp/integration-test-runs/20260629T032135Z-2484408/`，summary `checks.publish_static_site=true` 和 `checks.share_lan_readonly=true`；精确泄漏扫描 `share-token|Authorization:|Bearer [A-Za-z0-9._-]+|Cookie:|PRIVATE_BODY_SENTINEL|VAULT_BODY_SENTINEL|PUBLISHED_BODY_SENTINEL|raw prompt|provider payload|hidden system prompt|private tool arguments|chain-of-thought` 无命中。

- [x] 6.3 Owner: `cli/pinax`; Lane: E; Depends on: all implementation tasks; Scope: 更新 live spec 并校验 OpenSpec。
  - Acceptance: `openspec/specs/static-site-publishing/spec.md` 归档后包含 local/dev/frontend renderer/LAN share/GitHub/Vercel/Cloudflare 要求。
  - Validation: `openspec validate pinax-static-publish-renderer-platforms --strict && openspec validate --all --strict`。
  - Expected: 0 failed。
  - Failure re-check: 如果 delta spec 和 live spec 冲突，先修 spec，不让实现漂移。
  - Evidence: 2026-06-29 更新 static-site-publishing delta spec，补 `publish dev --watch --once`、当前 `vault-readonly` metadata-only API、share evidence 场景；`openspec validate pinax-static-publish-renderer-platforms --strict && openspec validate --all --strict` 通过，53 passed / 0 failed。

- [x] 6.4 Owner: `cli/pinax`; Lane: E; Depends on: all implementation tasks; Scope: 完整质量门禁。
  - Acceptance: `task check` 通过，包含 Go fmt/lint/test/build、renderer test/build、OpenSpec validate 和 sidecar protocol tests。
  - Validation: `task check`。
  - Expected: 全部通过。
  - Failure re-check: 如果 renderer 依赖缺失导致 `task check` 失败，补 doctor 和 dev setup 文档，不跳过 renderer gate。
  - Evidence: 2026-06-29 `task check` 通过，覆盖 `golangci-lint run`、`golangci-lint fmt --diff`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`、renderer `bun run test`/`bun run build` 和 sidecar protocol tests。

## 实施证据

- 2026-06-29：新增 `TestPublishBuildPinaxWebStaticSite` 后运行 `go test ./cmd/pinax -run TestPublishBuildPinaxWebStaticSite -count=1`，观察到 RED：`publish_not_implemented`。
- 2026-06-29：新增 additive `local` target 和最小 `pinax-web` 静态输出后，`go test ./cmd/pinax -run TestPublishBuildPinaxWebStaticSite -count=1` 通过；随后 `go test ./internal/app ./cmd/pinax -run 'PublishBuild|Publish.*StaticSite' -count=1` 通过。
- 2026-06-29：新增 `TestPublishProfileInitAcceptsStaticPlatformTargets` 后运行 `go test ./cmd/pinax -run TestPublishProfileInitAcceptsStaticPlatformTargets -count=1`，观察到 RED：`vercel` 返回 `publish_target_invalid`；加入 `local`、`vercel`、`cloudflare-pages` additive target validation 后该命令通过。
- 2026-06-29：`go test ./internal/domain ./internal/app ./cmd/pinax -run 'Publish.*Target|PublishProfile|PublishBuild|Publish.*StaticSite' -count=1` 通过。
- 2026-06-29：新增 `internal/app/publishops/bundle_test.go` 后运行 `go test ./internal/app/publishops -run 'Publish.*Bundle' -count=1`，观察到 RED：缺少 `BuildPublishBundle` 和 `PublishBundleRequest`；实现 `internal/app/publishops/bundle.go` 后该命令通过。
- 2026-06-29：`go test ./internal/app ./internal/app/publishops -run 'Publish.*Bundle|PublishPlan' -count=1` 通过，覆盖 publish-safe bundle 数据文件、asset copy、skip reason 和 existing `PublishPlan` 选择/跳过逻辑。
- 2026-06-29：`go test ./internal/app/publishops -run 'Publish.*Scan|Redaction' -count=1` 起初显示 `[no tests to run]`，补充 `TestPublishBundleScanFindsNestedJSONAndHTMLLeaks` 后，`go test ./internal/app/publishops -run 'Publish.*Scan|Redaction' -count=1 -v` 执行该测试并通过，覆盖嵌套 JSON、HTML、`.pinax` path 和敏感内容不回显。
- 2026-06-29：增强 `TestPublishBuildPinaxWebStaticSite` 要求 `output_hash` 和 `receipt` facts，先观察到 RED：缺少 `receipt`；新增 additive `receipt` fact 后该测试通过。
- 2026-06-29：新增 `TestPublishBuildPinaxWebBlocksPlanViolations` 覆盖 local build 的 blocking plan，不写 `index.html`、禁止 asset 和 success receipt；`go test ./internal/app ./cmd/pinax -run 'PublishBuild|Publish.*StaticSite' -count=1` 通过。
- 2026-06-29：新增 `TestPublishServeLoopbackPinaxWebStaticSite` 覆盖 `publish serve --once` 预览 local pinax-web output；`go test ./internal/app ./cmd/pinax -run 'PublishServe|Loopback' -count=1` 通过。
- 2026-06-29：新增 `TestPublishPreviewApproveWritesReceipt` 和 GitHub Pages deploy preview gate 断言后，`go test ./internal/app ./cmd/pinax -run 'PublishPreviewApprove|PublishDeployRequiresPreview' -count=1 -v` 通过，覆盖未 build 返回 `preview_build_required`、approve 写 `preview_approved` receipt、deploy 未 approve 返回 `preview_required`、approve 后才写 deploy repo。
- 2026-06-29：`go test ./cmd/pinax -run Publish -count=1` 通过，确认新增 preview gate 未破坏既有 publish profile/build/serve/deploy/doctor/theme 命令测试。
- 2026-06-29：`go test ./internal/app ./cmd/pinax -run 'PublishDeployGitHubPages' -count=1` 起初显示 `[no tests to run]`；将现有 GitHub Pages deploy 测试重命名为 `TestPublishDeployGitHubPagesRequiresPreviewApprovalAndCommitsLocalRepo` 并补充 vault-root 拒绝断言后，该命令通过，覆盖 `approval_required`、`preview_required`、unsafe repo 拒绝和 approve 后写入 deploy repo/branch。
