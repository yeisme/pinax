# provider-mocking-integration Specification

## Purpose
TBD - created by archiving change pinax-e2e-test-suite. Update Purpose after archive.
## Requirements
### Requirement: Network-isolated Provider CLI Mocking
测试套件 SHALL 具备在本地沙盒执行环境中动态注入 Mock 二进制文件（例如伪造的 `lark-cli` 和 `ntn`）的能力，以实现无网络环境的流式事件与同步机制覆盖。

#### Scenario: Sync Offline Simulation with Event Stream
- **WHEN** 拦截外部 API 调用，往 PATH 中注入伪装命令并执行带有 `--events` 的 `pinax sync` 同步时
- **THEN** 测试脚本能正确捕获 stdout 中流式 NDJSON 的开始 (start) 与结束 (end) 契约事件

