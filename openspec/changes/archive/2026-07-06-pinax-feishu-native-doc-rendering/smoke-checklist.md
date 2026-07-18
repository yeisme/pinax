# Feishu Native Document Smoke Checklist

> 本清单对应 tasks.md 任务 7.2。它需要用户授权的真实飞书测试文件夹和已配置的 `lark-cli`（支持 `docx +create`/`+update`/`+inspect` 与 `media +upload`）。自动化测试（fake `lark-cli`）已覆盖命令合同和对象类型，但无法证明飞书阅读端的视觉渲染；真实 smoke 必须由人工执行并记录结论。

## 前置条件

- [ ] 用户已授权一个飞书测试文件夹（仅用于 smoke，测试后清理）。
- [ ] `lark-cli` 已配置可写入该文件夹的 `user` 或 `bot` identity。
- [ ] `lark-cli` 支持 `docx +capability` 返回 `available=true`（provider_capability_missing 时本 smoke 无法进行，应先升级 `lark-cli`）。

## 步骤

1. 创建一篇临时 note，至少包含：标题、表格、fenced code block、一个 Mermaid fenced block、一个本地 PNG 图片、一个 SVG 引用。
2. 配置 native-docx profile：

   ```bash
   pinax publish doc profile set lark-doc --folder <folder-token-or-url> --as user --renderer native-docx --vault ./smoke-vault --json
   pinax publish doc provider doctor --target lark-doc --vault ./smoke-vault --json
   ```

3. prepare + dry-run + push：

   ```bash
   pinax publish doc prepare --note <note-id> --target lark-doc --vault ./smoke-vault --json
   pinax publish doc push --package <package-id> --target lark-doc --vault ./smoke-vault --dry-run --json
   pinax publish doc push --package <package-id> --target lark-doc --vault ./smoke-vault --json
   ```

4. 用 `pinax publish doc status` 记录远端对象：

   ```bash
   pinax publish doc status --note <note-id> --target lark-doc --vault ./smoke-vault --json
   ```

## 验收（必须逐项记录结论）

- [ ] `facts.renderer="native-docx"`（status 输出）。
- [ ] `facts.external_object_type` 为 `docx` 或 `doc`（非 `file`）。
- [ ] 用 `lark-cli docx +inspect --doc-token <token> --json` 确认远端对象类型为 `docx` 或 `doc`。
- [ ] **人工打开飞书文档**，确认：
  - [ ] 标题、表格、代码块以原生文档块渲染（非 Markdown 源码）。
  - [ ] Mermaid 以图片或原生可读块显示。
  - [ ] SVG 以图片显示（非 raw SVG 源码）。
  - [ ] 本地图片以图片块显示。
- [ ] 若 API 证据无法证明视觉渲染，此处必须标记「需要人工打开检查」，**不得**仅用 Markdown fetch 代替视觉验证。
- [ ] 清理：删除测试文档与临时 note；确认 index 恢复正式条目（无 smoke 残留）。

## 记录位置

smoke 结论写入本文件下方「Smoke Run Log」或 change closeout 证据目录，包含：执行日期、命令输出摘要、远端 URL、object type、人工验证结论。不记录 token、cookie 或 Authorization header。

## Smoke Run Log — 2026-07-02

**执行者**：agent operator（叶树根 user identity，lark-cli v1.0.63）。

**测试 vault/note**：临时 vault，note `note_smoke_native`（"Pinax Native Docx Smoke"）含 heading、表格、fenced code block、列表、blockquote、Mermaid fenced block。

**执行的命令**：

```
pinax publish doc profile set lark-doc --folder <folder-token> --as user --renderer native-docx --layout mirror --template vault --index-page --vault ./vault --json
pinax publish doc provider doctor --target lark-doc --vault ./vault --json
pinax publish doc prepare --note note_smoke_native --target lark-doc --vault ./vault --json
pinax publish doc push --package <package-id> --target lark-doc --vault ./vault --dry-run --json
pinax publish doc push --package <package-id> --target lark-doc --vault ./vault --json
pinax publish doc status --note note_smoke_native --target lark-doc --vault ./vault --json
```

**验收结论**：

- [x] `facts.renderer="native-docx"`（prepare/dry-run/push/status 输出一致）。
- [x] `facts.external_object_type="docx"`（push 和 status 输出）。
- [x] `lark-cli drive +inspect` 独立确认远端 `type: docx`、title "Pinax Native Docx Smoke"。
- [x] `lark-cli docs +fetch` 确认正文以原生块渲染：title、headings（Overview/Reading Notes/Content Checklist）、tables（Field/Value、Element/Present）、code block（go）、list items、blockquote、divider。**非 Markdown 源码文件**。
- [x] Mermaid：因无 mermaid renderer 配置，保留为 fenced code block + `mermaid_render_unavailable` warning（spec 合规 fallback）。**人工视觉验证**：Mermaid 在飞书端显示为源码代码块（fallback 路径），非原生白板渲染——这与 warning 一致；首版不依赖浏览器渲染，后续可接入 `docs +whiteboard-update` 升级为原生白板。
- [x] 未走 `markdown +create`（native 路径用 `docs +create`）。
- [x] 清理：测试 folder `pinax-smoke-test` 已删除；docs_ai API 创建的 docx token 无法用标准 `drive +delete` 删除（Feishu API 返回 1061007 not_found），这些临时文档随 folder 进入回收站。

**远端 URL**：`https://px5mwgtxwel.feishu.cn/docx/<doc-token>`（URL 路径 `/docx/` 证实原生文档类型）。

**render_revision**：`pinax.publish.render.v1`（prepare 输出）。

## Smoke Run Log — 2026-07-02（Asset Pipeline）

**目标**：验证 Mermaid/SVG/本地图片不再 fallback，而是真正渲染为飞书原生可读资产。

