## Why

当前 Capsa Sync 的远端地址、bucket、prefix、workspace、provider profile 和加密引用主要落在每台设备的 `.pinax/cloud/config.yaml` 或本机 profile 中。新设备、远程开发容器和多平台迁移必须重复执行后端配置，且直接同步运行时配置会混入设备状态和敏感引用，容易造成 key ID mismatch、设备互相覆盖和密钥泄露。

用户需要类似 Ansible 的声明式体验：同步拓扑进入 Pinax 仓库，加密敏感配置以密文形式进入仓库，设备第一次 bootstrap 后自动生成本地运行配置，后续只需 clone/pull 后运行一个稳定命令即可工作。

## What Changes

- 新增仓库级声明同步配置 `pinax-sync.yaml`，描述 backend、workspace、应用/租户命名空间、加密 key identity、设备策略和同步安全策略。
- 新增加密 secrets 资产 `pinax-sync.secrets.age` 的抽象和 CLI 管理流程；密钥明文只在解密运行时存在，不写回仓库、vault、日志或环境文件。
- 新增 `pinax sync repo init|secret|bootstrap|plan|apply|doctor` 命令族。
- 将仓库声明配置编译为本机 `.pinax/cloud/config.yaml`，将 device id、sync receipt、daemon runtime 和 provider session 保留在本机。
- 明确首次 bootstrap、配置漂移、密钥不可用、设备冲突和远程删除的安全门禁。
- 改进 Cloud Sync、configuration-layer、profile-management 的相关 spec，使多设备、远程开发和多应用隔离遵循同一合同。

## What Does Not Change

- 不把明文 credentials、token、SecretKey 或加密密钥提交到仓库。
- 不把 `.pinax/cloud/config.yaml`、daemon state、sync receipt 变成用户手工维护的共享文件。
- 不改变现有 manifest、revision CAS、冲突解决和端侧加密协议。
- 不在本变更中实现完整的 SaaS 多租户 RBAC、计费、配额和服务端控制面。
- 不实现实时协同编辑或 CRDT。

## Capabilities

### New Capabilities

- `pinax-declarative-sync-config`

### Modified Capabilities

- `pinax-cloud-sync`
- `configuration-layer`
- `pinax-profile-management`

## Impact

- CLI：新增 `sync repo` 命令和稳定的 human、`--agent`、`--json`、`--events` 输出合同。
- 应用层：增加声明配置解析、密文 secrets 解锁、编译本机 runtime config 和 drift/doctor 服务。
- 安全：新增密钥来源、redaction、权限模式和 bootstrap 失败策略。
- 测试：增加跨平台配置 fixture、双设备/远程容器 testscript、密钥泄露扫描和回滚验证。
- 文档：更新 Capsa、sync、profile 和新设备 onboarding 文档。
