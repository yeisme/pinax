package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	gitstore "github.com/yeisme/pinax/internal/git"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/records"
)

type RecordRequest struct {
	VaultPath string
	NoteRef   string
	Plan      bool
}

func (s *Service) RecordInit(ctx context.Context, req RecordRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("record.init", err), err
	}
	if err := records.NewService(root).Init(ctx); err != nil {
		return errorProjection("record.init", err), err
	}
	projection := domain.NewProjection("record.init", "Record ledger initialized.")
	projection.Facts["records_path"] = ".pinax/records"
	return projection, nil
}

func (s *Service) RecordStatus(ctx context.Context, req RecordRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("record.status", err), err
	}
	state, err := records.NewService(root).Replay(ctx)
	if err != nil {
		return errorProjection("record.status", err), err
	}
	projection := domain.NewProjection("record.status", "Record ledger status read.")
	projection.Facts["records"] = fmt.Sprint(len(state.Records))
	projection.Facts["tombstones"] = fmt.Sprint(len(state.Tombstones))
	projection.Facts["ledger_seq"] = fmt.Sprint(state.Version.LastSeq)
	projection.Facts["schema_version"] = state.SchemaVersion
	projection.Data = state
	return projection, nil
}

func (s *Service) RecordAdopt(ctx context.Context, req RecordRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("record.adopt", err), err
	}
	notes, err := scanNotes(root)
	notes = ordinaryNotes(notes)
	if err != nil {
		return errorProjection("record.adopt", err), err
	}
	if req.Plan {
		status, _ := noteindex.Inspect(root, notes)
		candidates, err := adoptableMarkdownCandidates(root, notes, strings.TrimSpace(req.NoteRef), status.Status)
		if err != nil {
			return errorProjection("record.adopt", err), err
		}
		if strings.TrimSpace(req.NoteRef) != "" && len(candidates) > 1 {
			err := &domain.CommandError{Code: domain.ErrorCodeVaultObjectRefAmbiguous, Message: "record adopt query matched multiple candidates", Hint: "Retry with a more specific filename or full path"}
			projection := domain.NewErrorProjection("record.adopt", err)
			projection.Data = map[string]any{"candidates": candidates}
			projection.Facts["candidates"] = fmt.Sprint(len(candidates))
			return projection, err
		}
		projection := domain.NewProjection("record.adopt", "Record adoption plan generated.")
		projection.Facts["writes"] = "false"
		projection.Facts["adopted"] = "0"
		projection.Facts["candidates"] = fmt.Sprint(len(candidates))
		projection.Facts["operations"] = fmt.Sprint(len(candidates))
		projection.Facts["index_status"] = status.Status
		if len(candidates) > 0 {
			cmd := "pinax record adopt"
			if strings.TrimSpace(req.NoteRef) != "" {
				cmd += " " + shellQuote(req.NoteRef)
			}
			projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("%s --vault %s", cmd, shellQuote(root))}}
		}
		projection.Data = map[string]any{"operations": candidates, "candidates": candidates}
		return projection, nil
	}
	svc := records.NewService(root)
	if err := svc.Init(ctx); err != nil {
		return errorProjection("record.adopt", err), err
	}
	state, err := svc.Replay(ctx)
	if err != nil {
		return errorProjection("record.adopt", err), err
	}
	adopted := 0
	for _, note := range notes {
		noteID := strings.TrimSpace(note.ID)
		if noteID == "" {
			noteID = stableNoteID(note.Path)
		}
		if _, exists := state.Records[noteID]; exists {
			continue
		}
		event := domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "adopt:" + noteID + ":" + note.Path, NoteID: noteID, Path: note.Path, Title: note.Title, ContentRevision: domain.ContentRevision{Hash: hashString(note.Title + "\x00" + note.Body), Size: int64(len(note.Body))}, VersionEvidence: gitstore.Evidence(ctx, root, note.Path), Evidence: []string{"source=adopt"}}
		if _, err := svc.AppendEvent(ctx, event); err != nil {
			return errorProjection("record.adopt", err), err
		}
		adopted++
	}
	state, err = svc.Replay(ctx)
	if err != nil {
		return errorProjection("record.adopt", err), err
	}
	projection := domain.NewProjection("record.adopt", "Record ledger adoption completed.")
	projection.Facts["adopted"] = fmt.Sprint(adopted)
	projection.Facts["records"] = fmt.Sprint(len(state.Records))
	projection.Facts["ledger_seq"] = fmt.Sprint(state.Version.LastSeq)
	projection.Data = state
	return projection, nil
}

