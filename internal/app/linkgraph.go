package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
)

// NoteLinkGraphService 提供统一的双联关系图查询。
// CLI、doctor、repair、organize、dashboard 和 MCP 都从此服务读取。
type NoteLinkGraphService struct{}

// NoteLinkGraphRequest 描述出链查询请求。
type NoteLinkGraphRequest struct {
	VaultPath      string
	NoteRef        string
	BrokenOnly     bool
	Kind           string // wiki|markdown|""(all)
	IncludeIgnored bool
	Limit          int
}

// NoteBacklinkGraphRequest 描述反链查询请求。
type NoteBacklinkGraphRequest struct {
	VaultPath     string
	NoteRef       string
	IncludeBroken bool
	Limit         int
}

// NoteOrphansRequest 描述孤立笔记查询请求。
type NoteOrphansRequest struct {
	VaultPath   string
	Mode        string // full|no-incoming|no-outgoing
	ExcludeKind string
}

// ParseRawLink 描述从 Markdown body 解析出的一条原始链接。
type ParseRawLink struct {
	Kind    string // wiki|markdown
	Raw     string // 原始目标文本（含 heading/alias）
	Target  string // 归一化目标（去掉 alias 和 heading）
	Alias   string // wiki alias 或 markdown label
	Heading string // #heading 片段
	Line    int    // 1-based 行号
}

// ResolveResult 描述单条链接的解析结果。
type ResolveResult struct {
	Link       domain.NoteLink
	Candidates []domain.NoteLinkCandidate
}

// ParseNoteLinks 从 note body 解析所有 wiki link 和 markdown relative link。
// 忽略外部 URL、mailto、纯 heading 和非 Markdown 附件。
func ParseNoteLinks(body string) []ParseRawLink {
	return parseNoteLinks(body)
}

// parseNoteLinks 实际解析逻辑。使用行号跟踪和详细 alias/heading 提取。
func parseNoteLinks(body string) []ParseRawLink {
	links := make([]ParseRawLink, 0)
	seen := map[string]bool{}
	line := 0

	for _, bodyLine := range strings.Split(body, "\n") {
		line++
		// 解析 wiki links: [[Target]], [[Target|Alias]], [[Target#Heading]]
		for _, match := range vaultWikiLinkPattern.FindAllStringSubmatch(bodyLine, -1) {
			if len(match) < 2 {
				continue
			}
			raw := strings.TrimSpace(match[1])
			if raw == "" {
				continue
			}
			target, alias, heading := splitWikiLinkParts(raw)
			key := "wiki\x00" + target
			if !seen[key] {
				links = append(links, ParseRawLink{Kind: "wiki", Raw: raw, Target: target, Alias: alias, Heading: heading, Line: line})
				seen[key] = true
			}
		}
		// 解析 markdown links: [label](relative-note.md)
		for _, match := range vaultMarkdownLinkPattern.FindAllStringSubmatch(bodyLine, -1) {
			if len(match) < 2 {
				continue
			}
			rawTarget := strings.TrimSpace(match[1])
			if rawTarget == "" || isExternalOrHeadingLink(rawTarget) {
				continue
			}
			target := normalizeMarkdownLinkTarget(rawTarget)
			if target == "" || !strings.EqualFold(filepath.Ext(target), ".md") {
				continue
			}
			key := "markdown\x00" + target
			if !seen[key] {
				links = append(links, ParseRawLink{Kind: "markdown", Raw: rawTarget, Target: target, Line: line})
				seen[key] = true
			}
		}
	}
	return links
}

// splitWikiLinkParts 将 wiki link target 拆分为 target、alias、heading。
// 输入 "Title|Alias" -> ("Title", "Alias", "")
// 输入 "Title#Heading" -> ("Title", "", "Heading")
// 输入 "Title|Alias#Heading" -> ("Title", "Alias", "Heading")
func splitWikiLinkParts(raw string) (target, alias, heading string) {
	raw = strings.TrimSpace(raw)
	// 先分离 alias（| 前面的部分是 target）
	if before, after, ok := strings.Cut(raw, "|"); ok {
		target = strings.TrimSpace(before)
		alias = strings.TrimSpace(after)
	} else {
		target = raw
	}
	// 从 target 中分离 heading
	if before, after, ok := strings.Cut(target, "#"); ok {
		target = strings.TrimSpace(before)
		heading = strings.TrimSpace(after)
	}
	// alias 里也可能有 heading，但 wiki link 语法中 alias 是纯展示文本
	return target, alias, heading
}

