# 浏览器工具操作

工具路由只见 [入口](../SKILL.md#按交付目标路由)。浏览器专属任务直接进入本流程，不要求 Firecrawl 失败记录。

## 选择现有工具

优先当前运行时已有浏览器能力；维护测试用项目现有 Playwright 配置，一次视觉检查可用已配置的 agent-browser 或 browser-use。只检查选中的工具，不遍历安装所有后端。下面是命令示例，先用当前 help 确认支持的子命令；不要把 `npx playwright` 当作能直接执行任意点击的通用命令。

```bash
agent-browser --help
agent-browser open "https://example.com"
agent-browser snapshot
agent-browser screenshot /tmp/page.png
agent-browser close
```

```bash
npx playwright --help
npx playwright test --headed
npx playwright show-report
```

只在项目已有依赖时使用对应 npx 命令；工具缺失不自动下载安装。先检查项目测试入口，不盲目运行 npm test 或全量 e2e。

## 状态与操作

- 用最新 snapshot 的引用操作，不猜测 @ref；导航或重渲染后重新获取引用。
- 提取字段可检查 `window.__NEXT_DATA__`、`window.__NUXT__`、`window.__INITIAL_STATE__`、`window._ROUTER_DATA` 或 JSON script；仅读取任务所需字段。
- 页面自身 XHR/API 可以帮助提取，但不能用它替代用户要求的点击、弹窗、分页或登录 UI 验收。
- 复用授权会话，不擅自导出 cookie、改变 profile、账号或浏览器配置。截图和日志避开秘密信息。
- 写操作前确认当前授权覆盖目标；明确授权继续有效。提交后以页面读回或业务记录核对，超时不盲重试。

## 启动失败

先区分工具启动与站点故障。仅在当前版本支持时运行只读 doctor，避免自动修复修改环境。CDP 启动失败不循环重开；可换已有可用浏览器。Playwright 缺浏览器时，优先兼容的已安装浏览器或现有配置；需要安装时先判断当前任务是否授权相应环境修改，不自动更新全局安装。

备用工具仍须满足同一验收需求。截图任务不能退化为正文抓取并声称完成；不可用时保留已验证部分和具体阻塞。

## 验收

检查最终 URL、身份范围、可见状态和业务结果；截图存在不等于内容正确。按需要保存截图、网络或 console 证据。说明浏览器选择理由仅在影响交付时有用，不强制输出虚构的 Firecrawl 失败原因。