func (s *Service) RecordHistory(ctx context.Context, req RecordRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("record.history", err), err
	}
	state, err := records.NewService(root).Replay(ctx)
	if err != nil {
		return errorProjection("record.history", err), err
	}
	key := strings.TrimSpace(req.NoteRef)
	if key == "" {
		err := &domain.CommandError{Code: "record_ref_required", Message: "record history requires a note query", Hint: "pinax record history <query> --vault <vault>"}
		return domain.NewErrorProjection("record.history", err), err
	}

	lookupKeys := []string{key}
	resolverResult, err := s.ResolveVaultObject(ctx, ResolverRequest{VaultPath: root, Query: key, Scope: "registered", Kind: "note"})
	if err != nil {
		return errorProjection("record.history", err), err
	}
	if len(resolverResult.Candidates) > 1 {
		err := &domain.CommandError{Code: domain.ErrorCodeVaultObjectRefAmbiguous, Message: "record history query matched multiple candidates", Hint: "Retry with a more specific note_id, filename, or full path"}
		return resolverWriteGuardErrorProjection("record.history", resolverResult, err), err
	}
	if len(resolverResult.Candidates) == 1 {
		candidate := resolverResult.Candidates[0]
		for _, value := range []string{candidate.NoteID, candidate.Path, candidate.Title} {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			seen := false
			for _, existing := range lookupKeys {
				if existing == value {
					seen = true
					break
				}
			}
			if !seen {
				lookupKeys = append(lookupKeys, value)
			}
		}
	}

	record := domain.NoteRecord{}
	ok := false
	for _, lookup := range lookupKeys {
		if candidate, exists := state.Records[lookup]; exists {
			record = candidate
			ok = true
			break
		}
	}
	if !ok {
		for _, candidate := range state.Records {
			for _, lookup := range lookupKeys {
				if candidate.Path == lookup || candidate.Title == lookup {
					record = candidate
					ok = true
					break
				}
			}
			if ok {
				break
			}
		}
	}
	if !ok {
		err := &domain.CommandError{Code: "record_not_found", Message: "Record not found", Hint: "Run pinax record adopt --vault <vault>"}
		return domain.NewErrorProjection("record.history", err), err
	}

	projection := domain.NewProjection("record.history", "Record history read.")
	projection.Facts["note_id"] = record.NoteID
	projection.Facts["path"] = record.Path
	projection.Facts["lifecycle"] = string(record.Lifecycle)
	projection.Facts["record_version"] = fmt.Sprint(record.RecordVersion)
	projection.Facts["ledger_seq"] = fmt.Sprint(record.LedgerSeq)
	projection.Facts["candidates"] = fmt.Sprint(len(resolverResult.Candidates))
	projection.Facts["index_status"] = resolverResult.Facts.IndexStatus
	if resolverResult.Facts.MatchField != "" {
		projection.Facts["match_field"] = resolverResult.Facts.MatchField
	}
	projection.Data = record
	return projection, nil
}

func appendNoteRecordEvent(ctx context.Context, root string, kind domain.RecordEventKind, idempotency string, note domain.Note, oldPath string) (domain.RecordEvent, error) {
	noteID := strings.TrimSpace(note.ID)
	if noteID == "" {
		noteID = stableNoteID(note.Path)
	}
	event := domain.RecordEvent{Kind: kind, IdempotencyKey: idempotency, NoteID: noteID, Path: note.Path, OldPath: oldPath, Title: note.Title, ContentRevision: domain.ContentRevision{Hash: hashString(note.Title + "\x00" + note.Body), Size: int64(len(note.Body))}, VersionEvidence: gitstore.Evidence(ctx, root, note.Path), Evidence: []string{"source=" + string(kind)}}
	svc := records.NewService(root)
	created, err := svc.AppendEvent(ctx, event)
	if err == nil || kind == domain.RecordEventNoteCreated || domain.ErrorCode(err) != "record_lifecycle_invalid" {
		return created, err
	}
	ensure := domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "record.ensure:" + noteID + ":" + note.Path, NoteID: noteID, Path: note.Path, Title: note.Title, ContentRevision: event.ContentRevision, VersionEvidence: event.VersionEvidence, Evidence: []string{"source=record.ensure"}}
	if _, ensureErr := svc.AppendEvent(ctx, ensure); ensureErr != nil && domain.ErrorCode(ensureErr) != "" {
		return domain.RecordEvent{}, ensureErr
	}
	return svc.AppendEvent(ctx, event)
}

func applyRecordEventFacts(projection *domain.Projection, event domain.RecordEvent) {
	if projection == nil || event.Seq == 0 {
		return
	}
	projection.Facts["ledger_status"] = "updated"
	projection.Facts["ledger_seq"] = fmt.Sprint(event.Seq)
	projection.Facts["record_event"] = string(event.Kind)
	projection.Facts["record_event_id"] = event.EventID
	projection.Facts["version_backend"] = recordDefaultString(event.VersionEvidence.Backend, "none")
	projection.Facts["worktree_state"] = recordDefaultString(event.VersionEvidence.WorktreeState, "unknown")
}

func applyRecordStateFacts(ctx context.Context, projection *domain.Projection, root, noteID string) error {
	if projection == nil || strings.TrimSpace(noteID) == "" {
		return nil
	}
	state, err := records.NewService(root).Replay(ctx)
	if err != nil {
		return err
	}
	record, ok := state.Records[noteID]
	if !ok {
		return nil
	}
	projection.Facts["record_version"] = fmt.Sprint(record.RecordVersion)
	return nil
}

func recordDefaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