// isExternalOrHeadingLink 判断是否为外部 URL、mailto、纯 heading 链接。
func isExternalOrHeadingLink(target string) bool {
	t := strings.TrimSpace(target)
	return strings.HasPrefix(t, "http://") ||
		strings.HasPrefix(t, "https://") ||
		strings.HasPrefix(t, "mailto:") ||
		strings.HasPrefix(t, "#") ||
		strings.HasPrefix(t, "ftp://")
}

// ResolverSnapshot 存储 note 的查找索引，用于确定性解析。
// 解析优先级：note_id > vault-relative path > exact title > case-insensitive unique title > alias。
type ResolverSnapshot struct {
	byNoteID      map[string]domain.Note
	byPath        map[string]domain.Note
	byTitle       map[string]domain.Note // case-insensitive exact
	byTitleCounts map[string]int         // case-insensitive title occurrence count
	byAlias       map[string][]domain.Note
	notes         []domain.Note
}

// BuildResolverSnapshot 从 notes 列表构建解析索引。
func BuildResolverSnapshot(notes []domain.Note) ResolverSnapshot {
	s := ResolverSnapshot{
		byNoteID:      map[string]domain.Note{},
		byPath:        map[string]domain.Note{},
		byTitle:       map[string]domain.Note{},
		byTitleCounts: map[string]int{},
		byAlias:       map[string][]domain.Note{},
		notes:         notes,
	}
	for _, note := range notes {
		s.byNoteID[strings.ToLower(note.ID)] = note
		s.byPath[note.Path] = note
		s.byPath[strings.TrimPrefix(note.Path, "notes/")] = note
		lowerTitle := strings.ToLower(note.Title)
		s.byTitleCounts[lowerTitle]++
		s.byTitle[lowerTitle] = note
		// 未来可读取 frontmatter aliases，当前使用 slug 和 file stem 作为 fallback。
		stem := strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path))
		s.byAlias[strings.ToLower(stem)] = append(s.byAlias[strings.ToLower(stem)], note)
	}
	return s
}

