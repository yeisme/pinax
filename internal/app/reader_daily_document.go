package app

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// ReaderDailyDocumentRequest is an owner-only service contract until a
// dedicated transport binding is registered. No caller-controlled path or
// allow-system flag is accepted by the ordinary document API.
type ReaderDailyDocumentRequest struct {
	Document ReaderDocumentRequest
	Date     string
	Timezone string
}

func (s *Service) ReaderDailyDocument(ctx context.Context, req ReaderDailyDocumentRequest) (domain.Projection, error) {
	const command = "reader.daily.document"
	fail := func(err error) (domain.Projection, error) { return errorProjection(command, err), err }
	doc := req.Document
	if !doc.Session.HasScope(ReaderScopeBody) || !doc.Session.HasScope(ReaderScopeMetadata) {
		return fail(ReaderError("insufficient_scope", "Daily document requires reader.body and reader.metadata", ""))
	}
	hash, err := hex.DecodeString(strings.TrimPrefix(doc.Revision, "sha256:"))
	if err != nil || len(hash) != 32 || !strings.HasPrefix(doc.Revision, "sha256:") || doc.ObjectID == "" || (doc.Cursor != "" && doc.Anchor != "") {
		return fail(ReaderError("reader_request_invalid", "Daily document requires an exact object and revision", ""))
	}
	daily := ReaderDailyRequest{VaultPath: doc.VaultPath, Session: doc.Session, Date: req.Date, Timezone: req.Timezone}
	before, err := s.ReaderDaily(ctx, daily)
	if err != nil {
		return fail(err)
	}
	if before.Note == nil || before.Note.ObjectID != doc.ObjectID {
		return fail(ReaderError("reader_object_not_found", "The requested daily object is not visible", ""))
	}
	// This lane deliberately serves only the current fixed observation. An old
	// body must not be silently substituted after the daily locator changes.
	if before.Note.Revision != doc.Revision {
		return fail(ReaderError("revision_unavailable", "The daily revision changed; reopen the original object", ""))
	}
	root, err := cleanVaultPath(doc.VaultPath)
	if err != nil {
		return fail(err)
	}
	notes, _, _, err := navigationSnapshot(ctx, root, true, false)
	if err != nil {
		return fail(err)
	}
	note, found := findNoteByObjectID(notes, doc.ObjectID)
	if !found || note.Kind != "daily" || isSystemIndexNote(note) {
		return fail(ReaderError("reader_object_not_found", "The daily object is unavailable", ""))
	}
	if readerNoteRevision(note) != doc.Revision {
		return fail(ReaderError("revision_unavailable", "The daily source changed while opening", ""))
	}
	projection, err := s.readerDocumentSelected(ctx, doc, root, note, ordinaryNotes(notes))
	if err != nil {
		// The ordinary renderer may suggest its source-export route. That route
		// does not grant daily access; do not advertise it as a recovery bypass.
		projection.Actions = nil
		projection.Command = command
		return projection, err
	}
	after, err := s.ReaderDaily(ctx, daily)
	if err != nil {
		return fail(err)
	}
	if after.Note == nil || after.Note.ObjectID != doc.ObjectID || after.Note.Revision != doc.Revision || after.Watermark != before.Watermark {
		return fail(ReaderError("source_changed", "Daily identity or source changed during reading", ""))
	}
	projection.Command = command
	return projection, nil
}
