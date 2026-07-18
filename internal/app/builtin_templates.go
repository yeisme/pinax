package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/templateengine"
)

func builtInTemplates() map[string]string {
	templates := map[string]string{
		// legacy `daily` 是 simple 模板，只保留给旧命令和用户自定义覆盖兼容；推荐 journal 流程使用 `journal.daily`。
		"daily": "# {{date}}\n\n## Notes\n\n- \n",
		"note":  "# {{title}}\n\n",
		// 推荐 journal 模板声明输出路径和 managed block，供 journal open/show/append 共享同一内容结构。
		"journal.daily":   strings.Join([]string{"---", "schema_version: pinax.template.v2", "kind: journal_template", "name: journal.daily", "title: Daily Note", "engine: go-template", "output:", "  path_pattern: daily/{{ .Date }}.md", "defaults:", "  kind: daily", "  status: active", "---", "# {{ .Date }}", "", "## Today's Focus", "", "- ", "", "## Notes", "", "- ", "", "## TaskBridge Daily Todo", "<!-- pinax:managed name=planning-daily -->", "", "<!-- /pinax:managed -->", "", "## Daily Task Review", "<!-- pinax:managed name=daily-task-review -->", "", "<!-- /pinax:managed -->", "", "## Pinax Captures", "<!-- pinax:managed name=daily-captures -->", "", "<!-- /pinax:managed -->", "", "## Review", ""}, "\n"),
		"journal.weekly":  strings.Join([]string{"---", "schema_version: pinax.template.v2", "kind: journal_template", "name: journal.weekly", "title: Weekly Note", "engine: go-template", "output:", "  path_pattern: weekly/{{ .Week }}.md", "defaults:", "  kind: weekly", "  status: active", "---", "# {{ .Date }}", "", "## Weekly Focus", "", "- ", "", "## Pinax Captures", "<!-- pinax:managed name=weekly-captures -->", "", "<!-- /pinax:managed -->", "", "## Review", ""}, "\n"),
		"journal.monthly": strings.Join([]string{"---", "schema_version: pinax.template.v2", "kind: journal_template", "name: journal.monthly", "title: Monthly Note", "engine: go-template", "output:", "  path_pattern: monthly/{{ .Month }}.md", "defaults:", "  kind: monthly", "  status: active", "---", "# {{ .Date }}", "", "## Monthly Themes", "", "- ", "", "## Pinax Captures", "<!-- pinax:managed name=monthly-captures -->", "", "<!-- /pinax:managed -->", "", "## Review", ""}, "\n"),
		// 推荐 index 模板只把可刷新内容放进 managed block，托管区块外的用户导航说明必须保留。
		"index.home": strings.Join([]string{"---", "schema_version: pinax.template.v2", "kind: index_template", "name: index.home", "title: Home Index", "engine: go-template", "output:", "  path_pattern: index/home.md", "defaults:", "  kind: index", "  status: system", "---", "# Home Index", "", "Users can write their own navigation notes outside managed blocks.", "", "<!-- pinax:managed name=recent -->", "No index results yet.", "<!-- /pinax:managed -->", ""}, "\n"),
		"project":    "# {{title}}\n\nproject: {{project}}\ntags: {{tags}}\n\n## Goals\n\n## Progress\n",
		"yaml":       "```yaml\ntitle: {{title}}\nproject: {{project}}\ntags: [{{tags}}]\nupdated_at: {{datetime}}\n```\n",
		"mermaid":    "# {{title}}\n\n```mermaid\nflowchart TD\n    A[{{title}}] --> B[{{project}}]\n```\n",
	}
	for name, body := range builtInIndexTemplates() {
		templates[name] = body
	}
	for name, body := range builtInNoteTemplates() {
		templates[name] = body
	}
	return templates
}

func builtInIndexTemplates() map[string]string {
	return map[string]string{
		// index query 只声明 metadata，由 Pinax SQL service 执行；模板函数和 Cobra command 都不拼 raw SQLite。
		"index.decisions": builtInIndexTemplate("index.decisions", "Decision Index", "index/decisions.md", "recent-decisions", "recent_decisions", "SELECT title, path, updated_at FROM notes WHERE kind = 'decision' ORDER BY updated_at DESC LIMIT 20"),
		"index.learning":  builtInIndexTemplate("index.learning", "Learning Index", "index/learning.md", "recent-learning", "recent_learning", "SELECT title, path, updated_at FROM notes WHERE kind = 'learning' ORDER BY updated_at DESC LIMIT 20"),
		"index.meetings":  builtInIndexTemplate("index.meetings", "Meeting Index", "index/meetings.md", "recent-meetings", "recent_meetings", "SELECT title, path, updated_at FROM notes WHERE kind = 'meeting' ORDER BY updated_at DESC LIMIT 20"),
		"index.research":  builtInIndexTemplate("index.research", "Research Index", "index/research.md", "recent-research", "recent_research", "SELECT title, path, updated_at FROM notes WHERE kind = 'research' ORDER BY updated_at DESC LIMIT 20"),
		"index.ideas":     builtInIndexTemplate("index.ideas", "Ideas Index", "index/ideas.md", "parked-ideas", "parked_ideas", "SELECT title, path, updated_at FROM notes WHERE kind = 'idea' AND status = 'parked' ORDER BY updated_at DESC LIMIT 50"),
		"index.inbox":     builtInIndexTemplate("index.inbox", "Inbox Index", "index/inbox.md", "inbox-queue", "inbox_queue", "SELECT title, path, updated_at FROM notes WHERE status = 'inbox' ORDER BY updated_at DESC LIMIT 20"),
		"index.drafts":    builtInIndexTemplate("index.drafts", "Drafts Index", "index/drafts.md", "drafts-queue", "drafts_queue", "SELECT title, path, updated_at FROM notes WHERE status = 'draft' ORDER BY updated_at DESC LIMIT 20"),
	}
}

func builtInIndexTemplate(name, title, pathPattern, blockName, queryName, sql string) string {
	lines := []string{"---", "schema_version: pinax.template.v2", "kind: index_template", "name: " + name, "title: " + title, "engine: go-template", "output:", "  path_pattern: " + pathPattern, "defaults:", "  kind: index", "  status: system", "queries:", "  " + queryName + ":", "    language: sql", "    kind: table", "    max_rows: 20", "    sql: |"}
	for _, line := range strings.Split(sql, "\n") {
		lines = append(lines, "      "+line)
	}
	lines = append(lines, "---", "# "+title, "", "<!-- pinax:managed name="+blockName+" -->", "No index results yet.", "<!-- /pinax:managed -->", "")
	return strings.Join(lines, "\n")
}

