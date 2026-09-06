package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/fsutil"
	noteindex "github.com/yeisme/pinax/internal/index"
)

// Note operations: links/backlinks/orphans, attachments, import/export, edit,
// rename/move/archive/delete, tag/property patching, bulk tag/folder operations,
// and supporting folder helpers. Extracted from service.go to isolate the note
// mutation and query surface.

func (s *Service) NoteLinks(ctx context.Context, req NoteLinkRequest) (domain.Projection, error) {
	// 尝试使用增强链接图
	enhancedReq := NoteLinkGraphRequest{VaultPath: req.VaultPath, NoteRef: req.NoteRef, All: req.All, BrokenOnly: req.BrokenOnly, Kind: req.Kind, Status: req.Status, IncludeIgnored: req.IncludeIgnored, Limit: req.Limit}
	projection, err := s.QueryOutgoingLinks(ctx, enhancedReq)
	if err == nil {
		return projection, nil
	}
	// fallback: 原有实现
	root, err2 := cleanVaultPath(req.VaultPath)
	if err2 != nil {
		return errorProjection("note.links", err2), err2
	}
	graph, graphErr := buildNoteLinkGraph(root)
	if graphErr != nil {
		return errorProjection("note.links", graphErr), graphErr
	}
	note, resolveErr := resolveNoteRef(graph.notes, req.NoteRef)
	if resolveErr != nil {
		return errorProjection("note.links", resolveErr), resolveErr
	}
	links := graph.outgoing[note.Path]
	projection = domain.NewProjection("note.links", "Note links listed.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["links"] = fmt.Sprint(len(links))
	projection.Facts["resolved"] = fmt.Sprint(countResolvedLinks(links))
	projection.Facts["broken"] = fmt.Sprint(countBrokenLinks(links))
	projection.Facts["ambiguous"] = "0"
	projection.Facts["engine"] = "scan"
	addLinkCompatibilityFacts(projection.Facts)
	if countBrokenLinks(links) > 0 {
		projection.Status = "partial"
		projection.Actions = []domain.Action{{Name: "repair_plan", Command: fmt.Sprintf("pinax repair plan --vault %s", shellQuote(root))}}
	}
	projection.Data = map[string]any{"note": noteGraphNoteSummary(note), "links": links}
	return projection, nil
}

func (s *Service) NoteBacklinks(ctx context.Context, req NoteLinkRequest) (domain.Projection, error) {
	// 尝试使用增强链接图
	enhancedReq := NoteBacklinkGraphRequest{VaultPath: req.VaultPath, NoteRef: req.NoteRef, IncludeBroken: true}
	projection, err := s.QueryBacklinks(ctx, enhancedReq)
	if err == nil {
		return projection, nil
	}
	// fallback: 原有实现
	root, err2 := cleanVaultPath(req.VaultPath)
	if err2 != nil {
		return errorProjection("note.backlinks", err2), err2
	}
	graph, graphErr := buildNoteLinkGraph(root)
	if graphErr != nil {
		return errorProjection("note.backlinks", graphErr), graphErr
	}
	note, resolveErr := resolveNoteRef(graph.notes, req.NoteRef)
	if resolveErr != nil {
		return errorProjection("note.backlinks", resolveErr), resolveErr
	}
	backlinks := graph.incoming[note.Path]
	projection = domain.NewProjection("note.backlinks", "Note backlinks listed.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["backlinks"] = fmt.Sprint(len(backlinks))
	projection.Facts["unresolved"] = "0"
	projection.Facts["engine"] = "scan"
	addLinkCompatibilityFacts(projection.Facts)
	projection.Data = map[string]any{"note": note, "backlinks": backlinks}
	return projection, nil
}

func (s *Service) NoteOrphans(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	// 尝试使用增强链接图
	enhancedReq := NoteOrphansRequest{VaultPath: req.VaultPath, Mode: "full"}
	projection, err := s.QueryOrphans(ctx, enhancedReq)
	if err == nil {
		return projection, nil
	}
	// fallback: 原有实现
	root, err2 := cleanVaultPath(req.VaultPath)
	if err2 != nil {
		return errorProjection("note.orphans", err2), err2
	}
	graph, graphErr := buildNoteLinkGraph(root)
	if graphErr != nil {
		return errorProjection("note.orphans", graphErr), graphErr
	}
	orphans := make([]domain.Note, 0)
	for _, note := range graph.notes {
		if len(graph.outgoing[note.Path]) == 0 && len(graph.incoming[note.Path]) == 0 {
			orphans = append(orphans, note)
		}
	}
	projection = domain.NewProjection("note.orphans", "Orphan notes listed.")
	projection.Facts["notes"] = fmt.Sprint(len(graph.notes))
	projection.Facts["orphans"] = fmt.Sprint(len(orphans))
	projection.Facts["engine"] = "scan"
	// 与 QueryOrphans 保持同一套 agent-safe 投影：fallback 路径也剥离 Body。
	summaries := make([]domain.Note, 0, len(orphans))
	for _, note := range orphans {
		summaries = append(summaries, noteGraphNoteSummary(note))
	}
	projection.Data = map[string]any{"orphans": summaries}
	return projection, nil
}
func (s *Service) AttachNoteFile(ctx context.Context, req NoteAttachRequest) (domain.Projection, error) {
	root, note, notePath, content, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	source := strings.TrimSpace(req.SourcePath)
	if source == "" {
		err := &domain.CommandError{Code: "attachment_source_required", Message: "note attach requires a source file", Hint: "pinax note attach <note> <file> --vault <vault>"}
		return domain.NewErrorProjection("note.attach", err), err
	}
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		commandErr := &domain.CommandError{Code: "attachment_source_missing", Message: "Attachment source file does not exist", Hint: "Check the source file path, then retry"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	if info.IsDir() {
		commandErr := &domain.CommandError{Code: "attachment_source_is_directory", Message: "Attachment source path is a directory", Hint: "Provide a single file path"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	mode := normalizedAttachmentMode(req.Mode)
	if mode == "" {
		commandErr := &domain.CommandError{Code: "attachment_mode_invalid", Message: "Attachment write mode is invalid", Hint: "Use --mode copy, move, or register"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	if mode == "move" && !req.Yes {
		commandErr := &domain.CommandError{Code: "approval_required", Message: "Moving the source file requires explicit confirmation", Hint: "pinax note attach " + req.NoteRef + " " + source + " --mode move --yes --vault " + root + " --json"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	filename := filepath.Base(source)
	if strings.TrimSpace(req.Rename) != "" {
		filename = req.Rename
	}
	placement := normalizedAttachmentPlacement(req.Placement)
	attachmentRel := ""
	if mode == "register" {
		if strings.TrimSpace(req.Rename) != "" {
			commandErr := &domain.CommandError{Code: "attachment_rename_requires_copy_or_move", Message: "register mode does not rename files", Hint: "Remove --rename, or switch to --mode copy"}
			return domain.NewErrorProjection("note.attach", commandErr), commandErr
		}
		attachmentRel, err = registeredAttachmentRel(root, source)
		if err != nil {
			return errorProjection("note.attach", err), err
		}
	} else {
		attachmentRel, err = uniqueAttachmentRelWithPlacement(root, note, filename, placement)
		if err != nil {
			return errorProjection("note.attach", err), err
		}
	}
	linkStyle, reference, err := attachmentReference(note.Path, attachmentRel, req.LinkStyle, req.Embed)
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	attachmentPath, err := safeJoin(root, attachmentRel)
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	if mode != "register" {
		if err := os.MkdirAll(filepath.Dir(attachmentPath), 0o755); err != nil {
			return errorProjection("note.attach", err), err
		}
		if err := fsutil.CopyFile(attachmentPath, source); err != nil {
			return errorProjection("note.attach", err), err
		}
	}
	updated := strings.TrimRight(content, "\n") + "\n\n" + reference + "\n"
	if err := commitNoteContent(notePath, notePath, updated); err != nil {
		return errorProjection("note.attach", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("note.attach", err), err
	}
	if mode == "move" {
		if err := os.Remove(source); err != nil {
			return errorProjection("note.attach", err), err
		}
	}
	appendEventWarned(root, "note.attach", "success", map[string]string{"path": note.Path, "attachment_path": attachmentRel})
	projection := domain.NewProjection("note.attach", "Attachment added to note.")
	projection.Facts["path"] = note.Path
	projection.Facts["attachment_path"] = attachmentRel
	projection.Facts["source_path"] = source
	projection.Facts["media_type"] = attachmentMediaType(attachmentRel)
	projection.Facts["placement"] = string(placement)
	projection.Facts["link_style"] = linkStyle
	projection.Facts["mode"] = mode
	projection.Facts["reference"] = reference
	projection.Facts["index_status"] = "fresh"
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{note.Path, attachmentRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"note": note, "attachment": domain.NoteAttachment{NotePath: note.Path, ReferenceText: reference, Path: attachmentRel, TargetPath: attachmentRel, MediaType: attachmentMediaType(attachmentRel), Exists: true}}
	return projection, nil
}

func (s *Service) NoteAttachments(_ context.Context, req NoteLinkRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.attachments", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("note.attachments", err), err
	}
	note, err := resolveNoteRef(notes, req.NoteRef)
	if err != nil {
		return errorProjection("note.attachments", err), err
	}
	attachments := noteAttachmentsFromBody(root, note)
	if req.PathStyle != "" || req.IncludePaths {
		if err := applyAttachmentDisplayPaths(root, note.Path, req.PathStyle, attachments); err != nil {
			return errorProjection("note.attachments", err), err
		}
	}
	projection := domain.NewProjection("note.attachments", "Note attachments listed.")
	projection.Facts["path"] = note.Path
	projection.Facts["attachments"] = fmt.Sprint(len(attachments))
	projection.Facts["missing"] = fmt.Sprint(countMissingAttachments(attachments))
	if req.PathStyle != "" {
		projection.Facts["path_style"] = req.PathStyle
	}
	projection.Data = map[string]any{"note": note, "attachments": attachments}
	return projection, nil
}
func (s *Service) ImportMarkdown(_ context.Context, req ImportMarkdownRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("import.markdown", err), err
	}
	safeTags, tagErr := normalizeTagsForWrite(req.Tags)
	if tagErr != nil {
		return domain.NewErrorProjection("import.markdown", tagErr), tagErr
	}
	req.Tags = safeTags
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("import.markdown", err), err
	}
	source, err := cleanVaultPath(req.Source)
	if err != nil {
		return errorProjection("import.markdown", err), err
	}
	plans, err := planMarkdownImport(root, source, req)
	if err != nil {
		return errorProjection("import.markdown", err), err
	}
	projection := domain.NewProjection("import.markdown", "Markdown import plan generated.")
	projection.Facts["planned"] = fmt.Sprint(len(plans))
	projection.Facts["written"] = "0"
	projection.Facts["renamed"] = fmt.Sprint(countImportPlans(plans, "rename"))
	projection.Facts["overwritten"] = fmt.Sprint(countImportPlans(plans, "overwrite"))
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Data = map[string]any{"plans": plans, "dry_run": req.DryRun}
	if req.DryRun {
		return projection, nil
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "import markdown requires --yes", Hint: "Preview the plan with --dry-run first, then add --yes after confirming"}
		return domain.NewErrorProjection("import.markdown", err), err
	}
	written := 0
	for _, plan := range plans {
		if plan.Status == "skip" {
			continue
		}
		content, err := os.ReadFile(plan.SourcePath)
		if err != nil {
			return errorProjection("import.markdown", err), err
		}
		note := parseNote(filepath.Base(plan.SourcePath), string(content))
		body := note.Body
		if strings.TrimSpace(body) == "" {
			_, body = splitFrontmatter(string(content))
		}
		if strings.TrimSpace(body) == "" {
			body = string(content)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		folder := strings.TrimSpace(req.Folder)
		if folder == "" {
			folder = filepath.ToSlash(filepath.Dir(strings.TrimPrefix(plan.TargetPath, "notes/")))
			if folder == "." || folder == strings.TrimSpace(req.Group) {
				folder = ""
			}
		}
		output := buildNoteContentWithStatus(note.Title, plan.TargetPath, strings.TrimSpace(req.Group), folder, strings.TrimSpace(req.Kind), cleanTags(req.Tags), strings.TrimSpace(req.Status), now, body)
		target, err := safeJoin(root, plan.TargetPath)
		if err != nil {
			return errorProjection("import.markdown", err), err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errorProjection("import.markdown", err), err
		}
		if err := atomicWriteFile(target, []byte(output), 0o644); err != nil {
			return errorProjection("import.markdown", err), err
		}
		written++
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("import.markdown", err), err
	}
	receiptRel, err := writeReceipt(root, "import", map[string]any{"source": source, "plans": plans, "written": written})
	if err != nil {
		return errorProjection("import.markdown", err), err
	}
	appendEventWarned(root, "import.markdown", "success", map[string]string{"written": fmt.Sprint(written), "receipt_path": receiptRel})
	projection.Summary = "Markdown imported."
	projection.Facts["written"] = fmt.Sprint(written)
	projection.Facts["overwritten"] = fmt.Sprint(countImportPlans(plans, "overwrite"))
	projection.Facts["receipt_path"] = receiptRel
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{receiptRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"plans": plans, "written": written, "receipt_path": receiptRel}
	return projection, nil
}

func (s *Service) ExportMarkdown(_ context.Context, req ExportMarkdownRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("export.markdown", err), err
	}
	out, err := cleanVaultPath(req.OutputDir)
	if err != nil {
		return errorProjection("export.markdown", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("export.markdown", err), err
	}
	filter := NoteListRequest{VaultPath: root, Tags: cleanTags(req.Tags), Group: strings.TrimSpace(req.Group), Folder: strings.TrimSpace(req.Folder), Kind: strings.TrimSpace(req.Kind), Status: strings.TrimSpace(req.Status)}
	selected := make([]domain.Note, 0)
	for _, note := range notes {
		if noteMatchesQuery(note, filter) {
			selected = append(selected, note)
		}
	}
	attachmentsCopied := 0
	for _, note := range selected {
		source, err := safeJoin(root, note.Path)
		if err != nil {
			return errorProjection("export.markdown", err), err
		}
		if err := copyVaultFile(source, filepath.Join(out, filepath.FromSlash(note.Path))); err != nil {
			return errorProjection("export.markdown", err), err
		}
		for _, attachment := range noteAttachmentsFromBody(root, note) {
			if !attachment.Exists {
				continue
			}
			attachmentSource := filepath.Join(root, filepath.FromSlash(attachment.TargetPath))
			if err := copyVaultFile(attachmentSource, filepath.Join(out, filepath.FromSlash(attachment.TargetPath))); err != nil {
				return errorProjection("export.markdown", err), err
			}
			attachmentsCopied++
		}
	}
	receiptRel, err := writeReceipt(root, "export", map[string]any{"output_dir": out, "notes": len(selected), "attachments": attachmentsCopied})
	if err != nil {
		return errorProjection("export.markdown", err), err
	}
	projection := domain.NewProjection("export.markdown", "Markdown exported.")
	projection.Facts["output_dir"] = out
	projection.Facts["notes"] = fmt.Sprint(len(selected))
	projection.Facts["attachments"] = fmt.Sprint(attachmentsCopied)
	projection.Facts["receipt_path"] = receiptRel
	projection.Evidence = []string{out, receiptRel}
	projection.Data = map[string]any{"notes": selected, "attachments": attachmentsCopied, "receipt_path": receiptRel}
	return projection, nil
}

func (s *Service) EditNote(ctx context.Context, req NoteEditRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.edit", err), err
	}
	note, err := s.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.NoteRef})
	if err != nil {
		return errorProjection("note.edit", err), err
	}
	editorText := strings.TrimSpace(req.Editor)
	if editorText == "" {
		editorText = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	editor, err := parseEditorCommand(editorText)
	if err != nil {
		return errorProjection("note.edit", err), err
	}
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return errorProjection("note.edit", err), err
	}
	args := append(append([]string{}, editor.Args...), path)
	cmd := exec.CommandContext(ctx, editor.Executable, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		commandErr := &domain.CommandError{Code: "editor_failed", Message: "Editor execution failed", Hint: "Check the executable pointed to by --editor or $EDITOR"}
		return domain.NewErrorProjection("note.edit", commandErr), commandErr
	}
	projection := domain.NewProjection("note.edit", "Note opened in editor.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["editor"] = editor.Raw
	projection.Facts["editor_executable"] = editor.Executable
	projection.Facts["editor_args"] = strings.Join(editor.Args, " ")
	projection.Data = map[string]any{"note": note, "editor": editor}
	return projection, nil
}

func (s *Service) RenameNote(ctx context.Context, req NoteMutationRequest) (domain.Projection, error) {
	root, note, path, content, meta, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.rename", err), err
	}
	notesBefore, _ := scanNotes(root)
	_, indexErr := os.Stat(filepath.Join(root, ".pinax", "index.sqlite"))
	indexWasFresh := indexErr == nil
	newTitle := strings.TrimSpace(req.Title)
	if newTitle == "" {
		err := &domain.CommandError{Code: "title_required", Message: "note rename requires a new title", Hint: "pinax note rename <note> <title> --vault <vault>"}
		return domain.NewErrorProjection("note.rename", err), err
	}
	meta["title"] = newTitle
	meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	targetRel := filepath.ToSlash(filepath.Join(filepath.Dir(note.Path), slugify(newTitle)+".md"))
	if targetRel == filepath.ToSlash(filepath.Dir(note.Path))+"/.md" {
		targetRel = filepath.ToSlash(filepath.Join(filepath.Dir(note.Path), deterministicShortID(newTitle)+".md"))
	}
	rewriteOperations := linkRewriteOperationsForTarget(notesBefore, note, targetRel, newTitle)
	target, err := safeJoin(root, targetRel)
	if err != nil {
		return errorProjection("note.rename", err), err
	}
	if targetRel != note.Path {
		if _, err := os.Stat(target); err == nil {
			err := &domain.CommandError{Code: "note_path_conflict", Message: "Target note path already exists", Hint: "Choose another title or move the existing file first"}
			return domain.NewErrorProjection("note.rename", err), err
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errorProjection("note.rename", err), err
		}
	}
	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, target, updated); err != nil {
		return errorProjection("note.rename", err), err
	}
	appendEventWarned(root, "note.rename", "success", map[string]string{"from": note.Path, "to": targetRel})
	projection := noteMutationProjection("note.rename", "Note renamed.", targetRel, meta)
	recordNote := domain.Note{ID: meta["note_id"], Title: newTitle, Path: targetRel, Body: strings.TrimSpace(strings.TrimPrefix(updated, renderFrontmatter(meta, "")))}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteRenamed, "note.rename:"+recordNote.ID+":"+targetRel, recordNote, note.Path)
	if recordErr != nil {
		return errorProjection("note.rename", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	if indexWasFresh {
		parsed := parseNote(targetRel, updated)
		if _, indexErr := noteindex.UpdateNote(root, noteindex.NoteUpdate{OldPath: note.Path, Note: parsed}); indexErr != nil {
			projection.Status = "partial"
			projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		}
	}
	attachLinkRewriteOperations(&projection, rewriteOperations, root)
	return projection, nil
}

func (s *Service) MoveNote(ctx context.Context, req NoteMutationRequest) (domain.Projection, error) {
	root, note, path, _, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.move", err), err
	}
	notesBefore, _ := scanNotes(root)
	_, indexErr := os.Stat(filepath.Join(root, ".pinax", "index.sqlite"))
	indexWasFresh := indexErr == nil
	dir, err := validateNoteDir(req.TargetDir)
	if err != nil {
		return errorProjection("note.move", err), err
	}
	oldPath := note.Path
	targetRel := filepath.ToSlash(filepath.Join(dir, filepath.Base(note.Path)))
	rewriteOperations := linkRewriteOperationsForTarget(notesBefore, note, targetRel, note.Title)
	if len(rewriteOperations) == 0 {
		rewriteOperations = indexedLinkRewriteOperations(root, note, targetRel, note.Title)
	}
	target, err := safeJoin(root, targetRel)
	if err != nil {
		return errorProjection("note.move", err), err
	}
	if _, err := os.Stat(target); err == nil {
		err := &domain.CommandError{Code: "note_path_conflict", Message: "Target note path already exists", Hint: "Choose another directory or move the existing file first"}
		return domain.NewErrorProjection("note.move", err), err
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errorProjection("note.move", err), err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return errorProjection("note.move", err), err
	}
	if err := os.Rename(path, target); err != nil {
		return errorProjection("note.move", err), err
	}
	appendEventWarned(root, "note.move", "success", map[string]string{"from": note.Path, "to": targetRel})
	projection := noteMutationProjection("note.move", "Note moved.", targetRel, map[string]string{"note_id": note.ID, "title": note.Title})
	note.Path = targetRel
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMoved, "note.move:"+note.ID+":"+oldPath+":"+targetRel, note, oldPath)
	if recordErr != nil {
		return errorProjection("note.move", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	if indexWasFresh {
		if _, indexErr := noteindex.UpdateNote(root, noteindex.NoteUpdate{OldPath: oldPath, Note: note}); indexErr != nil {
			projection.Status = "partial"
			projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		}
	}
	attachLinkRewriteOperations(&projection, rewriteOperations, root)
	return projection, nil
}

func (s *Service) ArchiveNote(ctx context.Context, req NoteMutationRequest) (domain.Projection, error) {
	root, note, path, content, meta, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.archive", err), err
	}
	meta["status"] = "archived"
	meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, path, updated); err != nil {
		return errorProjection("note.archive", err), err
	}
	appendEventWarned(root, "note.archive", "success", map[string]string{"path": note.Path})
	projection := noteMutationProjection("note.archive", "Note archived.", note.Path, meta)
	projection.Facts["status"] = "archived"
	note.Status = "archived"
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteArchived, "note.archive:"+note.ID+":"+note.Path, note, "")
	if recordErr != nil {
		return errorProjection("note.archive", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	return projection, nil
}

func (s *Service) DeleteNote(ctx context.Context, req NoteDeleteRequest) (domain.Projection, error) {
	root, note, path, _, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.delete", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "note delete requires --yes", Hint: "Add --yes after confirming; hard delete also requires --hard"}
		return domain.NewErrorProjection("note.delete", err), err
	}
	projection := domain.NewProjection("note.delete", "Note deleted.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	if req.Hard {
		if err := os.Remove(path); err != nil {
			return errorProjection("note.delete", err), err
		}
		appendEventWarned(root, "note.delete", "success", map[string]string{"path": note.Path, "hard": "true"})
		projection.Facts["hard"] = "true"
		recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteDeleted, "note.delete:"+note.ID+":"+note.Path, note, "")
		if recordErr != nil {
			return errorProjection("note.delete", recordErr), recordErr
		}
		applyRecordEventFacts(&projection, recordEvent)
		return finalizeNoteDeleteIndex(root, projection), nil
	}
	trashRel, err := uniqueTrashRel(root, note.Path, time.Now().UTC())
	if err != nil {
		return errorProjection("note.delete", err), err
	}
	trashPath, err := safeJoin(root, trashRel)
	if err != nil {
		return errorProjection("note.delete", err), err
	}
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		return errorProjection("note.delete", err), err
	}
	if err := os.Rename(path, trashPath); err != nil {
		return errorProjection("note.delete", err), err
	}
	appendEventWarned(root, "note.delete", "success", map[string]string{"path": note.Path, "trash_path": trashRel})
	projection.Summary = "Note moved to trash."
	projection.Facts["trash_path"] = trashRel
	projection.Data = map[string]any{"note": note, "trash_path": trashRel}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteTrashed, "note.trash:"+note.ID+":"+note.Path+":"+trashRel, note, note.Path, func(event *domain.RecordEvent) { event.TrashPath = trashRel })
	if recordErr != nil {
		return errorProjection("note.delete", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	return finalizeNoteDeleteIndex(root, projection), nil
}

func finalizeNoteDeleteIndex(root string, projection domain.Projection) domain.Projection {
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{
			Name:    "rebuild_index",
			Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root)),
		})
		return projection
	}
	projection.Facts["index_updated"] = "true"
	return projection
}

