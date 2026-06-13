package app

import (
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
		"journal.daily":   strings.Join([]string{"---", "schema_version: pinax.template.v2", "kind: journal_template", "name: journal.daily", "title: Daily Note", "engine: go-template", "output:", "  path_pattern: daily/{{ .Date }}.md", "defaults:", "  kind: daily", "  status: active", "---", "# {{ .Date }}", "", "## Today's Focus", "", "- ", "", "## Notes", "", "- ", "", "## Pinax Captures", "<!-- pinax:managed name=daily-captures -->", "", "<!-- /pinax:managed -->", "", "## Review", ""}, "\n"),
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
		// Meeting模板只包含议题、决议、行动项，避免冗长Meeting纪要框架。
		"meeting.notes": builtInNoteTemplate("meeting.notes", "Meeting Notes", []string{"Meeting", "Sync"}, []string{"meeting", "notes"}, "focused", false, "meetings/{{ .Title }}.md", map[string]string{"kind": "meeting", "status": "active"}, []string{"# {{ .Title }}", "", "## 议题", "", "- ", "", "## 决议", "", "- ", "", "## 行动项", "", "- "}),
		// Decision模板聚焦背景、选择和后果，便于后续索引和复盘。
		"decision.record": builtInNoteTemplate("decision.record", "Decision Record", []string{"Decision", "Tradeoffs"}, []string{"decision", "record"}, "focused", false, "decisions/{{ .Title }}.md", map[string]string{"kind": "decision", "status": "active"}, []string{"# {{ .Title }}", "", "## Background", "", "## Decision", "", "## Tradeoffs", "", "## Follow-up Check", ""}),
		// Project brief 模板只保留目标、范围、风险和Next，适合作为Project入口页。
		"project.brief": builtInNoteTemplate("project.brief", "Project Brief", []string{"Project", "Plan"}, []string{"project", "brief"}, "focused", false, "projects/{{ .Title }}.md", map[string]string{"kind": "project", "status": "active"}, []string{"# {{ .Title }}", "", "## Goals", "", "## Scope", "", "## Risks", "", "## Next Steps", ""}),
		// Video Learning模板只记录来源、要点和可复用片段，避免转录全文。
		"learning.video": builtInNoteTemplate("learning.video", "Video Learning", []string{"Learning", "Video"}, []string{"video", "learning"}, "starter", true, "learning/videos/{{ .Title }}.md", map[string]string{"kind": "learning", "status": "active"}, []string{"# {{ .Title }}", "", "## Source", "", "## Key Points", "", "- ", "", "## Reusable Snippets", "", "- "}),
		// Book Learning模板保留章节、摘录和应用，便于渐进补充。
		"learning.book": builtInNoteTemplate("learning.book", "Book Learning", []string{"Learning", "Reading"}, []string{"book", "reading"}, "starter", true, "learning/books/{{ .Title }}.md", map[string]string{"kind": "learning", "status": "active"}, []string{"# {{ .Title }}", "", "## Chapters", "", "## Excerpts", "", "- ", "", "## Application", ""}),
		// Research Topic模板用问题、证据、结论分层，避免混杂资料堆放。
		"research.topic": builtInNoteTemplate("research.topic", "Research Topic", []string{"Research", "Reference Curation"}, []string{"research", "topic"}, "focused", false, "research/{{ .Title }}.md", map[string]string{"kind": "research", "status": "active"}, []string{"# {{ .Title }}", "", "## Questions", "", "## Evidence", "", "- ", "", "## Working Conclusion", ""}),
		// Person Profile模板只保留关系、上下文和待跟进，避免收集无关个人信息。
		"person.profile": builtInNoteTemplate("person.profile", "Person Profile", []string{"Person", "Relationship Follow-up"}, []string{"person", "profile"}, "focused", false, "people/{{ .Title }}.md", map[string]string{"kind": "person", "status": "active"}, []string{"# {{ .Title }}", "", "## Relationship", "", "## Context", "", "## Follow-up", "", "- "}),
	}
}

func builtInNoteTemplate(name, title string, useCases, aliases []string, difficulty string, starter bool, pathPattern string, defaults map[string]string, body []string) string {
	lines := []string{"---", "schema_version: pinax.template.v2", "kind: note_template", "name: " + name, "title: " + title, "engine: go-template", "use_cases:"}
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
	lines = append(lines, "output:", "  path_pattern: \""+pathPattern+"\"", "defaults:")
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
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Kind       string   `json:"kind"`
	Title      string   `json:"title,omitempty"`
	UseCases   []string `json:"use_cases,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	Difficulty string   `json:"difficulty,omitempty"`
	Starter    bool     `json:"starter"`
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
	kind := doc.Metadata.Kind
	if kind == "" {
		kind = "template"
	}
	starter := false
	if doc.Metadata.Starter != nil {
		starter = *doc.Metadata.Starter
	}
	return TemplateCatalogItem{Name: name, Source: source, Kind: kind, Title: doc.Metadata.Title, UseCases: doc.Metadata.UseCases, Aliases: doc.Metadata.Aliases, Difficulty: doc.Metadata.Difficulty, Starter: starter}, true
}

func filterTemplateCatalog(items []TemplateCatalogItem, pack, useCase string) []TemplateCatalogItem {
	pack = strings.ToLower(strings.TrimSpace(pack))
	useCase = strings.ToLower(strings.TrimSpace(useCase))
	filtered := make([]TemplateCatalogItem, 0, len(items))
	for _, item := range items {
		if pack == "starter" && !item.Starter {
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
		if item.Kind == "note_template" && needle != "" && templateCatalogMatches(item, needle) {
			return item
		}
	}
	for _, item := range items {
		if needle != "" && templateCatalogMatches(item, needle) {
			return item
		}
	}
	for _, fallback := range []string{"note.quick", "inbox.capture"} {
		for _, item := range items {
			if item.Name == fallback {
				return item
			}
		}
	}
	if len(items) > 0 {
		return items[0]
	}
	return TemplateCatalogItem{Name: "note.quick", Source: "builtin", Kind: "note_template", Starter: true}
}

func templateCatalogMatches(item TemplateCatalogItem, needle string) bool {
	// 模板推荐是 metadata-only 本地匹配，不调用 LLM、不联网、不渲染模板，也不执行 SQL。
	values := []string{item.Name, item.Title, item.Kind, item.Difficulty}
	values = append(values, item.UseCases...)
	values = append(values, item.Aliases...)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}
