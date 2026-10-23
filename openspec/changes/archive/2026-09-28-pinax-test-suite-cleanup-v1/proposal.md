# 测试清理

## Why

证据工具测试同时位于工具归属包和 e2e 包，直接调用库的检查也会启动 e2e TestMain 构建三个可执行文件。部分历史断言只验证整页哈希、源码字符串或同一个已退役入口的多个子路径，增加维护成本而未提供对应的行为证据。

## What Changes

- 把四个 evidence 合同/脱敏测试从 e2e 归回工具包，合并重复失败用例，保留完整证据目录、退出码、schema、脱敏和反向泄漏检测。
- 删除 Explore 整页 SHA 断言与同步冲突 Cobra 源码字符串扫描，保留现有页面合同、安全和实际冲突流程测试。
- KB 退役测试保留顶层 unknown-command 和禁止旧 compat stub，删除无关 vault 初始化及重复子路径。

## Capabilities

### New Capabilities

- `test-suite-maintenance`: 本次测试归属、精简与保留覆盖的验收要求。

### Modified Capabilities

- 无运行时合同变化。

## Impact

仅测试与本 change 文档；不改生产代码、命令、配置或历史验收。保留已有 dirty 工作树、前一轮 daily-knowledge 测试及全部 legacy 协议兼容测试。无新测试框架。
