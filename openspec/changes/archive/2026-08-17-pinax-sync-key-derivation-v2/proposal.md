# Capsa 同步密钥派生 v2

## 目标

修复 `DeriveKey` 的两个密码学弱点：全局静态 PBKDF2 salt（`capsa-sync-salt-v1`，
所有用户共享，支持跨用户预计算表）与 100k PBKDF2-SHA256 迭代（低于 OWASP 当前
600k 指引）。

## 方案

- v2 派生：600k 迭代；salt 从共享密钥本身派生（SHA-256 域分离
  `capsa-sync-salt-v2\x00||secret`）。同 secret 的所有设备派生同一密钥（多设备
  兼容），不同 secret 得到不同 salt（逐用户预计算防护），无需任何持久化状态或
  远端对象，因此不需要迁移步骤。
- 读兼容：解密改用 `CryptoKeys` keychain（Active v2 + Legacy v1），按 envelope
  的 KeyID 选择密钥。旧 envelope 继续可读；新写入全部使用 v2。
- 写路径总是 Active v2。旧 blob 只有在其内容变化重传时才逐步转为 v2；
  完全迁移可在任意时刻 `sync push --yes` 全量重写（可选）。

## 非目标

不改 AES-256-GCM 信封格式、schema 常量或飞书/KDF 之外的任何传输语义。
