# judgment-sdk (Go)

独立 Go module：`github.com/yeisme/judgment-sdk`（拟议发布名，当前仅作本地 module path；未发布）。纯 Go、零第三方依赖、`CGO_ENABLED=0` 兼容。许可证 Apache-2.0（见本目录 `LICENSE`）。

## 边界

- 不 import Aigora `internal/**`、不依赖 gateway 服务/数据库/供应商 SDK（由 `TestSourceHygiene` 在测试中持续断言）。
- 不读取环境或凭据：transport 一律注入，Authorization 由调用方通过 `HTTPTransportOptions.Authorize` 注入。
- 从不自动重试：一次 `Evaluate` 恰好一次 transport 调用；提交后超时分类为 `outcome_unknown` + `reconcile_first`。

## 验证

```bash
CGO_ENABLED=0 go test ./...
```

覆盖：90 条共享 conformance 向量（与生成 manifest 的 sha256 绑定）、spec 全部 Scenario、HTTP/stdio transport 边界、独立安装零凭据。向量由 `../schema/generate.py` 生成至 `testdata/`，禁止手改。

## 入口速览

- `ParseRequest(data, RequestOptions)` / `ParseCapabilities(data)` / `ParseResult(data, req, policy)`：严格解码+验证，失败返回带稳定 reason token 的 `*ValidationError`。
- `CheckCapabilities(caps, req, GateOptions)`：提交前能力门（primitive/上限/语言/底层 revision/概率/扩展）。
- `NewClient(transport, ClientOptions)`：`DescribeCapabilities`=`Capabilities(ctx)`、`Evaluate`、可选 `Reconcile`/`Cancel`（仅能力声明支持时）。
- `CanonicalJSON` / `Digest`：RFC 8785 / JCS 等价规范化。
- `FixtureTransport`：离线 fixture；`NewHTTPTransport` / `NewStdioTransport`：注入 transport。
