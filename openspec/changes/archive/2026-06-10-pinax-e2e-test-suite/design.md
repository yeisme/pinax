## Context

目前 Pinax 项目的 E2E 测试位于 `tests/e2e`，虽然使用了 `testscript` 作为主测试驱动，但测试场景仍局限于最基础的 CLI 交互。当前测试套件面临以下痛点：
1. 缺乏对 `--json` Envelope 统一结构体（如 `spec_version`, `mode`, `status`）的自动化契约断言。
2. 缺乏物理 Markdown 笔记变动对底层 SQLite 索引投影（例如出链、入链关系）的一致性黑盒验证。
3. 同步功能依赖外部 API/CLI 且直接暴露在有网环境，缺乏 Mock 机制，导致 CI 环境中同步逻辑与事件流（`--events`）未被有效覆盖。
4. 缺乏对 Token 等敏感身份凭据泄露的防护与脱敏（Redaction）规则测试。

## Goals / Non-Goals

**Goals:**
* 在现有的 `testscript` 套件中扩充多模式渲染断言（Summary/Agent/JSON/Events），确保输出符合 `ai-native-cli-output-contract` 规定。
* 编写双向链接、元数据增删改后，后端 SQLite 投影的一致性与自愈黑盒校验脚本。
* 设计并实现一套 fake 外部 CLI（如 fake `lark-cli` 和 `ntn`）注入机制，支持在 CI/本地无网络环境下进行 Sync 与流式事件（`--events`）的闭环测试。
* 在 E2E 脚本中引入敏感信息脱敏检测用例，拦截任何在日志和 stdout 中泄露未掩码 Token 的行为。

**Non-Goals:**
* 不涉及浏览器界面的 TUI / UI 交互测试。
* 不与真实飞书/Lark 公网环境及云端 API 进行网络级数据集成。
* 不引入 `database/sql` 绕过服务层直接读取 SQLite 进行断言，所有投影状态应通过 CLI 本身对外提供的查询入口（如 `pinax query`）进行审计。

## Decisions

* **Decision 1：使用 Testscript 黑盒校验代替细粒度的 Handler Mock 单元测试**
  * *Rationale*：由于 Pinax 作为一个 Agent 命令行工具，其输出结构体对于上层 AI 具有高度敏感性，黑盒测试可以最真实地模拟 AI 代理在调用 CLI 时遇到的 stdout/stderr 分离、信封结构和脱敏效果，确保契约稳定性。
* **Decision 2：在 Testscript Setup 阶段动态注入编译好的 Fake CLI 模拟外部 Provider**
  * *Rationale*：通过在 `TestMain` 或 setup 中编译 fake 二进制并置于 testscript 沙盒的 `PATH` 最前端，从而拦截对外部 `lark-cli`、`ntn` 的调用，并返回预期的 mock 事实。避免了硬编码网络请求或复杂的运行时钩子，支持纯本地并发测试。
* **Decision 3：基于 CLI 查询指令（如 `pinax query`）验证底层 State Projection**
  * *Rationale*：保证测试行为不对后端 GORM Repository 之外的物理 SQLite 文件做侵入式读取，满足 `AGENTS.md` 中“用例逻辑隔离”和“不绕过 App Service”的软件架构规则。

## Risks / Trade-offs

* **[Risk] Testscript 运行缓慢，每次都要 go build 二进制**
  * $\rightarrow$ *Mitigation*：在 `TestMain` 中统一将 `pinax` CLI 及 fake CLI 仅编译一次到共享临时目录中，并在 testscript 执行的 params 参数中复用该 `PATH`，将编译成本分摊到每个测试案例，保持 E2E 套件能够在数秒内迅速跑完。
* **[Risk] 敏感数据匹配误报或漏报**
  * $\rightarrow$ *Mitigation*：在测试用例中明确硬编码特殊特征字符串（如 `mock_token_secret_12345`），并在命令执行后使用 `! stdout 'mock_token_secret_12345'` 和 `! stderr` 进行反向校验，以提供精准的漏报安全网。
