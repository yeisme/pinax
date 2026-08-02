# Inferrum

Inferrum 是 yeisme 的矢量 + RAG 平台产品，提供统一 operator CLI、私有单组织 daemon，以及供 auctra / pinax / eikona 消费的共享 Go SDK。仓库、module、package、CLI、sidecar 与协议均使用 Inferrum 作为唯一产品身份。

## 定位

- **规范产品入口**：`inferrum` 同时提供 operator 命令与 `serve` daemon。
- **产品中立的矢量/RAG 抽象**：embedding provider（gemini/openai/ollama/fake）、统一 sidecar 协议（`inferrum.sidecar.v1`）、检索管线，域无关。
- 从 pinax `internal/semantic` + eikona `internal/visualindex` 抽取并公共化(各自的 `internal/` 禁止跨 module import,故抽出)。
- 战略定位与架构见根 [`docs/architecture/inferrum-vector-rag-platform.md`](../../docs/architecture/inferrum-vector-rag-platform.md)。

## 命令入口

构建规范 Inferrum CLI：

```bash
CGO_ENABLED=0 go build -o ./inferrum ./cmd/inferrum
./inferrum provider list --json
./inferrum --help
```

## 接入(消费方)

本地开发用 replace 指向本目录:

```
// cli/eikona/go.mod
require github.com/yeisme/inferrum v0.0.0
replace github.com/yeisme/inferrum => ../inferrum
```

通过 `Provider` / `Domain` / `Store` / `RetrievalPipeline` 接入(契约见根架构文档)。

## 组件

| 组件 | 说明 |
| --- | --- |
| `Provider` | 文本 → 向量。gemini / openai / ollama / fake 四个内置 provider。 |
| `SidecarClient` | 域多路复用的 sidecar 协议客户端(`inferrum.sidecar.v1`),stdin/stdout JSON。 |
| `Store` | 矢量存储入口(`VectorStore` 走 sidecar,`FakeStore` 内存测试)。 |
| `RetrievalPipeline` | 7 阶段 RAG 管线:embed → resolve permission → search → hydrate → rerank → assemble context → emit manifest。 |
| `Domain` | 各 CLI 注册的域适配器(~50-80 行)。 |

## 稳定契约

- 产品与 operator CLI：`Inferrum` / `inferrum` / `inferrum.*`。
- daemon：`inferrum serve`，实验性 `private_single_org` 部署面。
- SDK：`github.com/yeisme/inferrum` 与 package `inferrum`。
- sidecar：`inferrum-lancedb-sidecar`、Python package `inferrum_lancedb_sidecar` 与 `inferrum.sidecar.v1`。
- owner checkout：根仓库通过 `cli/inferrum` submodule 引用 `https://github.com/yeisme/inferrum.git`。

## 规范

OpenSpec 见 `openspec/`。子项目约定见 `AGENTS.md`。

`private_single_org` daemon 的构建、运行、证据与延期 gate 见 [`docs/inferrum-daemon.md`](docs/inferrum-daemon.md)。
