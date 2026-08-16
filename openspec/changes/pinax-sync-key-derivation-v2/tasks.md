# 任务

- [x] `DeriveKeyV2`（600k 迭代 + secret 派生 salt）与 `DeriveKeyLegacy`；`CryptoKeys` keychain 按 envelope KeyID 选钥。
- [x] `DecryptBlob`/`DecryptManifest` 接受 keychain；`Encrypt*` 继续单 key（调用方传 Active）。
- [x] cloud sync 读写路径接入 keychain（snapshot 携带 Keys；pull/push/rebase 共用）。
- [x] 回归测试：legacy envelope 经 keychain 可解；异 KeyID fail-closed；多设备同 secret 同钥（既有 push/pull 集成测试覆盖）。

## 验证

```bash
go test ./internal/remote ./internal/app ./cmd/pinax -count=1
task check
```
