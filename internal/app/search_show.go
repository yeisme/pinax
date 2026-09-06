package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/app/searchops"
	"github.com/yeisme/pinax/internal/domain"
)

// SearchShowRequest 是 pinax search show 的请求；ref 解析歧义时 fail-closed。
type SearchShowRequest struct {
	VaultPath string
	NoteRef   string
}

// SearchShowTrust 是详情卡的信任面板（分级 + 关键事件 + stale_after）。
type SearchShowTrust struct {
	Tier             string `json:"tier"`
	GeneratedBy      string `json:"generated_by,omitempty"`
	GeneratedAt      string `json:"generated_at,omitempty"`
	VerifiedCount    int    `json:"verified_count"`
	VerifiedAtLatest string `json:"verified_at_latest,omitempty"`
	LatestHumanBy    string `json:"latest_human_by,omitempty"`
	LatestHumanAt    string `json:"latest_human_at,omitempty"`
	StaleAfter       string `json:"stale_after,omitempty"`
}

// SearchShowRef 是有界的链引用（不含正文）。
type SearchShowRef struct {
	Path   string `json:"path"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
}

// SearchShowLinks 是出入链计数与有界样本。
type SearchShowLinks struct {
	Outgoing     int             `json:"outgoing"`
	OutgoingRefs []SearchShowRef `json:"outgoing_refs,omitempty"`
	Incoming     int             `json:"incoming"`
	IncomingRefs []SearchShowRef `json:"incoming_refs,omitempty"`
}

// SearchShowResult 是聚合详情卡（bounded snippet + 元数据 + 信任面板 + 链计数 + 邻居）。
type SearchShowResult struct {
	Title     string          `json:"title"`
	Path      string          `json:"path"`
	NoteID    string          `json:"note_id,omitempty"`
	Kind      string          `json:"kind,omitempty"`
	Status    string          `json:"status,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
	Tags      []string        `json:"tags,omitempty"`
	Trust     SearchShowTrust `json:"trust"`
	Fresh     string          `json:"fresh"`
	Snippet   string          `json:"snippet,omitempty"`
	Links     SearchShowLinks `json:"links"`
	Neighbors []SearchShowRef `json:"neighbors,omitempty"`
}

const searchShowRefLimit = 5

