## 默认选择

Pinax 的搜索与 solution resolve 在调用方没有指定 locale 时使用 `en`。对包含 locale 的 exact template address，CLI 不注入默认覆盖值，由 promptrepo 验证地址本身。

## 领域边界

模板骨架语言与业务内容语言分离。中文笔记、会议资料和最终笔记语言 可以继续使用中文或其他语言。中文模板译文只供人类审阅。

## 兼容与验证

旧测试 fixture 可以显式指定 `zh-CN` 验证兼容路径；新增默认路径和共享 canary 使用 `en`。运行 Pinax prompt repository focused tests 与根级五消费者 canary。
