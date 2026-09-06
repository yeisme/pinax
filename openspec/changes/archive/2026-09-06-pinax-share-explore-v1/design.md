# Design — Vault Explore 自包含交互面（pinax-share-explore-v1）

## 1. 体验目标与 OKF 对齐

OKF `visualize` 的四件套（搜索、类型过滤、backlinks、图/列表切换）+ Pinax 独有的信任/新鲜度徽标。目标旅程：

```
$ pinax share start --scope vault-readonly --view explore
Share endpoint ready
  URL    http://127.0.0.1:8787/explore?token=…
  Mode   readonly · loopback · auth=token
Next: open the URL; Ctrl-C to stop.
```

页面（单文件，无外联）：

```
┌──────────────────────────────────────────────────────────┐
│ 🔍 [search title/tag/kind…   ]  kind▾ trust▾ fresh▾ [graph|list] │
├───────────────────────────────┬──────────────────────────┤
│                               │ auth-design              │
│        force-directed         │ reference · human ✓ fresh│
│        graph canvas           │ tags: auth, security     │
│   (● human  ◐ machine  ○ none)│ ──────────────────────── │
│                               │ Backlinks (2)            │
│  [auth-design]──[auth-runbook]│  gateway-design, securi… │
│                               │ Out (3) · same-tag (2)   │
└───────────────────────────────┴──────────────────────────┘
```

- 选中节点右侧出卡：元数据、信任徽标、backlinks/out/同标签邻居（计数与 id，不含正文）。
- "Load preview" 按钮：经带 token 的 `/explore/note/<id>` 拉取**有界**渲染预览（复用 note preview 有界投影），不嵌全文。
- broken link 以虚线边呈现（OKF "broken links MUST be tolerated" 精神）。

## 2. 数据投影 `pinax.explore_bundle.v1`

```json
{
  "schema_version": "pinax.explore_bundle.v1",
  "generated_at": "…",
  "counts": {"nodes": 412, "edges": 933, "truncated": false},
  "nodes": [{"id","title","kind","tags","trust","fresh","summary","updated_at"}],
  "edges": [{"from","to","broken":false}]
}
```

- 组装：graph 投影（nodes/edges/broken）+ index 投影（kind/tags/trust/fresh/summary）。无正文、无绝对路径（相对 vault 路径）、无 token。
- 上限（fail-safe 截断并置 `truncated=true`）：节点 5000、边 20000、title ≤160、summary ≤240、tags ≤8。
- 信任信号依赖 `pinax-okf-trust-discovery-v1`；该 change 未落地时字段缺省 `unverified`/`fresh`，不阻塞。

## 3. share 接线

- `pinax share start --view explore`（新 flag；`--view` 缺省 `published` 保持现状）：
  - `--scope vault-readonly` 时启用 explore；`--scope published` + explore 返回稳定错误提示组合用法。
  - 复用现有端点生命周期：host/port/auth（token-file/loopback no-auth）、`--allow-lan`、`--once`。
- 路由：`/explore`（HTML）、`/explore/data.json`（bundle）、`/explore/note/<id>`（有界预览，经既有 redaction + `paneUnsafe` 同源 denylist 思路）。
- HTML 内嵌渲染数据（单文件自包含）或 `data.json` 按需取；默认内嵌，保证"一个文件打开即用"；`--no-embed` 留给大 vault（data.json 模式）。

## 4. 自包含页面合同

- 零外联：无 CDN、无字体/脚本外链；JS/CSS 全内联。构建时以静态检查（golden + 扫描 `src=|href=` 外链）强制。
- 布局：graph（SVG 力导向，纯手写或内联轻量布局，不引外部库）与 list（表格 + 即时过滤）双模式切换；prefers-reduced-motion 时禁用动画。
- 搜索/过滤纯客户端（title/tags/kind/trust/fresh）；URL hash 记录过滤态可分享（`#q=auth&trust=human`）。
- 键盘可达：`/` 聚焦搜索、↑↓ 列表导航、Enter 打开卡。

## 5. 安全与红线

- explore 面严格只读；所有数据端点走既有 share 认证（token 或 loopback no-auth）。
- 数据组装复用 bounded projection + redaction 门（`ApplyProjectionRedaction` 路径）；note 预览端点拒绝任何写语义。
- `--once` 冒烟：请求 `/explore`、`/explore/data.json`、一个 note 预览后退出，纳入 testscript。
- 不新增任何 vault 写、`.pinax/**` 只读（bundle 内存组装，不落盘）。

## 6. 测试策略

- bundle 组装单测：截断、broken 标记、无正文递归扫描。
- golden：小 fixture vault 的 HTML 输出（含内联数据）稳定哈希（数据含时间戳字段做归一化后比对）。
- testscript e2e：`share start --view explore --once`（loopback no-auth）三端点 200 + 认证缺失 401 + 外联扫描 0 命中。
- 回归：默认 `share start`（无 `--view`）输出与现状 golden 一致。

## 7. 风险与取舍

- **力导向布局自研成本**：5000 节点级用简单迭代布局（Barnes-Hut 不必要）；list 模式兜底大 vault。不引 d3/cytoscape（外联与供应链红线）。
- **内嵌数据体积**：5000 节点 × ~300B ≈ 1.5MB，单文件可接受；超限走 data.json 模式。
- **与 publish 的边界**：explore 永不出 loopback/LAN（外部发布走 publish 管线），spec 冻结该边界防漂移。