// ResolveLinkTarget 按确定性优先级解析链接目标。
// 返回解析结果，包含 status、候选列表和 evidence。
func ResolveLinkTarget(source domain.Note, rawLink ParseRawLink, snap ResolverSnapshot) ResolveResult {
	result := ResolveResult{}
	link := domain.NoteLink{
		SourcePath:    source.Path,
		SourceTitle:   source.Title,
		SourceNoteID:  source.ID,
		Target:        rawLink.Target,
		TargetRaw:     rawLink.Raw,
		TargetAlias:   rawLink.Alias,
		TargetHeading: rawLink.Heading,
		Kind:          rawLink.Kind,
		Line:          rawLink.Line,
	}
	target := rawLink.Target

	// 1. note_id 精确匹配
	if note, ok := snap.byNoteID[strings.ToLower(target)]; ok {
		link.TargetPath = note.Path
		link.TargetTitle = note.Title
		link.TargetNoteID = note.ID
		link.Status = string(domain.LinkStatusResolved)
		link.Broken = false
		link.Evidence = "resolved by note_id"
		result.Link = link
		return result
	}

	// 2. vault-relative path 精确匹配
	if note, ok := snap.byPath[target]; ok {
		link.TargetPath = note.Path
		link.TargetTitle = note.Title
		link.TargetNoteID = note.ID
		link.Status = string(domain.LinkStatusResolved)
		link.Broken = false
		link.Evidence = "resolved by path"
		result.Link = link
		return result
	}
	// 也尝试 notes/ 前缀
	if note, ok := snap.byPath[filepath.ToSlash(filepath.Join("notes", target))]; ok {
		link.TargetPath = note.Path
		link.TargetTitle = note.Title
		link.TargetNoteID = note.ID
		link.Status = string(domain.LinkStatusResolved)
		link.Broken = false
		link.Evidence = "resolved by path (notes/ prefix)"
		result.Link = link
		return result
	}

	// 对于 markdown 链接，尝试相对于 source path 的路径解析
	if rawLink.Kind == "markdown" {
		cleanTarget := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(source.Path), target)))
		if note, ok := snap.byPath[cleanTarget]; ok {
			link.TargetPath = note.Path
			link.TargetTitle = note.Title
			link.TargetNoteID = note.ID
			link.Status = string(domain.LinkStatusResolved)
			link.Broken = false
			link.Evidence = "resolved by relative path"
			result.Link = link
			return result
		}
	}

	// 3. exact title 匹配
	lowerTarget := strings.ToLower(target)
	if note, ok := snap.byTitle[lowerTarget]; ok {
		count := snap.byTitleCounts[lowerTarget]
		if count == 1 {
			link.TargetPath = note.Path
			link.TargetTitle = note.Title
			link.TargetNoteID = note.ID
			link.Status = string(domain.LinkStatusResolved)
			link.Broken = false
			link.Evidence = "resolved by exact title"
			result.Link = link
			return result
		}
		// 多个同名 -> ambiguous
		link.Status = string(domain.LinkStatusAmbiguous)
		link.Broken = false
		link.Evidence = fmt.Sprintf("ambiguous: %d notes with same title", count)
		for _, n := range snap.notes {
			if strings.ToLower(n.Title) == lowerTarget {
				result.Candidates = append(result.Candidates, domain.NoteLinkCandidate{Path: n.Path, Title: n.Title, NoteID: n.ID})
			}
		}
		link.Candidates = result.Candidates
		result.Link = link
		return result
	}

	// 4. alias/fallback 匹配
	if candidates, ok := snap.byAlias[lowerTarget]; ok && len(candidates) > 0 {
		if len(candidates) == 1 {
			note := candidates[0]
			link.TargetPath = note.Path
			link.TargetTitle = note.Title
			link.TargetNoteID = note.ID
			link.Status = string(domain.LinkStatusResolved)
			link.Broken = false
			link.Evidence = "resolved by alias/stem"
			result.Link = link
			return result
		}
		link.Status = string(domain.LinkStatusAmbiguous)
		link.Broken = false
		link.Evidence = fmt.Sprintf("ambiguous: %d candidates by alias", len(candidates))
		for _, n := range candidates {
			result.Candidates = append(result.Candidates, domain.NoteLinkCandidate{Path: n.Path, Title: n.Title, NoteID: n.ID})
		}
		link.Candidates = result.Candidates
		result.Link = link
		return result
	}

	// 5. 未找到 -> broken
	link.Status = string(domain.LinkStatusBroken)
	link.Broken = true
	link.Evidence = "target not found"
	result.Link = link
	return result
}

// BuildEnhancedLinkGraph 构建增强的双联关系图。
// 返回所有出链、入链索引和增强的 NoteLink 列表。
func BuildEnhancedLinkGraph(notes []domain.Note) (
	enhancedLinks map[string][]domain.NoteLink,
	incoming map[string][]domain.NoteLink,
) {
	snap := BuildResolverSnapshot(notes)
	enhancedLinks = map[string][]domain.NoteLink{}
	incoming = map[string][]domain.NoteLink{}

	for _, note := range notes {
		rawLinks := parseNoteLinks(note.Body)
		for _, raw := range rawLinks {
			result := ResolveLinkTarget(note, raw, snap)
			link := result.Link
			enhancedLinks[note.Path] = append(enhancedLinks[note.Path], link)
			// 只有 resolved 的链接才记录 incoming
			if link.Status == string(domain.LinkStatusResolved) && link.TargetPath != "" {
				incoming[link.TargetPath] = append(incoming[link.TargetPath], link)
			}
		}
		sortNoteLinks(enhancedLinks[note.Path])
	}
	for path := range incoming {
		sortNoteLinks(incoming[path])
	}
	return enhancedLinks, incoming
}

