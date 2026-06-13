## Why

Pinax 已经有 note 创建、预览、列表和单笔 tag patch，但属性管理、tag taxonomy 批量治理和自定义 frontmatter 属性索引仍不完整，用户需要手改机器可读 metadata 才能完成常见整理动作。

这次变更补齐最小可用的本地优先 metadata 管理面：让属性和 tags 都通过 CLI/application service 写入，并让写入结果立即进入本地索引投影。

## What Changes

- 新增 `pinax note property set <note> <property> <value>` 和 `pinax note property remove <note> <property>`，用于管理单条笔记的非保留 frontmatter 属性。
- 扩展 `pinax note tags`，新增 `rename <old> <new>` 和 `delete <tag>`，并要求写入动作显式传入 `--yes`，预览动作使用 `--dry-run`。
- 索引层 SHALL 采集 Pinax note frontmatter 中的任意非空自定义属性，使 `note list --property ... --strict-properties` 能查询这些属性。
- 属性和 tag 写入 SHALL 通过 application service 刷新 record/index 事实，并复用既有 JSON/agent 输出合同。
- 批量文件夹和文件重组暂不纳入本次实现；后续应作为 snapshot-protected plan/apply 变更处理。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `note-command-ux`: 补充 note property 和 tag taxonomy 批量管理命令的用户行为和输出要求。
- `database-views-query`: 补充任意 frontmatter 属性进入 typed property 投影并可被 strict property 查询的要求。

## Impact

- 影响代码：`internal/cli/note_cmd.go`、`internal/app/service.go`、`internal/domain/types.go`、`internal/index/property.go`、`internal/output/render.go`、`cmd/pinax/main_test.go`、`internal/app/service_test.go`。
- 影响命令：`pinax note property set/remove`、`pinax note tags rename/delete`、`pinax note list --property ... --strict-properties`。
- 不新增依赖、不新增 daemon、不直接接外部 provider、不改变 Markdown vault 作为真源的定位。
