package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/internal/records"
	"github.com/yeisme/pinax/pkg/inputcontract"
)

const inputAttachmentCapability = "input.attachment.apply"

// StagedAttachment is owner-only: inputrequests verifies ownership and receipt
// before supplying bytes. No remote request may supply a source path.
type StagedAttachment struct {
	Name   string
	MIME   string
	SHA256 string
	Bytes  []byte
}

type attachmentPlan struct {
	proposal  inputcontract.AttachmentProposal
	note      domain.Note
	content   string
	updated   string
	assetPath string
}

func (s *Service) inputAttachmentPlan(ctx context.Context, root string, req inputcontract.AttachmentRequest, file StagedAttachment) (attachmentPlan, error) {
	var empty attachmentPlan
	root, cleanErr := cleanVaultPath(root)
	if cleanErr != nil {
		return empty, cleanErr
	}
	if req.InputRequestID == "" || req.OperationID == "" || req.ObjectID == "" || req.Revision == "" {
		return empty, collaborationError("attachment_request_invalid", "Input, operation, note identity and exact revision are required")
	}
	if len(file.Bytes) == 0 || len(file.Bytes) > 2<<20 || len(file.Name) > 255 || file.Name == "" || filepath.Base(file.Name) != file.Name || strings.ContainsAny(file.Name, "\\\x00\r\n") {
		return empty, collaborationError("attachment_invalid", "Attachment metadata or size is invalid")
	}
	sum := sha256.Sum256(file.Bytes)
	if hex.EncodeToString(sum[:]) != file.SHA256 {
		return empty, collaborationError("attachment_invalid", "Attachment digest changed")
	}
	ext := ""
	switch file.MIME {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	case "application/pdf":
		ext = ".pdf"
	case "text/plain":
		ext = ".txt"
	case "text/markdown":
		ext = ".md"
	default:
		return empty, collaborationError("attachment_invalid", "Attachment MIME is not supported")
	}
	if strings.HasPrefix(file.MIME, "text/") {
		if !utf8.Valid(file.Bytes) || strings.ContainsRune(string(file.Bytes), 0) {
			return empty, collaborationError("attachment_invalid", "Text attachment is invalid")
		}
	} else if http.DetectContentType(file.Bytes) != file.MIME {
		return empty, collaborationError("attachment_invalid", "Attachment content does not match MIME")
	}
	// The read-only editor scan rejects duplicate IDs and stale full-source revisions.
	session := ReaderSessionInfo{Scopes: []string{WriterScopeEditor, ReaderScopeBody}}
	source, err := s.WriterEditorSource(ctx, WriterEditorSourceRequest{VaultPath: root, ObjectID: req.ObjectID, Revision: req.Revision, Session: session})
	if err != nil {
		return empty, err
	}
	if !source.Editable {
		return empty, collaborationError("frontmatter_unsupported", "Source cannot be edited losslessly")
	}
	notes, _, _, err := navigationSnapshot(ctx, root, true, false)
	if err != nil {
		return empty, err
	}
	note, found := findNoteByObjectID(notes, req.ObjectID)
	if !found {
		return empty, collaborationError("note_not_found", "Note identity is unavailable")
	}
	if err := collaborationSafePath(root, note.Path); err != nil {
		return empty, err
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(note.Path)))
	if err != nil {
		return empty, err
	}
	if collaborationRevision(string(raw)) != req.Revision {
		return empty, collaborationError("revision_conflict", "Source revision changed")
	}
	if req.Placement == "" {
		req.Placement = "per-note"
	}
	// Unique owner-selected filenames keep old attachments immutable on replacement.
	filename := strings.TrimPrefix(operation.IdentityDigest(req.OperationID, file.SHA256), "sha256:") + ext
	rel, err := uniqueAttachmentRelWithPlacement(root, note, filename, assets.AttachmentPlacementPolicy(req.Placement))
	if err != nil {
		return empty, err
	}
	if err := collaborationSafePath(root, rel); err != nil {
		return empty, err
	}
	reference := markdownAttachmentReferenceWithEmbed(note.Path, rel, false)
	body := strings.TrimRight(source.Body, "\n") + "\n\n" + reference + "\n"
	if req.ReplaceReference != "" {
		matches := 0
		for _, attachment := range noteAttachmentsFromBody(root, domain.Note{ID: note.ID, Path: note.Path, Body: source.Body}) {
			if attachment.ReferenceText == req.ReplaceReference {
				matches++
			}
		}
		if matches != 1 || strings.Count(source.Body, req.ReplaceReference) != 1 {
			return empty, collaborationError("attachment_reference_invalid", "Replacement must identify one existing attachment reference")
		}
		body = strings.Replace(source.Body, req.ReplaceReference, reference, 1)
	}
	change := CollaborationChange{OperationID: req.OperationID, Action: "replace", NoteRef: note.Path, ExpectedRevision: req.Revision, Body: body}
	if err := validateCollaborationChange(change); err != nil {
		return empty, err
	}
	proposal := inputcontract.AttachmentProposal{SchemaVersion: inputcontract.AttachmentSchema, Request: req, Reference: reference, MIME: file.MIME, Size: int64(len(file.Bytes)), SHA256: file.SHA256}
	proposal.PreviewDigest, err = operation.CanonicalDigest([]any{root, proposal, collaborationDigest(root, change)})
	if err != nil {
		return empty, err
	}
	_, oldBody := splitFrontmatter(string(raw))
	updated := strings.TrimSuffix(string(raw), oldBody) + body
	if len(updated) > CollaborationMaxBodyBytes {
		return empty, collaborationError("body_too_large", "Attachment reference exceeds the note budget")
	}
	return attachmentPlan{proposal: proposal, note: note, content: string(raw), updated: updated, assetPath: rel}, nil
}

