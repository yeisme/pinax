# 意图分类

这里仅选择阅读材料，工具选择见 [入口路由表](../SKILL.md#按交付目标路由)。

| 意图 | 用户目标 | 按需参考 |
| --- | --- | --- |
| lookup | 单个事实、版本或 URL | [lightweight](lightweight.md) |
| research / verify | 比较或核验主张 | [standard](standard.md)、[evidence_policy](evidence_policy.md) |
| model-search | 大模型联网搜索、模糊或跨语言发现 | [model_search](model_search.md) |
| media-research | 获取并理解平台视频、拆解风格、寻找相似素材 | [media_research](media_research.md) |
| extract | 提取指定字段或正文 | [source_priority](source_priority.md)，缺内容时读 [dynamic_pages](dynamic_pages.md) |
| deep-research | 系统扫描或指定大量有效案例 | [deep_research](deep_research.md) |
| platform | 社交/视频/社区内容或能力诊断 | [agent_reach](agent_reach.md) |
| download | 视频、音频或字幕文件 | [media_download](media_download.md) |
| interact | 实际页面操作、截图、登录态检查 | [autonomous](autonomous.md)、[browser_tools](browser_tools.md) |
| automate | 可重复运行的流程 | [browser_tools](browser_tools.md)，沿用项目现有设施 |
| local-research-infra | 明确指定本机 OpenWebUI / 搜索基础设施 | [local_research_infra](local_research_infra.md) |

多个意图按真实依赖组合，例如先找官方 URL 再截图；已给 URL 就无需先搜索。用户只说“最新”不代表必须深度研究，要求截图则直接浏览器。不要把内部分类问题交给用户。