// SearchShow 聚合既有只读投影为详情卡。不输出正文全文（body 红线），snippet 有界。
// 只读：不触发 lazy index rebuild，不写任何 vault/index 文件。
func (s *Service) SearchShow(_ context.Context, req SearchShowRequest) (domain.Projection, error) {
	command := "search.show"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes = ordinaryNotes(notes)
	note, err := resolveNoteRef(notes, req.NoteRef)
	if err != nil {
		return errorProjection(command, err), err
	}
	outgoing, incoming := BuildEnhancedLinkGraph(notes)
	noteByPath := map[string]domain.Note{}
	for _, candidate := range notes {
		noteByPath[candidate.Path] = candidate
	}

	result := SearchShowResult{
		Title:     note.Title,
		Path:      note.Path,
		NoteID:    note.ID,
		Kind:      note.Kind,
		Status:    note.Status,
		UpdatedAt: note.UpdatedAt,
		Tags:      note.Tags,
		Fresh:     domain.FreshnessOf(note.Trust, s.currentTimeUTC()),
		Snippet:   searchShowSnippet(note),
	}
	if note.Trust != nil {
		result.Trust = SearchShowTrust{
			Tier:             domain.TrustTierOf(note.Trust),
			GeneratedBy:      note.Trust.Generated.By,
			GeneratedAt:      note.Trust.Generated.At,
			VerifiedCount:    len(note.Trust.Verified),
			VerifiedAtLatest: note.Trust.LatestVerifiedAt(),
			StaleAfter:       note.Trust.StaleAfter,
		}
		if event, ok := note.Trust.LatestHumanVerified(); ok {
			result.Trust.LatestHumanBy = event.By
			result.Trust.LatestHumanAt = event.At
		}
	} else {
		result.Trust = SearchShowTrust{Tier: domain.TrustTierUnverified}
	}
	for _, link := range outgoing[note.Path] {
		result.Links.Outgoing++
		if len(result.Links.OutgoingRefs) < searchShowRefLimit {
			result.Links.OutgoingRefs = append(result.Links.OutgoingRefs, searchShowRefForLink(noteByPath, link))
		}
	}
	for _, link := range incoming[note.Path] {
		result.Links.Incoming++
		if len(result.Links.IncomingRefs) < searchShowRefLimit {
			if source, ok := noteByPath[link.SourcePath]; ok {
				result.Links.IncomingRefs = append(result.Links.IncomingRefs, SearchShowRef{Path: source.Path, Title: source.Title, Status: source.Status})
			}
		}
	}
	result.Neighbors = searchShowNeighbors(notes, note)

	projection := domain.NewProjection(command, "Search result detail card composed.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["trust"] = result.Trust.Tier
	projection.Facts["fresh"] = result.Fresh
	projection.Facts["links_out"] = fmt.Sprint(result.Links.Outgoing)
	projection.Facts["links_in"] = fmt.Sprint(result.Links.Incoming)
	projection.Facts["neighbors"] = fmt.Sprint(len(result.Neighbors))
	projection.Facts["writes"] = "false"
	projection.Evidence = []string{note.Path}
	projection.Actions = []domain.Action{
		{Name: "read", Command: fmt.Sprintf("pinax note show %s --vault %s", shellQuote(note.Title), shellQuote(root))},
		{Name: "backlinks", Command: fmt.Sprintf("pinax note backlinks %s --vault %s", shellQuote(note.Title), shellQuote(root))},
	}
	if result.Fresh == domain.FreshnessStale {
		projection.Actions = append(projection.Actions, domain.Action{Name: "verify", Command: fmt.Sprintf("pinax note verify %s --vault %s", shellQuote(note.Path), shellQuote(root))})
	}
	projection.Data = result
	return projection, nil
}

// searchShowSnippet 优先用 note summary/description frontmatter，其次正文有界摘录。
func searchShowSnippet(note domain.Note) string {
	for _, key := range []string{"description", "summary"} {
		if value := strings.TrimSpace(note.Frontmatter[key]); value != "" {
			return boundedSnippet(value)
		}
	}
	return boundedSnippet(searchops.FirstSnippet(note.Body, ""))
}

func boundedSnippet(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) <= 200 {
		return value
	}
	return value[:200]
}

func searchShowRefForLink(noteByPath map[string]domain.Note, link domain.NoteLink) SearchShowRef {
	if target, ok := noteByPath[link.TargetPath]; ok {
		return SearchShowRef{Path: target.Path, Title: target.Title, Status: target.Status}
	}
	return SearchShowRef{Path: link.TargetPath, Title: link.TargetTitle, Status: strings.TrimSpace(string(link.Status))}
}

// searchShowNeighbors 列出共享任一 tag 的其他 note（按 path 稳定排序，有界）。
func searchShowNeighbors(notes []domain.Note, note domain.Note) []SearchShowRef {
	tags := map[string]bool{}
	for _, tag := range note.Tags {
		tags[strings.TrimSpace(tag)] = true
	}
	if len(tags) == 0 {
		return nil
	}
	neighbors := make([]SearchShowRef, 0)
	for _, candidate := range notes {
		if candidate.Path == note.Path {
			continue
		}
		matched := false
		for _, tag := range candidate.Tags {
			if tags[strings.TrimSpace(tag)] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		neighbors = append(neighbors, SearchShowRef{Path: candidate.Path, Title: candidate.Title, Status: candidate.Status})
		if len(neighbors) >= searchShowRefLimit {
			break
		}
	}
	sort.Slice(neighbors, func(i, j int) bool { return neighbors[i].Path < neighbors[j].Path })
	return neighbors
}
