# 任务

## Bug 修复（P0/P1）

- [ ] B1 修复 syncdaemon watcher forward() 错误通道无守卫阻塞发送，注入双错误测试断言不阻塞。
- [ ] B2 Debounce flush 改 select 发送防关停泄漏；删 publishDevDebounce 复用 syncdaemon.Debounce（新增 coalesce 参数）。
- [ ] B3 cloudclient requireVaultID panic 改错误上抛，app 层包 CommandError；清理库代码 Must* 非 Must 用法；空 workspace 配置 sync 命令返回错误封套而非崩溃。
- [ ] B5 Service 注入 now func() time.Time，删除 PINAX_TEST_NOW 生产读取。
- [ ] B6 删 capsa_bridge Summary/Error.Message 盲目 ReplaceAll，措辞参数化；capsa fixture 允许更新，cloud 主路径零 diff。
- [ ] B7 warnPersistFailure 收编 monitor/sync 静默丢弃的证据写入。
- [ ] B8 share.go http.ErrServerClosed 改 errors.Is。
- [ ] B4a VersionStatus 使用调用方 ctx；ctx 丢弃清单记入 design.md。

## 架构简化切片

- [ ] S1 helper 去重：fsutil.copyFile（对拍测试固化两份行为后合并）、jsonutil.ReadJSON、mustJSON、writeReceipt。
- [ ] S2 remoteRPCRequestForCommand 表驱动 + 与命令定义共址注册 + 守护测试。
- [ ] S3 buildCloudSyncProjection 拆阶段函数；attachContentDiff 闭包改显式 struct；executeCloudPushWithCredential 拆三段；cloud_sync.go 拆文件。
- [ ] S4 service.go 分解：CreateNote→noteops 管道、version→versionops、monitor→monitor_runner.go；service.go <1500 行。
- [ ] S5 render.go summaryFactLabel 表驱动（先固化现状对拍测试）；dataMap 失败加 stderr 诊断。
- [ ] S6 index/store.go 同包拆 store_notes/store_records/store_migration/store_query.go。

## 测试与 CI

- [ ] 新增 Taskfile `race` target（风险面包）。
- [ ] 修复 TestObjectStoreTransportLockFallback flaky（-race -count=20 复现并修复）。
- [ ] 新增 .github/workflows/pinax-ci.yml（PR 触发 ci+race）。

## 验证

```bash
task check
task race
openspec validate --all
```
