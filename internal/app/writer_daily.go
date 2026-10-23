package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/identity"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/internal/records"
	"github.com/yeisme/pinax/internal/templateengine"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

const writerDailyApplyCapability = "writer.daily.apply"

// The proposal body lives only in the caller's memory. The ledger stores a
// digest and bounded identity/receipt facts, never this struct or its text.
type WriterDailyProposal struct {
	SchemaVersion        string `json:"schema_version"`
	OperationID          string `json:"operation_id"`
	Date                 string `json:"date"`
	Timezone             string `json:"timezone"`
	ObjectID             string `json:"object_id"`
	Path                 string `json:"path"`
	Title                string `json:"title"`
	Before               string `json:"before"`
	After                string `json:"after"`
	TemplateDigest       string `json:"template_digest"`
	PreviewDigest        string `json:"preview_digest"`
	CreatedAt            string `json:"created_at"`
	BindingID            string `json:"binding_id"`
	FrontmatterProtected bool   `json:"frontmatter_protected"`
}

type WriterDailyRequest struct {
	VaultPath   string
	Session     ReaderSessionInfo
	OperationID string
	Date        string
	Timezone    string
	// Private per-server key, never exposed to the client or persisted.
	PreviewSecret []byte
}

func dailyWriterGate(req WriterDailyRequest) error {
	if !req.Session.HasScope(WriterScopeEditor) || !req.Session.HasScope(ReaderScopeBody) || !req.Session.HasScope(ReaderScopeMetadata) {
		return collaborationError("insufficient_scope", "Daily creation requires writer.editor, reader.body and reader.metadata")
	}
	if len(req.PreviewSecret) < 16 || req.Session.SessionID == "" || req.Session.BindingID == "" {
		return collaborationError("writer_unavailable", "Daily creation requires a supervised session")
	}
	if catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterDailyProposeRequestV1Alpha1, map[string]any{"operation_id": req.OperationID, "date": req.Date, "timezone": req.Timezone}) != nil {
		return collaborationError("writer_request_invalid", "Daily creation requires a date, timezone and original operation identity")
	}
	return nil
}

const dailyApplyWireMaxBytes = 128 * 1024

func dailyProposalFitsWire(p WriterDailyProposal) bool {
	encoded, err := json.Marshal(struct {
		Proposal      WriterDailyProposal `json:"proposal"`
		Authorization string              `json:"authorization"`
	}{p, "explicit_instruction"})
	return err == nil && len(encoded) <= dailyApplyWireMaxBytes
}

func dailyProposalDigest(root string, session ReaderSessionInfo, p WriterDailyProposal, secret []byte) string {
	p.PreviewDigest = ""
	encoded, _ := json.Marshal([]any{"pinax.writer.daily.proposal.v1", root, session.SessionID, session.BindingID, session.scopeDigest(), p})
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(encoded)
	return "sha256:" + hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) WriterDailyPropose(ctx context.Context, req WriterDailyRequest) (WriterDailyProposal, error) {
	var empty WriterDailyProposal
	if err := dailyWriterGate(req); err != nil {
		return empty, err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return empty, collaborationError("writer_unavailable", "The authorized vault is unavailable")
	}
	day, err := s.ReaderDaily(ctx, ReaderDailyRequest{VaultPath: root, Session: req.Session, Date: req.Date, Timezone: req.Timezone})
	if err != nil {
		return empty, err
	}
	if day.Note != nil {
		return empty, collaborationError("daily_exists", "The daily journal already exists; open its current revision")
	}
	template, err := readerDailyTemplate(root)
	if err != nil {
		return empty, err
	}
	// The legacy template parser only recognizes LF frontmatter. Do not
	// render unsupported metadata as body or silently normalize its meaning.
	if strings.HasPrefix(template, "---\r\n") || strings.HasPrefix(template, "\ufeff") {
		return empty, collaborationError("template_unsupported", "Daily template frontmatter must use supported LF encoding")
	}
	doc, err := templateengine.ParseDocument("journal.daily", template)
	if err != nil || strings.HasPrefix(template, "---\n") && doc.Metadata.SchemaVersion != templateengine.SchemaVersionV2 || len(doc.Metadata.Queries) > 0 || len(doc.Metadata.AfterCreateActions) > 0 || len(doc.Issues) > 0 {
		return empty, collaborationError("template_unsupported", "Daily creation requires a bounded template without executable queries")
	}
	path := journalPathFromPattern(doc.Metadata.Output.PathPattern, "daily", req.Date)
	if !readerDailyVisiblePath(path) || len(path) > 512 {
		return empty, collaborationError("template_unsupported", "Daily template destination is unsupported")
	}
	if err := dailyTargetAbsent(root, path); err != nil {
		return empty, err
	}
	title := journalTitle("daily", req.Date)
	rendered, err := templateengine.New().RenderBounded(ctx, doc, templateengine.Context{Title: title, Date: req.Date, Vars: map[string]string{"date": req.Date}}, catalogschema.WriterMaxBodyBytes)
	if err != nil {
		return empty, collaborationError("template_unsupported", "Daily template could not be rendered within its budget")
	}
	id, err := s.allocateObjectID(identity.KindNote, root, "writer.daily:"+path+":"+rand.Text())
	if err != nil {
		return empty, collaborationError("writer_unavailable", "Daily identity could not be allocated")
	}
	p := WriterDailyProposal{SchemaVersion: catalogschema.WriterDailyProposalV1Alpha1, OperationID: req.OperationID, Date: req.Date, Timezone: req.Timezone, ObjectID: id, Path: path, Title: title, After: "\n" + strings.TrimSpace(rendered.Body) + "\n", TemplateDigest: collaborationRevision(template), CreatedAt: s.currentTimeUTC().Format(time.RFC3339), BindingID: req.Session.BindingID, FrontmatterProtected: true}
	content := dailyProposalContent(p)
	_, p.After = splitFrontmatter(content)
	if len(content) > catalogschema.WriterMaxBodyBytes || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return empty, collaborationError("body_too_large", "Daily source exceeds the 64 KiB interactive budget")
	}
	p.PreviewDigest = dailyProposalDigest(root, req.Session, p, req.PreviewSecret)
	if !dailyProposalFitsWire(p) {
		return empty, collaborationError("body_too_large", "Daily proposal exceeds the 128 KiB JSON transport budget")
	}
	return p, nil
}

