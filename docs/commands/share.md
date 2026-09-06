# share

`pinax share` exposes an explicit read-only Web/API surface for local or LAN review. It is separate from `pinax publish serve`, which is loopback-only, and from `pinax api serve`, which is the local REST/RPC projection adapter.

## Published Scope

Share an already-built static site and its bounded publish API:

```bash
pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes --json
pinax share start --scope published --profile public --out ./dist/site --host 0.0.0.0 --port 8787 --allow-lan --readonly --vault ./my-notes --json
```

`published` scope serves only the generated output directory. Its `/api/share/notes` route reads the publish-safe search index and returns bounded metadata fields such as `id`, `title`, `path`, `tags`, `kind`, and `status`. It does not read the private vault root, `.pinax/**`, provider config, token files, sync state, draft/private/unpublished notes, or note bodies.

## Vault-Readonly Scope

Share a controlled metadata-only read projection from the vault:

```bash
pinax share start --scope vault-readonly --host 0.0.0.0 --port 8787 --allow-lan --readonly --token-file ~/.config/pinax/share-token --vault ./my-notes --json
```

`vault-readonly` scope requires token auth unless it is explicitly started with `--no-auth` on loopback. The first route group exposes a minimal HTML shell plus `/api/share/status` and `/api/share/notes`. Notes are metadata-only card projections with no full body route. Mutation methods return `405` after authentication.

For CI smoke tests, `--once` starts the server, performs authenticated Web/API smoke requests, records `web_smoke=true` and `api_smoke=true`, and exits:

```bash
pinax share start --scope vault-readonly --host 127.0.0.1 --port 0 --readonly --token-file ~/.config/pinax/share-token --once --vault ./my-notes --json
```

## Explore 视图（`--view explore`）

`--view explore` 在 `--scope vault-readonly` 上提供一个自包含的 vault 探索页（OKF visualize 体验的 Pinax 落地）：打开浏览器即可搜索、按 kind/trust/fresh 过滤、在力导向图与列表之间切换、查看 backlinks 面板，并把整个 vault 的结构当作一个图来浏览。

```bash
pinax share start --scope vault-readonly --view explore --host 127.0.0.1 --port 8787 --readonly --token-file ~/.config/pinax/share-token --vault ./my-notes --json
```

- 启动后打开 `explore_url`（默认 `http://127.0.0.1:8787/explore`）。token 认证模式下在 URL 后追加 `?token=<share token 文件内容>`，页面会把它转换为后续请求的 Bearer 头；loopback `--no-auth` 模式无需 token。
- 页面是单文件自包含 HTML：JS/CSS 全内联、零 CDN、零字体/脚本外链、零外部网络请求；搜索与过滤全部在客户端完成。`/` 聚焦搜索、`↑↓` 导航、`Enter` 打开节点卡，过滤状态记录在 URL hash（可分享），并在 `prefers-reduced-motion` 下禁用动画。
- 选中节点右侧出卡：元数据、信任/新鲜度徽标、backlinks/out/同标签邻居（只有 id 与标题，不含正文）；"Load preview" 按钮经认证端点 `/explore/note/<id>` 拉取有界预览（脱敏 + 字节上限，不嵌全文）。

### 数据投影 `pinax.explore_bundle.v1`

`/explore/data.json`（默认同时内嵌进页面）提供有界数据投影，由 graph 投影与 note 索引在内存合成，不落盘：

- 节点携带 `id`/`title`/`kind`/`tags`/`trust`/`fresh`/`summary`/`updated_at`；边携带 `from`/`to`/`broken`。绝不携带 note 正文、绝对路径或凭据；`summary` 只来自 frontmatter 的 `summary`/`description`。
- 容量上限 fail-safe 截断并置 `truncated=true`：节点 5000、边 20000、title ≤160、summary ≤240、tags ≤8。截断时页面提示改用 list 布局或收窄过滤；超大 vault 可加 `--no-embed` 让页面改为按需拉取 `data.json`。
- `trust`（unverified/machine/human）与 `fresh`（fresh/stale）按 OKF 规则从 note frontmatter 的 `verified`/`stale_after` 消费时派生：无 verified ⇒ unverified；任一 `human:` ⇒ human；全非 human ⇒ machine；`now >= stale_after` ⇒ stale。缺字段时缺省 unverified/fresh，不惩罚存量笔记。

### 安全边界

- explore 只允许与 `--scope vault-readonly` 组合；`--scope published --view explore` 返回稳定错误 `share_view_scope_invalid`。缺省 `--view`（published）行为与既有输出零变更。
- explore 面严格只读：全部 `/explore*` 端点复用既有 share 认证（token 或 loopback `--no-auth`）与 GET-only 门禁，未认证请求返回 401 且不泄漏 bundle 数据；不写 vault 或 `.pinax/**`。
- `--once` 冒烟依次请求 `/explore`、`/explore/data.json` 与一个 note 有界预览，并对页面执行外链扫描（src=/href= 外部引用零命中）；token 模式下额外验证未认证 401。
- explore 永不出 loopback/LAN share 边界；对外发布仍走 `pinax publish` 管线。

## Security Gates

- Non-loopback hosts require `--allow-lan`; otherwise Pinax returns `share_allow_lan_required` before binding a socket.
- All share modes require `--readonly`; otherwise Pinax returns `share_readonly_required`.
- `vault-readonly` without token auth returns `share_auth_required`, except loopback-only `--no-auth` mode.
- Token file contents, token file paths, local vault roots, private note bodies, provider payloads, and `.pinax/**` internals must not appear in stdout, stderr, events, docs, fixtures, screenshots, or integration evidence.

See also [`publish`](./publish.md), [`api`](./api.md), and [`token`](./token.md).
