## MODIFIED Requirements

### Requirement: 端侧加密保护明文

Manifest 和 blob SHALL 使用 client-side encryption；明文 SHALL NOT 离开本地设备。加密密钥 SHALL 通过 Capsa SDK 从 secret reference 解析出真实密钥值，SHALL NOT 使用引用字符串本身作为密钥材料。Capsa SDK Go module source SHALL 指向版本化 submodule `backend-server/capsa/sdk`，SHALL NOT 依赖 plain directory copy 作为生产 SDK source。

#### Scenario: 加密 manifest 和 blob

- **WHEN** 客户端上传 manifest 或 blob
- **THEN** 数据 SHALL 使用端侧加密 envelope
- **AND** 后端或 direct object store SHALL 只看到 encrypted payload 和非敏感 revision metadata

#### Scenario: profile:// 引用解析为真实密钥

- **GIVEN** `secret_ref` 配置为 `profile://tencent-cos-pinax`
- **WHEN** Pinax 执行 Cloud Sync 加密操作
- **THEN** Pinax SHALL 调用 `github.com/yeisme/capsa` SDK crypto API 解析为真实的 `aws_secret_access_key`
- **AND** SHALL NOT 使用字面字符串 `profile://tencent-cos-pinax` 作为 PBKDF2 输入
- **AND** 如果 profile 不存在或无法解析，SHALL 返回错误

#### Scenario: SDK salt migration requires re-push

- **GIVEN** 远端 COS/S3 对象由旧 Pinax-local crypto path 写入
- **WHEN** Pinax 升级到 Capsa SDK crypto path
- **THEN** key derivation SHALL use `capsa-sync-salt-v1`
- **AND** 旧 `pinax-cloud-sync-salt-v1` 加密对象 SHALL require a fresh `pinax sync push --target capsa --yes` from a complete local vault
- **AND** docs SHALL describe the migration rather than promising transparent remote decryption

#### Scenario: 弱加密密钥警告

- **GIVEN** 用户运行 `pinax capsa backend set s3` 未指定 `--encryption-secret-ref`
- **AND** `secret_ref` 不是 `env://` 或 `keychain://` scheme
- **WHEN** Pinax 配置 Capsa Sync backend
- **THEN** projection SHALL 包含 `weak_encryption_key` 警告

#### Scenario: SDK source 指向版本化 submodule

- **GIVEN** Pinax `go.mod` 声明 `require github.com/yeisme/capsa v0.0.0`
- **WHEN** 开发者编译 sync 或 crypto 相关代码
- **THEN** `go.mod` replace SHALL 指向 `../../backend-server/capsa/sdk`
- **AND** SHALL NOT 指向 `../../shared/capsa` plain directory
- **AND** public module path SHALL 保持 `github.com/yeisme/capsa` 不变
- **AND** 加密 salt 与 envelope schema SHALL 不因 source 迁移改变
