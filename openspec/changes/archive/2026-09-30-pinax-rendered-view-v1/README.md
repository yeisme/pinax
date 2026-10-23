# 当前实施授权与补充决定

2026-09-30：用户明确批准完成全部可本地实施的 Pinax spec，并要求实施计划。该决定解除本 change 的 0.1 时间门，允许在 2026-10-10 前实施 rendered-view；continuity 的真实观察与最终决策仍按原门槛执行。

用户选择由调用方携带 floating 首次解析的 resolved_revision。CLI 后续通过 --resolved-revision 传回，MCP 通过同名字段传回；解析保持只读，不创建引用状态或快照。既有 note preview 默认行为保留，显式 --view rendered 启用新增合同。
