# promptrepo

## Agent 提示包合同（实验）

新增 `RecipeV1`、`RecipeStepV1`、`InputBindingV1` 与 `PromptPackageV1`。`RecipeOrder` 校验静态 DAG 并返回稳定顺序；`ValidatePromptPackage` 和 `VerifyPromptPackageFiles` 校验跨平台路径、步骤状态、inventory 与逐文件 digest。`DecodePromptJSON` 复用已有严格 JSON 解码，拒绝重复 key、过深输入和未知 typed 字段。

这些能力不改变现有 `RenderTemplate`、catalog、TemplateContract 或引用形状。它们没有文件、网络或 Provider 副作用；会话、资料导入、用户确认和文件导出由 Template Registry 消费应用拥有。合同目前使用 `promptrepo.recipe.v0.1` 与 `promptrepo.prompt-package.v0.1`，不得把待前序输出的步骤标为可直接运行。

`promptrepo` 是一个独立、纯 Go 的提示词仓库 SDK。它提供用户级 repository
profile、不可变 catalog snapshot、确定性搜索、精确 `promptrepo://` 引用、模板
正文的 digest 校验读取，以及 staged installation receipt。它不执行 Prompt、不调用
模型，也不依赖 Template Registry 服务端或其内部包。

`promptrepo` is a standalone, pure-Go prompt repository SDK. It provides
user-scoped repository profiles, immutable catalog snapshots, deterministic
search, exact `promptrepo://` references, digest-verified template reads, and
staged installation receipts. It does not execute prompts, call models, or
depend on Template Registry server/internal packages.

## 模板寻址、检查与预览（v0.3.0）

从 v0.3.0 起，旧 solution ref 保持不变：

```text
promptrepo://official/audio/podcast-narration@1.0.0?locale=zh-CN
```

SDK 另提供 `ParseTemplateAddress` / `FormatTemplateAddress` 表达不可变模板
投影。它仍以相同 solution identity 为底座，但 query 固定按
`kind,locale,role,path,selector,digest,snapshot` 排序。例如：

```text
promptrepo://official/audio/podcast-narration@1.0.0?kind=template&locale=zh-CN&role=main&path=prompts%2Fmain.zh-CN.md&digest=sha256%3A...&snapshot=sha256%3A...
```

这不是 repository source URI：profile 的 `file://`、`git+https://`、
`github://`、`s3://` 仍只用来同步 catalog；Address 只在已经同步的 solution
内标识模板、可选 selector 和不可变 digest/snapshot。user/project scoped source
alias 属于后续 profile/source 路由工作，本阶段不改变这些 source。

`engine.Manager` 保持原有 `Client` 能力，并额外实现可选的 `Inspector`、
`ContractResolver`、`Validator`、`Renderer` 和 `Previewer`。`Render` 只处理调用者提供的内存正文；
`Preview` 才会经既有 adapter 作受 digest 校验的只读加载后调用 Render。输入定义、
license、permissions 和可选 contract digest 由新 `TemplateContract` 随 inspect、
validate、preview 请求/结果显式携带；不会写入已发布 catalog 或 state。消费者应先
inspect 获取输入状态与 readiness，再调用 preview；不应从提示词正文猜测输入。
若提供 contract digest，它必须是小写的 `sha256:<64 hex>`；空 digest 允许用于草稿
contract。

Template Registry 生成的 `promptrepo.template-contract.v0.1` companion 可由
`ContractResolver.ResolveTemplateContract` 从同一已解析 source 读取。Git/GitHub
按 exact commit 读取并返回 `consistency=snapshot_pinned`；file/S3 因已发布 catalog
没有 contract digest，只能以 sidecar 自身 digest 与 template binding 校验当前对象，
因此保守返回 `consistency=content_bound`。SDK 按 `<solution>/prompts/...` 到
`<solution>/contracts/<role>.<locale>.json` 的约定寻址，并同时校验 sidecar digest、
solution identity、template path 与 template digest。第三方 source adapter 可选择实现
`source.CompanionReader`；旧 `source.Adapter` 方法集保持不变。

本 SDK 不新增独立 CLI，Sonora/Eikona 的命令尚未随本模块发布；公开 README 不把未来
命令形态当作可运行命令记录。

preview 是**不执行**的本地内存渲染：它严格替换已声明的 `{{name}}`，校验未知、缺失、
类型、enum 与约束，并只报告 readiness、rendered digest、字节/字符数以及
`provider_calls=false`、`state_writes=false`、`usage_recorded=false`。安全的
JSON/YAML 结果不含模板或渲染正文。若某个 CLI 需要把内容写为本地文件，显式
`--output <path>` 是该消费者自己的责任，不是 inspect/preview 的默认副作用。
Address 的 selector grammar 已预留用于后续 conformance-tested selector engine；本
v0.3 preview 对非空 selector 以 `SELECTOR_UNSUPPORTED` fail closed。

## 结构化模板文档（下一 additive minor，尚未发布）

