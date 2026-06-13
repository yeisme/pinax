## 1. 基础同步加密与存储抽象模型

- [x] 1.1 在 `internal/cloud` 中定义统一的 `StorageBackend` 接口（封装 `Get`, `Put`, `Stat` 等操作）。
- [x] 1.2 实现 `s3` 存储后端，通过 S3 API 处理读写，并处理 `If-Match` 并发锁逻辑。
- [x] 1.3 实现 `file` 存储后端，支持本地目录或诸如 JuiceFS / FUSE 挂载的网络盘读写，并封装基于系统调用的原子并发控制。
- [x] 1.4 在 `internal/cloud` 中实现基于密码的 Sync Secret 密钥派生（Argon2 / PBKDF2）。
- [x] 1.5 实现 `AES-256-GCM` 的加解密包装函数，用于文件内容和 Manifest 的端到端保护。

## 2. Manifest 与清单比对

- [x] 2.1 定义跨端同步用 JSON Manifest 的数据结构，包含文件 hash 与版本信息。
- [x] 2.2 在 `internal/sync/planner.go` 中支持拉取服务端 Manifest、本地生成当前状态 Manifest 并进行 Diff 比对。
- [x] 2.3 补充对云端盲存储后端的 HTTP 409 Conflict 异常处理测试（乐观锁版本验证）。

## 3. 并发冲突处理与就地备份

- [x] 3.1 修改 `internal/sync/planner.go`，在发现并发写入冲突时，将本地冲突文件在原目录就地重命名为带有时间戳和 `.conflict.md` 后缀的文件保留上下文。
- [x] 3.2 验证冲突文件是否正确在原目录落盘且不干扰主干笔记的同步流程。
- [x] 3.3 补充用服务端最新版本安全覆盖本地原路径文件的逻辑，保障主干笔记一致。

## 4. 基础同步命令与配置交互 (Phase 4)
- [x] 4.1 修改 `internal/app/service.go` 中的 `SyncInit`，调用 `internal/cloud` 建立 `.pinax/cloud/config.json`。
- [x] 4.2 修改 `SyncStatus`，调用 `cloud.LoadConfig`，并增加对配置连通性的探针或简单的 doctor 检查。
- [x] 4.3 新增 `cmd/pinax/sync_cmd.go` 并实现 `pinax sync init` 及其必填 flag：`--endpoint`, `--workspace`, `--device`, `--secret-ref`。
- [x] 4.4 新增 `pinax sync status` 命令，打印出远端后端的简要信息。
- [x] 4.5 新增 `pinax sync` (全量同步/双向合并)，按序执行 `sync pull` 和 `sync push`，并输出综合状态。
- [x] 4.6 编写 `SyncInit` 与 `SyncStatus` 的 contract tests，确保符合 ai-native-cli-output-contract。

## 5. 面向 Agent 的冲突检测命令 (Phase 5)
- [x] 5.1 在 `cmd/pinax/sync_conflicts.go` 中新增 `conflicts list` 命令。
- [x] 5.2 确保 `conflicts list` 支持 `--json` 输出，仅包含冲突文件的路径及其对应的时间戳和冲突类型（如有）。
- [x] 5.3 实现 `conflicts diff <file>` 以及 `conflicts show <file> --json`。后者应按照 `original_content` 与 `conflict_content` 的 JSON 格式吐出。
- [x] 5.4 新增 `conflicts resolve` 命令及其三个参数：`--keep-local`、`--keep-remote` 和 `--merged <path>`。

## 6. 集成测试与验证 (Phase 6)
- [x] 6.1 在根目录运行 `task test` 或 `go test ./...`，确保所有 sync 和 cloud 的包单元测试均通过。
- [x] 6.2 （可选）创建一个小型集成测试或使用 `pinax sync push` 搭配本地存储进行端到端流转测试。
- [x] 6.3 检查代码结构与错误处理，特别是 `internal/sync/planner.go` 与 `executor.go` 的异常路径是否都能产生合规的 errorProjection。
- [x] 6.4 提交前运行 `openspec validate --all` 并归档任务状态。
