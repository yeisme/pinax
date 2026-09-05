## 为什么

Pinax 已接入公共 promptrepo，但 federated catalog 搜索、preview 与安装 的默认 locale 仍可能选择中文模板，与统一 Agent 编译政策不一致。

## 变更内容

- Prompt Repository 默认 locale 改为 `en`。
- next action、CLI help、Skill 和当前操作文档改用英文 exact ref。
- 中文笔记、会议资料和最终笔记语言 继续作为业务字段传入，不改变领域语言能力。
- 旧中文 exact address 保留兼容检查，不被默认英文覆盖。

## 影响

只改变模板仓选择默认值，不改变 Pinax 的领域状态、provider、review、费用或资产生命周期。
