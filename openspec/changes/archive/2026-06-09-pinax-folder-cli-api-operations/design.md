## Context

当前 Pinax 已经有 `note folders` 维度列表、`note folders rename` 批量重命名、`note move` 单笔移动，以及 `asset ... --plan` 文件管理计划。但这些能力仍围绕 note 或 asset，缺少一个统一的目录生命周期入口。

用户希望远程调用也能操作目录，并且不要让 agent 直接 `mkdir`。这要求目录操作具备和 note/asset 一样的 projection、approval、snapshot、event、index 和 registry 合同。

## Goals / Non-Goals

**Goals:**

- 建立一级 `pinax folder` 命令面，覆盖目录 create/list/show/rename/move/delete/adopt/repair plan。
- 让 REST/RPC 通过同一 folder service 暴露目录操作，route registry 和 OpenAPI schema 自动派生。
- 所有写入都经过 vault boundary、unsafe path、conflict、approval、snapshot、idempotency 和 hook gate。
- 支持空目录的 CLI-authored registry，使远程创建目录不会因为 Git 或索引只看文件而丢失语义。
- 写入后触发 folder lifecycle event 和 index refresh/incremental update。

**Non-Goals:**

- 不支持公网多用户 hosted API、跨 vault 目录移动、任意系统路径操作或 provider 远端文件系统直接写入。
- 不让 REST/RPC handler 直接读写文件；handler 只做参数解析和 projection JSON 序列化。
- 不把 `pinax folder` 设计成 `mkdir` 的薄包装；它必须携带 Pinax 事件、审批、索引和 registry 语义。
- 不自动重写所有 Markdown 相对链接；涉及链接重写的目录移动先进入 plan/manual review 或后续 repair workflow。

## Command Shape

`pinax folder` 是目录生命周期主入口：

```text
pinax folder list [--purpose notes|assets|generic|all] [--include-empty] [--depth N]
pinax folder show <path>
pinax folder create <path> [--purpose notes|assets|generic] [--dry-run]
pinax folder rename <old> <new> --dry-run|--yes
pinax folder move <path> <target-parent> --dry-run|--yes
pinax folder delete <path> --empty-only --dry-run|--yes
pinax folder adopt <path> [--purpose notes|assets|generic] --dry-run|--yes
pinax folder repair --plan
```

Compatibility:

- `pinax note folders` remains the note dimension browser.
- Existing `pinax note folders rename` may delegate to the same service, but help and next actions should prefer `pinax folder rename`.
- A `mkdir` alias MAY exist as `pinax folder mkdir <path>` for discoverability, but documentation should prefer `create`.

## Domain Model

Proposed models:

```go
type FolderPurpose string // notes, assets, generic
type FolderOperation string // create, rename, move, delete, adopt, repair

type FolderRecord struct {
    Path string
    Purpose FolderPurpose
    ManagedStatus string // managed, discovered, missing
    Empty bool
    NoteCount int
    AssetCount int
    UpdatedAt string
    Evidence []string
}

type FolderOperationPlan struct {
    PlanID string
    Operation FolderOperation
    Path string
    TargetPath string
    Purpose FolderPurpose
    Risk string // low, medium, high
    RequiresApproval bool
    RequiresSnapshot bool
    Operations []domain.PlanOperation
}
```

Source of truth:

- Non-empty目录的存在事实来自 vault 文件系统。
- 空目录和目录 purpose/managed evidence 由 CLI-authored `.pinax/folders.json` 记录。
- Note/asset 仍以 Markdown 和 asset manifest/index 为真源，folder registry 不能替代 note frontmatter 或 asset manifest。

## Service Boundary

新增 `internal/app` folder service 方法：

- `ListFolders(ctx, FolderListRequest)`
- `ShowFolder(ctx, FolderRequest)`
- `PlanFolderOperation(ctx, FolderOperationRequest)`
- `ApplyFolderOperation(ctx, FolderOperationRequest)` 或按 create/rename/move/delete/adopt 拆分薄 wrapper