func builtInNoteTemplates() map[string]string {
	return map[string]string{
		// 快速记录模板只保留标题、正文和Next，避免把 capture 变成表单。
		"note.quick": builtInNoteTemplate("note.quick", "Quick Note", []string{"Quick Capture", "Scratch Idea"}, []string{"quick", "scratch"}, "starter", true, "{{ .Title }}.md", map[string]string{"kind": "note", "status": "active"}, []string{"# {{ .Title }}", "", "## Notes", "", "- ", "", "## Next Steps", "", "- "}),
		// Inbox Capture模板只问来源和待分拣动作，保持低摩擦入口。
		"inbox.capture": builtInNoteTemplate("inbox.capture", "Inbox Capture", []string{"Inbox", "Triage Later"}, []string{"inbox", "capture"}, "starter", true, "inbox/{{ .Title }}.md", map[string]string{"kind": "inbox", "status": "inbox"}, []string{"# {{ .Title }}", "", "## Capture", "", "- ", "", "## Source", "", "- "}),
		// Sticky模板是短文档捕获入口：进入 inbox，保留上下文线索，但不写 board_column 或受控 project item 元数据。
		"sticky.capture":        builtInNoteTemplate("sticky.capture", "便签", []string{"便签", "短文档", "快速记录", "Sticky Note"}, []string{"sticky", "便签", "短文档", "capture"}, "starter", true, "inbox/sticky/{{ .Title | slug }}.md", map[string]string{"kind": "sticky", "status": "inbox", "tags": "sticky,capture"}, []string{"# {{ .Title }}", "", "## 记录", "", "- ", "", "## 上下文", "", "- 时间：{{ date \"2006-01-02\" }}", "- 项目：{{ .Project }}", "", "## 分拣提示", "", "- "}),
		"sticky.quote":          builtInNoteTemplate("sticky.quote", "摘录便签", []string{"摘录", "引用", "短文档", "Quote Sticky"}, []string{"sticky", "quote", "摘录", "引用", "便签"}, "starter", true, "inbox/sticky/quotes/{{ .Title | slug }}.md", map[string]string{"kind": "sticky", "status": "inbox", "tags": "sticky,quote"}, []string{"# {{ .Title }}", "", "## 摘录", "", "> ", "", "## 来源", "", "- ", "", "## 为什么留下", "", "- "}),
		"sticky.link":           builtInNoteTemplate("sticky.link", "链接便签", []string{"链接", "网页", "资料线索", "Link Sticky"}, []string{"sticky", "link", "链接", "网页", "资料", "便签"}, "starter", true, "inbox/sticky/links/{{ .Title | slug }}.md", map[string]string{"kind": "sticky", "status": "inbox", "tags": "sticky,link"}, []string{"# {{ .Title }}", "", "Link: {{ default \"<paste link>\" (index .Vars \"url\") }}", "", "## 为什么收藏", "", "- ", "", "## 可能归档到", "", "- "}),
		"sticky.question":       builtInNoteTemplate("sticky.question", "问题便签", []string{"问题", "待查", "疑问", "Question Sticky"}, []string{"sticky", "question", "问题", "待查", "疑问", "便签"}, "starter", true, "inbox/sticky/questions/{{ .Title | slug }}.md", map[string]string{"kind": "sticky", "status": "inbox", "tags": "sticky,question"}, []string{"# {{ .Title }}", "", "## 问题", "", "- ", "", "## 已知线索", "", "- ", "", "## 需要补充", "", "- "}),
		"sticky.term":           builtInNoteTemplate("sticky.term", "术语便签", []string{"术语", "概念", "名词", "Term Sticky"}, []string{"sticky", "term", "术语", "概念", "名词", "便签"}, "starter", true, "inbox/sticky/terms/{{ .Title | slug }}.md", map[string]string{"kind": "sticky", "status": "inbox", "tags": "sticky,term"}, []string{"# {{ .Title }}", "", "## 初步理解", "", "- ", "", "## 出现场景", "", "- ", "", "## 相关词", "", "- "}),
		"sticky.person_signal":  builtInNoteTemplate("sticky.person_signal", "人物线索便签", []string{"人物线索", "人名", "组织线索", "Person Signal"}, []string{"sticky", "person", "people", "人物", "人名", "组织", "便签"}, "starter", true, "inbox/sticky/people/{{ .Title | slug }}.md", map[string]string{"kind": "sticky", "status": "inbox", "tags": "sticky,person"}, []string{"# {{ .Title }}", "", "## 是谁", "", "- ", "", "## 出现在哪里", "", "- ", "", "## 后续关联", "", "- "}),
		"sticky.project_signal": builtInNoteTemplate("sticky.project_signal", "项目线索便签", []string{"项目线索", "子项目看板线索", "看板上下文", "Project Signal"}, []string{"sticky", "project", "subproject", "board", "项目", "子项目", "看板", "线索", "便签"}, "starter", true, "inbox/sticky/projects/{{ .Title | slug }}.md", map[string]string{"kind": "sticky", "status": "inbox", "tags": "sticky,project-signal"}, []string{"# {{ .Title }}", "", "## 线索", "", "- ", "", "## 项目上下文", "", "- 项目：{{ .Project }}", "- 子项目/看板：{{ default \"<workspace or board hint>\" (index .Vars \"target\") }}", "", "## 分拣建议", "", "- 如需成为受控看板项，使用 `pinax project item add` 创建。"}),
		// Meeting模板只包含议题、决议、行动项，避免冗长Meeting纪要框架。
		"meeting.notes": builtInNoteTemplate("meeting.notes", "Meeting Notes", []string{"Meeting", "Sync"}, []string{"meeting", "notes"}, "focused", false, "meetings/{{ .Title }}.md", map[string]string{"kind": "meeting", "status": "active"}, []string{"# {{ .Title }}", "", "## 议题", "", "- ", "", "## 决议", "", "- ", "", "## 行动项", "", "- "}),
		// Decision模板聚焦背景、选择和后果，便于后续索引和复盘。
		"decision.record": builtInNoteTemplate("decision.record", "Decision Record", []string{"Decision", "Tradeoffs"}, []string{"decision", "record"}, "focused", false, "decisions/{{ .Title }}.md", map[string]string{"kind": "decision", "status": "active"}, []string{"# {{ .Title }}", "", "## Background", "", "## Decision", "", "## Tradeoffs", "", "## Follow-up Check", ""}),
		// Project brief 模板只保留目标、范围、风险和Next，适合作为Project入口页。
		"project.brief": builtInNoteTemplate("project.brief", "Project Brief", []string{"Project", "Plan"}, []string{"project", "brief"}, "focused", false, "projects/{{ .Title }}.md", map[string]string{"kind": "project", "status": "active"}, []string{"# {{ .Title }}", "", "## Goals", "", "## Scope", "", "## Risks", "", "## Next Steps", ""}),
		// Video Learning模板保留来源、观点、例子和待查问题，避免转录全文。
		"learning.video": builtInNoteTemplate("learning.video", "视频笔记", []string{"视频学习", "课程笔记", "Video Learning", "视频"}, []string{"video", "learning", "视频", "课程"}, "starter", true, "learning/videos/{{ .Title | slug }}.md", map[string]string{"kind": "learning", "status": "active", "tags": "learning,video"}, []string{"# {{ .Title }}", "", "## 来源信息", "", "- 链接：{{ .Vars.url }}", "- 作者/频道：", "- 发布时间：", "- 观看时间：", "", "## 一句话结论", "", "## 关键观点", "", "- ", "", "## 证据或例子", "", "- ", "", "## 可复用片段", "", "- ", "", "## 待查问题", "", "- ", "", "## 相关笔记", "", "- "}),
		// Book Learning模板保留章节脉络、摘录、实践和反对意见，适合长文渐进阅读。
		"learning.book":                builtInNoteTemplate("learning.book", "书籍长文阅读", []string{"书籍阅读", "长文阅读", "Learning", "Reading"}, []string{"book", "reading", "书籍", "长文", "阅读"}, "starter", true, "learning/books/{{ .Title | slug }}.md", map[string]string{"kind": "learning", "status": "active", "tags": "learning,book"}, []string{"# {{ .Title }}", "", "## 书籍信息", "", "- 作者：", "- 版本/来源：", "- 阅读时间：", "", "## 章节脉络", "", "## 关键观点", "", "- ", "", "## 摘录", "", "- ", "", "## 可以实践的地方", "", "- ", "", "## 反对意见或疑问", "", "- ", "", "## 相关笔记", "", "- "}),
		"learning.term":                builtInNoteTemplate("learning.term", "术语卡", []string{"术语卡", "概念卡", "Learning Term"}, []string{"learning", "term", "术语", "概念", "名词"}, "starter", true, "learning/terms/{{ .Title | slug }}.md", map[string]string{"kind": "learning", "status": "active", "tags": "learning,term"}, []string{"# {{ .Title }}", "", "## 定义", "", "- ", "", "## 使用场景", "", "- ", "", "## 常见误解", "", "- ", "", "## 来源", "", "- ", "", "## 相关术语", "", "- "}),
		"learning.source":              builtInNoteTemplate("learning.source", "学习资料来源", []string{"资料来源", "课程来源", "Learning Source"}, []string{"learning", "source", "资料", "来源", "课程"}, "starter", true, "learning/sources/{{ .Title | slug }}.md", map[string]string{"kind": "reference", "status": "active", "tags": "learning,source"}, []string{"# {{ .Title }}", "", "## 来源信息", "", "- 链接：{{ .Vars.url }}", "- 作者/机构：", "- 发布时间：", "- 记录时间：{{ date \"2006-01-02\" }}", "", "## 可信度", "", "- ", "", "## 关键内容", "", "- ", "", "## 待核验", "", "- "}),
		"learning.practice_log":        builtInNoteTemplate("learning.practice_log", "练习记录", []string{"练习记录", "实践记录", "Learning Practice"}, []string{"learning", "practice", "练习", "实践", "记录"}, "focused", false, "learning/practice/{{ .Title | slug }}.md", map[string]string{"kind": "learning", "status": "active", "tags": "learning,practice"}, []string{"# {{ .Title }}", "", "## 练习目标", "", "## 方法", "", "- ", "", "## 结果", "", "- ", "", "## 错误", "", "- ", "", "## 下次调整", "", "- "}),
		"learning.weekly_review":       builtInNoteTemplate("learning.weekly_review", "学习周复盘", []string{"周复盘", "学习复盘", "Weekly Learning Review"}, []string{"learning", "weekly", "review", "周复盘", "复盘"}, "focused", false, "learning/reviews/{{ .Title | slug }}.md", map[string]string{"kind": "review", "status": "active", "tags": "learning,weekly-review"}, []string{"# {{ .Title }}", "", "## 本周学习", "", "- ", "", "## 已掌握", "", "- ", "", "## 仍然模糊", "", "- ", "", "## 错误与修正", "", "- ", "", "## 下周动作", "", "- "}),
		"learning.case_review":         builtInNoteTemplate("learning.case_review", "学习案例复盘", []string{"案例复盘", "案例学习", "Learning Case Review"}, []string{"learning", "case", "review", "案例", "复盘"}, "focused", false, "learning/cases/{{ .Title | slug }}.md", map[string]string{"kind": "case_review", "status": "active", "tags": "learning,case-review"}, []string{"# {{ .Title }}", "", "## 案例背景", "", "## 观察", "", "- ", "", "## 当时判断", "", "- ", "", "## 结果", "", "- ", "", "## 学到什么", "", "- ", "", "## 后续问题", "", "- "}),
		"learning.stock.term":          builtInNoteTemplate("learning.stock.term", "股票术语卡", []string{"股票术语", "交易术语", "Stock Term"}, []string{"stock", "股票", "炒股", "交易", "术语", "概念"}, "starter", true, "learning/stock/terms/{{ .Title | slug }}.md", map[string]string{"kind": "learning", "status": "active", "tags": "learning,stock,term"}, []string{"# {{ .Title }}", "", "## 定义", "", "- ", "", "## 在股票学习中的作用", "", "- ", "", "## 容易误解的点", "", "- ", "", "## 来源", "", "- ", "", "## 边界", "", "仅用于学习和复盘，不构成投资建议。"}),
		"learning.stock.indicator":     builtInNoteTemplate("learning.stock.indicator", "股票指标笔记", []string{"股票指标", "K线指标", "技术分析学习"}, []string{"stock", "股票", "炒股", "K线", "成交量", "指标", "技术分析"}, "starter", true, "learning/stock/indicators/{{ .Title | slug }}.md", map[string]string{"kind": "learning", "status": "active", "tags": "learning,stock,indicator"}, []string{"# {{ .Title }}", "", "## 指标定义", "", "- ", "", "## 观察维度", "", "- ", "", "## 常见误读", "", "- ", "", "## 历史案例", "", "- ", "", "## 边界", "", "仅用于学习、历史观察和模拟练习，不构成投资建议或自动交易决策。"}),
		"learning.stock.case_review":   builtInNoteTemplate("learning.stock.case_review", "股票案例复盘", []string{"股票案例", "行情复盘", "Stock Case Review"}, []string{"stock", "股票", "炒股", "案例", "行情", "复盘"}, "focused", false, "learning/stock/cases/{{ .Title | slug }}.md", map[string]string{"kind": "case_review", "status": "active", "tags": "learning,stock,case-review"}, []string{"# {{ .Title }}", "", "## 案例背景", "", "## 当时可见信息", "", "- ", "", "## 走势记录", "", "- ", "", "## 事后复盘", "", "- ", "", "## 错误清单", "", "- ", "", "## 边界", "", "仅用于历史案例学习，不构成投资建议。"}),
		"learning.stock.trade_journal": builtInNoteTemplate("learning.stock.trade_journal", "模拟交易日志", []string{"模拟交易", "交易日志", "Paper Trade Journal"}, []string{"stock", "股票", "炒股", "模拟", "交易日志", "复盘"}, "focused", false, "learning/stock/journal/{{ .Title | slug }}.md", map[string]string{"kind": "journal", "status": "active", "tags": "learning,stock,paper-trade"}, []string{"# {{ .Title }}", "", "## 模拟场景", "", "- ", "", "## 计划", "", "- ", "", "## 执行记录", "", "- ", "", "## 结果", "", "- ", "", "## 复盘", "", "- ", "", "## 边界", "", "仅用于模拟练习和学习复盘，不构成实际交易建议。"}),
		"learning.stock.risk_rule":     builtInNoteTemplate("learning.stock.risk_rule", "股票风险规则", []string{"风险规则", "交易纪律", "Stock Risk Rule"}, []string{"stock", "股票", "炒股", "风险", "规则", "纪律", "止损"}, "focused", false, "learning/stock/risk/{{ .Title | slug }}.md", map[string]string{"kind": "rule", "status": "active", "tags": "learning,stock,risk-rule"}, []string{"# {{ .Title }}", "", "## 规则", "", "- ", "", "## 触发条件", "", "- ", "", "## 为什么需要", "", "- ", "", "## 违反后的复盘", "", "- ", "", "## 边界", "", "仅用于风险教育和自我约束记录，不构成投资建议。"}),
		"learning.stock.weekly_review": builtInNoteTemplate("learning.stock.weekly_review", "股票学习周复盘", []string{"股票学习复盘", "炒股周复盘", "Stock Weekly Review"}, []string{"stock", "股票", "炒股", "周复盘", "复盘", "学习"}, "focused", false, "learning/stock/reviews/{{ .Title | slug }}.md", map[string]string{"kind": "review", "status": "active", "tags": "learning,stock,weekly-review"}, []string{"# {{ .Title }}", "", "## 本周学习", "", "- ", "", "## 新增术语", "", "- ", "", "## 案例复盘", "", "- ", "", "## 错误与风险", "", "- ", "", "## 下周动作", "", "- ", "", "## 边界", "", "仅用于学习复盘，不构成投资建议。"}),
		// Research Topic模板用研究问题、证据、暂定判断和反例分层，避免混杂资料堆放。
		"research.topic": builtInNoteTemplate("research.topic", "研究主题", []string{"研究主题", "资料整理", "Research", "Reference Curation"}, []string{"research", "topic", "研究", "资料"}, "focused", false, "research/{{ .Title | slug }}.md", map[string]string{"kind": "research", "status": "active", "tags": "research,topic"}, []string{"# {{ .Title }}", "", "## 研究问题", "", "## 范围", "", "## 已有线索", "", "- ", "", "## 证据表", "", "- 来源：", "- 结论：", "- 可信度：", "", "## 暂定判断", "", "## 反例或不确定点", "", "- ", "", "## 下一轮问题", "", "- ", "", "## 相关笔记", "", "- "}),
		// Idea模板只停放观察、线索和问题，不默认生成任务。
		"idea.research_seed": builtInNoteTemplate("idea.research_seed", "想法研究种子", []string{"想法", "研究种子", "日后调查", "Idea", "Research Seed"}, []string{"idea", "research-seed", "想法", "调查", "日后"}, "starter", true, "ideas/research/{{ .Title | slug }}.md", map[string]string{"kind": "idea", "status": "parked", "tags": "idea,research-seed"}, []string{"# {{ .Title }}", "", "## 触发点", "", "## 为什么值得查", "", "## 核心问题", "", "- ", "", "## 已有线索", "", "- ", "", "## 相关笔记", "", "- "}),
		"idea.drama_watch":   builtInNoteTemplate("idea.drama_watch", "看剧想法", []string{"看剧", "剧集", "影视想法", "Drama"}, []string{"drama", "series", "看剧", "电视剧", "影视"}, "starter", true, "ideas/drama/{{ .Title | slug }}.md", map[string]string{"kind": "idea", "status": "parked", "tags": "idea,media,drama"}, []string{"# {{ .Title }}", "", "## 触发点", "", "## 想观察什么", "", "## 剧情线索", "", "- ", "", "## 人物线索", "", "- ", "", "## 可能关联", "", "- ", "", "## 日后问题", "", "- "}),
		"idea.anime_watch":   builtInNoteTemplate("idea.anime_watch", "动漫想法", []string{"动漫", "动画", "番剧", "Anime"}, []string{"anime", "animation", "动漫", "动画", "番剧"}, "starter", true, "ideas/anime/{{ .Title | slug }}.md", map[string]string{"kind": "idea", "status": "parked", "tags": "idea,media,anime"}, []string{"# {{ .Title }}", "", "## 触发点", "", "## 设定兴趣", "", "## 角色线索", "", "- ", "", "## 演出线索", "", "- ", "", "## 可借鉴点", "", "- ", "", "## 相关笔记", "", "- "}),
		"idea.game_explore":  builtInNoteTemplate("idea.game_explore", "游戏探索想法", []string{"游戏", "玩法研究", "Game"}, []string{"game", "play", "游戏", "玩法"}, "starter", true, "ideas/games/{{ .Title | slug }}.md", map[string]string{"kind": "idea", "status": "parked", "tags": "idea,game"}, []string{"# {{ .Title }}", "", "## 触发点", "", "## 想体验或研究", "", "## 玩法线索", "", "- ", "", "## 美术或叙事线索", "", "- ", "", "## 待查问题", "", "- "}),
		"idea.paper_read":    builtInNoteTemplate("idea.paper_read", "论文阅读想法", []string{"论文", "论文阅读", "Paper"}, []string{"paper", "论文", "阅读", "研究"}, "starter", true, "ideas/papers/{{ .Title | slug }}.md", map[string]string{"kind": "idea", "status": "parked", "tags": "idea,research,paper"}, []string{"# {{ .Title }}", "", "## 为什么要读", "", "## 要回答的问题", "", "- ", "", "## 关键词", "", "- ", "", "## 来源线索", "", "- ", "", "## 相关主题", "", "- "}),
		"idea.novel_read":    builtInNoteTemplate("idea.novel_read", "小说阅读想法", []string{"小说阅读", "阅读想法", "Novel"}, []string{"novel", "reading", "小说", "阅读"}, "starter", true, "ideas/novels/reading/{{ .Title | slug }}.md", map[string]string{"kind": "idea", "status": "parked", "tags": "idea,reading,novel"}, []string{"# {{ .Title }}", "", "## 想读原因", "", "## 想观察的技法", "", "- ", "", "## 作者或作品线索", "", "- ", "", "## 相关笔记", "", "- "}),
		"idea.novel_write":   builtInNoteTemplate("idea.novel_write", "小说创作想法", []string{"写小说", "小说创作", "Novel Writing"}, []string{"novel", "writing", "写小说", "创作", "小说"}, "starter", true, "ideas/novels/writing/{{ .Title | slug }}.md", map[string]string{"kind": "idea", "status": "parked", "tags": "idea,writing,novel"}, []string{"# {{ .Title }}", "", "## 灵感", "", "## 核心冲突", "", "## 人物种子", "", "- ", "", "## 世界规则", "", "- ", "", "## 不确定点", "", "- "}),
		"idea.video_note":    builtInNoteTemplate("idea.video_note", "视频笔记想法", []string{"视频", "视频笔记", "Video"}, []string{"video", "视频", "收藏", "笔记"}, "starter", true, "ideas/videos/{{ .Title | slug }}.md", map[string]string{"kind": "idea", "status": "parked", "tags": "idea,video"}, []string{"# {{ .Title }}", "", "## 为什么收藏", "", "## 想提炼什么", "", "## 来源线索", "", "- ", "", "## 关键词", "", "- ", "", "## 相关笔记", "", "- "}),
		"media.drama":        builtInNoteTemplate("media.drama", "剧集笔记", []string{"看剧", "剧集复盘", "Drama"}, []string{"drama", "series", "看剧", "电视剧", "影视"}, "focused", false, "media/drama/{{ .Title | slug }}.md", map[string]string{"kind": "media", "status": "active", "tags": "media,drama"}, []string{"# {{ .Title }}", "", "## 基本信息", "", "- 类型：", "- 年份/平台：", "- 观看进度：", "", "## 观看背景", "", "## 人物关系", "", "## 剧情结构", "", "## 主题与情绪", "", "## 桥段记录", "", "- ", "", "## 复盘问题", "", "- ", "", "## 相关笔记", "", "- "}),
		"media.anime":        builtInNoteTemplate("media.anime", "动漫笔记", []string{"动漫", "动画复盘", "Anime"}, []string{"anime", "animation", "动漫", "动画", "番剧"}, "focused", false, "media/anime/{{ .Title | slug }}.md", map[string]string{"kind": "media", "status": "active", "tags": "media,anime"}, []string{"# {{ .Title }}", "", "## 基本信息", "", "- 类型：", "- 年份/制作：", "- 观看进度：", "", "## 设定与世界观", "", "## 角色弧线", "", "## 分镜与演出", "", "## 主题表达", "", "## 可借鉴点", "", "- ", "", "## 相关笔记", "", "- "}),
		"game.playlog":       builtInNoteTemplate("game.playlog", "游戏体验笔记", []string{"游戏", "游戏体验", "玩法复盘", "Game"}, []string{"game", "playlog", "游戏", "玩法"}, "focused", false, "games/{{ .Title | slug }}.md", map[string]string{"kind": "game", "status": "active", "tags": "game,playlog"}, []string{"# {{ .Title }}", "", "## 基本信息", "", "- 平台：", "- 类型：", "- 体验进度：", "", "## 核心循环", "", "## 系统与数值", "", "## 关卡体验", "", "## 叙事与美术", "", "## 可借鉴点", "", "- ", "", "## 复盘问题", "", "- "}),
		"reading.paper":      builtInNoteTemplate("reading.paper", "论文阅读笔记", []string{"论文阅读", "Paper Reading", "研究"}, []string{"paper", "论文", "阅读", "研究"}, "focused", false, "reading/papers/{{ .Title | slug }}.md", map[string]string{"kind": "research", "status": "active", "tags": "research,paper"}, []string{"# {{ .Title }}", "", "## 论文信息", "", "- 作者：", "- 来源：", "- 年份：", "", "## 研究问题", "", "## 方法", "", "## 核心结论", "", "## 证据质量", "", "## 局限", "", "## 可引用句", "", "- ", "", "## 相关笔记", "", "- "}),
		"reading.novel":      builtInNoteTemplate("reading.novel", "小说阅读笔记", []string{"小说阅读", "Novel Reading", "阅读"}, []string{"novel", "reading", "小说", "阅读"}, "focused", false, "reading/novels/{{ .Title | slug }}.md", map[string]string{"kind": "reading", "status": "active", "tags": "reading,novel"}, []string{"# {{ .Title }}", "", "## 作品信息", "", "- 作者：", "- 类型：", "- 阅读进度：", "", "## 阅读动机", "", "## 人物关系", "", "## 叙事结构", "", "## 主题", "", "## 技法摘录", "", "- ", "", "## 阅读感受", "", "## 相关笔记", "", "- "}),
		"writing.novel":      builtInNoteTemplate("writing.novel", "小说创作笔记", []string{"写小说", "小说创作", "Novel Writing"}, []string{"novel", "writing", "写小说", "创作", "小说"}, "focused", false, "writing/novels/{{ .Title | slug }}.md", map[string]string{"kind": "writing", "status": "active", "tags": "writing,novel"}, []string{"# {{ .Title }}", "", "## 核心概念", "", "## 主题承诺", "", "## 主角与欲望", "", "## 冲突", "", "## 世界规则", "", "## 章节骨架", "", "## 素材线索", "", "- ", "", "## 风险问题", "", "- "}),
		// Source GitHub模板把外部仓库保存成可长期复查的资料源卡片；不联网抓取事实，只保留用户提供的URL和审稿结构。
		"source.github": builtInNoteTemplate("source.github", "GitHub Source", []string{"External Source", "GitHub Repository", "Reference Curation"}, []string{"source", "github", "repo"}, "focused", false, "sources/github/{{ .Title | slug }}.md", map[string]string{"kind": "source", "status": "active", "tags": "source/github,reference/source"}, []string{"# {{ .Title }}", "", "Source: {{ .Vars.url }}", "", "## 一句话", "", "", "## Source facts", "", "- Repository: {{ .Vars.url }}", "- Maintainer:", "- License:", "- Last checked:", "", "## Canonical URLs", "", "- README: {{ .Vars.url }}", "- License:", "- Documentation:", "- Related repositories:", "", "## Use decision", "", "- Decision: review before production use", "- Allowed use:", "- Avoid:", "", "## Risk and boundary", "", "- Stability:", "- Legal/license:", "- Supply chain:", "", "## Verification", "", "- [ ] Recheck repository availability", "- [ ] Review license and usage boundary", "- [ ] Test only with explicit user approval", "", "## Related notes", "", "- ", "", "## Next actions", "", "- [ ] Decide whether this source belongs in a project workflow"}),
		// Person Profile模板只保留关系、上下文和待跟进，避免收集无关个人信息。
		"person.profile": builtInNoteTemplate("person.profile", "Person Profile", []string{"Person", "Relationship Follow-up"}, []string{"person", "profile"}, "focused", false, "people/{{ .Title }}.md", map[string]string{"kind": "person", "status": "active"}, []string{"# {{ .Title }}", "", "## Relationship", "", "## Context", "", "## Follow-up", "", "- "}),
	}
}

