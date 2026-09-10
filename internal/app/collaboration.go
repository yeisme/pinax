package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
)

const CollaborationMaxBodyBytes = 64 << 10

type CollaborationSearchResult struct {
	Query       string               `json:"query"`
	Notes       []domain.NoteDisplay `json:"notes"`
	Offset      int                  `json:"offset"`
	Limit       int                  `json:"limit"`
	Total       int                  `json:"total"`
	HasMore     bool                 `json:"has_more"`
	Truncated   bool                 `json:"truncated"`
	IndexStatus string               `json:"index_status"`
}

func (s *Service) CollaborationSearch(ctx context.Context, root, query string, offset, limit int) (CollaborationSearchResult, error) {
	root, err := cleanVaultPath(root)
	if err != nil {
		return CollaborationSearchResult{}, err
	}
	if limit < 1 || limit > 20 || offset < 0 || offset > 980 {
		return CollaborationSearchResult{}, collaborationError("page_invalid", "Use a limit of 1-20 and an offset of 0-980")
	}
	result, err := s.SearchNotes(ctx, SearchRequest{VaultPath: root, Query: query, Limit: offset + limit + 1, Engine: "scan"})
	if err != nil {
		return CollaborationSearchResult{}, err
	}
	v := CollaborationSearchResult{Query: query, Notes: []domain.NoteDisplay{}, Offset: offset, Limit: limit, Total: result.Total, IndexStatus: result.IndexStatus}
	for i := offset; i < len(result.Notes) && i < offset+limit; i++ {
		if err := collaborationSafePath(root, result.Notes[i].Path); err != nil {
			return CollaborationSearchResult{}, err
		}
		v.Notes = append(v.Notes, buildNoteDisplay(result.Notes[i], domain.NoteDisplayCard, domain.NoteExposureAgent))
	}
	v.HasMore = len(result.Notes) > offset+limit && offset+limit < 1000
	v.Truncated = result.Total > offset+len(v.Notes)
	return v, nil
}

// CollaborationPolicy is owner configuration, never deserialized from tool input.
type CollaborationPolicy struct{ AllowBody, AllowWrite bool }

