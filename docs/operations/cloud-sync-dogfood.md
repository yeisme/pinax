# Cloud Sync v1.0 验收记录

本文记录 Pinax v1.0 的 Cloud Sync 真实 transport 证据。fake server 和 `file://` 只用于本地开发验证，不能填写为 GA 证据。

## 运行命令

在具备 owner 授权的环境中运行：

```bash
PINAX_SYNC_REAL_ENDPOINT=https://<endpoint> \
PINAX_SYNC_REAL_WORKSPACE=<workspace> \
PINAX_SYNC_REAL_SECRET_REF=env:PINAX_SYNC_SECRET \
task integration:sync-real
```

证据目录必须由 runner 生成在：

```text
temp/integration-test-runs/<run-id>/
```

不得把 secret、Authorization header、正文或 provider raw payload 写入 evidence。

## GA 门槛

| Gate | 要求 | 状态 | Evidence |
| --- | --- | --- | --- |
| G1 | 真实 Pinax CLI ↔ Capsa 完成 device bind、push、pull、conflict resolve | `pending` | 待真实 endpoint |
| G2 | 两台独立设备通过真实 server/S3-compatible transport 收敛 | `pending` | 待真实 endpoint |
| G3 | 已知 lock/head flaky case 稳定通过或有明确缓解记录 | `pending` | 待复核 |
| C3 | Pinax cloudclient 与 Capsa handler 的请求、响应、错误码、revision 无 drift | `pending` | 待 backend contract probe |

## 必须回读的事实

- push 的 `remote_write=true` 只在 durable remote commit 和 revision read-back 后出现；
- pull 回读相同 revision 和对象身份；
- divergent edits 保留 conflict copy，不静默覆盖；
- delete marker、restore 和 up-to-date 状态可解释；
- 失败状态包含可执行恢复动作，未知 mutation 必须先查询原 operation。

在 G1、G2、G3、C3 全部完成前，README 中 Cloud Sync 保持 `Preview`，不得发布 v1.0 GA 声明。
