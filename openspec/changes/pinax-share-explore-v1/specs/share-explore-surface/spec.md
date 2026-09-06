## ADDED Requirements

### Requirement: Explore 数据投影 SHALL 有界且不含正文
`pinax.explore_bundle.v1` SHALL 由 graph 与 index 投影合成：节点携带 id/title/kind/tags/派生 trust/fresh/有界 summary，边携带 broken 标记；MUST NOT 携带 note 正文、绝对路径或凭据。容量超限时 MUST fail-safe 截断并以 `truncated=true` 显式声明。

#### Scenario: 截断
- **WHEN** vault 规模超过节点/边上限
- **THEN** bundle MUST 截断至上限并置 `truncated=true`
- **AND** 输出 MUST 提示改用 list 模式或收窄过滤。

#### Scenario: 无正文保证
- **WHEN** 递归扫描 bundle 任意深度
- **THEN** MUST NOT 出现 body/note_body/raw_body 类字段或正文片段（summary 为有界摘要）。

### Requirement: Explore 页面 SHALL 自包含且零外部请求
`/explore` 页面 MUST 为单文件自包含 HTML：JS/CSS 全内联，无 CDN、字体、脚本或任何外部 URL 引用；搜索、kind/trust/fresh 过滤、图/列表布局切换、backlinks 面板 MUST 全部客户端完成。

#### Scenario: 外联扫描
- **WHEN** 对页面 HTML 执行外链扫描（src/href 指向非相对路径）
- **THEN** MUST 零命中。

#### Scenario: 键盘可达
- **WHEN** 用户不使用鼠标
- **THEN** `/` 聚焦搜索、方向键导航列表、Enter 打开节点卡 MUST 可用。

### Requirement: explore 接线 SHALL 复用 share 只读与认证边界
`pinax share start --view explore` SHALL 仅在 `--scope vault-readonly` 下启用，复用现有 host/port/auth/`--allow-lan`/`--once` 生命周期。explore 全部数据端点 MUST 走既有认证（token 或 loopback no-auth）；note 预览端点 MUST 返回有界渲染并复用既有 redaction；explore MUST 严格只读且不向 vault 或 `.pinax/**` 写入任何文件。缺省 `--view`（published）行为 MUST 与现状零变更。

#### Scenario: 认证缺失
- **WHEN** 未认证请求 `/explore/data.json`
- **THEN** MUST 返回 401，MUST NOT 泄漏任何 bundle 数据。

#### Scenario: 默认行为回归
- **WHEN** 执行不带 `--view` 的 `pinax share start --once`
- **THEN** 输出 MUST 与既有 golden 一致（无 explore 路由注册）。

### Requirement: explore 冒烟 SHALL 纳入既有 once 合同
`pinax share start --view explore --once` SHALL 依次请求 `/explore`、`/explore/data.json` 与至少一个 note 有界预览并校验响应，随后退出；该冒烟 MUST 纳入 testscript e2e。

#### Scenario: once 冒烟
- **WHEN** 在 fixture vault 上执行 explore once 冒烟
- **THEN** 三个端点 MUST 返回成功且页面外联扫描零命中。