func (s *Service) TagNote(ctx context.Context, req NoteTagRequest) (domain.Projection, error) {
	requestTags, tagErr := normalizeTagsForWrite(req.Tags)
	if tagErr != nil {
		return domain.NewErrorProjection("note.tag", tagErr), tagErr
	}
	root, note, path, content, meta, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.tag", err), err
	}
	tags, tagErr := normalizeTagsForWrite(note.Tags)
	if tagErr != nil {
		return domain.NewErrorProjection("note.tag", tagErr), tagErr
	}
	switch req.Operation {
	case "add":
		tags = mergeTags(tags, requestTags)
	case "remove":
		tags = removeTags(tags, requestTags)
	case "set":
		tags = requestTags
	default:
		err := &domain.CommandError{Code: "invalid_tag_operation", Message: "Unknown tag operation", Hint: "Use add, remove, or set"}
		return domain.NewErrorProjection("note.tag", err), err
	}
	meta["tags"] = formatTags(tags)
	meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, path, updated); err != nil {
		return errorProjection("note.tag", err), err
	}
	appendEventWarned(root, "note.tag", "success", map[string]string{"path": note.Path, "operation": req.Operation})
	projection := noteMutationProjection("note.tag", "Note tags updated.", note.Path, meta)
	projection.Facts["tags"] = strings.Join(tags, ",")
	projection.Data = map[string]any{"note": domain.Note{ID: note.ID, Title: note.Title, Path: note.Path, Tags: tags, Project: note.Project, Status: meta["status"]}}
	recordNote := domain.Note{ID: note.ID, Title: note.Title, Path: note.Path, Tags: tags, Body: strings.TrimSpace(strings.TrimPrefix(updated, renderFrontmatter(meta, ""))), Project: note.Project, Status: meta["status"]}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, "note.tag:"+recordNote.ID+":"+req.Operation+":"+strings.Join(tags, ","), recordNote, "")
	if recordErr != nil {
		return errorProjection("note.tag", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	if err := applyRecordStateFacts(ctx, &projection, root, recordEvent.NoteID); err != nil {
		return errorProjection("note.tag", err), err
	}
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

// NoteVerify 追加一条 verified 事件到 note frontmatter（OKF 信任合同）。
//
// - 幂等：同 actor 同 UTC 日已存在事件时不重复追加，返回既有事件。
// - actor 必填（默认由 CLI 从配置 identity 解析，service 层再次 fail-closed）。
// - 写入走 YAML 节点级 atomic patch（commitNoteContent），正文与其余 frontmatter 不变。
func (s *Service) NoteVerify(ctx context.Context, req NoteVerifyRequest) (domain.Projection, error) {
	command := "note.verify"
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		err := &domain.CommandError{Code: "actor_required", Message: "note verify requires an actor and no identity is configured", Hint: "Configure `pinax config set identity <id>` or pass --actor human:<id> explicitly"}
		return domain.NewErrorProjection(command, err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	note, err := resolveNoteRef(notes, req.NoteRef)
	if err != nil {
		return errorProjection(command, err), err
	}
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return errorProjection(command, err), err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return errorProjection(command, err), err
	}
	signals, err := domain.ParseTrustSignals(content)
	if err != nil {
		commandErr := &domain.CommandError{Code: "trust_field_invalid", Message: fmt.Sprintf("%s in %s", err.Error(), note.Path), Hint: "Fix the existing trust timestamp to ISO8601 with UTC offset before verifying"}
		return domain.NewErrorProjection(command, commandErr), commandErr
	}
	now := s.now()
	if existing, ok := signals.FindVerifiedEventSameDay(actor, now); ok {
		projection := domain.NewProjection(command, "Verification event already recorded today.")
		projection.Facts["path"] = note.Path
		projection.Facts["note_id"] = note.ID
		projection.Facts["actor"] = actor
		projection.Facts["at"] = existing.At
		projection.Facts["idempotent"] = "true"
		projection.Facts["trust"] = domain.TrustTierOf(&signals)
		projection.Facts["writes"] = "false"
		projection.Data = map[string]any{"note": noteGraphNoteSummary(note), "event": existing, "idempotent": true}
		return projection, nil
	}
	event := domain.TrustActorEvent{By: actor, At: now.UTC().Format(time.RFC3339), Note: strings.TrimSpace(req.Note)}
	updated, err := domain.AppendTrustVerifiedEvent(content, event)
	if err != nil {
		return errorProjection(command, err), err
	}
	if err := commitNoteContent(path, path, string(updated)); err != nil {
		return errorProjection(command, err), err
	}
	updatedSignals, err := domain.ParseTrustSignals(updated)
	if err != nil {
		return errorProjection(command, err), err
	}
	note.Trust = &updatedSignals
	appendEventWarned(root, command, "success", map[string]string{"path": note.Path, "actor": actor})
	projection := domain.NewProjection(command, "Verification event appended.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["actor"] = actor
	projection.Facts["at"] = event.At
	projection.Facts["idempotent"] = "false"
	projection.Facts["trust"] = domain.TrustTierOf(&updatedSignals)
	projection.Evidence = []string{note.Path}
	parsed := parseNote(note.Path, string(updated))
	parsed.Trust = &updatedSignals
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, "note.verify:"+parsed.ID+":"+actor+":"+now.UTC().Format("2006-01-02"), parsed, "")
	if recordErr != nil {
		return errorProjection(command, recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	if err := applyRecordStateFacts(ctx, &projection, root, recordEvent.NoteID); err != nil {
		return errorProjection(command, err), err
	}
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	projection.Data = map[string]any{"note": noteGraphNoteSummary(note), "event": event, "idempotent": false}
	return projection, nil
}

func (s *Service) PatchNoteProperty(ctx context.Context, req NotePropertyRequest) (domain.Projection, error) {
	key, keyErr := normalizePropertyKey(req.Key)
	if keyErr != nil {
		return domain.NewErrorProjection("note.property", keyErr), keyErr
	}
	operation := strings.TrimSpace(req.Operation)
	root, note, path, content, meta, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.property", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	fields := map[string]string{"updated_at": now}
	removed := []string{}
	value := ""
	summary := "Note property updated."
	switch operation {
	case "set":
		formatted, valueErr := formatPropertyValue(req.Value)
		if valueErr != nil {
			return domain.NewErrorProjection("note.property", valueErr), valueErr
		}
		fields[key] = formatted
		meta[key] = formatted
		value = strings.Trim(strings.TrimSpace(formatted), "\"")
	case "remove":
		removed = []string{key}
		delete(meta, key)
		summary = "Note property removed."
	default:
		err := &domain.CommandError{Code: "invalid_property_operation", Message: "Unknown property operation", Hint: "Use set or remove"}
		return domain.NewErrorProjection("note.property", err), err
	}
	meta["updated_at"] = now
	updated, _ := patchFrontmatterFieldsRemoving(content, fields, removed)
	if err := commitNoteContent(path, path, updated); err != nil {
		return errorProjection("note.property", err), err
	}
	appendEventWarned(root, "note.property", "success", map[string]string{"path": note.Path, "operation": operation, "property": key})
	projection := noteMutationProjection("note.property", summary, note.Path, meta)
	projection.Facts["operation"] = operation
	projection.Facts["property"] = key
	if value != "" {
		projection.Facts["value"] = value
	}
	parsed := parseNote(note.Path, updated)
	projection.Data = map[string]any{"note": parsed, "frontmatter": meta, "property": key, "operation": operation, "value": value}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, "note.property:"+parsed.ID+":"+operation+":"+key+":"+value, parsed, "")
	if recordErr != nil {
		return errorProjection("note.property", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	if err := applyRecordStateFacts(ctx, &projection, root, recordEvent.NoteID); err != nil {
		return errorProjection("note.property", err), err
	}
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func (s *Service) BulkTag(ctx context.Context, req NoteTagBulkRequest) (domain.Projection, error) {
	oldTags, tagErr := normalizeTagsForWrite([]string{req.OldTag})
	if tagErr != nil {
		return domain.NewErrorProjection("tag."+req.Operation, tagErr), tagErr
	}
	if len(oldTags) != 1 {
		err := &domain.CommandError{Code: "invalid_tag", Message: "A tag is required", Hint: "pinax note tags rename <old> <new> --vault <vault> --yes"}
		return domain.NewErrorProjection("tag."+req.Operation, err), err
	}
	oldTag := oldTags[0]
	newTag := ""
	if req.Operation == "rename" {
		newTags, tagErr := normalizeTagsForWrite([]string{req.NewTag})
		if tagErr != nil {
			return domain.NewErrorProjection("tag.rename", tagErr), tagErr
		}
		if len(newTags) != 1 {
			err := &domain.CommandError{Code: "invalid_tag", Message: "rename requires a new tag", Hint: "pinax note tags rename <old> <new> --vault <vault> --yes"}
			return domain.NewErrorProjection("tag.rename", err), err
		}
		newTag = newTags[0]
	}
	command := "tag." + req.Operation
	if req.Operation != "rename" && req.Operation != "delete" {
		err := &domain.CommandError{Code: "invalid_tag_operation", Message: "Unknown tag batch operation", Hint: "Use rename or delete"}
		return domain.NewErrorProjection(command, err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if !req.DryRun && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "Batch tag writes require --yes", Hint: "Add --dry-run to preview first, then add --yes after confirming"}
		return domain.NewErrorProjection(command, err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	changed := make([]domain.Note, 0)
	for _, note := range notes {
		if !containsString(cleanTags(note.Tags), oldTag) {
			continue
		}
		updatedTags := removeTags(note.Tags, []string{oldTag})
		if req.Operation == "rename" {
			updatedTags = mergeTags(updatedTags, []string{newTag})
		}
		note.Tags = updatedTags
		changed = append(changed, note)
	}
	projection := domain.NewProjection(command, "Tag batch plan generated.")
	projection.Facts["old_tag"] = oldTag
	if newTag != "" {
		projection.Facts["new_tag"] = newTag
	}
	projection.Facts["matched"] = fmt.Sprint(len(changed))
	projection.Facts["changed"] = fmt.Sprint(len(changed))
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["writes"] = fmt.Sprint(!req.DryRun)
	projection.Data = map[string]any{"notes": changed, "old_tag": oldTag, "new_tag": newTag, "operation": req.Operation, "dry_run": req.DryRun}
	if req.DryRun || len(changed) == 0 {
		return projection, nil
	}
	projection.Summary = "Tag batch update applied."
	recordEvents := 0
	for _, note := range changed {
		_, current, path, content, meta, err := loadMutableResolvedNote(root, note)
		if err != nil {
			return errorProjection(command, err), err
		}
		meta["tags"] = formatTags(note.Tags)
		meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		updated, _ := patchFrontmatterFields(content, meta)
		if err := commitNoteContent(path, path, updated); err != nil {
			return errorProjection(command, err), err
		}
		parsed := parseNote(current.Path, updated)
		if _, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, command+":"+parsed.ID+":"+oldTag+":"+newTag, parsed, ""); err != nil {
			return errorProjection(command, err), err
		}
		recordEvents++
	}
	appendEventWarned(root, command, "success", map[string]string{"old_tag": oldTag, "new_tag": newTag, "changed": fmt.Sprint(len(changed))})
	projection.Facts["record_events"] = fmt.Sprint(recordEvents)
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func (s *Service) BulkFolder(ctx context.Context, req NoteFolderBulkRequest) (domain.Projection, error) {
	command := "folder." + strings.TrimSpace(req.Operation)
	if command == "folder." {
		command = "folder.rename"
	}
	if strings.TrimSpace(req.Operation) != "rename" {
		err := &domain.CommandError{Code: "invalid_folder_operation", Message: "Unknown folder batch operation", Hint: "Use rename"}
		return domain.NewErrorProjection(command, err), err
	}
	oldFolder, folderErr := validateRequiredNoteFolder(req.OldFolder, "old")
	if folderErr != nil {
		return domain.NewErrorProjection(command, folderErr), folderErr
	}
	newFolder, folderErr := validateRequiredNoteFolder(req.NewFolder, "new")
	if folderErr != nil {
		return domain.NewErrorProjection(command, folderErr), folderErr
	}
	if oldFolder == newFolder {
		err := &domain.CommandError{Code: "invalid_folder", Message: "Old and new folders cannot be the same", Hint: "Provide a different target folder, or use pinax note folders list to view existing folders"}
		return domain.NewErrorProjection(command, err), err
	}
	if !req.DryRun && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "Batch folder writes require --yes", Hint: "Preview first with pinax note folders rename <old> <new> --dry-run --vault <vault> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	type folderChange struct {
		Note       domain.Note `json:"note"`
		OldPath    string      `json:"old_path"`
		TargetPath string      `json:"target_path"`
		OldFolder  string      `json:"old_folder"`
		NewFolder  string      `json:"new_folder"`
	}
	changes := []folderChange{}
	seenTargets := map[string]string{}
	for _, note := range notes {
		if !noteHasFolder(note, oldFolder) {
			continue
		}
		targetRel := folderRenameTargetPath(note, oldFolder, newFolder)
		change := folderChange{Note: note, OldPath: note.Path, TargetPath: targetRel, OldFolder: oldFolder, NewFolder: newFolder}
		changes = append(changes, change)
		if previous := seenTargets[targetRel]; previous != "" && previous != note.Path {
			err := &domain.CommandError{Code: "note_path_conflict", Message: "Multiple notes would be written to the same target path", Hint: "Rename conflicting notes first or choose a more specific folder"}
			return domain.NewErrorProjection(command, err), err
		}
		seenTargets[targetRel] = note.Path
	}
	for _, change := range changes {
		if change.TargetPath == change.OldPath {
			continue
		}
		target, err := safeJoin(root, change.TargetPath)
		if err != nil {
			return errorProjection(command, err), err
		}
		if _, err := os.Stat(target); err == nil {
			err := &domain.CommandError{Code: "note_path_conflict", Message: "Target note path already exists", Hint: "Move or rename the same-named note in the target directory first"}
			return domain.NewErrorProjection(command, err), err
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errorProjection(command, err), err
		}
	}
	projection := domain.NewProjection(command, "Folder batch rename plan generated.")
	projection.Facts["old_folder"] = oldFolder
	projection.Facts["new_folder"] = newFolder
	projection.Facts["matched"] = fmt.Sprint(len(changes))
	projection.Facts["changed"] = fmt.Sprint(len(changes))
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["writes"] = fmt.Sprint(!req.DryRun)
	projection.Facts["requires_snapshot"] = "true"
	projection.Data = map[string]any{"changes": changes, "old_folder": oldFolder, "new_folder": newFolder, "operation": "rename", "dry_run": req.DryRun}
	if req.DryRun || len(changes) == 0 {
		return projection, nil
	}
	projection.Summary = "Folder batch rename applied."
	recordEvents := 0
	for _, change := range changes {
		_, note, path, content, meta, err := loadMutableResolvedNote(root, change.Note)
		if err != nil {
			return errorProjection(command, err), err
		}
		meta["folder"] = newFolder
		meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		updated, _ := patchFrontmatterFields(content, meta)
		target, err := safeJoin(root, change.TargetPath)
		if err != nil {
			return errorProjection(command, err), err
		}
		if err := commitNoteContent(path, target, updated); err != nil {
			return errorProjection(command, err), err
		}
		parsed := parseNote(change.TargetPath, updated)
		parsed.Path = change.TargetPath
		if parsed.ID == "" {
			parsed.ID = note.ID
		}
		if _, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMoved, command+":"+parsed.ID+":"+oldFolder+":"+newFolder+":"+change.TargetPath, parsed, change.OldPath); err != nil {
			return errorProjection(command, err), err
		}
		recordEvents++
	}
	appendEventWarned(root, command, "success", map[string]string{"old_folder": oldFolder, "new_folder": newFolder, "changed": fmt.Sprint(len(changes))})
	projection.Facts["record_events"] = fmt.Sprint(recordEvents)
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func validateRequiredNoteFolder(raw, label string) (string, *domain.CommandError) {
	folder, err := validateOptionalNoteFolder(raw)
	if err != nil {
		if commandErr, ok := err.(*domain.CommandError); ok {
			return "", commandErr
		}
		return "", &domain.CommandError{Code: "invalid_folder", Message: err.Error(), Hint: "Use a folder like inbox, reference, or work/research"}
	}
	if folder == "" {
		return "", &domain.CommandError{Code: "invalid_folder", Message: label + " folder cannot be empty", Hint: "pinax note folders rename <old> <new> --vault <vault> --dry-run"}
	}
	return folder, nil
}

func noteHasFolder(note domain.Note, folder string) bool {
	for _, value := range noteDimensionValues(note, "folder") {
		if value == folder {
			return true
		}
	}
	return false
}

func folderRenameTargetPath(note domain.Note, oldFolder, newFolder string) string {
	dir := filepath.ToSlash(filepath.Dir(note.Path))
	if dir == "." {
		dir = ""
	}
	base := filepath.Base(note.Path)
	if dir == oldFolder {
		return filepath.ToSlash(filepath.Join(newFolder, base))
	}
	if strings.HasPrefix(dir, "notes/") && strings.TrimPrefix(dir, "notes/") == oldFolder {
		return filepath.ToSlash(filepath.Join("notes", newFolder, base))
	}
	if strings.HasSuffix(dir, "/"+oldFolder) {
		prefix := strings.TrimSuffix(dir, oldFolder)
		return filepath.ToSlash(filepath.Join(prefix, newFolder, base))
	}
	return filepath.ToSlash(filepath.Join(newFolder, base))
}

// Tag and frontmatter property normalization helpers used across note mutations.
func cleanTags(tags []string) []string {
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

func splitCommaValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func normalizeTagsForWrite(tags []string) ([]string, *domain.CommandError) {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(tags))
	for _, raw := range tags {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		tag := strings.TrimPrefix(trimmed, "#")
		if tag == "" {
			return nil, invalidTagError()
		}
		if !isSafeTagValue(tag) {
			return nil, invalidTagError()
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		cleaned = append(cleaned, tag)
	}
	return cleaned, nil
}

func isSafeTagValue(tag string) bool {
	for _, r := range tag {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			continue
		}
		switch r {
		case '_', '-', '/':
			continue
		default:
			return false
		}
	}
	return true
}

func invalidTagError() *domain.CommandError {
	return &domain.CommandError{Code: "invalid_tag", Message: "tag may only contain letters, numbers, CJK characters, _, -, or /, and cannot contain YAML structural characters, commas, whitespace, or control characters", Hint: "For example, pinax note tag add <note> research/work --vault <vault>"}
}

func normalizePropertyKey(raw string) (string, *domain.CommandError) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", invalidPropertyKeyError()
	}
	blocked := map[string]string{
		"schema_version": "schema_version is managed by Pinax",
		"note_id":        "note_id is managed by the record ledger",
		"tags":           "Use pinax note tag add|remove|set for tags",
		"title":          "Use pinax note rename for title",
		"created_at":     "created_at is maintained by Pinax at creation time",
		"updated_at":     "updated_at is maintained by Pinax write commands",
	}
	if reason := blocked[key]; reason != "" {
		return "", &domain.CommandError{Code: "reserved_property", Message: "Reserved property cannot be modified through note property: " + key, Hint: reason}
	}
	for i, r := range key {
		if i == 0 && r != '_' && !unicode.IsLetter(r) {
			return "", invalidPropertyKeyError()
		}
		if r != '_' && r != '-' && r != '.' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return "", invalidPropertyKeyError()
		}
	}
	return key, nil
}

func invalidPropertyKeyError() *domain.CommandError {
	return &domain.CommandError{Code: "invalid_property", Message: "property key may only contain letters, numbers, _, -, or ., and cannot start with a number or symbol", Hint: "For example, pinax note property set <note> priority 2 --vault <vault>"}
}

func formatPropertyValue(raw string) (string, *domain.CommandError) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", &domain.CommandError{Code: "invalid_property_value", Message: "property value cannot be empty", Hint: "To delete a property, use pinax note property remove <note> <property> --vault <vault>"}
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || unicode.IsControl(r) {
			return "", &domain.CommandError{Code: "invalid_property_value", Message: "property value cannot contain newlines or control characters", Hint: "Use a single-line scalar value; put complex content in the body"}
		}
	}
	if propertyValueNeedsQuote(value) {
		return quoteFrontmatterValue(value), nil
	}
	return value, nil
}

func propertyValueNeedsQuote(value string) bool {
	if strings.TrimSpace(value) != value {
		return true
	}
	return strings.ContainsAny(value, ":#[]{}&*!|>'\"%@`,")
}

func quoteFrontmatterValue(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
	return "\"" + replacer.Replace(value) + "\""
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	return "[" + strings.Join(tags, ", ") + "]"
}

func linkRewriteOperationsForTarget(notes []domain.Note, target domain.Note, newPath, newTitle string) []domain.PlanOperation {
	_, incoming := BuildEnhancedLinkGraph(notes)
	links := incoming[target.Path]
	operations := make([]domain.PlanOperation, 0, len(links))
	for _, link := range links {
		operations = append(operations, domain.PlanOperation{Kind: "link_rewrite", Path: link.SourcePath, Target: newPath, Reason: "Review Markdown link text after target rename or move; object edge remains stable.", Status: "manual_review", Evidence: []string{"source_object_id=" + link.SourceObjectID, "target_object_id=" + target.ID, "old_target=" + link.TargetRaw, "new_title=" + newTitle}})
	}
	return operations
}

func indexedLinkRewriteOperations(root string, target domain.Note, newPath, newTitle string) []domain.PlanOperation {
	rows, err := noteindex.LinksByTargetObjectID(root, target.ID)
	if err != nil {
		return nil
	}
	operations := make([]domain.PlanOperation, 0, len(rows))
	for _, row := range rows {
		operations = append(operations, domain.PlanOperation{Kind: "link_rewrite", Path: row.NotePath, Target: newPath, Reason: "Review Markdown link text after target rename or move; object edge remains stable.", Status: "manual_review", Evidence: []string{"source_object_id=" + row.SourceObjectID, "target_object_id=" + target.ID, "old_target=" + row.TargetRaw, "new_title=" + newTitle}})
	}
	return operations
}

func attachLinkRewriteOperations(projection *domain.Projection, operations []domain.PlanOperation, root string) {
	projection.Facts["link_rewrite_operations"] = fmt.Sprint(len(operations))
	data, ok := projection.Data.(map[string]any)
	if !ok || data == nil {
		data = map[string]any{}
	}
	data["link_rewrite_operations"] = operations
	projection.Data = data
	if len(operations) > 0 {
		projection.Actions = append(projection.Actions, domain.Action{Name: "review_links", Command: fmt.Sprintf("pinax note links --all --vault %s --json", shellQuote(root))})
	}
}
