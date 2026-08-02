# Inferrum Agents

遵循根 [AGENTS.md](../../AGENTS.md)。

本子项目是独立维护的 Inferrum 矢量 + RAG 平台产品。唯一入口是 `inferrum` operator CLI 与 `inferrum serve` daemon；Go module、Go package、sidecar 和协议统一使用 Inferrum 身份。

## 技术栈与所有权

- Go SDK 与 `cmd/inferrum` CLI/daemon：纯 Go，正常构建和测试必须支持 `CGO_ENABLED=0`。
- LanceDB 集成：`tools/inferrum-lancedb-sidecar` Python 子进程；不要把 Python/LanceDB 绑定嵌入 Go 主进程。
- 实现计划、验证和收尾：本目录 `openspec/`。
- 产品、协议、operator、实现、QA 与 release 文档：本目录 `docs/`。
- 根仓库只通过 `cli/inferrum` Git submodule、跨项目架构文档和 skill profile 引用本项目。

## 产品中立铁律

SDK **不懂任何消费方的域语义、权限模型、业务实体**。矢量记录 = `Record` 信封,`Metadata` 是对 sidecar 不透明的 JSON。权限过滤由消费方域适配器(`Domain.ResolvePermission`)做,**从不在 sidecar 内**。各 CLI(eikona = `visual`、pinax = `kb`、auctra = `story`)只写自己的域适配器。

## 边界

- sidecar 协议 `inferrum.sidecar.v1` 是稳定契约；不提供旧产品协议别名。
- `inferrum.*` 是唯一 operator command namespace。
- sidecar 把 metadata 当不透明 JSON 存取,**从不解释它**。
- manifest 是证据不是库副本:绝不包含图片字节、原始 prompt、provider payload、secret 或 chain-of-thought。
- 不依赖 pinax / auctra / eikona 任何消费方模块(零 `internal/` import)。
- go.mod 只有 stdlib,无 AWS/S3 依赖(与 capsa 不同)。

OpenSpec 规范见 `openspec/`。

## 禁止事项

- 不得更改 Go module path `github.com/yeisme/inferrum`，除非先通过兼容迁移 OpenSpec。
- 不得改名 `inferrum-lancedb-sidecar`、改名 `inferrum.sidecar.v1` 或移动 `cli/inferrum`，除非新 OpenSpec 包含消费者清单、迁移窗口与 rollback。
- 不得在 sidecar 中解释消费方域语义或权限规则。
- 不得将 credential、raw prompt、provider payload、secret 或 chain-of-thought 写入日志、manifest、fixture 或测试证据。
- 不得直接修改根仓库或消费方代码；跨项目变更由根 session 负责集成。

## 验证命令

```bash
CGO_ENABLED=0 go test ./... -count=1
go build ./...
PYTHONPATH=tools/inferrum-lancedb-sidecar/src python3 -m unittest discover tools/inferrum-lancedb-sidecar/tests
openspec validate --all --strict
```

完成标准：Go/Python 测试通过，CLI 可构建，OpenSpec 严格校验通过，且没有未说明的稳定契约变化。