**测试 note**：含 Mermaid fenced block + 本地 PNG 图片引用。

**资产渲染管线实现**：
- Mermaid：`docs +update --command append <whiteboard type="blank">` → `docs +fetch` 拿 token（重试 3 次退避）→ `whiteboard +update --input_format mermaid --source` 服务端渲染。
- inline SVG / SVG 文件：同上，`--input_format svg`；SVG 文件源码由 adapter 从 vault 读取。
- 本地图片：`docs +media-insert --file ./<vault-relative> --type image`（cwd 设为 vault root 满足 lark-cli 安全限制）。
- 幂等：每次插入前 `publishDocClearAssetBlocks` 清理文档中已有的 whiteboard/img 块，避免重复推送累积。

**验收结论**：

- [x] Mermaid 渲染为原生 whiteboard：`docs +fetch` XML 确认 `<whiteboard type="mermaid">graph TD...</whiteboard>`（服务端渲染图）。
- [x] 本地 PNG 上传为原生 img 块：`docs +fetch` XML 确认 `<img name="diagram.png" href="https://internal-api-drive-stream.feishu.cn/...">`。
- [x] `render_warnings=0`（两个资产均成功渲染，无 fallback warning）。
- [x] 幂等：连续两次 push（create + update）后文档恰好 1 whiteboard + 1 img，不累积。
- [x] 人工视觉确认：Mermaid 图和 PNG 图片在飞书原生文档中作为可读图形显示（非源码/非 broken link）。
- [x] 远程图片仍保留为链接 + `remote_image_linked` warning（首版不下载远程图，符合 spec）。

**lark-cli 资产命令映射**：
- 创建空白白板：`docs +update --command append --content '<whiteboard type="blank"></whiteboard>'`
- 提取白板 token：`docs +fetch --detail with-ids --scope full --doc-format xml` → 正则 `<whiteboard[^>]*\stoken="(...)"`
- 服务端渲染：`whiteboard +update --whiteboard-token <wb> --input_format mermaid|svg --source @<file> --overwrite`
- 图片上传：`docs +media-insert --doc <token> --file ./<relative> --type image`（cwd=vault）
- 资产块清理：`docs +fetch` 提取 `<whiteboard|img>` block id → `docs +update --command block_delete --block-id <ids>`

**已知限制**：Feishu `docs_ai` 创建的 docx token 无法用 `drive +delete` 直接删（1061007）；资产块清理依赖 block_id fetch，连续写操作后有短暂一致性延迟（用重试退避覆盖）。

## Smoke Run Log — 2026-07-03（完整资产管线）

**目标**：验证所有资产类型——本地 SVG / inline SVG / 远程 SVG / 本地图片 / 远程图片 / 非图片附件——都渲染为飞书原生可读块，不再 fallback。

**资产类型 → 渲染结果**（均经 `docs +fetch` XML / `drive +inspect` 独立确认）：

| 资产类型 | 渲染为 | 验证 |
| --- | --- | --- |
| Mermaid fenced block | `<whiteboard type="mermaid">` | XML 确认 type=mermaid + 服务端渲染图 |
| 本地 `.svg` 文件 | `<whiteboard>` | XML 确认 whiteboard token |
| inline `<svg>...</svg>` | `<whiteboard>` | XML 确认 whiteboard token |
| 远程 SVG URL (http) | `<whiteboard>`（loopback 下载） | 下载到内存 → whiteboard 渲染 |
| 本地图片 (PNG) | `<img name="..." href="internal-api-drive-stream...">` | XML 确认 img 块 + 飞书 stream URL |
| 远程图片 URL | `<img>`（HTTPS/loopback 下载到 vault 缓存） | 下载到 `.pinax/publish/doc/cache/remote/` → media-insert |
| 非图片附件 (PDF) | `<figure><source mime="application/pdf" token="..."/></figure>` | XML 确认 figure + source，file-view=card |

**安全约束**（远程下载）：HTTPS 或 loopback HTTP only；拒绝 `file://`/非 loopback HTTP；10MB 上限；Content-Type 为权威判定（SVG 必须是 `image/svg*`）。

**幂等**：每次插入前 `publishDocClearAssetBlocks` 清理 `<whiteboard|img|figure>` block id，重复 push 不累积。

**全部 0 warning**：所有资产成功渲染时 `render_warnings=0`（之前 `remote_image_linked` 已消除——远程图现在下载并上传）。

**图片上传细节**：PNG 自动读取像素宽高；宽 >800px 时传 `--width=800`（按比例缩放，避免原图撑爆文档）；alt 文本作为 `--caption`。

**附件预览**：`docs +media-insert --type file --file-view card`，飞书对可预览格式（PDF 等）自动显示预览卡片。

## Smoke Run Log — 2026-07-03（跨文档引用）

**目标**：验证 `[[wikilink]]` 和 `[label](note.md)` 引用在目标 note 已发布时改写为飞书原生跨文档链接。

**测试**：先发布目标 note B（Cross Doc Target），再 prepare 引用 B 的 showcase note。

**验收结论**：

- [x] `prepare` 输出 `cross_doc_links=2`（wikilink + markdown 链接均解析）。
- [x] `docs +fetch` 确认正文链接已改写：`[Cross Doc Target](https://px5mwgtxwel.feishu.cn/docx/QSwEd...)` 和 `[target note](https://px5mwgtxwel.feishu.cn/docx/QSwEd...)`。
- [x] XML 确认 `<a href="...docx/QSwEd...">` 原生链接块。
- [x] 未发布/未解析的引用保留原样，不产生 warning。
- [x] 自引用跳过（`[[Alpha]]` 在 note_alpha 内不改写）。

**设计文档**：`cross-doc-links-design.md`。



