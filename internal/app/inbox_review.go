package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type InboxReviewRequest struct {
	VaultPath string
	Limit     int
	After     string
}

type InboxReviewItem struct {
	NoteID  string          `json:"note_id,omitempty"`
	Path    string          `json:"path"`
	Title   string          `json:"title"`
	Source  string          `json:"source"`
	Status  string          `json:"status,omitempty"`
	Snippet string          `json:"snippet,omitempty"`
	Reason  string          `json:"reason"`
	Related []SearchShowRef `json:"related,omitempty"`
	Actions []domain.Action `json:"actions"`
}

type InboxReviewResult struct {
	Items     []InboxReviewItem `json:"items"`
	Total     int               `json:"total"`
	Remaining int               `json:"remaining"`
	NextAfter string            `json:"next_after,omitempty"`
}

// InboxReview 只装配现有笔记投影，不写索引/审阅状态，也不调用模型。
func (s *Service) InboxReview(_ context.Context, req InboxReviewRequest) (domain.Projection, error) {
	const command = "inbox.review"
	if req.Limit < 1 || req.Limit > 20 {
		err := &domain.CommandError{Code: "invalid_limit", Message: "Review batch size must be between 1 and 20", Hint: "Use --limit 5"}
		return errorProjection(command, err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if info, statErr := os.Stat(filepath.Join(root, ".pinax", "config.yaml")); statErr != nil || !info.Mode().IsRegular() {
		err := &domain.CommandError{Code: "vault_not_initialized", Message: "Review requires an initialized local vault", Hint: "Run pinax vault list --agent and select the intended local vault with --vault"}
		return errorProjection(command, err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes = ordinaryNotes(notes)
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	pending := make([]domain.Note, 0)
	seen := map[string]bool{}
	for _, note := range notes {
		if !isDailyIntakeNote(note) || seen[stableNoteRef(note)] {
			continue
		}
		seen[stableNoteRef(note)] = true
		pending = append(pending, note)
	}
	start := 0
	if req.After != "" {
		cursor, resolveErr := resolveNoteRef(pending, req.After)
		if resolveErr != nil {
			err := &domain.CommandError{Code: "review_cursor_stale", Message: "The review cursor is missing or ambiguous in the pending list", Hint: "Run pinax inbox review again without --after; no notes were changed"}
			return errorProjection(command, err), err
		}
		for i, note := range pending {
			if note.Path == cursor.Path {
				start = i + 1
				break
			}
		}
	}
	end := min(start+req.Limit, len(pending))
	result := InboxReviewResult{Items: []InboxReviewItem{}, Total: len(pending), Remaining: len(pending) - end}
	projection := domain.NewProjection(command, "Pending notes ready for review; nothing changed.")
	projection.Facts["writes"] = "false"
	projection.Facts["total"] = fmt.Sprint(result.Total)
	projection.Facts["returned"] = fmt.Sprint(end - start)
	projection.Facts["remaining"] = fmt.Sprint(result.Remaining)
	projection.Facts["suggestion_basis"] = "shared_tags_only"
	for i, note := range pending[start:end] {
		source := "index"
		if note.Status == "inbox" {
			source = "inbox"
		}
		ref := stableNoteRef(note)
		related := searchShowNeighbors(notes, note)
		reason := "No tag-based destination evidence; read before choosing a folder."
		if len(related) > 0 {
			reason = "Related notes share tags; review them before choosing a destination."
		}
		actions := []domain.Action{
			{Name: "inspect", Command: fmt.Sprintf("pinax search show %s --vault %s", shellQuote(ref), shellQuote(root))},
			{Name: "open", Command: fmt.Sprintf("pinax note open %s --vault %s", shellQuote(ref), shellQuote(root))},
		}
		result.Items = append(result.Items, InboxReviewItem{NoteID: note.ID, Path: note.Path, Title: note.Title, Source: source, Status: note.Status, Snippet: searchShowSnippet(note), Reason: reason, Related: related, Actions: actions})
		prefix := fmt.Sprintf("item.%d.", i+1)
		projection.Facts[prefix+"ref"] = ref
		projection.Facts[prefix+"title"] = note.Title
		projection.Facts[prefix+"source"] = source
		projection.Facts[prefix+"snippet"] = searchShowSnippet(note)
		projection.Facts[prefix+"reason"] = reason
		projection.Actions = append(projection.Actions, domain.Action{Name: fmt.Sprintf("inspect_%d", i+1), Command: actions[0].Command})
	}
	if result.Remaining > 0 {
		result.NextAfter = stableNoteRef(pending[end-1])
		projection.Actions = append(projection.Actions, domain.Action{Name: "next", Command: fmt.Sprintf("pinax inbox review --after %s --limit %d --vault %s", shellQuote(result.NextAfter), req.Limit, shellQuote(root))})
	}
	refs := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		refs = append(refs, item.NoteID)
	}
	if selected, err := selectedOrganizeNotes(root, refs); err == nil {
		selection := make([]domain.OrganizeSelection, 0, len(selected))
		for _, note := range selected {
			selection = append(selection, domain.OrganizeSelection{ObjectID: note.ID})
		}
		projection.Actions = append(projection.Actions, domain.Action{Name: "organize_preview", Command: scopedOrganizeCommand(root, selection)})
	}
	projection.Data = result
	return projection, nil
}

func isDailyIntakeNote(note domain.Note) bool {
	if note.Status == "archived" || note.Status == "discarded" || note.Status == "system" {
		return false
	}
	return note.Status == "inbox" || (strings.HasPrefix(note.Path, "notes/index/") && note.Project == "")
}
