# 任务

- [x] `DeriveKeyV2`（600k 迭代 + secret 派生 salt）与 `DeriveKeyLegacy`；`CryptoKeys` keychain 按 envelope KeyID 选钥。
- [x] `DecryptBlob`/`DecryptManifest` 接受 keychain；`Encrypt*` 继续单 key（调用方传 Active）。
- [x] cloud sync 读写路径接入 keychain（snapshot 携带 Keys；pull/push/rebase 共用）。
- [x] 回归测试：legacy envelope 经 keychain 可解；异 KeyID fail-closed；多设备同 secret 同钥（既有 push/pull 集成测试覆盖）。

- [x] `pinax sync keys --json` 验证工具：报告 active/legacy KeyID 与远端 manifest envelope 的 derivation 分类（v2/legacy/empty/unknown），legacy 时给出 reencrypt action。
- [x] up-to-date 快路径加 `remoteSnapshotFullyUnderKey` 守卫：内容未变但存在 legacy KeyID 对象时仍走重加密 push（否则迁移会被跳过）。
- [x] 端到端 CLI 测试：模拟 legacy 远端 → keys 报 legacy + pull 兼容读取 → push 重加密 → keys 报 v2；docs/commands/sync.md 增加迁移 runbook。

## 验证

```bash
go test ./internal/remote ./internal/app ./cmd/pinax -count=1
task check
```