// --- Service methods for NoteLinkGraphService ---

// QueryOutgoingLinks 查询指定 note 的出链。
func (s *Service) QueryOutgoingLinks(_ context.Context, req NoteLinkGraphRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.links", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("note.links", err), err
	}
	note, err := resolveNoteRef(notes, req.NoteRef)
	if err != nil {
		return errorProjection("note.links", err), err
	}
	outgoing, _ := BuildEnhancedLinkGraph(notes)
	links := filterLinks(outgoing[note.Path], req.BrokenOnly, req.Kind, req.IncludeIgnored)
	if req.Limit > 0 && len(links) > req.Limit {
		links = links[:req.Limit]
	}
	engine, indexStatus := linkGraphEngineStatus(root)
	projection := domain.NewProjection("note.links", "笔记链接已列出。")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["links"] = fmt.Sprint(len(links))
	projection.Facts["resolved"] = fmt.Sprint(countLinksWithStatus(links, "resolved"))
	projection.Facts["broken"] = fmt.Sprint(countLinksWithStatus(links, "broken"))
	projection.Facts["ambiguous"] = fmt.Sprint(countLinksWithStatus(links, "ambiguous"))
	projection.Facts["engine"] = engine
	if indexStatus != "" {
		projection.Facts["index_status"] = indexStatus
	}
	projection.Data = map[string]any{"note": note, "links": links}
	return projection, nil
}

// QueryBacklinks 查询指定 note 的反链。
func (s *Service) QueryBacklinks(_ context.Context, req NoteBacklinkGraphRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.backlinks", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("note.backlinks", err), err
	}
	note, err := resolveNoteRef(notes, req.NoteRef)
	if err != nil {
		return errorProjection("note.backlinks", err), err
	}
	_, incoming := BuildEnhancedLinkGraph(notes)
	backlinks := incoming[note.Path]
	if !req.IncludeBroken {
		backlinks = filterLinks(backlinks, false, "", true)
		backlinks = filterByStatus(backlinks, "broken", false)
	}
	if req.Limit > 0 && len(backlinks) > req.Limit {
		backlinks = backlinks[:req.Limit]
	}
	engine, indexStatus := linkGraphEngineStatus(root)
	projection := domain.NewProjection("note.backlinks", "笔记反链已列出。")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["backlinks"] = fmt.Sprint(len(backlinks))
	projection.Facts["unresolved"] = fmt.Sprint(countLinksWithStatus(backlinks, "broken"))
	projection.Facts["engine"] = engine
	if indexStatus != "" {
		projection.Facts["index_status"] = indexStatus
	}
	projection.Data = map[string]any{"note": note, "backlinks": backlinks}
	return projection, nil
}

// QueryOrphans 查询孤立笔记。
func (s *Service) QueryOrphans(_ context.Context, req NoteOrphansRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.orphans", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("note.orphans", err), err
	}
	outgoing, incoming := BuildEnhancedLinkGraph(notes)
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "full"
	}
	orphans := make([]domain.Note, 0)
	for _, note := range notes {
		if req.ExcludeKind != "" && note.Kind == req.ExcludeKind {
			continue
		}
		outCount := len(outgoing[note.Path])
		inCount := len(incoming[note.Path])
		switch mode {
		case "no-incoming":
			if inCount == 0 {
				orphans = append(orphans, note)
			}
		case "no-outgoing":
			if outCount == 0 {
				orphans = append(orphans, note)
			}
		default: // full
			if outCount == 0 && inCount == 0 {
				orphans = append(orphans, note)
			}
		}
	}
	engine, indexStatus := linkGraphEngineStatus(root)
	projection := domain.NewProjection("note.orphans", "孤立笔记已列出。")
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["orphans"] = fmt.Sprint(len(orphans))
	projection.Facts["mode"] = mode
	projection.Facts["engine"] = engine
	if indexStatus != "" {
		projection.Facts["index_status"] = indexStatus
	}
	projection.Data = map[string]any{"orphans": orphans}
	return projection, nil
}

