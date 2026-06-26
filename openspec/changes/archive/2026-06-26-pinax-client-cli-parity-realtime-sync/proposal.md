# Pinax 客户端 CLI 覆盖与实时同步计划

## 背景

Pinax 已经有两条客户端相关能力：

- `pinax api serve` / `--api-url` / REST/RPC：让本地工具、agent、dashboard 或 CLI client 操作一台服务端 vault。
- `pinax sync daemon`：让多设备本地 vault 通过 Cloud Sync encrypted revision 实时收敛。

用户希望客户端最终支持全部 CLI 功能，并且实时同步实现可被清晰使用和说明。当前实现已经覆盖部分 note、folder、project、inbox、draft 和 sync RPC 能力，但文档没有把“全 CLI 覆盖目标”和“实时同步边界”讲清楚，也没有给后续实现建立可验证矩阵。

## 目标

- 建立客户端 CLI parity 的正式路线：所有可远程客户端化的 CLI 能力都通过 capability registry、REST/RPC 或 Remote API Mode additive 暴露。
- 保持 Remote API Mode 和 Cloud Sync daemon 的边界清楚：前者操作一个服务端 vault，后者同步多个本地 vault。
- 明确 local-only 命令、危险写操作、snapshot/approval/dry-run/receipt/redaction 门禁。
- 更新文档入口，让用户知道当前覆盖、目标覆盖、实时同步启动方式和后续验证命令。

## 非目标

- 不新增 public Internet hosted API。
- 不新增一个万能远程 shell 或任意命令执行 RPC。
- 不绕过 application service 直接写 Markdown、`.pinax/**`、SQLite、sync-state、provider 配置或 token 文件。
- 不把 `sync daemon` 变成 Cloud Sync 后端服务；daemon 仍是每台设备上的本地进程。

## 稳定合同影响

- HTTP/RPC/API：后续新增 route、RPC method 和 capability 是 additive；不删除或改名现有 route、field、error code。
- CLI 输出：继续使用 `pinax.projection.v1` envelope；允许新增 optional facts/data/actions/evidence。
- Config：`remote.api_url` 等现有 key 保持语义不变。

本变更本身只新增文档和交付计划，不改变运行时合同。
