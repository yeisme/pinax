package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"

	"github.com/yeisme/pinax/internal/operation"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

const writerDailyEditCapability = "writer.daily.edit"

// These owner service types are not yet HTTP contracts. Bodies remain in
// memory; only digests and the shared operation receipt enter the ledger.
type WriterDailyEditRequest struct {
	Daily    WriterDailyRequest
	ObjectID string
	Revision string
	Body     string
}

type WriterDailyEditProposal struct {
	SchemaVersion string               `json:"schema_version"`
	Date          string               `json:"date"`
	Timezone      string               `json:"timezone"`
	ObjectID      string               `json:"object_id"`
	Preview       CollaborationPreview `json:"preview"`
	Digest        string               `json:"preview_digest"`
}

func dailyEditDigest(root string, req WriterDailyRequest, p WriterDailyEditProposal) string {
	p.Digest = ""
	encoded, _ := json.Marshal([]any{"pinax.writer.daily.edit.v1", root, req.Session.SessionID, req.Session.BindingID, req.Session.scopeDigest(), p})
	mac := hmac.New(sha256.New, req.PreviewSecret)
	_, _ = mac.Write(encoded)
	return "sha256:" + hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) WriterDailyEditPropose(ctx context.Context, req WriterDailyEditRequest) (WriterDailyEditProposal, error) {
	var empty WriterDailyEditProposal
	if err := dailyWriterGate(req.Daily); err != nil {
		return empty, err
	}
	sourceReq := WriterDailyEditorSourceRequest{Source: WriterEditorSourceRequest{VaultPath: req.Daily.VaultPath, Session: req.Daily.Session, ObjectID: req.ObjectID, Revision: req.Revision}, Date: req.Daily.Date, Timezone: req.Daily.Timezone}
	source, err := s.WriterDailyEditorSource(ctx, sourceReq)
	if err != nil {
		return empty, err
	}
	if !source.Editable {
		return empty, collaborationError("frontmatter_unsupported", "Daily frontmatter cannot be edited losslessly")
	}
	root, err := cleanVaultPath(req.Daily.VaultPath)
	if err != nil {
		return empty, err
	}
	// Preview must not open the mutable resolver's index connection. Resolve
	// the already-authorized identity through the read-only source scan.
	notes, _, _, err := navigationSnapshot(ctx, root, true, false)
	if err != nil {
		return empty, err
	}
	note, found := findNoteByObjectID(notes, req.ObjectID)
	if !found || note.Kind != "daily" || readerNoteRevision(note) != req.Revision {
		return empty, collaborationError("revision_conflict", "Daily source changed during preview")
	}
	change := CollaborationChange{OperationID: req.Daily.OperationID, Action: "replace", NoteRef: note.Path, ExpectedRevision: req.Revision, Body: req.Body}
	if err := validateCollaborationChange(change); err != nil {
		return empty, err
	}
	preview := CollaborationPreview{Change: change, Path: note.Path, Before: source.Body, After: req.Body, State: "preview", Digest: collaborationDigest(root, change)}
	if err := operation.ValidateCreateRequest(collaborationOperation(root, change, preview.Digest)); err != nil {
		return empty, err
	}
	// Source metadata plus replacement must fit, not merely the body itself.
	if preview.Before != source.Body || source.Budget.SourceBytes-source.Budget.BodyBytes+len(preview.After) > CollaborationMaxBodyBytes {
		return empty, collaborationError("body_too_large", "Daily source cannot be replaced within the interactive budget")
	}
	p := WriterDailyEditProposal{SchemaVersion: catalogschema.WriterDailyEditProposalV1Alpha1, Date: req.Daily.Date, Timezone: req.Daily.Timezone, ObjectID: req.ObjectID, Preview: preview}
	if err = s.dailyEditGuard(ctx, req.Daily, p); err != nil {
		return empty, err
	}
	p.Digest = dailyEditDigest(root, req.Daily, p)
	wire, err := json.Marshal(p)
	if err != nil || len(wire) > dailyApplyWireMaxBytes-1024 {
		return empty, collaborationError("body_too_large", "Daily edit proposal exceeds its transport budget")
	}
	return p, nil
}

func (s *Service) dailyEditGuard(ctx context.Context, req WriterDailyRequest, p WriterDailyEditProposal) error {
	source, err := s.WriterDailyEditorSource(ctx, WriterDailyEditorSourceRequest{Source: WriterEditorSourceRequest{VaultPath: req.VaultPath, Session: req.Session, ObjectID: p.ObjectID, Revision: p.Preview.Change.ExpectedRevision}, Date: req.Date, Timezone: req.Timezone})
	if err != nil {
		return err
	}
	if !source.Editable || source.Body != p.Preview.Before {
		return collaborationError("revision_conflict", "Daily editor source changed")
	}
	if err = dailyStorageSafe(ctx, req.VaultPath); err != nil {
		return err
	}
	opened, err := os.OpenRoot(req.VaultPath)
	if err != nil {
		return collaborationError("writer_unavailable", "Daily source is unavailable")
	}
	defer func() { _ = opened.Close() }()
	raw, err := readWriterEditorCandidate(opened, p.Preview.Path)
	if err != nil {
		return err
	}
	if len(raw) > CollaborationMaxBodyBytes || collaborationRevision(string(raw)) != p.Preview.Change.ExpectedRevision || parseNote(p.Preview.Path, string(raw)).ID != p.ObjectID {
		return collaborationError("revision_conflict", "Daily destination changed")
	}
	return ctx.Err()
}

func (s *Service) WriterDailyEditApply(ctx context.Context, req WriterDailyRequest, p WriterDailyEditProposal, authorization string) (operation.View, error) {
	var empty operation.View
	if err := dailyWriterGate(req); err != nil {
		return empty, err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return empty, err
	}
	req.VaultPath = root
	change := p.Preview.Change
	if req.Date != p.Date || req.Timezone != p.Timezone || req.OperationID != change.OperationID || change.Action != "replace" || change.NoteRef != p.Preview.Path || p.ObjectID == "" || !hmac.Equal([]byte(p.Digest), []byte(dailyEditDigest(root, req, p))) {
		return empty, collaborationError("preview_mismatch", "Apply requires the unchanged daily edit proposal and session")
	}
	if err = dailyStorageSafe(ctx, root); err != nil {
		return empty, err
	}
	access := WriterOperationAccess(req.Session.SessionID, req.Session.BindingID, req.Session.Scopes)
	request := operation.CreateRequest{OperationID: req.OperationID, IdempotencyKey: req.OperationID, CapabilityID: writerDailyEditCapability, BindingID: "rest.writer.daily.edit", PrincipalDigest: access.Access.PrincipalDigest, ScopeDigest: access.Access.ScopeDigest, RequestDigest: p.Digest, RevisionBefore: change.ExpectedRevision}
	return s.collaborationApplyGuarded(ctx, root, change, p.Preview.Digest, authorization, writerPolicy(), request, func() error { return s.dailyEditGuard(ctx, req, p) })
}