func dailyProposalContent(p WriterDailyProposal) string {
	return buildNoteContentWithObjectID(p.ObjectID, p.Title, "", "daily", "daily", []string{"daily"}, "journal", p.CreatedAt, p.After)
}

func dailyTargetAbsent(root, relative string) error {
	if err := collaborationSafePath(root, relative); err != nil {
		return err
	}
	path, err := safeJoin(root, relative)
	if err != nil {
		return collaborationError("unsafe_path", "Daily destination is invalid")
	}
	if _, err = os.Lstat(path); err == nil {
		return collaborationError("daily_exists", "Daily destination already exists")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return collaborationError("writer_unavailable", "Daily destination could not be inspected")
	}
	return nil
}

type dailyOperationResult struct {
	Date          string `json:"date"`
	Timezone      string `json:"timezone"`
	NoteID        string `json:"note_id"`
	Path          string `json:"path"`
	ContentDigest string `json:"content_digest"`
	RecordEventID string `json:"record_event_id,omitempty"`
}

func (s *Service) WriterDailyApply(ctx context.Context, req WriterDailyRequest, p WriterDailyProposal, authorization string) (operation.View, error) {
	var empty operation.View
	if err := dailyWriterGate(req); err != nil {
		return empty, err
	}
	if authorization != "reviewed_proposal" && authorization != "explicit_instruction" {
		return empty, collaborationError("authorization_required", "Daily creation requires a reviewed proposal or explicit instruction assertion")
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return empty, err
	}
	if p.OperationID != req.OperationID || p.Date != req.Date || p.Timezone != req.Timezone || p.BindingID != req.Session.BindingID || !hmac.Equal([]byte(p.PreviewDigest), []byte(dailyProposalDigest(root, req.Session, p, req.PreviewSecret))) {
		return empty, collaborationError("preview_mismatch", "Apply requires the unchanged proposal from this supervised session")
	}
	if p.SchemaVersion != catalogschema.WriterDailyProposalV1Alpha1 || !p.FrontmatterProtected || !readerDailyVisiblePath(p.Path) || p.Before != "" {
		return empty, collaborationError("preview_mismatch", "Daily proposal is invalid")
	}
	if _, err := identity.ParseObjectID(p.ObjectID); err != nil {
		return empty, collaborationError("preview_mismatch", "Daily object identity is invalid")
	}
	content := dailyProposalContent(p)
	if len(content) > catalogschema.WriterMaxBodyBytes || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return empty, collaborationError("body_too_large", "Daily source exceeds its budget")
	}
	access := WriterOperationAccess(req.Session.SessionID, req.Session.BindingID, req.Session.Scopes)
	request := operation.CreateRequest{OperationID: p.OperationID, IdempotencyKey: p.OperationID, CapabilityID: writerDailyApplyCapability, BindingID: "rest.writer.daily.apply", PrincipalDigest: access.Access.PrincipalDigest, ScopeDigest: access.Access.ScopeDigest, RequestDigest: p.PreviewDigest}
	if err := operation.ValidateCreateRequest(request); err != nil {
		return empty, err
	}
	if err := dailyStorageSafe(ctx, root); err != nil {
		return empty, err
	}
	for _, rel := range []string{".pinax/sync/operation.lock", p.Path} {
		if err := collaborationSafePath(root, rel); err != nil {
			return empty, err
		}
	}
	lock, err := syncdaemon.AcquireOperationLock(root, writerDailyApplyCapability)
	if err != nil {
		return empty, err
	}
	defer lock.Release()
	store, err := operation.Open(root)
	if err != nil {
		return empty, err
	}
	defer func() { _ = store.Close() }()
	if replay, found, err := store.FindReplay(ctx, request); err != nil {
		return empty, err
	} else if found {
		return operation.ViewFromRow(replay.Operation), nil
	}
	if err := dailyLedgerAvailable(ctx, store, p.Date); err != nil {
		return empty, err
	}
	day, err := s.ReaderDaily(ctx, ReaderDailyRequest{VaultPath: root, Session: req.Session, Date: p.Date, Timezone: p.Timezone})
	if err != nil {
		return empty, err
	}
	if day.Note != nil {
		return empty, collaborationError("daily_exists", "The daily journal was created after preview")
	}
	template, err := readerDailyTemplate(root)
	if err != nil {
		return empty, err
	}
	if collaborationRevision(template) != p.TemplateDigest {
		return empty, collaborationError("preview_mismatch", "The daily template changed; review a new proposal")
	}
	if err := dailyTargetAbsent(root, p.Path); err != nil {
		return empty, err
	}
	created, err := store.CreateAccepted(ctx, request)
	if err != nil {
		return empty, err
	}
	if created.Replay {
		return operation.ViewFromRow(created.Operation), nil
	}
	result := dailyOperationResult{Date: p.Date, Timezone: p.Timezone, NoteID: p.ObjectID, Path: p.Path, ContentDigest: collaborationRevision(content)}
	outcome := operation.Outcome{ResourceRef: "pinax://note/" + p.ObjectID}
	outcome.Result, _ = json.Marshal(result)
	if _, err := store.StartApplyingWithOutcome(ctx, p.OperationID, outcome); err != nil {
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	snapshot, err := s.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "Before daily journal " + p.OperationID})
	if err != nil {
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	outcome.ReceiptRef = "snapshot:" + snapshot.Facts["snapshot_id"]
	if err := store.CheckpointApplying(ctx, p.OperationID, outcome); err != nil {
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	if err := ctx.Err(); err != nil {
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	currentTemplate, err := readerDailyTemplate(root)
	if err != nil {
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	if collaborationRevision(currentTemplate) != p.TemplateDigest {
		outcome.ErrorCode, outcome.ErrorMessage = "preview_mismatch", "Daily template changed before publication"
		row, err := store.CompleteFailed(ctx, p.OperationID, outcome)
		if err != nil {
			return s.collaborationFailed(store, p.OperationID, outcome, err)
		}
		return operation.ViewFromRow(row), nil
	}
	if err := publishDailyWithoutReplace(root, p.Path, []byte(content)); err != nil {
		if errors.Is(err, os.ErrExist) {
			outcome.ErrorCode = "daily_exists"
			outcome.ErrorMessage = "Daily destination already exists"
			row, saveErr := store.CompleteFailed(ctx, p.OperationID, outcome)
			if saveErr != nil {
				return empty, saveErr
			}
			return operation.ViewFromRow(row), nil
		}
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	if err := dailyStorageSafe(ctx, root); err != nil {
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	note := parseNote(p.Path, content)
	event, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteCreated, "writer.daily:"+p.OperationID, note, "")
	if err != nil {
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	result.RecordEventID = event.EventID
	outcome.Result, _ = json.Marshal(result)
	outcome.Receipt, _ = json.Marshal(map[string]string{"operation_id": p.OperationID, "snapshot": outcome.ReceiptRef, "record_event_id": event.EventID})
	outcome.RevisionAfter = result.ContentDigest
	if err := refreshIndex(root); err != nil {
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	row, err := store.CompleteSucceeded(ctx, p.OperationID, outcome)
	if err != nil {
		return s.collaborationFailed(store, p.OperationID, outcome, err)
	}
	return operation.ViewFromRow(row), nil
}

// The ledger, not the current template path, fences a calendar day across
// path changes and new operation keys. Keep ambiguous attempts blocked.
func dailyLedgerAvailable(ctx context.Context, store *operation.Store, date string) error {
	after := ""
	for {
		rows, err := store.ListCapability(ctx, writerDailyApplyCapability, after, 200)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.Status == operation.StatusFailed {
				continue
			}
			var result dailyOperationResult
			if json.Unmarshal(row.ResultJSON, &result) != nil || result.Date == "" {
				return collaborationError("reconcile_required", "An earlier daily operation needs owner inspection")
			}
			if result.Date != date {
				continue
			}
			if row.Status == operation.StatusSucceeded {
				return collaborationError("daily_exists", "A daily journal was already created for this calendar date; locate the original object")
			}
			return collaborationError("reconcile_required", "The original daily operation must be inspected before another creation")
		}
		if len(rows) < 200 {
			return nil
		}
		after = rows[len(rows)-1].OperationID
	}
}

// A link publishes an already-fsynced staging file only when the destination
// does not exist. Unsupported filesystems fail closed; never fall back to rename.
func publishDailyWithoutReplace(rootPath, relative string, body []byte) error {
	if err := collaborationSafePath(rootPath, relative); err != nil {
		return err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	dir := filepath.Dir(relative)
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := collaborationSafePath(rootPath, relative); err != nil {
		return err
	}
	stage := filepath.Join(dir, ".daily-stage-"+rand.Text())
	file, err := root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(stage) }()
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := collaborationSafePath(rootPath, relative); err != nil {
		return err
	}
	return root.Link(stage, relative)
}

// Shared snapshot/record repositories predate supervised sessions. Reject
// linked or special metadata before allowing them to access this source.
func dailyStorageSafe(ctx context.Context, root string) error {
	for _, rel := range []string{".pinax/events.jsonl", ".pinax/last_snapshot", ".pinax/index.sqlite", ".pinax/index.sqlite-wal", ".pinax/index.sqlite-shm", ".pinax/records", ".pinax/version", ".pinax/api"} {
		if err := collaborationSafePath(root, rel); err != nil {
			return err
		}
		err := filepath.WalkDir(filepath.Join(root, rel), func(_ string, entry fs.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return collaborationError("writer_unavailable", "Daily metadata could not be inspected")
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.IsDir() && !entry.Type().IsRegular() {
				return collaborationError("unsafe_path", "Daily metadata must not contain symlinks or special files")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) dailyReconcileEvidence(ctx context.Context, root string, row operation.OperationRow) (operation.ReconcileEvidence, error) {
	if row.CapabilityID != writerDailyApplyCapability {
		return operation.ReconcileEvidence{}, nil
	}
	if err := dailyStorageSafe(ctx, root); err != nil {
		return operation.ReconcileEvidence{}, err
	}
	var result dailyOperationResult
	if json.Unmarshal(row.ResultJSON, &result) != nil || result.NoteID == "" || !readerDailyVisiblePath(result.Path) || result.ContentDigest == "" {
		return operation.ReconcileEvidence{}, nil
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	defer func() { _ = opened.Close() }()
	raw, err := readWriterEditorCandidate(opened, result.Path)
	if err != nil {
		return operation.ReconcileEvidence{}, nil
	}
	if len(raw) > catalogschema.WriterMaxBodyBytes || collaborationRevision(string(raw)) != result.ContentDigest || parseNote(result.Path, string(raw)).ID != result.NoteID {
		return operation.ReconcileEvidence{}, nil
	}
	event, found, err := records.NewService(root).FindEventByIdempotency(ctx, "writer.daily:"+row.OperationID)
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	note := parseNote(result.Path, string(raw))
	if !found || event.Kind != domain.RecordEventNoteCreated || event.NoteID != result.NoteID || filepath.ToSlash(event.Path) != result.Path || event.ContentRevision.Hash != hashString(note.Title+"\x00"+note.Body) {
		return operation.ReconcileEvidence{}, nil
	}
	result.RecordEventID = event.EventID
	encoded, _ := json.Marshal(result)
	receipt, _ := json.Marshal(map[string]string{"record_event_id": event.EventID, "proof": "record_and_content"})
	return operation.ReconcileEvidence{Applied: true, ProofRef: "record:" + event.EventID, Result: encoded, Receipt: receipt, ReceiptRef: row.ReceiptRef, ResourceRef: "pinax://note/" + result.NoteID, RevisionAfter: result.ContentDigest}, nil
}
