# Design

## Bug 修复设计

### B1/B2 watcher 与 debounce

`fsnotifyWatcher.forward()`（internal/app/syncdaemon/watcher.go）的事件与错误
转发都是无守卫阻塞发送：errors 通道 cap-1，消费者未及时 drain 时第二个错误即
永久阻塞 goroutine，事件转发静默停摆。修复：错误发送改为
`select { case ch <- err: default: stderr }` 非阻塞兜底。

`Debounce.flush()` 在 `ctx.Done()` 分支仍阻塞发送，消费者退出后 goroutine 永久
挂死（`publish dev --watch` 关停泄漏）。修复：flush 改
`select { case out <- batch: case <-ctx.Done(): }`；同时给 Debounce 增加
`coalesce` 参数，publish.go 的 `publishDevDebounce` 删除并复用 syncdaemon 实现。

### B3 cloudclient panic

`requireVaultID()` panic 由 7 个 client 方法触达，空 workspace 配置直接崩进程。
改为 `(string, error)` 上抛，app 层包 `CommandError`（code=cloud_vault_id_required）。
`identity.MustParseObjectID` 非 Must 上下文调用点改 `ParseObjectID`。

### B5 测试时钟

`Service` 增加 `now func() time.Time`（默认 `time.Now().UTC()`），删除
`PINAX_TEST_NOW` 生产读取；测试通过构造注入固定时钟。

### B6 capsa 措辞

删除对 Summary/Error.Message 的 `ReplaceAll("Cloud"/"cloud"→"Capsa")`；
保留 Command/Hint/Action 的精确前缀替换。云同步 Summary 文案在构造处经
`syncWording(target)` 参数化，避免误伤包含 "cloud" 的合法文本。

### B7 证据写入可见化

`warnPersistFailure(op, err)` 单行英文 stderr warn；grep 收编 internal/app
中 `_ = write*`/`_ = append*` 持久化点。

## 架构切片设计（每片独立 commit）

1. **S1 helper 去重**：`internal/app/fsutil.copyFile(dst, src)` 统一（现两份
   参数顺序相反）；`internal/jsonutil.ReadJSON[T]`；mustJSON/writeReceipt 先
   diff 格式再合并。
2. **S2 RPC 注册表**：`remoteCommandSpec{Method, ArgParams, Flags}` +
   `registerRemoteCommand` 与各 `*_cmd.go` 共址；root.go 的 310 行 switch 变
   ~30 行组装器；守护测试遍历命令树防漂移。
3. **S3 cloud_sync**：buildCloudSyncProjection 拆 syncLoadState/syncLoadManifests/
   syncLoadRemoteHead/syncPlan/syncExecute/syncProject；闭包 attachContentDiff
   改显式 struct；executeCloudPushWithCredential 拆解锁/传输/收据三段。
4. **S4 service.go**：CreateNote 拆 noteops 五步管道；version→versionops；
   monitor→monitor_runner.go。目标 service.go <1500 行。
5. **S5 render.go**：summaryFactLabel 先写全 key 对拍测试固化现状，再改
   `map[string]string` + 前缀分组表。
6. **S6 store.go**：同包拆 store_notes/store_records/store_migration/store_query。

## ctx 丢弃清单处理

`VersionStatus` 立即修复；其余 154 处不做大爆炸，随 S3/S4 搬运顺带修复
I/O 密集路径（sync/index/trash），AGENTS.md 补约定。

## 测试与 CI

- `task race`：`go test -race -count=1 ./internal/app/syncdaemon/... ./internal/cloudsync/... ./internal/app -run 'Monitor|Sync|Publish|Watch'`。
- C8 flaky：`-race -count=20` 复现后判定测试同步 vs 代码竞态，修掉后并入 race 门。
- `.github/workflows/pinax-ci.yml`：PR 触发 `task ci` + `task race`。
