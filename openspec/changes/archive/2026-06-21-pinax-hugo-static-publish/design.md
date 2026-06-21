## Context

Pinax 当前定位是本地优先 Markdown vault 的 agent-safe knowledge control plane：Markdown vault 是真源，SQLite/GORM 是可重建投影，Cloud Sync 只同步密文，REST/RPC 和 dashboard 只暴露本地投影。用户希望把基于 Pinax 的百科知识库发布到 GitHub Pages 或 GitHub Wiki，并允许使用 Hugo 作为前端静态编译渲染器。

这个能力和现有同步、dashboard、release pipeline 都不同：发布是从私有 vault 生成只读、筛选、脱敏的静态副本。GitHub Pages/Wiki 不能成为 vault 真源，也不能接收 `.pinax/**`、草稿、私有 note、provider raw payload、token 或未允许的完整正文。

## Goals / Non-Goals

**Goals:**

- 提供 `pinax publish` 命令族，覆盖 profile、plan、build、deploy、doctor。
- 使用 publish profile 明确发布规则，包括 note 选择、正文策略、资产策略、Hugo 主题、GitHub Pages/Wiki target 和安全 gate。
- `publish plan` 保持只读，输出将发布、跳过、阻断和人工审查项。
- `publish build --target github-pages` 生成 Hugo 输入目录，调用外部 `hugo` CLI，产出静态站点。
- `publish build --target github-wiki` 生成 GitHub Wiki 兼容 Markdown，不依赖 Hugo runtime。
- 构建前后递归扫描发布输入和输出，阻断 secret、Authorization/Cookie、provider payload、`.pinax`、绝对路径和未授权正文泄漏。
- 生成 publish manifest、receipt 和 redaction evidence，支持 CI 和本地复现。
- 部署只作用于发布仓库或输出目录，不修改私有 vault 正文；真实 deploy 必须显式确认。

**Non-Goals:**

- 不把 GitHub Pages/Wiki 作为 Pinax vault 的同步目标或真源。
- 不实现动态后端、登录态 Web app、GitHub OAuth 或 GitHub API 写入。
- 不把完整私有 vault mirror 到 GitHub。
- 不改变 Cloud Sync、Remote API Mode、MCP 或 GoReleaser release pipeline 的语义。
- 不在首版实现在线全文搜索服务；搜索索引必须是静态预生成文件。
- 不在首版实现所见即所得 Wiki 编辑回写；所有写回仍走 Pinax CLI/proof loop。

## Decisions

### 1. 新增 `static-site-publishing` 能力和 `publishops` 应用层包

发布逻辑放在新的 capability 边界：

- `internal/cli`/`cmd/pinax` 只负责 Cobra/pflag、参数校验和输出模式选择。
- `internal/app` 暴露 `PublishProfile*`、`PublishPlan`、`PublishBuild`、`PublishDeploy`、`PublishDoctor` facade 方法。
- `internal/app/publishops` 放纯规则：profile 匹配、note eligibility、asset eligibility、target policy、violation 分类、manifest shaping。
- `internal/publish` 或等价 adapter 包处理 Hugo staging、文件复制、产物扫描和 git deploy 边界。
- `internal/domain` 保存 `PublishProfile`、`PublishPlan`、`PublishManifest`、`PublishReceipt`、`PublishViolation` 等稳定模型。

理由：发布横跨 note、asset、search、version、redaction 和外部 Hugo/git，必须避免把业务判断散落在命令层或模板里。替代方案是把发布做成 dashboard 导出，但 dashboard 是本地只读视图，不适合承担静态发布和 deploy。

### 2. Profile 是 CLI-authored structured asset

发布 profile 默认存放在 `.pinax/publish/profiles/<name>.yaml`，只能由 `pinax publish profile init|set|validate` 创建或规范化。用户可以读它，但机器可读字段的修复和迁移必须由 CLI 完成。

建议首版字段：

```yaml
schema_version: pinax.publish_profile.v1
name: public
target: github-pages
renderer: hugo
site:
  title: My Encyclopedia
  base_url: https://example.github.io/knowledge/
  theme: builtin:pinax-encyclopedia
selection:
  include_publish_values: [public]
  include_statuses: [active]
  include_types: [concept, person, org, project, source, timeline, index]
  exclude_privacy_values: [private, secret]
body_policy: published-notes-only
assets:
  include_linked_assets: true
  allowed_extensions: [.png, .jpg, .jpeg, .gif, .svg, .webp, .pdf]
  max_bytes: 10485760
deploy:
  mode: none
  repo: ""
  branch: gh-pages
```

