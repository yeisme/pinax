package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type DraftPromoteRequest struct {
	VaultPath string
	NoteRef   string
	Status    string
	Folder    string
	Kind      string
	Yes       bool
	DryRun    bool
}

type InboxPromoteRequest struct {
	VaultPath string
	NoteRef   string
	To        string
	Group     string
	Folder    string
	Kind      string
	Yes       bool
	DryRun    bool
}

// isLifecycleStatus checks if status is a managed lifecycle status.
func isLifecycleStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "inbox", "draft", "active", "archived", "discarded":
		return true
	default:
		return false
	}
}

// isValidTransition checks if the lifecycle status state machine transition is valid.
func isValidTransition(from, to string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	if from == to {
		return true
	}
	switch from {
	case "inbox":
		return to == "draft" || to == "active" || to == "archived" || to == "discarded"
	case "draft":
		return to == "active" || to == "archived" || to == "discarded"
	case "active":
		return to == "archived" || to == "discarded"
	}
	return false
}

// inferLifecycleStatus infers lifecycle status from note properties.
func inferLifecycleStatus(status string, path string, kind string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if isLifecycleStatus(status) {
		return status
	}
	if status == "system" {
		return "system"
	}
	relPath := strings.ToLower(path)
	if strings.HasPrefix(relPath, "inbox/") || kind == "inbox" {
		return "inbox"
	}
	if strings.HasPrefix(relPath, "drafts/") || kind == "draft" {
		return "draft"
	}
	return "active"
}

