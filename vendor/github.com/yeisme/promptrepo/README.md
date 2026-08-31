# promptrepo

`promptrepo` 是一个独立、纯 Go 的提示词仓库 SDK。它提供用户级 repository
profile、不可变 catalog snapshot、确定性搜索、精确 `promptrepo://` 引用、模板
正文的 digest 校验读取，以及 staged installation receipt。它不执行 Prompt、不调用
模型，也不依赖 Template Registry 服务端或其内部包。

`promptrepo` is a standalone, pure-Go prompt repository SDK. It provides
user-scoped repository profiles, immutable catalog snapshots, deterministic
search, exact `promptrepo://` references, digest-verified template reads, and
staged installation receipts. It does not execute prompts, call models, or
depend on Template Registry server/internal packages.

## 模板寻址、检查与预览（计划 v0.3.0）

在未来的 v0.3.0 加性版本中，旧 solution ref 保持不变：

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

## 安装 / Install

```bash
go get github.com/yeisme/promptrepo@v0.2.0
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
```

See [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and
[docs/README.md](docs/README.md).