理由：发布规则必须可审计、可复现、可在 CI 中使用。替代方案是完全使用 CLI flags，但发布策略会变长，容易在脚本里分叉并误发布。

### 3. Plan 是只读安全 gate，Build 默认也不 deploy

`publish plan` 不写 Markdown、`.pinax`、Git、远端仓库或 provider。它只输出：

- selected notes/assets
- skipped notes/assets with reason
- blocking violations
- manual review items
- estimated output paths
- runnable next actions

`publish build` 可写 `--out` 和 CLI-authored receipt，但不推送 GitHub。`publish deploy` 必须显式 `--yes`，并且只对发布仓库工作树或已生成产物操作。

理由：发布是潜在数据泄漏操作，必须让用户先看到计划。替代方案是 `build` 自动部署，风险过高。

### 4. Hugo 只负责渲染，不负责安全判断

Pages target 的 build 分三步：

```text
Pinax vault
  -> publish plan / eligibility / redaction gate
  -> Hugo input staging: content/ data/ static/ config.yaml
  -> hugo --source <stage> --destination <out>
  -> recursive output leak scan
  -> manifest + receipt
```

Pinax 生成 Hugo 输入：

- `content/<slug>.md`：只包含允许发布的 note 正文和规范化 frontmatter。
- `data/pinax/manifest.json`：发布 manifest、graph summary、tags、source map、search metadata。
- `static/assets/**`：只复制 allowlist 资产。
- `hugo.yaml`：由 profile 和 Pinax 安全默认值生成。

Hugo theme 首版可以使用内置最小主题或用户指定主题；无论主题来自哪里，构建输入和最终输出都必须扫描。替代方案是 Go 内置 HTML renderer，但 Hugo 已成熟，主题生态更适合百科站。

### 4A. Hugo 集成采用“完整 staging project”，不复用用户现有 Hugo 站点

Pinax 不把 vault 直接当 Hugo content dir，也不默认接入用户已有 Hugo project。`publish build --target github-pages` 每次生成一个独立 staging project：

```text
<stage>/
  hugo.yaml
  content/
    entries/<slug>/index.md
    indexes/tags/<tag>.md
    indexes/sources/<source>.md
  data/
    pinax/
      manifest.json
      graph.json
      search-index.json
      taxonomies.json
      sources.json
      build.json
  static/
    assets/<asset-id>/<filename>
  themes/
    pinax-encyclopedia/   # 内置主题 materialized 到 staging，或通过 Hugo module/local theme 引用
```

Hugo 配置由 Pinax 生成并使用安全默认值：

- `baseURL`、`title`、`languageCode`、`theme` 来自 profile，经 schema 校验和路径规范化。
- `markup.goldmark.renderer.unsafe=false` 是默认值，避免原始 HTML 直通；若未来支持 `unsafe=true`，必须是单独 OpenSpec 变更并增加泄漏测试。
- 默认禁用或不生成不需要的 kind，例如 RSS、taxonomy term 以外的额外页面，避免意外扩大输出面。
- `params.pinaxThemeContract="pinax.publish_theme.v1"` 写入 Hugo config，让主题可以显式声明兼容合同。
- 构建环境固定为 `pinax-publish`，Pinax 传入最小环境变量，不把 provider、GitHub、sync 或用户 shell secrets 透传给 Hugo。

理由：完整 staging project 可复现、可扫描、可 golden test；复用用户现有 Hugo 站点会把安全边界扩散到用户模板、插件和构建脚本中，难以证明没有泄漏。高级用户未来可以通过 `publish build --mode content-only` 只导出 Hugo content/data，但首版不做。

### 4B. Pinax 主题合同是数据 schema，不是 Go 模板 API

主题只消费 Pinax 写入的 Hugo frontmatter 和 `data/pinax/*.json`，不调用 Pinax、SQLite、vault 或外部 provider。首版主题合同命名为 `pinax.publish_theme.v1`，包含：

- frontmatter 字段：`title`、`slug`、`type`、`tags`、`aliases`、`summary`、`source_refs`、`related_refs`、`updated_at`、`pinax_id`、`publish_path`。
- `data/pinax/manifest.json`：站点级清单、条目列表、输出路径、资产哈希、构建信息。
- `data/pinax/graph.json`：节点、边、关系类型、断链/跳过摘要，不包含未发布正文。
- `data/pinax/search-index.json`：只含已发布条目的标题、摘要、标签和允许索引的正文片段。
- `data/pinax/sources.json`：来源条目、引用关系和公开来源 URL，不包含私有抓取 payload。
- `data/pinax/build.json`：profile、target、renderer、theme、counts、schema versions 和 redaction summary。