func (s *Service) InputAttachmentPreview(ctx context.Context, root string, req inputcontract.AttachmentRequest, file StagedAttachment) (inputcontract.AttachmentProposal, error) {
	plan, err := s.inputAttachmentPlan(ctx, root, req, file)
	return plan.proposal, err
}

func (s *Service) InputAttachmentApply(ctx context.Context, root string, req inputcontract.AttachmentApply, file StagedAttachment, access OperationAccess) (operation.View, error) {
	root, cleanErr := cleanVaultPath(root)
	if cleanErr != nil {
		return operation.View{}, cleanErr
	}
	if !req.Confirm {
		return operation.View{}, collaborationError("approval_required", "Attachment adoption requires explicit confirmation")
	}
	p := req.Proposal
	if p.SchemaVersion != inputcontract.AttachmentSchema {
		return operation.View{}, collaborationError("attachment_request_invalid", "Unsupported attachment schema")
	}
	if access.OwnerLocal {
		access.PrincipalDigest = operation.IdentityDigest("pinax-owner", "local")
		access.ScopeDigest = operation.IdentityDigest("pinax-vault", root)
	}
	requestDigest, err := operation.CanonicalDigest(p)
	if err != nil {
		return operation.View{}, err
	}
	request := operation.CreateRequest{OperationID: p.Request.OperationID, IdempotencyKey: "input:" + p.Request.InputRequestID, CapabilityID: inputAttachmentCapability, BindingID: "mcp.input.attachment.apply", RequestDigest: requestDigest, PrincipalDigest: access.PrincipalDigest, ScopeDigest: access.ScopeDigest, RevisionBefore: p.Request.Revision}
	if err := operation.ValidateCreateRequest(request); err != nil {
		return operation.View{}, err
	}
	for _, rel := range []string{".pinax/api/operations.sqlite", ".pinax/sync/operation.lock", ".pinax/version", ".pinax/index.sqlite"} {
		if err := collaborationSafePath(root, rel); err != nil {
			return operation.View{}, err
		}
	}
	if err := dailyStorageSafe(ctx, root); err != nil {
		return operation.View{}, err
	}
	lock, err := syncdaemon.AcquireOperationLock(root, inputAttachmentCapability)
	if err != nil {
		return operation.View{}, err
	}
	defer lock.Release()
	// Invalid MIME/size/revision/placement must not create an operation or snapshot.
	if store, err := operation.OpenExisting(root); err == nil {
		replay, found, lookupErr := store.FindReplay(ctx, request)
		_ = store.Close()
		if lookupErr != nil {
			return operation.View{}, lookupErr
		}
		if found {
			return operation.ViewFromRow(replay.Operation), nil
		}
	} else if !operation.IsCode(err, operation.CodeNotFound) {
		return operation.View{}, err
	}
	plan, err := s.inputAttachmentPlan(ctx, root, p.Request, file)
	if err != nil {
		return operation.View{}, err
	}
	if p != plan.proposal {
		return operation.View{}, collaborationError("preview_mismatch", "Attachment proposal changed; review a fresh preview")
	}
	store, err := operation.Open(root)
	if err != nil {
		return operation.View{}, err
	}
	defer func() { _ = store.Close() }()
	created, err := store.CreateAccepted(ctx, request)
	if err != nil {
		return operation.View{}, err
	}
	if created.Replay {
		return operation.ViewFromRow(created.Operation), nil
	}
	if _, err := store.StartApplying(ctx, request.OperationID); err != nil {
		return operation.View{}, err
	}
	outcome := operation.Outcome{ResourceRef: "pinax://note/" + p.Request.ObjectID}
	fail := func(err error) (operation.View, error) {
		return s.collaborationFailed(store, request.OperationID, outcome, err)
	}
	snapshot, err := s.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "Before attachment " + request.OperationID})
	if err != nil {
		return fail(err)
	}
	outcome.ReceiptRef = "snapshot:" + snapshot.Facts["snapshot_id"]
	proof := attachmentProof{OperationID: request.OperationID, RecordDigest: hashString(plan.note.Title + "\x00" + func() string { _, body := splitFrontmatter(plan.updated); return body }()), PreviewDigest: p.PreviewDigest, NoteID: p.Request.ObjectID, NotePath: plan.note.Path, Revision: collaborationRevision(plan.updated), AssetPath: plan.assetPath, SHA256: file.SHA256}
	outcome.Result, _ = json.Marshal(proof)
	if err := store.CheckpointApplying(ctx, request.OperationID, outcome); err != nil {
		return fail(err)
	}
	if err := collaborationSafePath(root, plan.assetPath); err != nil {
		return fail(err)
	}
	if err := publishDailyWithoutReplace(root, plan.assetPath, file.Bytes); err != nil {
		return fail(err)
	}
	// Until the source is published the new blob is unreachable; rollback only
	// our exclusive new blob, never a shared or previously referenced attachment.
	published := false
	defer func() {
		if !published {
			_ = os.Remove(filepath.Join(root, filepath.FromSlash(plan.assetPath)))
		}
	}()
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := collaborationSafePath(root, plan.note.Path); err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(plan.note.Path)))
	if err != nil {
		return fail(err)
	}
	if string(raw) != plan.content {
		return fail(collaborationError("revision_conflict", "Source changed before attachment publication"))
	}
	notePath := filepath.Join(root, filepath.FromSlash(plan.note.Path))
	info, err := os.Stat(notePath)
	if err != nil {
		return fail(err)
	}
	if err := atomicWriteFile(notePath, []byte(plan.updated), info.Mode().Perm()); err != nil {
		return fail(err)
	}
	published = true
	_, plan.note.Body = splitFrontmatter(plan.updated)
	if _, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, request.OperationID, plan.note, ""); err != nil {
		return fail(err)
	}
	if err := refreshIndex(root); err != nil {
		return fail(err)
	}
	evidence, err := inputAttachmentEvidence(ctx, root, proof, outcome.ReceiptRef)
	if err != nil || !evidence.Applied {
		return fail(collaborationError("reconcile_required", "Attachment read-back requires inspection"))
	}
	outcome.RevisionAfter = proof.Revision
	outcome.Receipt, _ = json.Marshal(map[string]string{"operation_id": request.OperationID, "snapshot": outcome.ReceiptRef, "proof": "note_and_attachment_readback"})
	row, err := store.CompleteSucceeded(context.WithoutCancel(ctx), request.OperationID, outcome)
	if err != nil {
		return fail(err)
	}
	return operation.ViewFromRow(row), nil
}

