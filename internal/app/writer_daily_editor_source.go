package app

import (
	"context"

	"github.com/yeisme/pinax/internal/domain"
)

type WriterDailyEditorSourceRequest struct {
	Source   WriterEditorSourceRequest
	Date     string
	Timezone string
}

// WriterDailyEditorSource only reads source bytes. It does not authorize a
// mutation, create a proposal or make ordinary system-page writes available.
func (s *Service) WriterDailyEditorSource(ctx context.Context, req WriterDailyEditorSourceRequest) (WriterEditorSource, error) {
	var empty WriterEditorSource
	source := req.Source
	if !source.Session.HasScope(WriterScopeEditor) || !source.Session.HasScope(ReaderScopeBody) || !source.Session.HasScope(ReaderScopeMetadata) {
		return empty, collaborationError("insufficient_scope", "Daily editor source requires writer.editor, reader.body and reader.metadata")
	}
	daily := ReaderDailyRequest{VaultPath: source.VaultPath, Session: source.Session, Date: req.Date, Timezone: req.Timezone}
	before, err := s.ReaderDaily(ctx, daily)
	if err != nil {
		return empty, err
	}
	if before.Note == nil || before.Note.ObjectID != source.ObjectID {
		return empty, collaborationError("note_not_found", "The requested daily object is not visible")
	}
	if before.Note.Revision != source.Revision {
		return empty, collaborationError("revision_conflict", "The daily revision changed; reopen the original object")
	}
	result, err := s.writerEditorSourceSelected(ctx, source, func(note domain.Note) bool { return note.Kind == "daily" && !isSystemIndexNote(note) })
	if err != nil {
		return empty, err
	}
	after, err := s.ReaderDaily(ctx, daily)
	if err != nil {
		return empty, err
	}
	if after.Note == nil || after.Note.ObjectID != source.ObjectID || after.Note.Revision != source.Revision || after.Watermark != before.Watermark {
		return empty, collaborationError("revision_conflict", "Daily identity or source changed during reading")
	}
	return result, nil
}
