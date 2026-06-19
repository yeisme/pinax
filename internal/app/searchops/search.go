package searchops

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
)

type Request struct {
	VaultPath     string
	Query         string
	Tags          []string
	Group         string
	Folder        string
	Kind          string
	Status        string
	CreatedAfter  string
	UpdatedAfter  string
	LinkTarget    string
	HasAttachment bool
	Limit         int
	Sort          string
	AllowStale    bool
	At            string
	ChangedSince  string
	Revision      string
}

type Result struct {
	Engine               string                     `json:"engine"`
	IndexStatus          string                     `json:"index_status,omitempty"`
	IndexLoaded          string                     `json:"index_loaded,omitempty"`
	Total                int                        `json:"total"`
	Returned             int                        `json:"returned"`
	Notes                []domain.Note              `json:"notes,omitempty"`
	Results              []noteindex.ResultItem     `json:"results,omitempty"`
	LinkTargetStatus     string                     `json:"link_target_status,omitempty"`
	LinkTargetMatches    int                        `json:"link_target_matches,omitempty"`
	LinkTargetCandidates []domain.NoteLinkCandidate `json:"link_target_candidates,omitempty"`
}

type LinkGraphBuilder func([]domain.Note) map[string][]domain.NoteLink

type LinkTargetFilter struct {
	Active     bool
	Status     string
	SourcePath map[string]bool
	Matches    int
	Candidates []domain.NoteLinkCandidate
}

func ValidateVersionAware(req Request) error {
	if strings.TrimSpace(req.At) != "" && strings.TrimSpace(req.At) != "HEAD" {
		return &domain.CommandError{Code: "version_query_unsupported", Message: "search --at currently only supports HEAD", Hint: "Use pinax search <query> --at HEAD or remove --at"}
	}
	if strings.TrimSpace(req.Revision) != "" {
		return &domain.CommandError{Code: domain.ErrorCodeVersionReadUnavailable, Message: "Current version backend does not support reading historical projections by revision", Hint: "Use pinax version snapshot first or remove --revision"}
	}
	if strings.TrimSpace(req.ChangedSince) != "" {
		return &domain.CommandError{Code: "changed_since_unavailable", Message: "Current index has not cached changed-since historical projections", Hint: "Run pinax index sync first or remove --changed-since"}
	}
	return nil
}

func ValidateDateFilters(req Request) error {
	for _, item := range []struct {
		value string
	}{
		{value: req.CreatedAfter},
		{value: req.UpdatedAfter},
	} {
		if strings.TrimSpace(item.value) == "" {
			continue
		}
		if _, err := ParseUserDate(item.value); err != nil {
			return &domain.CommandError{Code: "invalid_date_filter", Message: "Date filter is invalid", Hint: "Use YYYY-MM-DD or an RFC3339 timestamp, for example 2026-01-01"}
		}
	}
	return nil
}

func LazyIndexAllowed(req Request, status noteindex.Status, notes []domain.Note) bool {
	if req.AllowStale {
		return false
	}
	if status.Status != "missing" && status.Status != "stale" {
		return false
	}
	const lazyIndexNoteBudget = 10000
	return len(notes) <= lazyIndexNoteBudget
}

func BuildIndexRequest(req Request, linkFilter LinkTargetFilter) noteindex.SearchRequest {
	indexReq := noteindex.SearchRequest{Query: req.Query, Tags: CleanTags(req.Tags), Group: req.Group, Folder: req.Folder, Kind: req.Kind, Status: req.Status, CreatedAfter: req.CreatedAfter, UpdatedAfter: req.UpdatedAfter, HasAttachment: req.HasAttachment, Limit: req.Limit, Sort: NormalizedSort(req.Sort)}
	if linkFilter.Active {
		indexReq.Limit = 0
	}
	return indexReq
}

