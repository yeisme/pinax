## Context

Pinax 通过本地 replace 直接 import 共享 Go module，并由 Inferrum 发布的短生命周期 Python sidecar 执行 LanceDB doctor/rebuild/search。Inferrum owner 切换改变 module/package、sidecar executable/import、protocol/error namespace；Record、metadata opaque、allowed IDs、LanceDB table/store layout 和 provider APIs 保持一致。Pinax 自有旧 Python sidecar distribution、CI 和新写路径在本次切换中删除。

当前 Pinax worktree 和 active `pinax-inferrum-ollama-local-kb-review-mvp` 含大量正在维护的 KB 证据与任务，迁移必须精确改名，不能回滚或重写无关业务结论。

## Goals / Non-Goals

**Goals:**

- Pinax 只解析 `github.com/yeisme/inferrum` 和 `inferrum.sidecar.v1`。
- 保持现有 KB generation、permission、evaluation、activation、rollback、receipt 和 CLI contract。
- 更新所有 focused tests、Taskfile、integration evidence 和 active docs。
- 非归档 Pinax 文件不再包含旧 Lance 产品合同；第三方 LanceDB 名称保持原样。

**Non-Goals:**

- 不迁移 vault、SQLite/GORM 数据或 LanceDB store。
- 不重做 legacy Pinax KB projection 读取语义。
- 不修改 Workbench runtime、provider model、Ollama quality threshold 或真实语料结论。

## Decisions

### 1. 直接替换 module 和 package qualifier

```mermaid
flowchart LR
  CLI["pinax kb commands"] --> APP["Pinax KB app services"]
  APP --> ADAPTER["internal/semantic Inferrum adapter"]
  ADAPTER --> SDK["github.com/yeisme/inferrum"]
  SDK --> SIDE["inferrum-lancedb-sidecar"]
  SIDE --> DB["existing LanceDB generations"]
```

Go imports 使用 alias `inferrum`；Pinax-owned files/types/tests 同步改名，避免代码继续把当前 dependency 称为 Lance。公共 Pinax CLI 和 receipt schema 不新增别名。

### 2. 协议 namespace 改名不触发 generation 数据迁移

现有 generation 中的 LanceDB tables、`.lance` directories、metadata 和 model identity 不变。仅 subprocess 请求/响应的 schema string 与错误映射改名。focused tests 使用相同 records 验证行为等价。

### 3. active OpenSpec 直接更新为当前事实

`pinax-inferrum-ollama-local-kb-review-mvp` 尚未归档，因此其中的依赖、任务和验收文本改为 Inferrum；历史 RED/GREEN evidence 可以保留命令事实，但新结论不得把 Lance 当成现行产品。

### 4. legacy_v1 只表示 Pinax 历史投影

如果 `legacy_v1_readonly` 代码仍被 active behavior 使用，它继续作为 Pinax-owned old projection reader；名称、errors、提示和文档不得暗示依赖已删除的 Lance module/protocol。删除时必须由原 KB change 的 N/N+1/N+2 规则决定，不在本迁移中擅自删除数据兼容。

## Risks / Trade-offs

- [大量测试名和证据文本漏改] → 精确禁止模式审计并运行 focused semantic/app/testkit tests。
- [机械替换误改 LanceDB] → 保留 `LanceDB`、`lancedb`、`.lance`，只替换项目身份与协议。
- [脏 worktree 混入无关修改] → 显式 path staging，提交前检查 staged diff。
- [legacy v1 语义被误删] → 将其重新描述为 Pinax historical projection，而不是顺手删除兼容业务路径。

## Migration Plan

1. 增加/更新新 module/protocol 的 focused tests。
2. 更新 go.mod、semantic adapter、sidecar config/errors、tests、Taskfile 和 evidence runner。
3. 更新 active KB OpenSpec 与 docs；保留业务验收结论。
4. 运行 focused tests、`task check`、严格 OpenSpec 和禁止模式审计。
5. 归档本 change，提交并推送；root 更新 submodule pointer。

回滚为 revert Pinax migration commit，并在 owner 侧固定 `v0.1.0-lance-final`；不需要 vault 或 LanceDB 数据回滚。

## Open Questions

- 无。依赖终态由 root Inferrum cutover 已确定。