主题不得依赖 `.pinax/**`、本机绝对路径、未发布 note slug、完整 provider trace、raw prompt 或内部 receipt。Pinax 在 staging scan 中把这些字段视为泄漏。

理由：把合同压到 JSON/frontmatter 可以让内置主题、用户主题、golden tests 和未来文档站工具共享一套稳定输入。替代方案是让主题读取任意 generated files，但会增加泄漏面和兼容成本。

### 4C. 内置主题 `pinax-encyclopedia` 的产品形态

首版内置主题应面向“私有知识库的公开/共享百科镜像”，不是营销站。设计目标：信息密度高、导航可预测、来源证据清楚、移动端可读、无后端依赖。

必备页面和组件：

- 首页：站点标题、最近更新、核心分类、标签入口、来源入口、健康/发布摘要。
- 条目页：标题、摘要、正文、标签、别名、相关条目、反向链接、来源引用、更新时间和发布 manifest path。
- 标签页/类型页：密集列表、计数、最近更新时间。
- 来源页：source note、外部 URL、附件引用、被引用条目列表。
- 图谱数据页：生成 `graph.json`，前端可用渐进增强渲染；无 JS 时提供关系列表 fallback。
- 搜索：生成静态 `search-index.json`；首版前端搜索只在浏览器本地运行，不调用远程服务。
- 404/未发布目标占位：内部链接指向未发布条目时显示“未发布或不可见”的安全文案，不泄漏原始路径或正文。

视觉和交互约束：

- 使用 CSS custom properties 定义 semantic tokens：`--px-bg`、`--px-text`、`--px-muted`、`--px-accent`、`--px-border`、`--px-warning`、`--px-danger`、`--px-code-bg`。
- 默认浅色、低饱和、工作型界面；避免大面积单一色系、渐变装饰、营销 hero、嵌套卡片和过圆组件。
- 布局以侧边导航 + 主内容 + 右侧关系/来源 rail 为桌面默认；移动端收敛成顶部导航和内容优先。
- 不从 CDN 加载字体、JS、CSS、分析脚本或图片；所有主题资产必须本地生成或随 Pinax 嵌入。
- JS 是渐进增强：搜索和图谱失败时，HTML 列表仍可浏览。

理由：百科站用户主要在查找、比较、追溯来源，不需要 landing page。主题必须体现 Pinax 的可信边界，而不是做成装饰性博客主题。

### 4D. 主题分发和覆盖策略

首版支持三类 theme source：

| Source | Profile value | 行为 |
| --- | --- | --- |
| 内置主题 | `builtin:pinax-encyclopedia` 或简写 `pinax-encyclopedia` | Pinax materialize 嵌入主题到 staging，默认推荐 |
| 本地主题目录 | `local:./themes/my-theme` | 复制到 staging 前校验路径在允许根内，不允许指向 vault 私密目录 |
| Hugo module/theme | 延后 | 首版不默认启用网络拉取；后续如支持必须锁定版本和无 secret 环境 |

为了让用户自定义而不破坏安全默认值，增加两个可选命令设计：

- `pinax publish theme list`：列出内置主题、合同版本和可用 layouts。
- `pinax publish theme eject pinax-encyclopedia --out ./theme`：把内置主题复制到用户指定目录，便于审查和修改。

`theme eject` 不写 vault，除非 `--out` 明确在 vault 内且通过路径安全检查。用户本地主题仍必须通过 staging/final scan，不能跳过 redaction gate。

理由：内置主题保证开箱即用；eject 保证可审查和可改；首版不做网络 module 是为了避免构建时隐式联网、供应链漂移和 secret 环境泄漏。

### 5. GitHub Wiki target 生成 Markdown 镜像，不使用 Hugo

Wiki target 输出：

- `Home.md`
- 每个发布 note 一个 slug 化 `.md`
- `_Sidebar.md` 和可选 `_Footer.md`
- 附件目录或链接重写后的相对资产
- `pinax-publish-manifest.json`

Wiki 不支持复杂前端 runtime，所以不生成 graph UI，只生成关系链接、标签索引和来源索引的 Markdown 页面。替代方案是强行把 Hugo 输出推到 Wiki，但 GitHub Wiki 的渲染模型不匹配，维护成本高。

### 6. 发布安全扫描是递归合同测试对象

发布扫描复用 `internal/redaction` 的敏感模式，并扩展为文件树扫描。扫描范围包括：

- Hugo staging dir
- Hugo final output dir
- Wiki output dir
- manifest/receipt/evidence
- stdout/stderr/events/explain 输出