func ResultFromIndex(req Request, indexLoaded string, result noteindex.SearchResult, linkFilter LinkTargetFilter) Result {
	if linkFilter.Active {
		result.Results = FilterResultItemsByLinkTarget(result.Results, linkFilter)
		result.Total = len(result.Results)
		if req.Limit > 0 && len(result.Results) > req.Limit {
			result.Results = result.Results[:req.Limit]
		}
		result.Returned = len(result.Results)
	}
	resultNotes := make([]domain.Note, 0, len(result.Results))
	for _, item := range result.Results {
		resultNotes = append(resultNotes, item.Note)
	}
	return Result{Engine: result.Engine, IndexStatus: result.IndexStatus, IndexLoaded: indexLoaded, Total: result.Total, Returned: result.Returned, Notes: resultNotes, Results: result.Results, LinkTargetStatus: linkFilter.Status, LinkTargetMatches: linkFilter.Matches, LinkTargetCandidates: linkFilter.Candidates}
}

func ResultFromFallback(req Request, engine string, notes []domain.Note, indexStatus string, linkFilter LinkTargetFilter) Result {
	filtered := FilterNotes(notes, req)
	if linkFilter.Active {
		filtered = FilterNotesByLinkTarget(filtered, linkFilter)
	}
	SortFallbackNotes(filtered, NormalizedSort(req.Sort))
	total := len(filtered)
	limit := req.Limit
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	items := make([]noteindex.ResultItem, 0, len(filtered))
	for _, note := range filtered {
		items = append(items, noteindex.ResultItem{Note: note, Score: 1, MatchedFields: []string{engine}, Snippet: FirstSnippet(note.Body, req.Query)})
	}
	return Result{Engine: engine, IndexStatus: indexStatus, Total: total, Returned: len(items), Notes: filtered, Results: items, LinkTargetStatus: linkFilter.Status, LinkTargetMatches: linkFilter.Matches, LinkTargetCandidates: linkFilter.Candidates}
}

