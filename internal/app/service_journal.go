package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

// Journal and inbox operations: daily/weekly/monthly open/show/append, journal note
// helpers, and inbox capture/list/triage. Extracted from service.go to isolate the
// periodic-journal and inbox surfaces.

func (s *Service) DailyOpen(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, "daily", req)
	if err != nil {
		return errorProjection("daily.open", err), err
	}
	projection, err := s.EditNote(ctx, NoteEditRequest{VaultPath: root, NoteRef: rel, Editor: req.Editor})
	projection.Command = "daily.open"
	projection.Summary = "Daily note opened."
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = "daily"
	projection.Facts["template"] = journalTemplateName("daily", req)
	return projection, err
}

func (s *Service) DailyShow(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, "daily", req)
	if err != nil {
		return errorProjection("daily.show", err), err
	}
	projection, err := s.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: rel})
	projection.Command = "daily.show"
	projection.Summary = "Daily note read."
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["template"] = journalTemplateName("daily", req)
	projection.Facts["period"] = "daily"
	return projection, err
}

func (s *Service) DailyAppend(_ context.Context, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, "daily", req)
	if err != nil {
		return errorProjection("daily.append", err), err
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		err := &domain.CommandError{Code: "body_required", Message: "daily append requires --body", Hint: "pinax daily append --body <text> --vault <vault>"}
		return domain.NewErrorProjection("daily.append", err), err
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return errorProjection("daily.append", err), err
	}
	if err := appendFile(path, "\n\n"+body+"\n"); err != nil {
		return errorProjection("daily.append", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("daily.append", err), err
	}
	_ = appendEvent(root, "daily.append", "success", map[string]string{"path": rel})
	projection := domain.NewProjection("daily.append", "Daily note appended.")
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["template"] = journalTemplateName("daily", req)
	projection.Facts["period"] = "daily"
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{rel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func (s *Service) WeeklyOpen(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.openJournal(ctx, "weekly", req)
}

func (s *Service) WeeklyShow(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.showJournal(ctx, "weekly", req)
}

func (s *Service) WeeklyAppend(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.appendJournal(ctx, "weekly", req)
}

func (s *Service) MonthlyOpen(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.openJournal(ctx, "monthly", req)
}

func (s *Service) MonthlyShow(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.showJournal(ctx, "monthly", req)
}

func (s *Service) MonthlyAppend(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.appendJournal(ctx, "monthly", req)
}

func (s *Service) openJournal(ctx context.Context, period string, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, period, req)
	if err != nil {
		return errorProjection(period+".open", err), err
	}
	projection, err := s.EditNote(ctx, NoteEditRequest{VaultPath: root, NoteRef: rel, Editor: req.Editor})
	projection.Command = period + ".open"
	projection.Summary = journalLabel(period) + " opened."
	projection.Facts["template"] = journalTemplateName(period, req)
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = period
	return projection, err
}

func (s *Service) showJournal(ctx context.Context, period string, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, period, req)
	if err != nil {
		return errorProjection(period+".show", err), err
	}
	projection, err := s.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: rel})
	projection.Command = period + ".show"
	projection.Facts["template"] = journalTemplateName(period, req)
	projection.Summary = journalLabel(period) + " read."
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = period
	return projection, err
}

func (s *Service) appendJournal(_ context.Context, period string, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, period, req)
	if err != nil {
		return errorProjection(period+".append", err), err
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		err := &domain.CommandError{Code: "body_required", Message: period + " append requires --body", Hint: "pinax " + period + " append --body <text> --vault <vault>"}
		return domain.NewErrorProjection(period+".append", err), err
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return errorProjection(period+".append", err), err
	}
	if err := appendFile(path, "\n\n"+body+"\n"); err != nil {
		return errorProjection(period+".append", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection(period+".append", err), err
	}
	_ = appendEvent(root, period+".append", "success", map[string]string{"path": rel})
	projection := domain.NewProjection(period+".append", journalLabel(period)+" appended.")
	projection.Facts["path"] = rel
	projection.Facts["template"] = journalTemplateName(period, req)
	projection.Facts["date"] = key
	projection.Facts["period"] = period
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{rel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func (s *Service) InboxCapture(ctx context.Context, req CreateNoteRequest) (domain.Projection, error) {
	req.Folder = "inbox"
	req.Kind = "inbox"
	req.Status = "inbox"
	projection, err := s.CreateNote(ctx, req)
	projection.Command = "inbox.capture"
	projection.Summary = "Inbox note captured."
	return projection, err
}

func (s *Service) InboxList(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	projection, err := s.ListNotesQuery(ctx, NoteListRequest{VaultPath: req.VaultPath, Status: "inbox", Sort: "updated"})
	projection.Command = "inbox.list"
	projection.Summary = "Inbox notes listed."
	return projection, err
}

func (s *Service) InboxTriage(_ context.Context, req InboxTriageRequest) (domain.Projection, error) {
	root, note, path, content, meta, _, err := loadMutableNote(req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("inbox.triage", err), err
	}
	group := strings.TrimSpace(req.Group)
	if group == "" {
		err := &domain.CommandError{Code: "group_required", Message: "inbox triage requires --group", Hint: "pinax inbox triage <note> --group <group> --vault <vault>"}
		return domain.NewErrorProjection("inbox.triage", err), err
	}
	folder, err := validateOptionalNoteFolder(req.Folder)
	if err != nil {
		return errorProjection("inbox.triage", err), err
	}
	if folder == "" {
		folder = "inbox"
	}
	projectPrefix := filepath.ToSlash(filepath.Join("notes", group))
	if project, err := findProject(root, group); err == nil && strings.TrimSpace(project.NotesPrefix) != "" {
		projectPrefix = project.NotesPrefix
	}
	targetRel := filepath.ToSlash(filepath.Join(projectPrefix, folder, filepath.Base(note.Path)))
	target, err := safeJoin(root, targetRel)
	if err != nil {
		return errorProjection("inbox.triage", err), err
	}
	if _, err := os.Stat(target); err == nil {
		err := &domain.CommandError{Code: "note_path_conflict", Message: "Target note path already exists", Hint: "Choose another folder or handle the existing file first"}
		return domain.NewErrorProjection("inbox.triage", err), err
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errorProjection("inbox.triage", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	meta["project"] = group
	meta["folder"] = folder
	if strings.TrimSpace(req.Kind) != "" {
		meta["kind"] = strings.TrimSpace(req.Kind)
	}
	if strings.TrimSpace(req.Status) != "" {
		meta["status"] = strings.TrimSpace(req.Status)
	}
	meta["updated_at"] = now
	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, target, updated); err != nil {
		return errorProjection("inbox.triage", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("inbox.triage", err), err
	}
	_ = appendEvent(root, "inbox.triage", "success", map[string]string{"from": note.Path, "to": targetRel})
	projection := noteMutationProjection("inbox.triage", "Inbox note triaged.", targetRel, meta)
	projection.Facts["path"] = targetRel
	projection.Facts["group"] = group
	projection.Facts["project"] = group
	projection.Facts["folder"] = folder
	projection.Facts["kind"] = meta["kind"]
	projection.Facts["status"] = meta["status"]
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{targetRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}