所有方法都返回 `domain.Projection`。CLI、REST、RPC、MCP 或 future dashboard 只能调用这些 service，不能直接调用 `os.MkdirAll`、`os.Rename` 或 `os.Remove`。

## Safety Gates

- Path 必须是 vault-relative，禁止绝对路径、`..`、`.pinax`、`.git`、trash、provider cache 和隐藏控制目录。
- `create` 在 CLI 可默认写入，提供 `--dry-run`；远程 create 必须显式 `yes=true` 或返回 `approval_required`。
- `rename`、`move`、`delete`、`adopt` 必须支持 `--dry-run|--yes`；写入多文件或删除非空目录必须要求 snapshot。
- 远程写入 route 仅在 `pinax api serve --allow-write` 且 loopback bind 下可用；默认 `--readonly` server 返回 `write_disabled`。
- 远程 mutation 必须接受 `idempotency_key` 或 `Idempotency-Key`，重复请求返回同一 projection 或当前 terminal state，不重复执行文件操作。
- 删除默认 `--empty-only`；非空目录删除先生成 plan，不做 recursive delete apply，除非后续明确设计 trash/restore 语义。

## API Shape

REST routes 从 `RemoteRoutes()` registry 导出：

```text
GET  /v1/folders
GET  /v1/folders/{path}
POST /v1/folders
POST /v1/folders/{path}:rename
POST /v1/folders/{path}:move
POST /v1/folders/{path}:delete
POST /v1/folders/{path}:adopt
POST /v1/folders:repair-plan
```

RPC methods：

```text
Pinax.Folder.List
Pinax.Folder.Show
Pinax.Folder.Create
Pinax.Folder.Rename
Pinax.Folder.Move
Pinax.Folder.Delete
Pinax.Folder.Adopt
Pinax.Folder.RepairPlan
```

Mutation request body fields use stable English keys:

```json
{
  "path": "projects/research",
  "target_path": "projects/archive",
  "purpose": "notes",
  "dry_run": true,
  "yes": false,
  "snapshot_id": "snap_...",
  "idempotency_key": "folder-rename-..."
}
```

All API responses remain Pinax projection envelopes. OpenAPI operations MUST include `x-pinax-command`, `x-pinax-capability`, `x-pinax-readonly`, `x-pinax-body-allowed`, `x-pinax-approval-required`, and `x-pinax-snapshot-required`.

## Hooks And Index

Folder service emits structured events before/after writes:

- `folder.plan`
- `folder.created`
- `folder.renamed`
- `folder.moved`
- `folder.deleted`
- `folder.adopted`
- `folder.repair_planned`

Index impact:

- Folder projection records folder path, purpose, managed status, note count, asset count, empty state, and source evidence.
- Folder create/adopt updates folder projection and registry without rebuilding unrelated note FTS.
- Folder rename/move updates note path/folder properties for affected notes and marks link/attachment projections stale when relative references may be affected.
- On incremental failure, projection status becomes `partial` with action `pinax index refresh --vault <vault>` or `pinax index repair --kind folder --dry-run`.

## Output Contract

Commands use the existing projection renderer. Stable facts include:

- `operation`
- `folder_path`
- `target_path`
- `purpose`
- `managed_status`
- `dry_run`
- `writes`
- `requires_snapshot`
- `matched`
- `changed`
- `note_count`
- `asset_count`
- `index_updated`
- `plan_id`

Default human output is concise Chinese summary. `--json` emits a single envelope; `--agent` emits stable key=value; no mode leaks provider payloads, tokens, raw request body, or hidden prompts.

## Migration Plan

1. 实现 `pinax folder list/show/create` 和 folder registry/index read path。
2. 将 `pinax note folders rename` service 抽到 folder service，并新增 `pinax folder rename` 主入口。
3. 接 REST/RPC read routes，再接 write plan routes。
4. 最后允许 `api serve --allow-write` 的 mutation apply，并加 idempotency/snapshot gate。
5. 更新 help/docs，把目录操作统一推荐为 `pinax folder ...`。
