# pinax-human-reading-api-v1

面向人类阅读的授权分页正文、关系与附件合同及受监督本机会话；保持旧 Agent 合同不变。

- [提案](proposal.md) / [设计](design.md) / [9项待办](tasks.md)
- Specs：[人类阅读](specs/human-reading-api/spec.md)、[受监督会话](specs/supervised-local-session/spec.md)
- [唯一详细API合同](../../../docs/interfaces/human-reading-api.md) / [DriveBridge消费交接](../../../docs/product/web-open-design.md)

P01–P07 已实现：七个只读 reader 端点、typed client、受监督会话与 component 证据全部落地（manifest digest 见 docs/interfaces/human-reading-api.md §7）。原五 change 的11项真实验收不变，消费者UI和三平台证据归DriveBridge。
