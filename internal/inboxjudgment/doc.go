// Package inboxjudgment 为笔记 inbox 分类、关联与重复线索提供显式 opt-in
// 的结构化判断建议（pinax-inbox-judgment-v1，exploratory）。
//
// 边界（design.md）：Pinax 是笔记与记忆事实 owner，模型仅辅助审阅。本包
// 只产出待审建议与脱敏 evidence，建议接受仍要经过原有 inbox 审阅入口与
// 显式采纳门；不得自动合并或删除笔记、确认长期记忆、修改 vault canon、
// 启动同步或发布。确定性规则（权限、必需字段、类型、可计算约束）先于
// 模型执行，不能用概率替代。
//
// 默认 mode=off：零发现、零远程调用；显式 shadow/assist 也必须注入
// transport 并复用领域授权 seam。SDK 合同形状（DescribeCapabilities/
// Evaluate、snake_case wire、schema_version "1.0"、8 个错误码 +
// submission_state/retry_class、choice/ordinal_score/binary 原语）由
// 本包 wire.go 以零外部依赖方式钉住；公共 SDK 模块就绪后通过 build-tag
// 隔离的 bridge 对接，默认构建不受 SDK 树漂移影响。
package inboxjudgment