func builtInNoteTemplate(name, title string, useCases, aliases []string, difficulty string, starter bool, pathPattern string, defaults map[string]string, body []string) string {
	scenarioID := workflowScenarioID(name)
	maturity := workflowMaturity(name, templateengine.Metadata{Kind: "note_template"})
	lines := []string{"---", "schema_version: pinax.template.v2", "kind: note_template", "template_kind: note_template", "name: " + name, "title: " + title, "engine: go-template", "scenario_id: " + scenarioID, "maturity: " + maturity, "lifecycle: published_executable", "pack:", "  id: " + difficulty, "  source: builtin", "  readiness: " + maturity, "intents:"}
	for _, item := range append(append([]string{}, useCases...), aliases...) {
		lines = append(lines, "  - "+item)
	}
	lines = append(lines, "use_cases:")
	for _, item := range useCases {
		lines = append(lines, "  - "+item)
	}
	lines = append(lines, "aliases:")
	for _, item := range aliases {
		lines = append(lines, "  - "+item)
	}
	lines = append(lines, "difficulty: "+difficulty)
	if starter {
		lines = append(lines, "starter: true")
	} else {
		lines = append(lines, "starter: false")
	}
	lines = append(lines, "output:", "  path_pattern: \""+pathPattern+"\"", "output_policy:", "  path_pattern: \""+pathPattern+"\"", "  allow_override: true", "  write_boundary: vault-content", "  legacy_compatible: true", "proof_gate:", "  status: review_optional")
	if name == "meeting.notes" || name == "decision.record" {
		lines = append(lines, "  manual_review: true")
	} else {
		lines = append(lines, "  manual_review: false")
	}
	lines = append(lines, "  snapshot_required: false", "  receipt_required: false", "after_create_actions:", "  - name: proof_review", "    command: \"pinax proof loop run --vault <vault> --json\"", "defaults:")
	keys := make([]string, 0, len(defaults))
	for key := range defaults {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, "  "+key+": "+defaults[key])
	}
	lines = append(lines, "---")
	lines = append(lines, body...)
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func TemplateCompletionItems(root, kind string, includeBuiltins, includeLocal bool) []string {
	items := []string{}
	if includeBuiltins {
		for name, body := range builtInTemplates() {
			if item := templateCompletionItem(name, body, "builtin", kind); item != "" {
				items = append(items, item)
			}
		}
	}
	if includeLocal {
		if names, err := listTemplates(root); err == nil {
			for _, name := range names {
				body, err := loadTemplate(root, name)
				if err != nil {
					continue
				}
				if item := templateCompletionItem(name, body, "local", kind); item != "" {
					items = append(items, item)
				}
			}
		}
	}
	sort.Strings(items)
	return items
}

