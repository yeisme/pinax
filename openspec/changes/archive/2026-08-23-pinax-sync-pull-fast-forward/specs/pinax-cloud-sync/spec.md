## ADDED Requirements

### Requirement: pull SHALL 只在本地偏离 base 时保留冲突副本

pull apply 阶段 SHALL 统一采用 preserveConflict 规则：base 信息缺失、planner 判定本地偏离 base、或本地内容 hash 与上次同步状态不一致时保留冲突副本；三者为假时远端内容 SHALL 静默 fast-forward 落盘，不产生副本与 conflict 计数。

#### Scenario: 顺序编辑

- **WHEN** 设备 A 编辑并 push，本地未改的设备 B pull
- **THEN** B SHALL 落盘远端内容且不产生 `*.conflict.md`
- **AND** `sync.conflicts` SHALL 为 0

#### Scenario: 双方编辑

- **WHEN** 两设备相对 base 各自编辑同一笔记后一方 pull
- **THEN** SHALL 保留本地内容为冲突副本
- **AND** `sync.conflicts` SHALL 计数

#### Scenario: plan 后本地又被编辑

- **WHEN** pull plan 生成后、apply 前本地文件再次被修改
- **THEN** apply SHALL 保留冲突副本，不静默覆盖

### Requirement: 收敛 pull SHALL 报告 up_to_date

pull 在无待应用操作且本地/远端 manifest 内容一致时 SHALL 报告 `result=up_to_date`，facts 与 sync_view SHALL 与 push 侧 up-to-date 快路径一致。

#### Scenario: 二次 pull

- **WHEN** 已收敛设备再次 pull
- **THEN** `sync.result` SHALL 为 `up_to_date`
- **AND** `files_applied` SHALL 为 0
