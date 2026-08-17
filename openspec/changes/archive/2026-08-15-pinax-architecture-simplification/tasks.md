# 任务

## Bug 修复（P0/P1）

- [x] B1 修复 syncdaemon watcher forward() 错误通道无守卫阻塞发送，注入双错误测试断言不阻塞。
- [x] B2 Debounce flush 改 select 发送防关停泄漏；删 publishDevDebounce 复用 syncdaemon.Debounce（新增 coalesce 参数）。
- [x] B3 cloudclient requireVaultID panic 改错误上抛，app 层包 CommandError；清理库代码 Must* 非 Must 用法；空 workspace 配置 sync 命令返回错误封套而非崩溃。
- [x] B5 Service 注入 now func() time.Time，删除 PINAX_TEST_NOW 生产读取。
- [x] B6 删 capsa_bridge Summary/Error.Message 盲目 ReplaceAll，措辞参数化；capsa fixture 允许更新，cloud 主路径零 diff。
- [x] B7 warnPersistFailure 收编 monitor/sync 静默丢弃的证据写入。
- [x] B8 share.go http.ErrServerClosed 改 errors.Is。
- [x] B4a VersionStatus 使用调用方 ctx；ctx 丢弃清单记入 design.md。

## 架构简化切片

- [x] S1 helper 去重：fsutil.copyFile（对拍测试固化两份行为后合并）、jsonutil.ReadJSON、mustJSON、writeReceipt。
- [x] S2 remoteRPCRequestForCommand 表驱动 + 与命令定义共址注册 + 守护测试。
- [x] S3 buildCloudSyncProjection 拆阶段函数；attachContentDiff 闭包改显式 struct；executeCloudPushWithCredential 拆三段；cloud_sync.go 拆文件。
- [x] S4 service.go 分解：CreateNote→noteops 管道、version→versionops、monitor→monitor_runner.go；service.go <1500 行。
- [x] S5 render.go summaryFactLabel 表驱动（先固化现状对拍测试）；dataMap 失败加 stderr 诊断。
- [x] S6 index/store.go 同包拆 store_notes/store_records/store_migration/store_query.go。

## 测试与 CI

- [x] 新增 Taskfile `race` target（风险面包）。
- [x] 修复 TestObjectStoreTransportLockFallback flaky（-race -count=20 复现并修复）。
- [x] 新增 .github/workflows/pinax-ci.yml（PR 触发 ci+race）。

## 验证

```bash
task check
task race
openspec validate --all
```

## 范围调整记录（2026-08-15 实施时证据驱动修订）

- S1：mustJSON×2（records 生产 vs testkit 测试脚手架）与 writeReceipt×2（.pinax/receipts 通用 map vs feishu delivery struct）为同名不同义，强行合并为负价值，只合并真正危险重复的 copyFile（参数顺序相反）。
- S4：CreateNote 跨包下沉到 noteops 需导出数十个包内 helper，改为就地抽取 resolveNoteTemplate/resolveNewNotePath 两个决策阶段；service.go 全包域下沉留待后续独立变更。
- S5：summaryFactLabel 本已是 map 表驱动（原探索报告"switch"定性不准），仅将 sync 前缀 switch 转 map 并加 golden 固化测试；dataMap 失败含正常 nil Data 场景，加 stderr 告警会产生噪音，未做。
- C8 flaky：90 次 `-race -count=30` 稳定，已被此前的 object_store sync.Mutex 修复解决，记录为已验证而非本轮修复。