func templateCompletionItem(name, body, source, kindFilter string) string {
	doc, err := templateengine.ParseDocument(name, body)
	if err != nil {
		return ""
	}
	kind := doc.Metadata.Kind
	if kind == "" {
		kind = "template"
	}
	if kindFilter != "" && kind != kindFilter {
		return ""
	}
	return name + "\t" + source + " " + kind
}

func TemplateVariableCompletionItems(root, name string) []string {
	body, err := loadTemplate(root, name)
	if err != nil {
		return nil
	}
	doc, err := templateengine.ParseDocument(name, body)
	if err != nil {
		return nil
	}
	items := make([]string, 0, len(doc.Metadata.Variables))
	for key, variable := range doc.Metadata.Variables {
		desc := "optional string"
		if variable.Required {
			desc = "required string"
		}
		if variable.Description != "" {
			desc += " " + variable.Description
		}
		items = append(items, key+"=\t"+desc)
	}
	sort.Strings(items)
	return items
}

type TemplateCatalogItem struct {
	Name               string                                     `json:"name"`
	Source             string                                     `json:"source"`
	Kind               string                                     `json:"kind"`
	Title              string                                     `json:"title,omitempty"`
	UseCases           []string                                   `json:"use_cases,omitempty"`
	Aliases            []string                                   `json:"aliases,omitempty"`
	Difficulty         string                                     `json:"difficulty,omitempty"`
	Starter            bool                                       `json:"starter"`
	ScenarioID         string                                     `json:"scenario_id,omitempty"`
	TemplateKind       string                                     `json:"template_kind,omitempty"`
	Intents            []string                                   `json:"intents,omitempty"`
	VariableSchema     map[string]templateengine.VariableMetadata `json:"variable_schema,omitempty"`
	OutputPolicy       templateengine.TemplateOutputPolicy        `json:"output_policy,omitempty"`
	AfterCreateActions []templateengine.TemplateActionMetadata    `json:"after_create_actions,omitempty"`
	Maturity           string                                     `json:"maturity,omitempty"`
	ProofGate          templateengine.TemplateProofGate           `json:"proof_gate,omitempty"`
	Pack               TemplatePack                               `json:"pack,omitempty"`
	Lifecycle          string                                     `json:"lifecycle,omitempty"`
	Replacement        string                                     `json:"replacement,omitempty"`
	Metrics            map[string]string                          `json:"metrics,omitempty"`
}

