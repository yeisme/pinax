# 统一模板发现合同

Manager.Discover 在共享用户仓库状态上提供独立的 DiscoveryRequest/DiscoveryResult：来源诊断、方案、角色语言文档、媒体、声明 consumer、能力与分页。旧 SearchRequest/SearchResult 不变。

默认缓存 TTL 5 分钟、最多 4 路并发、每来源 30 秒。网络操作在状态锁外完成；提交时比较 profile 和旧 snapshot，避免删除或修改后的配置被旧请求复活。失败保留旧 snapshot，partial 与 failed 区分可用缓存和全部不可用。Git 共用 mirror 的同步通过有界锁串行化。

TemplateRole 与 Solution 保持旧位置参数构造兼容。Catalog 新增独立 discovery 标注，CanonicalCatalogDigest 忽略这个描述性字段；原内容和引用不变，发现标注不参与执行授权。重复标记提供一个代表引用，避免大批相同模板产生平方级输出。

EikonaAdapter 使用固定本机 CLI 参数或远程 HTTP/MCP bridge 的已发现动作。不解析 provider payload，不读取 owner 数据库，不支持任意 shell 配置。来源凭据仅接受引用，经调用方注入的 CredentialResolver 在请求时解析；源端模板正文仍通过原 owner 显式读取。

新增 SetRepository 使用指针 patch 区分未提供和清除，来源／版本／凭据变更失效旧 snapshot。原用户配置路径和 schema 不变。

```bash
python3 scripts/test-discovery.py
CGO_ENABLED=0 go test ./...
go vet ./...
```

脚本仅包装现有 Go 测试并写入本 owner 的 temp/integration-test-runs；不引入新的测试框架。最终 Registry 与 Eikona 联调通过各自现有 runner 完成。

## 浏览查询与展示投影

新增 BrowseRequest 嵌入旧 DiscoveryRequest，并用 repositories/categories/media_types/consumers 等数组承载多选。旧字段不改类型。Manager.Browse 在一次源快照上投影记录，支持元数据关键词、角色语言、成熟度、权限声明和状态筛选；不读取正文。元数据中的其他语言名称也可搜索。

BrowseProjection 保存 records、groups、totals 和展示选择。entries 计算来源条目，unique_contents 仅以有效 SHA-256 digest 去重，缺少有效 digest 时按稳定引用独立计数。分组最多两级，多值成员关系可能重叠；统计使用分页前结果。默认按仓库/方案分组，支持明确的排序和方向；展示折叠不删除旧 solutions。

Conformance.BrowseCases 提供跨 Go 与 HTML 模型的测试样例，验证 OR/AND、未知字段、多语言和重叠统计。Query 元数据只描述发现，不提升编译或执行权限。