// GraphSummary 返回 vault 的链接图健康摘要。
func (s *Service) GraphSummary(_ context.Context, vaultPath string) (domain.NoteGraphProjection, error) {
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return domain.NoteGraphProjection{}, err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return domain.NoteGraphProjection{}, err
	}
	outgoing, incoming := BuildEnhancedLinkGraph(notes)
	engine, indexStatus := linkGraphEngineStatus(root)

	totalLinks := 0
	resolved := 0
	broken := 0
	ambiguous := 0
	ignored := 0
	for _, links := range outgoing {
		for _, link := range links {
			totalLinks++
			switch link.Status {
			case string(domain.LinkStatusResolved):
				resolved++
			case string(domain.LinkStatusBroken):
				broken++
			case string(domain.LinkStatusAmbiguous):
				ambiguous++
			case string(domain.LinkStatusIgnored), string(domain.LinkStatusExternal):
				ignored++
			default:
				if link.Broken {
					broken++
				} else {
					resolved++
				}
			}
		}
	}
	orphanCount := 0
	for _, note := range notes {
		if len(outgoing[note.Path]) == 0 && len(incoming[note.Path]) == 0 {
			orphanCount++
		}
	}
	summary := domain.NoteGraphProjection{
		Engine:      engine,
		IndexStatus: indexStatus,
		TotalNotes:  len(notes),
		TotalLinks:  totalLinks,
		Resolved:    resolved,
		Broken:      broken,
		Ambiguous:   ambiguous,
		Ignored:     ignored,
		Orphans:     orphanCount,
	}
	if broken > 0 || ambiguous > 0 {
		summary.Facts = map[string]string{"vault": root}
		summary.NextActions = []domain.Action{
			{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))},
		}
	}
	return summary, nil
}

// --- helpers ---

// linkGraphEngineStatus 判断链接图查询使用 index 还是 scan fallback。
func linkGraphEngineStatus(root string) (engine, indexStatus string) {
	path := filepath.Join(root, ".pinax", "index.sqlite")
	if _, err := os.Stat(path); err != nil {
		return "scan", "missing"
	}
	// 检查 schema version
	status, _ := noteindex.Init(root)
	if status.Status == "fresh" {
		return "index", "fresh"
	}
	return "scan", "stale"
}

// filterLinks 按条件过滤链接列表。
func filterLinks(links []domain.NoteLink, brokenOnly bool, kind string, includeIgnored bool) []domain.NoteLink {
	filtered := make([]domain.NoteLink, 0, len(links))
	for _, link := range links {
		if !includeIgnored && (link.Status == string(domain.LinkStatusIgnored) || link.Status == string(domain.LinkStatusExternal)) {
			continue
		}
		if brokenOnly && link.Status != string(domain.LinkStatusBroken) && !link.Broken {
			continue
		}
		if kind != "" && link.Kind != kind {
			continue
		}
		filtered = append(filtered, link)
	}
	return filtered
}

// filterByStatus 按状态过滤链接。
func filterByStatus(links []domain.NoteLink, status string, include bool) []domain.NoteLink {
	filtered := make([]domain.NoteLink, 0, len(links))
	for _, link := range links {
		if link.Status == status || (status == "broken" && link.Broken) {
			if include {
				filtered = append(filtered, link)
			}
		} else {
			if !include {
				filtered = append(filtered, link)
			}
		}
	}
	return filtered
}

// countLinksWithStatus 统计指定状态的链接数量。
func countLinksWithStatus(links []domain.NoteLink, status string) int {
	count := 0
	for _, link := range links {
		if link.Status == status {
			count++
		} else if status == "broken" && link.Broken && link.Status == "" {
			// 向后兼容：旧链接只有 Broken 字段
			count++
		}
	}
	return count
}

// parseRawLinksFromBody 从 body 解析所有原始链接（暴露给测试）。
func parseRawLinksFromBody(body string) []ParseRawLink {
	return parseNoteLinks(body)
}