当前开发分支新增独立的 `DocumentResolver`、`DocumentLoader` 和
`DocumentSelector` 可选接口；现有 `Client`、`TemplateRole`、`TemplateContent`、
`ReadTemplate`、`Render` 和 `Preview` 均不加字段、不改变行为。Template Registry
生成的 `promptrepo.template-document.v0.1` descriptor 位于：

```text
<solution>/contracts/documents/<role>.<locale>.document.json
```

descriptor 必须与 package、solution、version、role、locale、template path 和原始
source digest 完全一致。它显式声明 `markdown`、`text`、`json`、`yaml` 或 `jsonl`
格式、media type、大小/深度上限、selector、Schema ref/digest 与 compiler profile
ref/digest；扩展名只用于交叉校验，不能单独决定格式。

JSON/YAML 被严格归一化到 JSON 数据模型，并以 RFC 8785 JCS 计算 canonical digest；
YAML 只接受无 anchor、alias、merge key、自定义 tag、重复键和非 JSON 数值的安全子集。
JSONL 要求每行是带唯一稳定 ID 的 object、使用 LF、以换行结束，并按记录流式校验，
首版只支持 `jsonl-id:<record-id>` 精确定位。结构化 selector 还包括
`heading:<text>`、`json-pointer:/path` 和 `yaml-pointer:/path`。

`LoadedDocument` / `SelectedDocument` 的 `Body` 和 `Value` 仅存在于调用进程内存，
带有 `json:"-" yaml:"-"`，普通 machine projection 只输出 refs、digests、格式、
Schema/compiler lineage、大小和 readiness。Promptrepo 只校验这些声明及来源绑定；它
不执行 repository supplied compiler，也不实现 Scaena 的角色连续性规则或领域 Schema
语义。旧 Preview 仍不消费 selector；结构化定位必须显式调用 `SelectDocument`。

## 受限 UI template 合同（下一 additive minor，开发中）

当前开发分支新增独立的 `UITemplateAddress`、`UITemplateBundleV1`、
`UITemplateInspector` 与 `UITemplateLoader`，不会扩宽已发布的 `TemplateAddress`。
UI template 使用 `kind=ui-template`，canonical query 顺序固定为
`kind,locale,role,path,digest,snapshot`：

```text
promptrepo://official/scaena/storyboard-review@1.0.0?kind=ui-template&locale=zh-CN&role=review&path=ui%2Freview.zh-CN.html&digest=sha256%3A...&snapshot=sha256%3A...
```

一个 bundle 只包含声明式 HTML fragment、独立 CSS、slots、security profile、limits、
content digest 与 snapshot。HTML 用 `data-promptrepo-slot="<name>"` 标记注入点；slot
只声明 name、kind、required 和 cardinality，不包含 callback、endpoint、HTTP method
或 consumer mutation。`static-review-fragment-v1` profile 只接受明确列出的展示元素和
静态属性，并 fail closed 地拒绝 script、form controls、iframe/object/embed、SVG/MathML、
事件/URL/inline-style 属性、framework/template directive、除 slot marker 外的 `data-*`、
外部 CSS、`url()`、`@import`、`expression()`、危险 at-rule 和 parser error；SDK 不执行
sanitizer rewrite，也不运行或渲染模板。

V1 的 256 KiB HTML、256 KiB CSS、512 KiB body、64 slots 与 64-byte slot name 是
**单个 bundle 的安全边界**，不是项目资产数量、镜头数量或衍生资产数量上限。高质量
AI 电影/短剧可以拥有大量独立资产；具体产品只应给出工作量、复用和打包建议，不由本
合同实施全局 quota 或 hard cap。

`CanonicalUITemplateDigest` 使用长度分隔的 canonical byte stream，并覆盖 identity、
normalized slots/security/limits 与原始 HTML/CSS bytes；snapshot 是独立绑定。exact
load 同时验证 address digest、bundle digest 和 snapshot。`UITemplateBundleV1` 的
HTML/CSS 字段带 `json:"-" yaml:"-"`；inspect 只返回 metadata、大小和 validation，
不返回正文、consumer values 或私有绝对路径。`uitemplatefs` 只提供有界、无网络的本地
fixture loader。

Owner 边界保持明确：Promptrepo 负责地址、DTO、校验和摘要；Template Registry 负责
CLI-authored metadata、不可变发行、安装、审计和回退；Scaena 负责渲染、slot 注入、
本地 action、安全会话与审阅领域状态。停用新 optional interface 即可回滚，旧
template/document/catalog/state 行为不需要迁移。

## 统一仓库集合与策略判定（开发中，尚未发布）

当前开发分支新增 additive `RepositorySetReader` 与 `PolicyEvaluator`，既有 `Client`
保持不变。`EffectiveRepositorySet` 将四层调用时输入组合为安全投影：

```text
session exact > project pin > user preference > organization default > official fallback
```

这一顺序只决定候选位置，不授予权限。`EvaluateRepositoryPolicy` 独立求 source health、
organization/project/domain policy、operation permission、minimum trust、rights 和 required
capability 的交集；任意 deny、quarantine 或 blocker 都返回稳定 reason code，exact ref
不能绕过。

