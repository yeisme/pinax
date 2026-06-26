package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/notelinks"
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
type ParseRawLink = notelinks.RawLink

// ResolveResult 描述单条链接的解析结果。
type ResolveResult = notelinks.ResolveResult

// ResolverSnapshot 存储 note 的查找索引，用于确定性解析。
type ResolverSnapshot = notelinks.ResolverSnapshot

// ParseNoteLinks 从 note body 解析所有 wiki link 和 markdown relative link。
func ParseNoteLinks(body string) []ParseRawLink {
	return notelinks.ParseNoteLinks(body)
}

// parseNoteLinks 实际解析逻辑。保留未导出入口以兼容包内测试。
func parseNoteLinks(body string) []ParseRawLink {
	return notelinks.ParseNoteLinks(body)
}

// splitWikiLinkParts 将 wiki link target 拆分为 target、alias、heading。
func splitWikiLinkParts(raw string) (target, alias, heading string) {
	return notelinks.SplitWikiLinkParts(raw)
}

// isExternalOrHeadingLink 判断是否为外部 URL、mailto、纯 heading 链接。
func isExternalOrHeadingLink(target string) bool {
	return notelinks.IsExternalOrHeadingLink(target)
}

// BuildResolverSnapshot 从 notes 列表构建解析索引。
func BuildResolverSnapshot(notes []domain.Note) ResolverSnapshot {
	return notelinks.BuildResolverSnapshot(notes)
}

// ResolveLinkTarget 按确定性优先级解析链接目标。
func ResolveLinkTarget(source domain.Note, rawLink ParseRawLink, snap ResolverSnapshot) ResolveResult {
	return notelinks.ResolveLinkTarget(source, rawLink, snap)
}

// BuildEnhancedLinkGraph 构建增强的双联关系图。
// 返回所有出链、入链索引和增强的 NoteLink 列表。
func BuildEnhancedLinkGraph(notes []domain.Note) (
	enhancedLinks map[string][]domain.NoteLink,
	incoming map[string][]domain.NoteLink,
) {
	return notelinks.BuildGraph(notes)
}

// --- Service methods for NoteLinkGraphService ---

// QueryOutgoingLinks 查询指定 note 的出链。
func (s *Service) QueryOutgoingLinks(ctx context.Context, req NoteLinkGraphRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.links", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("note.links", err), err
	}
	note, err := s.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.NoteRef})
	if err != nil {
		return errorProjection("note.links", err), err
	}
	outgoing, _ := BuildEnhancedLinkGraph(notes)
	links := filterLinks(outgoing[note.Path], req.BrokenOnly, req.Kind, req.IncludeIgnored)
	if req.Limit > 0 && len(links) > req.Limit {
		links = links[:req.Limit]
	}
	engine, indexStatus := linkGraphEngineStatus(root)
	projection := domain.NewProjection("note.links", "Note links listed.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["links"] = fmt.Sprint(len(links))
	projection.Facts["resolved"] = fmt.Sprint(countLinksWithStatus(links, "resolved"))
	projection.Facts["broken"] = fmt.Sprint(countLinksWithStatus(links, "broken"))
	projection.Facts["ambiguous"] = fmt.Sprint(countLinksWithStatus(links, "ambiguous"))
	projection.Facts["engine"] = engine
	addLinkCompatibilityFacts(projection.Facts)
	if indexStatus != "" {
		projection.Facts["index_status"] = indexStatus
	}
	if countLinksWithStatus(links, "broken") > 0 || countLinksWithStatus(links, "ambiguous") > 0 {
		projection.Status = "partial"
		projection.Actions = []domain.Action{{Name: "repair_plan", Command: fmt.Sprintf("pinax repair plan --vault %s", shellQuote(root))}}
	}
	agentCtx := graphAgentContext(note, links)
	projection.Data = map[string]any{"note": noteGraphNoteSummary(note), "links": links, "agent_contexts": []domain.AgentContext{agentCtx}}
	return projection, nil
}

// QueryBacklinks 查询指定 note 的反链。
func (s *Service) QueryBacklinks(ctx context.Context, req NoteBacklinkGraphRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.backlinks", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("note.backlinks", err), err
	}
	note, err := s.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.NoteRef})
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
	projection := domain.NewProjection("note.backlinks", "Note backlinks listed.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["backlinks"] = fmt.Sprint(len(backlinks))
	projection.Facts["unresolved"] = fmt.Sprint(countLinksWithStatus(backlinks, "broken"))
	projection.Facts["engine"] = engine
	addLinkCompatibilityFacts(projection.Facts)
	if indexStatus != "" {
		projection.Facts["index_status"] = indexStatus
	}
	agentCtx := graphAgentContext(note, backlinks)
	projection.Data = map[string]any{"note": noteGraphNoteSummary(note), "backlinks": backlinks, "agent_contexts": []domain.AgentContext{agentCtx}}
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
	projection := domain.NewProjection("note.orphans", "Orphan notes listed.")
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["orphans"] = fmt.Sprint(len(orphans))
	projection.Facts["mode"] = mode
	projection.Facts["engine"] = engine
	if indexStatus != "" {
		projection.Facts["index_status"] = indexStatus
	}
	// 投影必须保持 agent-safe 边界：orphans 列表只输出 bounded 摘要，不能
	// 把完整 note body 序列化到 JSON/events/agent 输出中。与 links/backlinks
	// 保持一致，统一通过 noteGraphNoteSummary 剥离 Body 字段。
	summaries := make([]domain.Note, 0, len(orphans))
	for _, note := range orphans {
		summaries = append(summaries, noteGraphNoteSummary(note))
	}
	projection.Data = map[string]any{"orphans": summaries}
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
	summary.Facts = map[string]string{"vault": root}
	addLinkCompatibilityFacts(summary.Facts)
	if broken > 0 || ambiguous > 0 {
		summary.NextActions = []domain.Action{
			{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))},
		}
	}
	return summary, nil
}

func addLinkCompatibilityFacts(facts map[string]string) {
	facts["compat.wikilink"] = "supported"
	facts["compat.alias"] = "supported"
	facts["compat.heading"] = "supported"
	facts["compat.markdown_relative"] = "supported"
	facts["compat.backlink"] = "supported"
	facts["compat.graph"] = "supported"
	facts["compat.ambiguous_repair"] = "manual_review"
	facts["repair_mode"] = "plan_only"
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

func noteGraphNoteSummary(note domain.Note) domain.Note {
	return domain.Note{ID: note.ID, Title: note.Title, Path: note.Path, Tags: note.Tags, Project: note.Project, Folder: note.Folder, Kind: note.Kind, Status: note.Status, CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt}
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