// CollaborationChange is user content, not an audit payload. Only its digest is persisted.
type CollaborationChange struct {
	OperationID      string   `json:"operation_id"`
	Action           string   `json:"action"`
	NoteRef          string   `json:"note_ref,omitempty"`
	ExpectedRevision string   `json:"expected_revision,omitempty"`
	Title            string   `json:"title,omitempty"`
	Body             string   `json:"body,omitempty"`
	Dir              string   `json:"dir,omitempty"`
	TargetPath       string   `json:"target_path,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	TagOperation     string   `json:"tag_operation,omitempty"`
}

type CollaborationPreview struct {
	Change CollaborationChange `json:"change"`
	Digest string              `json:"preview_digest"`
	Path   string              `json:"path"`
	Before string              `json:"before"`
	After  string              `json:"after"`
	State  string              `json:"state"`
}

func collaborationError(code, message string) error {
	return &domain.CommandError{Code: code, Message: message}
}

func collaborationRevision(content string) string {
	h := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(h[:])
}

func collaborationSafePath(root, rel string) error {
	if _, err := safeJoin(root, rel); err != nil {
		return err
	}
	path := root
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return collaborationError("path_unavailable", "Note path is unavailable")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return collaborationError("unsafe_path", "Collaboration cannot follow a symlink inside the vault")
		}
	}
	return nil
}

func collaborationDigest(root string, change CollaborationChange) string {
	raw, _ := json.Marshal(change)
	return collaborationRevision(root + "\x00" + string(raw))
}

func validateCollaborationChange(change CollaborationChange) error {
	if len(change.Body) > CollaborationMaxBodyBytes || !utf8.ValidString(change.Body) || strings.ContainsRune(change.Body, 0) {
		return collaborationError("body_invalid", "Body must be valid UTF-8 text of at most 64 KiB")
	}
	if len(change.Title) > 512 || strings.ContainsAny(change.Title, "\r\n\x00") {
		return collaborationError("title_invalid", "Title must be a single line of at most 512 bytes")
	}
	if len(change.NoteRef) > 512 || len(change.Dir) > 512 || len(change.Tags) > 32 {
		return collaborationError("change_invalid", "Change exceeds the supported limits")
	}
	if _, err := normalizeTagsForWrite(change.Tags); err != nil {
		return err
	}
	switch change.Action {
	case "create":
		if strings.TrimSpace(change.Title) == "" || change.NoteRef != "" || change.ExpectedRevision != "" || change.TagOperation != "" {
			return collaborationError("change_invalid", "Create requires a title and no existing note reference")
		}
	case "append", "replace", "tags", "archive":
		if change.NoteRef == "" || change.Title != "" || change.Dir != "" || change.TargetPath != "" {
			return collaborationError("change_invalid", "Existing-note changes require a note reference and cannot change its path or title")
		}
		if change.Action == "tags" {
			if change.TagOperation != "add" && change.TagOperation != "remove" && change.TagOperation != "set" {
				return collaborationError("change_invalid", "Tag operation must be add, remove, or set")
			}
		} else if len(change.Tags) > 0 || change.TagOperation != "" {
			return collaborationError("change_invalid", "Tags are only accepted by the tags action")
		}
		if (change.Action == "tags" || change.Action == "archive") && change.Body != "" {
			return collaborationError("change_invalid", "This action cannot change the body")
		}
	default:
		return collaborationError("action_unsupported", "Action must be create, append, replace, tags, or archive")
	}
	return nil
}

func (s *Service) CollaborationRead(ctx context.Context, root, ref, intent string, policy CollaborationPolicy) (domain.Note, string, error) {
	if !policy.AllowBody {
		return domain.Note{}, "", collaborationError("body_disabled", "The owner has not enabled note body access")
	}
	if intent != "read" && intent != "summarize" && intent != "edit" {
		return domain.Note{}, "", collaborationError("intent_required", "Specify read, summarize, or edit for this note; this is a caller assertion, not proof of user approval")
	}
	root, err := cleanVaultPath(root)
	if err != nil {
		return domain.Note{}, "", err
	}
	_, note, _, content, _, err := s.loadMutableNoteForWrite(ctx, root, ref)
	if err != nil {
		return domain.Note{}, "", err
	}
	if err := collaborationSafePath(root, note.Path); err != nil {
		return domain.Note{}, "", err
	}
	if len(content) > CollaborationMaxBodyBytes {
		return domain.Note{}, "", collaborationError("body_too_large", "Note exceeds the 64 KiB interactive limit; use a local owner workflow")
	}
	// Never use the index body as the revision source: external editors may have changed the file.
	_, note.Body = splitFrontmatter(content)
	return note, collaborationRevision(content), nil
}

func (s *Service) CollaborationPreview(ctx context.Context, root string, change CollaborationChange, policy CollaborationPolicy) (CollaborationPreview, error) {
	if !policy.AllowBody || !policy.AllowWrite {
		return CollaborationPreview{}, collaborationError("write_disabled", "The owner must enable note body and write access")
	}
	root, err := cleanVaultPath(root)
	if err != nil {
		return CollaborationPreview{}, err
	}
	if err := validateCollaborationChange(change); err != nil {
		return CollaborationPreview{}, err
	}
	if change.OperationID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return CollaborationPreview{}, err
		}
		change.OperationID = "collab-" + hex.EncodeToString(id[:])
	}
	p := CollaborationPreview{Change: change, State: "preview", After: change.Body}
	if change.Action == "create" {
		if change.Dir == "" {
			change.Dir = "index"
		}
		plan, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: change.Title, Body: change.Body, Dir: change.Dir, Tags: change.Tags, DryRun: true})
		if err != nil {
			return p, err
		}
		p.Path = plan.Facts["path"]
		if err := collaborationSafePath(root, p.Path); err != nil {
			return p, err
		}
		if change.TargetPath != "" && change.TargetPath != p.Path {
			return p, collaborationError("revision_conflict", "The proposed destination changed; prepare a new preview")
		}
		change.TargetPath = p.Path
		// The existing create service normalizes surrounding whitespace. Review its
		// actual body projection rather than promising the raw input verbatim.
		data, _ := plan.Data.(map[string]any)
		body, _ := data["body_preview"].(string)
		p.After = "\n" + body + "\n"
	} else {
		note, revision, err := s.CollaborationRead(ctx, root, change.NoteRef, "edit", policy)
		if err != nil {
			return p, err
		}
		if change.ExpectedRevision != "" && change.ExpectedRevision != revision {
			return p, collaborationError("revision_conflict", "Note changed; read it again and prepare a new preview")
		}
		change.NoteRef, change.ExpectedRevision = note.Path, revision
		p.Path, p.Before = note.Path, note.Body
		switch change.Action {
		case "append":
			p.After = note.Body + change.Body
		case "tags":
			p.Before = strings.Join(note.Tags, ", ")
			tags, _ := normalizeTagsForWrite(change.Tags)
			switch change.TagOperation {
			case "add":
				tags = mergeTags(note.Tags, tags)
			case "remove":
				tags = removeTags(note.Tags, tags)
			}
			p.After = strings.Join(tags, ", ")
		case "archive":
			p.Before, p.After = note.Status, "archived"
		}
		if len(p.After) > CollaborationMaxBodyBytes {
			return p, collaborationError("body_too_large", "Result exceeds the 64 KiB interactive limit")
		}
	}
	p.Change = change
	p.Digest = collaborationDigest(root, change)
	if err := operation.ValidateCreateRequest(collaborationOperation(root, change, p.Digest)); err != nil {
		return p, err
	}
	return p, nil
}

func collaborationOperation(root string, change CollaborationChange, digest string) operation.CreateRequest {
	return operation.CreateRequest{OperationID: change.OperationID, IdempotencyKey: change.OperationID, CapabilityID: "note.collaboration", BindingID: "mcp.note.collaboration", PrincipalDigest: operation.IdentityDigest("pinax-owner", "local"), ScopeDigest: operation.IdentityDigest("pinax-vault", root), RequestDigest: digest, RevisionBefore: change.ExpectedRevision}
}

// CollaborationApply replays terminal outcomes without re-reading a now-stale preview.
// A caller assertion never grants access: the owner policy remains authoritative.
func (s *Service) CollaborationApply(ctx context.Context, root string, change CollaborationChange, digest, authorization string, policy CollaborationPolicy) (operation.View, error) {
	if !policy.AllowBody || !policy.AllowWrite {
		return operation.View{}, collaborationError("write_disabled", "The owner has not enabled note writes")
	}
	if authorization != "explicit_instruction" && authorization != "reviewed_proposal" {
		return operation.View{}, collaborationError("authorization_required", "State the caller-asserted explicit instruction or reviewed proposal; host approval still applies")
	}
	root, err := cleanVaultPath(root)
	if err != nil {
		return operation.View{}, err
	}
	if err := validateCollaborationChange(change); err != nil {
		return operation.View{}, err
	}
	if digest == "" || digest != collaborationDigest(root, change) {
		return operation.View{}, collaborationError("preview_mismatch", "Apply must use the unchanged preview, operation ID, and digest")
	}
	request := collaborationOperation(root, change, digest)
	if err := operation.ValidateCreateRequest(request); err != nil {
		return operation.View{}, err
	}
	for _, rel := range []string{".pinax/api/operations.sqlite", ".pinax/sync/operation.lock", ".pinax/records", ".pinax/version/snapshots", ".pinax/version/objects", ".pinax/index.sqlite"} {
		if err := collaborationSafePath(root, rel); err != nil {
			return operation.View{}, err
		}
	}
	// ponytail: one owner lock per vault; introduce finer locks only if measured contention warrants it.
	lock, err := syncdaemon.AcquireOperationLock(root, "note.collaboration")
	if err != nil {
		return operation.View{}, err
	}
	defer lock.Release()
	store, err := operation.Open(root)
	if err != nil {
		return operation.View{}, err
	}
	defer func() { _ = store.Close() }()
	if replay, found, err := store.FindReplay(ctx, request); err != nil {
		return operation.View{}, err
	} else if found {
		return operation.ViewFromRow(replay.Operation), nil
	}
	p, err := s.CollaborationPreview(ctx, root, change, policy)
	if err != nil {
		return operation.View{}, err
	}
	if p.Digest != digest {
		return operation.View{}, collaborationError("preview_mismatch", "Prepare a canonical preview before applying")
	}
	created, err := store.CreateAccepted(ctx, request)
	if err != nil {
		return operation.View{}, err
	}
	if created.Replay {
		return operation.ViewFromRow(created.Operation), nil
	}
	if _, err := store.StartApplying(ctx, change.OperationID); err != nil {
		return operation.View{}, err
	}
	// A canceled call after acceptance leaves a queryable identity, never a fresh retry.
	if err := ctx.Err(); err != nil {
		return s.collaborationFailed(store, change.OperationID, operation.Outcome{}, err)
	}
	outcome := operation.Outcome{}
	if change.Action != "create" {
		snapshot, err := s.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "Before collaboration " + change.OperationID})
		if err != nil {
			return s.collaborationFailed(store, change.OperationID, outcome, err)
		}
		outcome.ReceiptRef = "snapshot:" + snapshot.Facts["snapshot_id"]
	}
	outcome.ResourceRef = "pinax://note/" + url.PathEscape(p.Path)
	outcome.Result, _ = json.Marshal(map[string]string{"path": p.Path})
	if err := store.CheckpointApplying(ctx, change.OperationID, outcome); err != nil {
		return s.collaborationFailed(store, change.OperationID, outcome, err)
	}
	projection, err := s.applyCollaborationChange(ctx, root, p)
	if err != nil {
		return s.collaborationFailed(store, change.OperationID, outcome, err)
	}
	path := projection.Facts["path"]
	_, revision, err := s.CollaborationRead(ctx, root, path, "read", policy)
	if err != nil {
		return s.collaborationFailed(store, change.OperationID, outcome, err)
	}
	outcome.ResourceRef = "pinax://note/" + url.PathEscape(path)
	outcome.RevisionAfter = revision
	outcome.Result, _ = json.Marshal(map[string]string{"path": path, "note_id": projection.Facts["note_id"], "authorization_source": "caller_assertion", "authorization": authorization, "index_status": projection.Facts["index_status"]})
	outcome.Receipt, _ = json.Marshal(map[string]string{"operation_id": change.OperationID, "snapshot": outcome.ReceiptRef, "record_event_id": projection.Facts["record_event_id"]})
	row, err := store.CompleteSucceeded(ctx, change.OperationID, outcome)
	if err != nil {
		return s.collaborationFailed(store, change.OperationID, outcome, err)
	}
	return operation.ViewFromRow(row), nil
}

func (s *Service) collaborationFailed(store *operation.Store, id string, outcome operation.Outcome, _ error) (operation.View, error) {
	// Do not persist filesystem errors, user content, or tool arguments in the ledger.
	outcome.ErrorCode, outcome.ErrorMessage = "outcome_unknown", "Inspect the original operation and recovery snapshot before any retry"
	row, err := store.RequireReconcile(context.Background(), id, outcome)
	if err != nil {
		return operation.View{OperationID: id, Status: operation.StatusReconcileRequired, ReconcileRequired: true}, collaborationError("outcome_unknown", "Write outcome is unknown; query the original operation ID")
	}
	return operation.ViewFromRow(row), nil
}

func (s *Service) applyCollaborationChange(ctx context.Context, root string, p CollaborationPreview) (domain.Projection, error) {
	c := p.Change
	if c.Action == "create" {
		return s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: c.Title, Body: c.Body, Tags: c.Tags, Dir: c.Dir, PlannedPath: p.Path, RecordIdempotencyKey: c.OperationID})
	}
	_, note, path, content, _, err := s.loadMutableNoteForWrite(ctx, root, c.NoteRef)
	if err != nil {
		return domain.Projection{}, err
	}
	if collaborationRevision(content) != c.ExpectedRevision {
		return domain.Projection{}, collaborationError("revision_conflict", "Note changed before save; do not overwrite it")
	}
	if err := collaborationSafePath(root, note.Path); err != nil {
		return domain.Projection{}, err
	}
	switch c.Action {
	case "tags":
		return s.TagNote(ctx, NoteTagRequest{VaultPath: root, NoteRef: c.NoteRef, Operation: c.TagOperation, Tags: c.Tags, ExpectedRevision: c.ExpectedRevision})
	case "archive":
		return s.ArchiveNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: c.NoteRef, ExpectedRevision: c.ExpectedRevision})
	}
	_, oldBody := splitFrontmatter(content)
	updated := strings.TrimSuffix(content, oldBody) + p.After
	updated, _ = patchFrontmatterFields(updated, map[string]string{"updated_at": s.currentTimeUTC().Format("2006-01-02T15:04:05Z07:00")})
	info, err := os.Stat(path)
	if err != nil {
		return domain.Projection{}, err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return domain.Projection{}, err
	}
	if collaborationRevision(string(current)) != c.ExpectedRevision {
		return domain.Projection{}, collaborationError("revision_conflict", "Note changed before replacement")
	}
	if err := atomicWriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return domain.Projection{}, err
	}
	note.Body = p.After
	event, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, c.OperationID, note, "")
	if err != nil {
		return domain.Projection{}, err
	}
	projection := domain.NewProjection("note.collaboration", "Note saved.")
	projection.Facts["path"], projection.Facts["note_id"] = note.Path, note.ID
	applyRecordEventFacts(&projection, event)
	if err := refreshIndex(root); err != nil {
		projection.Facts["index_status"] = "stale"
	}
	return projection, nil
}

func (s *Service) CollaborationStatus(ctx context.Context, root, id string) (operation.View, error) {
	root, err := cleanVaultPath(root)
	if err != nil {
		return operation.View{}, err
	}
	row, err := loadOperation(ctx, OperationRequest{VaultPath: root, OperationID: id, Access: OperationAccess{OwnerLocal: true}})
	if err != nil {
		return operation.View{}, err
	}
	if row.CapabilityID != "note.collaboration" {
		return operation.View{}, collaborationError("operation_not_found", "Collaboration operation was not found")
	}
	return operation.ViewFromRow(row), nil
}

func (s *Service) CollaborationVersion(ctx context.Context, root, id string, policy CollaborationPolicy) (domain.Note, string, error) {
	if !policy.AllowBody {
		return domain.Note{}, "", collaborationError("body_disabled", "The owner has not enabled note body access")
	}
	view, err := s.CollaborationStatus(ctx, root, id)
	if err != nil {
		return domain.Note{}, "", err
	}
	if !strings.HasPrefix(view.ReceiptRef, "snapshot:") {
		return domain.Note{}, "", collaborationError("snapshot_unavailable", "This operation has no recovery snapshot")
	}
	var result map[string]string
	if err := json.Unmarshal(view.Result, &result); err != nil || result["path"] == "" {
		return domain.Note{}, "", collaborationError("snapshot_unavailable", "Recovery note reference is unavailable")
	}
	revision := strings.TrimPrefix(view.ReceiptRef, "snapshot:")
	projection, err := s.VersionShow(ctx, VersionShowRequest{VaultPath: root, Path: result["path"], Revision: revision})
	if err != nil {
		return domain.Note{}, "", err
	}
	data, _ := projection.Data.(map[string]any)
	file, ok := data["file"].(domain.VersionedFile)
	if !ok || len(file.Content) > CollaborationMaxBodyBytes {
		return domain.Note{}, "", collaborationError("snapshot_unavailable", "Recovery content is unavailable or exceeds the interactive limit")
	}
	return parseNote(result["path"], file.Content), revision, nil
}

func CollaborationErrorCode(err error) string {
	var opErr *operation.Error
	if errors.As(err, &opErr) {
		return string(opErr.Code)
	}
	if code := domain.ErrorCode(err); code != "" {
		return code
	}
	return "collaboration_failed"
}

func CollaborationSafeError(err error) string {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) && !strings.ContainsAny(commandErr.Message, "/\\") {
		return commandErr.Message
	}
	return fmt.Sprintf("Operation failed (%s); inspect the original operation or refresh the preview.", CollaborationErrorCode(err))
}
