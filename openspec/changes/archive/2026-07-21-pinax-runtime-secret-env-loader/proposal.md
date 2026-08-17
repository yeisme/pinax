## Why

上一阶段的声明式同步配置已经能把 backend、workspace 和 logical secret identity 放入仓库，但当前 `env` provider 仍依赖调用进程预先设置 `PINAX_SYNC_SECRET_<IDENTITY>`，无法让仓库自描述地携带一份加密 dotenv。另一方面，用户容易在本地生成 `.env` 后误提交到 Git，虽然 Capsa 的 `.pinaxignore` 已经排除部分环境文件，但 Git 忽略和运行时注入尚未形成统一合同。

需要一个安全的运行时 env 层：加密 dotenv 可以提交，解密后的 dotenv 默认不提交；Pinax 在每次命令或 daemon sync run 开始时解密并注入本进程/子进程，默认不生成明文文件；如用户明确要求 materialize，文件必须落在受保护目录、使用 `0600` 并自动加入 Git ignore。

## What Changes

- 新增加密 dotenv 资产 `.pinax/pinax-sync.env.age` 的声明、解析和 provider-neutral unlock 流程。
- 新增 `pinax sync env init|set|list|unlock|doctor|clean` 命令合同。
- 支持命令运行时内存注入和 daemon 在每个 sync run 边界动态 reload；不执行 shell source、命令替换或变量递归展开。
- 明文 `.env`、`.env.*`、`*.env`、`.pinax/runtime/*.env` 默认加入受管 `.gitignore`，已被 Git 跟踪的文件只告警，不自动删除。
- 将解密 dotenv 作为 protected path，禁止进入 Capsa content manifest、sync receipt、日志和结构化输出。
- 与既有 `.pinax/pinax-sync.secrets.yaml` 并存：dotenv 适合 provider/environment 键值；现有 logical secret API 继续适合单个值。

## What Does Not Change

- 不把明文 dotenv 写入仓库、远端对象、同步 receipt、日志或 stdout。
- 不让 dotenv 内容覆盖显式 CLI flags 或用户主动设置的高优先级环境变量。
- 不执行 dotenv 中的 shell 语法、命令、文件引用、网络请求或表达式。
- 不自动删除已被 Git 跟踪的 `.env`；清理必须由用户显式执行。
- 不在本变更中引入完整的云端 KMS、租户 RBAC 或实时协作。

## Capabilities

### New Capabilities

- `pinax-runtime-secret-env-loader`

### Modified Capabilities

- `pinax-declarative-sync-config`
- `pinax-cloud-sync`
- `configuration-layer`

## Impact

- `internal/remote`：加密 env envelope、严格 dotenv parser、unlock snapshot 和 source metadata。
- `internal/app` / `internal/cli`：env 命令、runtime injection、daemon reload 和 cleanup。
- `internal/vaultignore`：受管 Git ignore block 与 protected content paths。
- 测试：跨平台权限、secret redaction、动态 reload、tracked-file warning、fake provider 和 testscript evidence。
