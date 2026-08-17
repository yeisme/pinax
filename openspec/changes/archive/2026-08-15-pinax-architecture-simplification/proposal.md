# Pinax 架构简化与正确性修复

## 目标

在 KB/vector 砍除之后收拢 Pinax 的正确性与结构复杂度：修复并发/panic/静默失败类
真实 bug，削减 god file 与重复实现，为并发风险面建立 race 测试门。外部 CLI 行为、
输出合同（cli-output-contract）与黑盒测试保持零变化（capsa 错误文案修复除外）。

## 变更范围

- **Bug 修复**：
  - syncdaemon watcher 错误通道无守卫阻塞发送（事件转发可永久停摆）。
  - Debounce flush 阻塞发送导致 cancel 泄漏/关停挂死；与 publish 的重复实现合并。
  - cloudclient `requireVaultID` panic 改错误封套；库代码 `Must*` 非 Must 上下文清理。
  - 生产二进制读取 `PINAX_TEST_NOW` 环境变量伪造时间。
  - capsa_bridge 对 Summary/Error 的盲目 `ReplaceAll("Cloud"→"Capsa")` 文本替换。
  - monitor/sync 证据持久化失败静默丢弃，改 stderr warn。
  - `err != http.ErrServerClosed` 改 `errors.Is`。
  - `VersionStatus` 丢弃调用方 ctx；其余 154 处 ctx 丢弃随重构切片顺带修复。
- **架构简化切片**：helper 去重（copyFile/mustJSON/writeReceipt/JSON-read）、
  root.go RPC god-switch 表驱动化并与命令定义共址注册、cloud_sync 374 行投影
  函数拆阶段、service.go 沿 noteops/vaultops/versionops seam 下沉、
  render.go fact label 表驱动、index/store.go 拆文件。
- **测试基建**：新增 `task race` 目标（风险面包）、修复
  `TestObjectStoreTransportLockFallback...` flaky、新增 PR CI workflow。

## 非目标

不改 CLI 输出 fact key/JSON 键；不合并 note 域八包；不动 cloudclient/cloudsync
包边界；不引入第三方依赖；不对全量测试上 -race。