// transitionNoteLifecycle handles validation, approval checking, path updates/movement, frontmatter patching, index updates, and event logging.
func (s *Service) transitionNoteLifecycle(ctx context.Context, vaultPath string, noteRef string, toStatus string, group, folder, kind string, yes bool, dryRun bool, command string, successSummary string) (domain.Projection, error) {
	root, note, path, content, meta, _, err := s.loadMutableNoteForWrite(ctx, vaultPath, noteRef)
	if err != nil {
		return errorProjection(command, err), err
	}

	fromStatus := inferLifecycleStatus(note.Status, note.Path, note.Kind)
	if !isValidTransition(fromStatus, toStatus) {
		err := &domain.CommandError{
			Code:    "invalid_lifecycle_transition",
			Message: fmt.Sprintf("Invalid status transition: %s -> %s", fromStatus, toStatus),
			Hint:    "Confirm this lifecycle stage allows the operation",
		}
		return domain.NewErrorProjection(command, err), err
	}

	if !dryRun && !yes {
		err := &domain.CommandError{
			Code:    "approval_required",
			Message: "Operation requires confirmation",
			Hint:    fmt.Sprintf("Add --yes or yes=true to confirm this %s operation", command),
		}
		return domain.NewErrorProjection(command, err), err
	}

	// Calculate target path
	var targetRel string
	if group != "" || folder != "" {
		projectPrefix := "notes"
		if group != "" {
			projectPrefix = filepath.ToSlash(filepath.Join("notes", group))
			if project, err := findProject(root, group); err == nil && strings.TrimSpace(project.NotesPrefix) != "" {
				projectPrefix = project.NotesPrefix
			}
		}
		targetRel = filepath.ToSlash(filepath.Join(projectPrefix, folder, filepath.Base(note.Path)))
	} else if fromStatus == "inbox" && toStatus == "draft" && strings.HasPrefix(note.Path, "inbox/") {
		targetRel = filepath.ToSlash(filepath.Join("drafts", filepath.Base(note.Path)))
	} else {
		targetRel = note.Path
	}

	targetPath, err := safeJoin(root, targetRel)
	if err != nil {
		return errorProjection(command, err), err
	}

	if targetRel != note.Path {
		if _, err := os.Stat(targetPath); err == nil {
			err := &domain.CommandError{
				Code:    "note_path_conflict",
				Message: "Target note path already exists",
				Hint:    "Choose another folder or handle the existing file first",
			}
			return domain.NewErrorProjection(command, err), err
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errorProjection(command, err), err
		}
	}

	if dryRun {
		projection := domain.NewProjection(command, "Dry-run status transition preview generated.")
		projection.Facts = map[string]string{
			"writes":                   "false",
			"old_status":               fromStatus,
			"new_status":               toStatus,
			"path":                     note.Path,
			"planned_path":             targetRel,
			"approval_required":        fmt.Sprintf("%t", !yes),
			"index_update_expectation": "true",
		}
		return projection, nil
	}

	// Perform actual write
	now := time.Now().UTC().Format(time.RFC3339)
	meta["status"] = toStatus
	if kind != "" {
		meta["kind"] = kind
	}
	if group != "" {
		meta["project"] = group
	}
	if folder != "" {
		meta["folder"] = folder
	}
	meta["updated_at"] = now

	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, targetPath, updated); err != nil {
		return errorProjection(command, err), err
	}

	if err := refreshIndex(root); err != nil {
		return errorProjection(command, err), err
	}

	// Append event and metadata evidence
	_ = appendEvent(root, command, "success", map[string]string{
		"from":   note.Path,
		"to":     targetRel,
		"status": toStatus,
	})

	projection := noteMutationProjection(command, successSummary, targetRel, meta)
	projection.Facts["writes"] = "true"
	projection.Facts["status"] = toStatus
	projection.Facts["old_status"] = fromStatus
	projection.Facts["new_status"] = toStatus
	projection.Facts["path"] = targetRel
	projection.Facts["index_updated"] = "true"
	if command == "draft.discard" || command == "inbox.discard" {
		projection.Facts["deleted"] = "false"
	}

	projection.Evidence = []string{targetRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func extractNoteInfo(data any) (status, path, kind string) {
	if dataMap, ok := data.(map[string]any); ok {
		if noteVal, ok := dataMap["note"]; ok {
			if note, ok := noteVal.(domain.Note); ok {
				return note.Status, note.Path, note.Kind
			}
			if displayNote, ok := noteVal.(domain.NoteDisplay); ok {
				return displayNote.Status, displayNote.Path, displayNote.Kind
			}
		}
	}
	return "", "", ""
}

func (s *Service) DraftCreate(ctx context.Context, req CreateNoteRequest) (domain.Projection, error) {
	if req.Folder == "" {
		req.Folder = "drafts"
	}
	req.Status = "draft"
	projection, err := s.CreateNote(ctx, req)
	if err != nil {
		return errorProjection("draft.create", err), err
	}
	projection.Command = "draft.create"
	projection.Summary = "Draft note created."
	return projection, nil
}

func (s *Service) DraftList(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	projection, err := s.ListNotesQuery(ctx, NoteListRequest{VaultPath: req.VaultPath, Status: "draft", Sort: "updated"})
	if err != nil {
		return errorProjection("draft.list", err), err
	}
	projection.Command = "draft.list"
	projection.Summary = "Draft notes listed."
	if projection.Facts == nil {
		projection.Facts = map[string]string{}
	}
	projection.Facts["fact.returned"] = projection.Facts["returned"]
	projection.Facts["fact.total"] = projection.Facts["total"]
	projection.Facts["fact.filter.status"] = "draft"
	projection.Actions = []domain.Action{
		{
			Name:    "Promote draft",
			Command: "pinax draft promote <note_id>",
		},
	}
	return projection, nil
}

func (s *Service) DraftShow(ctx context.Context, req ShowNoteRequest) (domain.Projection, error) {
	projection, err := s.ShowNoteProjection(ctx, req)
	if err != nil {
		return projection, err
	}
	projection.Command = "draft.show"
	status, path, kind := extractNoteInfo(projection.Data)
	lcStatus := inferLifecycleStatus(status, path, kind)
	projection.Facts["status"] = status
	projection.Facts["lifecycle_status"] = lcStatus
	projection.Facts["body_exposure"] = "full"
	projection.Actions = []domain.Action{
		{
			Name:    "Promote draft",
			Command: "pinax draft promote <note_id>",
		},
		{
			Name:    "Discard draft",
			Command: "pinax draft discard <note_id>",
		},
	}
	return projection, nil
}

func (s *Service) DraftPromote(ctx context.Context, req DraftPromoteRequest) (domain.Projection, error) {
	targetStatus := req.Status
	if targetStatus == "" {
		targetStatus = "active"
	}
	return s.transitionNoteLifecycle(ctx, req.VaultPath, req.NoteRef, targetStatus, "", req.Folder, req.Kind, req.Yes, req.DryRun, "draft.promote", "Draft note promoted.")
}

func (s *Service) DraftArchive(ctx context.Context, req NoteMutationRequest) (domain.Projection, error) {
	return s.transitionNoteLifecycle(ctx, req.VaultPath, req.NoteRef, "archived", "", "", "", req.Yes, req.DryRun, "draft.archive", "Draft note archived.")
}

func (s *Service) DraftDiscard(ctx context.Context, req NoteMutationRequest) (domain.Projection, error) {
	return s.transitionNoteLifecycle(ctx, req.VaultPath, req.NoteRef, "discarded", "", "", "", req.Yes, req.DryRun, "draft.discard", "Draft note discarded.")
}

func (s *Service) InboxShow(ctx context.Context, req ShowNoteRequest) (domain.Projection, error) {
	projection, err := s.ShowNoteProjection(ctx, req)
	if err != nil {
		return projection, err
	}
	projection.Command = "inbox.show"
	status, path, kind := extractNoteInfo(projection.Data)
	lcStatus := inferLifecycleStatus(status, path, kind)
	projection.Facts["status"] = status
	projection.Facts["lifecycle_status"] = lcStatus
	projection.Facts["body_exposure"] = "full"
	projection.Actions = []domain.Action{
		{
			Name:    "Triage/promote",
			Command: "pinax inbox promote <note_id> --to <active|draft>",
		},
		{
			Name:    "Discard inbox item",
			Command: "pinax inbox discard <note_id>",
		},
	}
	return projection, nil
}

func (s *Service) InboxPromote(ctx context.Context, req InboxPromoteRequest) (domain.Projection, error) {
	targetStatus := req.To
	if targetStatus == "" {
		targetStatus = "active"
	}
	return s.transitionNoteLifecycle(ctx, req.VaultPath, req.NoteRef, targetStatus, req.Group, req.Folder, req.Kind, req.Yes, req.DryRun, "inbox.promote", "Inbox note promoted.")
}

func (s *Service) InboxDiscard(ctx context.Context, req NoteMutationRequest) (domain.Projection, error) {
	return s.transitionNoteLifecycle(ctx, req.VaultPath, req.NoteRef, "discarded", "", "", "", req.Yes, req.DryRun, "inbox.discard", "Inbox note discarded.")
}