type TemplatePack struct {
	ID        string `json:"id,omitempty"`
	Source    string `json:"source,omitempty"`
	Version   string `json:"version,omitempty"`
	Readiness string `json:"readiness,omitempty"`
}

type WorkflowRecommendation struct {
	Template           string                                  `json:"template"`
	ScenarioID         string                                  `json:"scenario_id,omitempty"`
	Maturity           string                                  `json:"maturity,omitempty"`
	Pack               TemplatePack                            `json:"pack,omitempty"`
	FitReason          string                                  `json:"fit_reason,omitempty"`
	PreviewCommand     string                                  `json:"preview_command,omitempty"`
	CreateCommand      string                                  `json:"create_command,omitempty"`
	EvidencePath       string                                  `json:"evidence_path,omitempty"`
	ProofGate          templateengine.TemplateProofGate        `json:"proof_gate,omitempty"`
	AfterCreateActions []templateengine.TemplateActionMetadata `json:"after_create_actions,omitempty"`
	Lifecycle          string                                  `json:"lifecycle,omitempty"`
	Executable         bool                                    `json:"executable"`
	Replacement        string                                  `json:"replacement,omitempty"`
}

func templateCatalogItems(root string) []TemplateCatalogItem {
	items := []TemplateCatalogItem{}
	for name, body := range builtInTemplates() {
		if item, ok := templateCatalogItem(name, body, "builtin"); ok {
			items = append(items, item)
		}
	}
	if names, err := listTemplates(root); err == nil {
		for _, name := range names {
			body, err := loadTemplate(root, name)
			if err != nil {
				continue
			}
			if item, ok := templateCatalogItem(name, body, "local"); ok {
				items = append(items, item)
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func templateCatalogItem(name, body, source string) (TemplateCatalogItem, bool) {
	doc, err := templateengine.ParseDocument(name, body)
	if err != nil {
		return TemplateCatalogItem{}, false
	}
	meta := templateWorkflowMetadata(name, source, doc.Metadata)
	kind := doc.Metadata.Kind
	if kind == "" {
		kind = "template"
	}
	starter := false
	if doc.Metadata.Starter != nil {
		starter = *doc.Metadata.Starter
	}
	return TemplateCatalogItem{Name: name, Source: source, Kind: kind, Title: doc.Metadata.Title, UseCases: doc.Metadata.UseCases, Aliases: doc.Metadata.Aliases, Difficulty: doc.Metadata.Difficulty, Starter: starter, ScenarioID: meta.ScenarioID, TemplateKind: meta.TemplateKind, Intents: meta.Intents, VariableSchema: meta.VariableSchema, OutputPolicy: meta.OutputPolicy, AfterCreateActions: meta.AfterCreateActions, Maturity: meta.Maturity, ProofGate: meta.ProofGate, Pack: templatePackFromMetadata(meta.Pack), Lifecycle: meta.Lifecycle, Replacement: meta.Replacement, Metrics: meta.Metrics}, true
}

func filterTemplateCatalog(items []TemplateCatalogItem, pack, useCase string) []TemplateCatalogItem {
	pack = strings.ToLower(strings.TrimSpace(pack))
	useCase = strings.ToLower(strings.TrimSpace(useCase))
	filtered := make([]TemplateCatalogItem, 0, len(items))
	for _, item := range items {
		matchesPack := pack == "" || pack == strings.ToLower(item.Pack.ID) || (pack == "starter" && item.Starter)
		if !matchesPack {
			continue
		}
		if useCase != "" && !templateCatalogMatches(item, useCase) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func recommendTemplate(items []TemplateCatalogItem, intent string) TemplateCatalogItem {
	needle := strings.ToLower(strings.TrimSpace(intent))
	for _, item := range items {
		if item.Kind == "note_template" && templateCatalogExecutable(item) && needle != "" && templateCatalogMatches(item, needle) {
			return item
		}
	}
	for _, item := range items {
		if templateCatalogExecutable(item) && needle != "" && templateCatalogMatches(item, needle) {
			return item
		}
	}
	for _, fallback := range []string{"note.quick", "inbox.capture"} {
		for _, item := range items {
			if item.Name == fallback && templateCatalogExecutable(item) {
				return item
			}
		}
	}
	if len(items) > 0 {
		for _, item := range items {
			if templateCatalogExecutable(item) {
				return item
			}
		}
		return items[0]
	}
	return TemplateCatalogItem{Name: "note.quick", Source: "builtin", Kind: "note_template", Starter: true, Lifecycle: "published_executable", Pack: TemplatePack{ID: "starter", Source: "builtin", Readiness: "mature"}}
}

func templateCatalogMatches(item TemplateCatalogItem, needle string) bool {
	// 模板推荐是 metadata-only 本地匹配，不调用 LLM、不联网、不渲染模板，也不执行 SQL。
	values := []string{item.Name, item.Title, item.Kind, item.Difficulty}
	values = append(values, item.UseCases...)
	values = append(values, item.Aliases...)
	values = append(values, item.Intents...)
	values = append(values, item.TemplateKind, item.Pack.ID, item.Maturity, item.Lifecycle)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func templateCatalogExecutable(item TemplateCatalogItem) bool {
	switch strings.ToLower(strings.TrimSpace(item.Lifecycle)) {
	case "draft_design", "deprecated":
		return false
	default:
		return true
	}
}

func workflowRecommendations(items []TemplateCatalogItem, primary TemplateCatalogItem, intent, root string) []WorkflowRecommendation {
	recs := []WorkflowRecommendation{}
	if primary.Name != "" {
		recs = append(recs, workflowRecommendation(primary, intent, root, true))
	}
	needle := strings.ToLower(strings.TrimSpace(intent))
	for _, item := range items {
		if len(recs) >= 4 {
			break
		}
		if item.Name == primary.Name || (needle != "" && !templateCatalogMatches(item, needle)) {
			continue
		}
		recs = append(recs, workflowRecommendation(item, intent, root, false))
	}
	return recs
}

func workflowRecommendation(item TemplateCatalogItem, intent, root string, primary bool) WorkflowRecommendation {
	fit := "metadata match"
	if strings.TrimSpace(intent) == "" {
		fit = "fallback: no intent provided"
	} else if !templateCatalogMatches(item, strings.ToLower(strings.TrimSpace(intent))) {
		fit = "fallback: conservative capture workflow"
	}
	if item.Lifecycle == "draft_design" {
		fit = "design-only draft; not executable"
	} else if item.Lifecycle == "deprecated" {
		fit = "deprecated; use replacement when available"
	} else if primary {
		fit = "primary local metadata match"
	}
	preview := fmt.Sprintf("pinax template preview %s --vault %s --json", shellQuote(item.Name), shellQuote(root))
	create := fmt.Sprintf("pinax note add <title> --template %s --vault %s --json", shellQuote(item.Name), shellQuote(root))
	if !templateCatalogExecutable(item) {
		create = ""
	}
	return WorkflowRecommendation{Template: item.Name, ScenarioID: item.ScenarioID, Maturity: item.Maturity, Pack: item.Pack, FitReason: fit, PreviewCommand: preview, CreateCommand: create, EvidencePath: "command stdout JSON envelope", ProofGate: item.ProofGate, AfterCreateActions: item.AfterCreateActions, Lifecycle: item.Lifecycle, Executable: templateCatalogExecutable(item), Replacement: item.Replacement}
}

func templateWorkflowMetadata(name, source string, meta templateengine.Metadata) templateengine.Metadata {
	if meta.TemplateKind == "" {
		meta.TemplateKind = meta.Kind
	}
	if meta.TemplateKind == "" {
		meta.TemplateKind = "template"
	}
	if meta.ScenarioID == "" {
		meta.ScenarioID = workflowScenarioID(name)
	}
	if len(meta.Intents) == 0 {
		meta.Intents = append([]string{}, meta.UseCases...)
		meta.Intents = append(meta.Intents, meta.Aliases...)
	}
	if len(meta.VariableSchema) == 0 && len(meta.Variables) > 0 {
		meta.VariableSchema = meta.Variables
	}
	if meta.Maturity == "" {
		meta.Maturity = workflowMaturity(name, meta)
	}
	if meta.Lifecycle == "" {
		meta.Lifecycle = "published_executable"
	}
	if meta.Pack.ID == "" {
		meta.Pack.ID = workflowPackID(meta)
	}
	if meta.Pack.Source == "" {
		if source == "local" {
			meta.Pack.Source = "vault-local"
		} else {
			meta.Pack.Source = source
		}
	}
	if meta.Pack.Readiness == "" {
		meta.Pack.Readiness = meta.Maturity
	}
	if meta.ProofGate.Status == "" {
		meta.ProofGate.Status = "review_optional"
	}
	if meta.OutputPolicy.PathPattern == "" {
		meta.OutputPolicy.PathPattern = meta.Output.PathPattern
	}
	if meta.OutputPolicy.WriteBoundary == "" {
		meta.OutputPolicy.WriteBoundary = "vault-content"
	}
	meta.OutputPolicy.LegacyCompatible = true
	if len(meta.AfterCreateActions) == 0 && meta.Kind == "note_template" {
		meta.AfterCreateActions = []templateengine.TemplateActionMetadata{{Name: "proof_review", Command: "pinax proof loop run --vault <vault> --json"}}
	}
	return meta
}

func templatePackFromMetadata(pack templateengine.TemplatePackMetadata) TemplatePack {
	return TemplatePack{ID: pack.ID, Source: pack.Source, Version: pack.Version, Readiness: pack.Readiness}
}

func workflowScenarioID(name string) string {
	switch {
	case name == "meeting.notes" || name == "decision.record":
		return "meeting-decision"
	case name == "sticky.capture" || strings.HasPrefix(name, "sticky."):
		return "capture-sticky"
	case name == "idea.research_seed" || strings.HasPrefix(name, "idea."):
		return "idea-research-seed"
	case strings.HasPrefix(name, "learning.stock."):
		return "stock-learning"
	case strings.HasPrefix(name, "learning."):
		return "learning-pack"
	case strings.HasPrefix(name, "index."):
		return "index-page"
	default:
		return strings.ReplaceAll(name, ".", "-")
	}
}

func workflowMaturity(name string, meta templateengine.Metadata) string {
	if strings.HasPrefix(name, "learning.stock.") {
		return "exploratory"
	}
	if strings.HasPrefix(name, "idea.") || strings.HasPrefix(name, "learning.") {
		return "first-support"
	}
	if meta.Kind == "template" || meta.Kind == "" {
		return "first-support"
	}
	return "mature"
}

func workflowPackID(meta templateengine.Metadata) string {
	if meta.Difficulty != "" {
		return meta.Difficulty
	}
	if meta.Kind == "" || meta.Kind == "template" {
		return "legacy"
	}
	return strings.TrimSuffix(meta.Kind, "_template")
}