embedded `engine.Manager` 只从既有 state 读取 canonical user profile；organization、
project 和 session binding 由调用方提供且不会被 SDK 持久化。输出只含 repository ID、
scope ref digest、health、trust、policy/snapshot digest、readiness 和 registered action，
不含 raw scope ref、source URL、credential、模板正文或输入值。首版 schema 为：

```text
promptrepo.repository-set.v0.1
promptrepo.policy-decision.v0.1
promptrepo.management-projection.v0.1
```

跨项目 automation 使用 `promptrepo.repository.sync`、`promptrepo.catalog.search`、
`promptrepo.template.inspect` 等 stable `operation_id`；每个领域 CLI 仍保留自己的命令树。

## 安装 / Install

```bash
go get github.com/yeisme/promptrepo@v0.3.0
```

模块要求 Go 1.24 或更高版本；常规构建和测试支持 `CGO_ENABLED=0`。
The module requires Go 1.24 or later; normal builds and tests support
`CGO_ENABLED=0`.

## 快速开始 / Quick start

```go
package main

import (
    "context"

    "github.com/yeisme/promptrepo"
    "github.com/yeisme/promptrepo/engine"
)

func main() error {
    client, err := engine.New(engine.Options{})
    if err != nil {
        return err
    }
    _, err = client.AddRepository(context.Background(), promptrepo.AddRepositoryRequest{
        Profile: promptrepo.RepositoryProfile{
            ID: "official", Source: "github://yeisme/prompt-templates", Trust: "official",
        },
    })
    return err
}
```

## 稳定合同 / Stable contract

- State schema: `promptrepo.state.v0.1`
- Catalog schema: `promptrepo.catalog.v0.1`
- Receipt schema: `promptrepo.stage_receipt.v0.1`
- Exact reference: `promptrepo://official/audio/podcast-narration@1.0.0?locale=zh-CN`

默认 state 位于 OS user config/cache 目录的 `yeisme/promptrepo`。State 由 engine
原子写入并使用跨进程锁；未来 schema 会以 `STATE_SCHEMA_TOO_NEW` fail closed。

Built-in sources are `file://`, Git (`git+file`, `git+https`, `git+ssh`,
`github://`), and anonymous read-only `s3://`. Profiles hold credential
references only, never credential values. See [docs/architecture.md](docs/architecture.md).

### Graph Kit structured-document conformance

Graph Kit 是现有 structured-document 能力的一种组合约定，不是新的 Promptrepo
领域 API。调用方继续组合 `DocumentResolver`、`DocumentLoader` 和
`DocumentSelector`：manifest 与 lens、view、validator 等 child 都使用已有
`TemplateRole`、descriptor、source digest 和 canonical digest。

对 Git/GitHub source，`SyncRepositories` 先把配置的 branch、tag 或 revision
解析为 exact commit；closure 中每个文档必须返回与 manifest 相同的
`SnapshotMetadata`。manifest 缺 child、descriptor/source digest 漂移、可变
snapshot 被直接用于读取、路径逃逸或 selector 不兼容时均 fail closed。安全的
JSON/YAML 投影只包含摘要和 snapshot lineage，不包含结构化正文。

仓库内 conformance test 使用本地 `git+file://` fixture，不访问网络，也不包含
Auctra 或具体小说数据。`github://owner/repository` 仅是 Git HTTPS remote 的规范化
入口；真实 GitHub canary 与发布仍需要维护者单独授权。

Consumer handoff：Graph Kit 不需要新增 SDK surface；Auctra 与 Registry 可继续精确
固定已发布的 `github.com/yeisme/promptrepo v0.4.0`。本变更没有创建 tag、发布模块或
写入远端。

## 私有 v0.1.0 迁移 / private v0.1.0 migration

This repository is the canonical public home for the SDK. Stable `v0.2.0` was
promoted from `v0.2.0-rc.1` after consumer canaries. It was extracted without behavioral change from private tag
`sdk/go/promptrepo/v0.1.0` at commit
`e59e5a060b98125e55197cb9f5ed9179cdacc46a`.

Consumers replace only the module/import prefix:

```text
github.com/yeisme/backend-server-template-registry/sdk/go/promptrepo
→ github.com/yeisme/promptrepo
```

The private v0.1.0 source is immutable. It will be deprecated after public
cutover, not modified or retagged. See [CHANGELOG.md](CHANGELOG.md) and
[docs/release.md](docs/release.md) for RC, stable, rollback, and migration
gates.

## 开发 / Development

```bash
go mod verify
go test ./...
go vet ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./...
openspec validate promptrepo-public-sdk-extraction-v1 --strict --no-interactive
openspec validate promptrepo-template-address-inspect-preview-v1 --strict --no-interactive
openspec validate promptrepo-structured-document-v1 --strict --no-interactive
openspec validate promptrepo-unified-management-v1 --strict --no-interactive
openspec validate promptrepo-ui-template-contract-v1 --strict --no-interactive
```

See [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and
[docs/README.md](docs/README.md).