func Projection(req Request, result Result, shellQuote func(string) string) domain.Projection {
	projection := domain.NewProjection("note.search", "Search completed.")
	projection.Facts["matches"] = fmt.Sprint(result.Returned)
	projection.Facts["total"] = fmt.Sprint(result.Total)
	projection.Facts["returned"] = fmt.Sprint(result.Returned)
	projection.Facts["engine"] = result.Engine
	projection.Facts["sort"] = NormalizedSort(req.Sort)
	if result.IndexStatus != "" {
		projection.Facts["index_status"] = result.IndexStatus
	}
	if result.IndexLoaded != "" {
		projection.Facts["index_loaded"] = result.IndexLoaded
	}
	if result.LinkTargetStatus != "" {
		projection.Facts["link_target.status"] = result.LinkTargetStatus
		projection.Facts["link_target.matches"] = fmt.Sprint(result.LinkTargetMatches)
		if len(result.LinkTargetCandidates) > 0 {
			projection.Facts["link_target.candidates"] = fmt.Sprint(len(result.LinkTargetCandidates))
		}
	}
	if result.Engine == "index" && result.IndexStatus == "stale" {
		projection.Status = "partial"
		projection.Actions = []domain.Action{{Name: "index_rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(req.VaultPath))}}
	}
	AddFilterFacts(projection.Facts, req)
	projection.Data = result
	return projection
}

func NormalizedSort(sortMode string) string {
	sortMode = strings.TrimSpace(sortMode)
	switch sortMode {
	case "", "relevance":
		return "relevance"
	case "updated", "created", "title", "path":
		return sortMode
	default:
		return "relevance"
	}
}

func SortFallbackNotes(notes []domain.Note, mode string) {
	sort.Slice(notes, func(i, j int) bool {
		a := notes[i]
		b := notes[j]
		switch mode {
		case "title":
			if a.Title == b.Title {
				return a.Path < b.Path
			}
			return a.Title < b.Title
		case "path":
			return a.Path < b.Path
		case "created":
			return noteTimeDesc(a.CreatedAt, b.CreatedAt, a.Path, b.Path)
		case "updated":
			return noteTimeDesc(a.UpdatedAt, b.UpdatedAt, a.Path, b.Path)
		default:
			return a.Path < b.Path
		}
	})
}

func BuildLinkTargetFilter(notes []domain.Note, target string, buildGraph LinkGraphBuilder) (LinkTargetFilter, error) {
	if target == "" {
		return LinkTargetFilter{}, nil
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return LinkTargetFilter{}, &domain.CommandError{Code: "invalid_link_filter", Message: "link target filter cannot be empty", Hint: "Provide a note_id, path, title, or unresolved raw target"}
	}
	matchedNote, candidates, ambiguous := resolveLinkTargetNote(notes, target)
	if ambiguous {
		return LinkTargetFilter{}, &domain.CommandError{Code: "link_target_ambiguous", Message: "link target matched multiple candidate notes", Hint: "Retry with a note_id or full path; candidates: " + formatLinkTargetCandidates(candidates)}
	}
	filter := LinkTargetFilter{Active: true, Status: "raw", SourcePath: map[string]bool{}, Candidates: candidates}
	for sourcePath, links := range buildGraph(notes) {
		for _, link := range links {
			if linkMatchesTarget(link, target, matchedNote) {
				filter.SourcePath[sourcePath] = true
				filter.Matches++
				switch link.Status {
				case string(domain.LinkStatusResolved):
					filter.Status = "resolved"
				case string(domain.LinkStatusBroken):
					if filter.Status == "raw" {
						filter.Status = "broken"
					}
				case string(domain.LinkStatusAmbiguous):
					if filter.Status == "raw" {
						filter.Status = "ambiguous"
					}
				}
			}
		}
	}
	return filter, nil
}

func FilterResultItemsByLinkTarget(items []noteindex.ResultItem, filter LinkTargetFilter) []noteindex.ResultItem {
	filtered := make([]noteindex.ResultItem, 0, len(items))
	for _, item := range items {
		if filter.SourcePath[item.Note.Path] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func FilterNotesByLinkTarget(notes []domain.Note, filter LinkTargetFilter) []domain.Note {
	filtered := make([]domain.Note, 0, len(notes))
	for _, note := range notes {
		if filter.SourcePath[note.Path] {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

func FilterNotes(notes []domain.Note, req Request) []domain.Note {
	filtered := make([]domain.Note, 0, len(notes))
	for _, note := range notes {
		if req.Status != "discarded" && note.Status == "discarded" {
			continue
		}
		if req.Group != "" && note.Project != req.Group {
			continue
		}
		if req.Folder != "" && note.Folder != req.Folder {
			continue
		}
		if req.Kind != "" && note.Kind != req.Kind {
			continue
		}
		if req.Status != "" && note.Status != req.Status {
			continue
		}
		if req.CreatedAfter != "" && !noteTimestampAfterOrEqual(note.CreatedAt, req.CreatedAfter) {
			continue
		}
		if req.UpdatedAfter != "" && !noteTimestampAfterOrEqual(note.UpdatedAt, req.UpdatedAfter) {
			continue
		}
		ok := true
		for _, tag := range CleanTags(req.Tags) {
			if !stringSliceContains(CleanTags(note.Tags), tag) {
				ok = false
				break
			}
		}
		if ok {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

func AddFilterFacts(facts map[string]string, req Request) {
	if tags := CleanTags(req.Tags); len(tags) > 0 {
		facts["filter.tag"] = strings.Join(tags, ",")
	}
	if req.Group != "" {
		facts["filter.group"] = req.Group
	}
	if req.Folder != "" {
		facts["filter.folder"] = req.Folder
	}
	if req.Kind != "" {
		facts["filter.kind"] = req.Kind
	}
	if req.Status != "" {
		facts["filter.status"] = req.Status
	}
	if req.CreatedAfter != "" {
		facts["filter.created_after"] = req.CreatedAfter
	}
	if req.UpdatedAfter != "" {
		facts["filter.updated_after"] = req.UpdatedAfter
	}
	if req.LinkTarget != "" {
		facts["filter.link_target"] = req.LinkTarget
	}
	if req.HasAttachment {
		facts["filter.has_attachment"] = "true"
	}
}

func CleanTags(tags []string) []string {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(tags))
	for _, tag := range tags {
		for _, part := range strings.Split(tag, ",") {
			part = strings.TrimPrefix(strings.TrimSpace(part), "#")
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			cleaned = append(cleaned, part)
		}
	}
	return cleaned
}

func FirstSnippet(body, query string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query != "" {
		idx := strings.Index(strings.ToLower(body), query)
		if idx >= 0 {
			start := idx - 30
			if start < 0 {
				start = 0
			}
			end := idx + len(query) + 60
			if end > len(body) {
				end = len(body)
			}
			return strings.TrimSpace(body[start:end])
		}
	}
	if len(body) > 120 {
		return body[:120]
	}
	return body
}

func ParseUserDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", value)
}

func noteTimeDesc(a, b, pathA, pathB string) bool {
	at, aErr := ParseUserDate(a)
	bt, bErr := ParseUserDate(b)
	if aErr != nil || bErr != nil || at.Equal(bt) {
		return pathA < pathB
	}
	return at.After(bt)
}

func resolveLinkTargetNote(notes []domain.Note, target string) (domain.Note, []domain.NoteLinkCandidate, bool) {
	lowerTarget := strings.ToLower(target)
	cleanTarget := filepath.ToSlash(filepath.Clean(target))
	if cleanTarget == "." {
		cleanTarget = target
	}
	for _, note := range notes {
		if note.ID != "" && strings.EqualFold(note.ID, target) {
			return note, []domain.NoteLinkCandidate{{Path: note.Path, Title: note.Title, NoteID: note.ID}}, false
		}
	}
	for _, note := range notes {
		if note.Path == cleanTarget || strings.TrimPrefix(note.Path, "notes/") == cleanTarget {
			return note, []domain.NoteLinkCandidate{{Path: note.Path, Title: note.Title, NoteID: note.ID}}, false
		}
	}
	titleMatches := make([]domain.NoteLinkCandidate, 0)
	var matched domain.Note
	for _, note := range notes {
		if strings.ToLower(note.Title) == lowerTarget {
			matched = note
			titleMatches = append(titleMatches, domain.NoteLinkCandidate{Path: note.Path, Title: note.Title, NoteID: note.ID})
		}
	}
	if len(titleMatches) > 1 {
		return domain.Note{}, titleMatches, true
	}
	if len(titleMatches) == 1 {
		return matched, titleMatches, false
	}
	return domain.Note{}, nil, false
}

func linkMatchesTarget(link domain.NoteLink, rawTarget string, matchedNote domain.Note) bool {
	if matchedNote.Path != "" {
		return link.Status == string(domain.LinkStatusResolved) && (link.TargetNoteID == matchedNote.ID || link.TargetPath == matchedNote.Path || strings.EqualFold(link.TargetTitle, matchedNote.Title))
	}
	return strings.EqualFold(link.Target, rawTarget) || strings.EqualFold(link.TargetRaw, rawTarget)
}

func formatLinkTargetCandidates(candidates []domain.NoteLinkCandidate) string {
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		parts = append(parts, candidate.Path)
	}
	return strings.Join(parts, ",")
}

func noteTimestampAfterOrEqual(value, boundary string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	valueTime, err := ParseUserDate(value)
	if err != nil {
		return false
	}
	boundaryTime, err := ParseUserDate(boundary)
	if err != nil {
		return false
	}
	return valueTime.Equal(boundaryTime) || valueTime.After(boundaryTime)
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