阻断类别至少包括：

- `.pinax/` 路径或文件内容进入发布产物
- `Authorization`、`Bearer`、`Cookie`、webhook URL、known secret key/value
- provider raw payload、raw prompt、trace body
- 绝对本机路径
- draft/private/secret note 正文
- 未在 manifest allowlist 中的 asset

理由：只扫描最终 Pages HTML 不够，Hugo 输入和 JSON data 也可能被部署或留在 CI artifact。

### 7. Deploy 使用本地 git 边界，首版不调 GitHub API

`publish deploy` 支持两类目标：

- `--repo <path-or-url> --branch gh-pages`：将产物同步到干净发布工作树并通过 git commit/push。
- `--wiki-repo <path-or-url>`：同步 Markdown 到 GitHub Wiki Git 仓库。

实现应使用集中的 git adapter 或受控 subprocess，测试用 fake git executable 和临时 bare repo。不要在普通业务逻辑里拼复杂 porcelain 解析，不记录 remote token URL。

理由：GitHub API 会引入 OAuth/token 范围、rate limit 和审计复杂度；首版用 Git over SSH/credential helper 更 boring。

### 8. 输出合同沿用 Pinax projection

所有 `publish` 命令支持默认中文摘要、`--json`、`--agent`、`--events`、`--explain`。机器字段使用英文 key。stdout/stderr 分离：机器 stdout 只输出选定协议，Hugo/git 诊断进入 stderr 或 evidence，并必须脱敏。

## Risks / Trade-offs

- [Risk] 用户误配置 profile 导致私密 note 被选中。→ Mitigation：默认拒绝 `privacy=private|secret`、`status=draft`、`publish=false`；`publish plan` 显示 selected/skipped/blocking；泄漏扫描发现敏感模式即失败。
- [Risk] Hugo theme 在模板中输出过多 frontmatter 或 data JSON。→ Mitigation：Pinax 只把允许字段写入 staging；staging 和 final output 都扫描；第三方 theme 不跳过安全 gate。
- [Risk] 用户主题引入外部 CDN、分析脚本或构建期联网。→ Mitigation：首版内置主题默认无外部依赖；`local:<path>` 主题必须显式配置并经过 staging/final scan；Hugo module/network theme 延后到单独变更。
- [Risk] GitHub Pages/Wiki 仓库保留历史泄漏。→ Mitigation：推荐独立发布仓库；deploy 支持 clean/orphan strategy；docs 强调不要直接发布 vault repo。
- [Risk] Wiki 的 Markdown 渲染和 Hugo Pages 行为不一致。→ Mitigation：把 Pages 和 Wiki 作为两个 target，分别 golden 测试，不共享 renderer 假设。
- [Risk] 大 vault 构建慢。→ Mitigation：首版先保证正确性；manifest 记录 duration/count；后续再做增量构建和性能优化。
- [Risk] `hugo` 未安装或版本不兼容。→ Mitigation：`publish doctor` 检测；`publish build` 返回 `hugo_unavailable` 或 `hugo_version_unsupported`；测试 fake hugo 路径。
- [Risk] deploy 需要 Git credentials。→ Mitigation：Pinax 不保存 token；错误输出脱敏 remote URL；测试只用本地 bare repo。

## Migration Plan

1. 新增 spec、domain model 和 profile parser，不影响现有命令。
2. 实现 `publish profile init|validate` 和 `publish doctor`，只读或只写 CLI-authored profile。
3. 实现 `publish plan`，先覆盖 note/body/asset eligibility 和安全 violation。
4. 实现 `publish build --target github-wiki`，用 Markdown golden 固化最小路径。
5. 实现 `publish build --target github-pages --renderer hugo`，通过 fake hugo 和可选真实 Hugo smoke 验证。
6. 实现 `publish deploy` 到本地 git/bare repo，最后再允许远端 URL。
7. 更新 README/docs/commands，记录 GitHub Pages/Wiki 是发布面而非 vault 真源。

Rollback：本变更新增命令和文件，不改变现有 vault 读写路径。若发布能力出现问题，可隐藏 `publish` 命令或禁用 deploy，已生成的 `--out` 目录和发布仓库由用户删除；私有 vault 正文不需要迁移回滚。

## Open Questions

- `publish deploy` 是否默认使用 orphan branch 清历史？建议支持 `--strategy clean-worktree|orphan`，默认 `clean-worktree`，文档推荐敏感场景用 `orphan`。
- 是否支持 private GitHub Pages？GitHub 权限和计划差异较多，首版只把它视为 GitHub 仓库部署，不承诺访问控制。
