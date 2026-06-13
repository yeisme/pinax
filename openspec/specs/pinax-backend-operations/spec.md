# pinax-backend-operations Specification

## Purpose
TBD - created by archiving change pinax-backend-auth-profile-cache. Update Purpose after archive.
## Requirements
### Requirement: BlobStore 支持列表和批量查询

`BlobStore` 接口 SHALL 扩展支持 List、Exists 和 BatchStat 操作。

#### Scenario: List 按前缀列出对象

- **GIVEN** S3 backend 中有 `pinax/notes/a.md`、`pinax/notes/b.md`、`pinax/assets/logo.png`
- **WHEN** 调用 `List(ctx, "pinax/notes/")`
- **THEN** SHALL 返回 `a.md` 和 `b.md` 的 `ObjectInfo`（含 Key、Size、Revision）
- **AND** SHALL NOT 返回 `assets/logo.png`

#### Scenario: Exists 检查对象存在

- **WHEN** 调用 `Exists(ctx, "pinax/notes/a.md")` 且对象存在
- **THEN** SHALL 返回 `(true, nil)`
- **WHEN** 对象不存在
- **THEN** SHALL 返回 `(false, nil)`，不返回 error

#### Scenario: BatchStat 批量查询 revision

- **GIVEN** keys 为 `["a.md", "b.md", "c.md"]`，其中 `c.md` 不存在
- **WHEN** 调用 `BatchStat`
- **THEN** SHALL 返回 `{"a.md": "rev1", "b.md": "rev2"}`
- **AND** 不存在的 key SHALL NOT 出现在结果中

#### Scenario: 向后兼容

- **GIVEN** 第三方代码实现了 `BlobStore` 接口但未实现 `ExtendedBlobStore`
- **WHEN** 代码编译和运行
- **THEN** SHALL NOT 编译失败
- **AND** sync 模块通过类型断言检测扩展能力，不支持时降级到逐个 Stat

### Requirement: backend CLI 暴露存储操作

CLI SHALL 提供 `backend ls/stat/cp/du` 命令。

#### Scenario: backend ls 列出远端对象

- **GIVEN** 存在 profile `my-s3`
- **WHEN** 用户运行 `pinax backend ls my-s3:notes/ --vault ./my-notes`
- **THEN** SHALL 列出远端 `notes/` 前缀下的所有对象
- **AND** 输出 SHALL 包含 key、size、revision 摘要
- **AND** 输出 SHALL NOT 包含对象内容

#### Scenario: backend stat 查看单个对象

- **WHEN** 用户运行 `pinax backend stat my-s3:manifest.json --vault ./my-notes --json`
- **THEN** stdout SHALL 包含 JSON envelope，data 包含 key、size、revision
- **AND** SHALL NOT 包含对象 body 或 secret

#### Scenario: backend cp 跨 profile 复制

- **WHEN** 用户运行 `pinax backend cp local:notes/ my-s3:notes/ --dry-run --vault ./my-notes`
- **THEN** SHALL 显示复制计划但不执行
- **AND** 计划 SHALL 包含源 key、目标 key、size 和操作类型

#### Scenario: backend du 统计用量

- **WHEN** 用户运行 `pinax backend du my-s3 --vault ./my-notes`
- **THEN** SHALL 输出远端存储的对象数量和总大小

### Requirement: BlobStore 缓存装饰器

SHALL 提供透明的 BlobStore 缓存装饰器。

#### Scenario: Get 命中本地缓存

- **GIVEN** 之前 `Get("notes/a.md")` 的结果已缓存在本地
- **AND** 缓存 rev 与远端 rev 一致
- **WHEN** 再次 `Get("notes/a.md")`
- **THEN** SHALL 返回缓存数据，不访问远端
- **AND** 响应 SHALL 标记 `cache_hit: true`

#### Scenario: Get 缓存 miss

- **GIVEN** 本地缓存不存在或 rev 不匹配
- **WHEN** `Get("notes/a.md")`
- **THEN** SHALL 从远端获取并更新缓存

#### Scenario: 缓存大小限制

- **GIVEN** 缓存目录超过配置的 `max_size`
- **WHEN** 写入新缓存项
- **THEN** SHALL 按 LRU 清理最旧缓存项直到低于阈值

