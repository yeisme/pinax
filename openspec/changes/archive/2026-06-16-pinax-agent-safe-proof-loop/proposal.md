## Why

CEO review 判断 Pinax 当前功能面已经足够宽，最大风险是缺少一个真实、可复现、可向用户展示的核心闭环。继续增加 provider、briefing、Cloud 或平台能力会稀释定位；Pinax 需要先证明自己作为本地优先 Markdown vault 工作台和 agent-safe context provider 的价值。

本变更把后续优先级锁定为 Proof Loop：用户和 agent 能在一个真实 vault 中完成 capture、index/search、health diagnosis、plan、snapshot、safe apply 和 bounded context 读取，并能从证据中看见 Pinax 比直接让 agent 改 Markdown 更安全。

## What Changes

- 新增一条可跑通的 Pinax Proof Loop，覆盖真实 vault 初始化、内容捕获、索引刷新、搜索/关系读取、健康检查、维修/整理计划、版本快照、受控 apply、MCP/JSON/agent 读取。
- 新增或收敛集成测试入口，要求端到端证据写入 `cli/pinax/temp/integration-test-runs/<run-id>/`，并覆盖 stdout、stderr、events、receipts、fixtures 的脱敏红线。
- 重排用户文档和 command map 主路径，突出五条核心工作流：Capture、Retrieve、Diagnose、Plan、Apply safely。
- 明确非目标：本变更不新增 provider、不扩展 briefing、不实现 hosted Cloud、不实现云端全文搜索、不让 agent 自动写 vault。

## Capabilities

### New Capabilities

- `pinax-agent-safe-proof-loop`：定义 Pinax 面向真实用户和 agent 的最小可演示闭环、证据标准、主路径文档和安全边界。

### Modified Capabilities

- 无。

## Impact

- 影响测试：新增或更新 `tests/e2e` / testscript / integration evidence wrapper，覆盖真实 vault proof loop。
- 影响 CLI 文档：`README.md`、`docs/README.md`、`docs/commands/README.md`、相关命令页需要把主路径前置。
- 影响输出合同：所有 proof loop 命令必须保持 default summary、`--json`、`--agent` 的 stdout/stderr 分离和脱敏。
- 影响实现范围：只连接已有能力并补缺端到端证据；不引入新的业务模块或平台依赖。