type attachmentProof struct {
	OperationID   string `json:"operation_id"`
	RecordDigest  string `json:"record_digest"`
	PreviewDigest string `json:"preview_digest"`
	NoteID        string `json:"note_id"`
	NotePath      string `json:"note_path"`
	Revision      string `json:"revision"`
	AssetPath     string `json:"asset_path"`
	SHA256        string `json:"sha256"`
}

func inputAttachmentEvidence(ctx context.Context, root string, proof attachmentProof, snapshot string) (operation.ReconcileEvidence, error) {
	var empty operation.ReconcileEvidence
	if proof.NoteID == "" || !strings.HasPrefix(snapshot, "snapshot:") {
		return empty, nil
	}
	for _, path := range []string{proof.NotePath, proof.AssetPath} {
		if err := collaborationSafePath(root, path); err != nil {
			return empty, err
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(proof.NotePath)))
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	if collaborationRevision(string(raw)) != proof.Revision || parseNote(proof.NotePath, string(raw)).ID != proof.NoteID {
		return empty, nil
	}
	asset, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(proof.AssetPath)))
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	sum := sha256.Sum256(asset)
	if hex.EncodeToString(sum[:]) != proof.SHA256 {
		return empty, nil
	}
	event, found, err := records.NewService(root).FindEventByIdempotency(ctx, proof.OperationID)
	if err != nil {
		return empty, err
	}
	if !found || event.NoteID != proof.NoteID || event.Kind != domain.RecordEventNoteMetadataUpdated || event.ContentRevision.Hash != proof.RecordDigest {
		return empty, nil
	}
	result, _ := json.Marshal(proof)
	return operation.ReconcileEvidence{Applied: true, Result: result, ReceiptRef: snapshot, ResourceRef: "pinax://note/" + proof.NoteID, RevisionAfter: proof.Revision}, nil
}
