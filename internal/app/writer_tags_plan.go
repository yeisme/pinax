package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/operation"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Owner-only types: public transport contracts and apply are separate slices.
// Selection is explicit; this is never a claim to have migrated a whole vault.
type WriterTagTarget struct {
	ObjectID string `json:"object_id"`
	Revision string `json:"revision"`
}

type WriterTagsRequest struct {
	VaultPath     string
	Session       ReaderSessionInfo
	OperationID   string
	PreviewSecret []byte
}

type WriterTagsPlanRequest struct {
	Writer    WriterTagsRequest
	Operation string
	OldTag    string
	NewTag    string
	Targets   []WriterTagTarget
}

type WriterTagPlanItem struct {
	ObjectID string   `json:"object_id"`
	Revision string   `json:"revision"`
	Path     string   `json:"path"`
	Before   []string `json:"before"`
	After    []string `json:"after"`
}

type WriterTagsPlan struct {
	SchemaVersion string              `json:"schema_version"`
	OperationID   string              `json:"operation_id"`
	Operation     string              `json:"operation"`
	OldTag        string              `json:"old_tag"`
	OldTagID      string              `json:"old_tag_id"`
	NewTag        string              `json:"new_tag"`
	NewTagID      string              `json:"new_tag_id"`
	Items         []WriterTagPlanItem `json:"items"`
	Digest        string              `json:"preview_digest"`
}

const writerTagsSourceBudget = 1024 * 1024

// Tags bind the complete granted scope set, unlike the legacy writer-only
// receipt identity. This does not change older writer/capture/daily operations.
func writerTagsOperationAccess(session ReaderSessionInfo) RemoteOperationIdentity {
	access := WriterOperationAccess(session.SessionID, session.BindingID, session.Scopes)
	scopes := append([]string(nil), session.Scopes...)
	sort.Strings(scopes)
	access.Access.ScopeDigest = operation.IdentityDigest(append([]string{"pinax-writer-tags-scopes-v1"}, scopes...)...)
	return access
}

func writerTagsGate(req WriterTagsRequest) error {
	if err := writerTagsIdentityGate(req); err != nil {
		return err
	}
	if len(req.PreviewSecret) < 16 {
		return collaborationError("writer_unavailable", "Tag plans require a supervised preview key")
	}
	return nil
}

func writerTagsIdentityGate(req WriterTagsRequest) error {
	if !req.Session.HasScope(WriterScopeEditor) || !req.Session.HasScope(ReaderScopeBody) || !req.Session.HasScope(ReaderScopeMetadata) {
		return collaborationError("insufficient_scope", "Tag plans require writer.editor, reader.body and reader.metadata")
	}
	if req.Session.SessionID == "" || req.Session.BindingID == "" {
		return collaborationError("writer_unavailable", "Tag operations require a supervised session")
	}
	access := writerTagsOperationAccess(req.Session)
	return operation.ValidateCreateRequest(operation.CreateRequest{OperationID: req.OperationID, IdempotencyKey: req.OperationID, CapabilityID: "writer.tags.apply", BindingID: "rest.writer.tags.apply", PrincipalDigest: access.Access.PrincipalDigest, ScopeDigest: access.Access.ScopeDigest, RequestDigest: collaborationRevision("tag-plan-validation")})
}

func writerTagsPlanDigest(root string, req WriterTagsRequest, plan WriterTagsPlan) string {
	plan.Digest = ""
	encoded, _ := json.Marshal([]any{"pinax.writer.tags.plan.v1", root, req.Session.SessionID, req.Session.BindingID, req.Session.scopeDigest(), plan})
	mac := hmac.New(sha256.New, req.PreviewSecret)
	_, _ = mac.Write(encoded)
	return "sha256:" + hex.EncodeToString(mac.Sum(nil))
}

// WriterTagsPlan prepares tag-only differences from exact owner source bytes.
// Nothing is persisted, including operation identities or proposal contents.
func (s *Service) WriterTagsPlan(ctx context.Context, req WriterTagsPlanRequest) (WriterTagsPlan, error) {
	var empty WriterTagsPlan
	if err := writerTagsGate(req.Writer); err != nil {
		return empty, err
	}
	old, ok := canonicalReaderTag(req.OldTag)
	if !ok || old != req.OldTag || len(req.Targets) == 0 || len(req.Targets) > 32 {
		return empty, collaborationError("writer_request_invalid", "Select 1 to 32 fixed objects and a canonical tag path")
	}
	next := ""
	switch req.Operation {
	case "rename":
		next, ok = canonicalReaderTag(req.NewTag)
		if !ok || next != req.NewTag || next == old {
			return empty, collaborationError("writer_request_invalid", "Rename requires a different canonical tag path")
		}
	case "delete":
		if req.NewTag != "" {
			return empty, collaborationError("writer_request_invalid", "Delete does not accept a new tag")
		}
	default:
		return empty, collaborationError("writer_request_invalid", "Tag plan operation must be rename or delete")
	}
	targets := append([]WriterTagTarget(nil), req.Targets...)
	sort.Slice(targets, func(i, j int) bool { return targets[i].ObjectID < targets[j].ObjectID })
	for i, target := range targets {
		if target.ObjectID == "" || target.Revision == "" || (i > 0 && targets[i-1].ObjectID == target.ObjectID) {
			return empty, collaborationError("writer_request_invalid", "Targets require unique identities and fixed revisions")
		}
	}
	root, err := cleanVaultPath(req.Writer.VaultPath)
	if err != nil {
		return empty, err
	}
	notes, _, _, err := navigationSnapshot(ctx, root, false, false)
	if err != nil {
		return empty, err
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		return empty, collaborationError("writer_unavailable", "The authorized vault is unavailable")
	}
	defer func() { _ = opened.Close() }()
	rootIdentity, err := opened.Stat(".")
	if err != nil {
		return empty, collaborationError("writer_unavailable", "The authorized vault is unavailable")
	}
	plan := WriterTagsPlan{SchemaVersion: catalogschema.WriterTagsPlanV1Alpha1, OperationID: req.Writer.OperationID, Operation: req.Operation, OldTag: old, OldTagID: readerTagID(old), NewTag: next, Items: make([]WriterTagPlanItem, 0, len(targets))}
	if next != "" {
		plan.NewTagID = readerTagID(next)
	}
	total := 0
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		source, err := s.WriterEditorSource(ctx, WriterEditorSourceRequest{VaultPath: root, Session: req.Writer.Session, ObjectID: target.ObjectID, Revision: target.Revision})
		if err != nil {
			return empty, err
		}
		if source.ProtectedMetadata.Kind == "daily" {
			return empty, collaborationError("note_not_found", "Daily objects require their dedicated owner workflow")
		}
		if !source.Editable {
			return empty, collaborationError("frontmatter_unsupported", "Selected metadata cannot be edited losslessly")
		}
		note, found := findNoteByObjectID(notes, target.ObjectID)
		if !found || readerNoteRevision(note) != target.Revision {
			return empty, collaborationError("revision_conflict", "Selected source changed during planning")
		}
		raw, err := readWriterEditorCandidate(opened, note.Path)
		if err != nil {
			return empty, err
		}
		if len(raw) > CollaborationMaxBodyBytes || len(raw) > writerTagsSourceBudget-total {
			return empty, collaborationError("body_too_large", "Selected sources exceed the tag plan byte budget")
		}
		total += len(raw)
		if collaborationRevision(string(raw)) != target.Revision {
			return empty, collaborationError("revision_conflict", "Selected source changed during planning")
		}
		before, tagErr := normalizeTagsForWrite(source.ProtectedMetadata.Tags)
		if tagErr != nil {
			return empty, tagErr
		}
		if len(before) > 32 || !containsString(before, old) {
			return empty, collaborationError("writer_request_invalid", "Every selected note must contain the exact source tag within the tag budget")
		}
		for _, tag := range before {
			if normalized, valid := canonicalReaderTag(tag); !valid || normalized != tag {
				return empty, collaborationError("frontmatter_unsupported", "Existing tags must be canonical before planning")
			}
		}
		after := removeTags(before, []string{old})
		if next != "" {
			after = mergeTags(after, []string{next})
		}
		if after == nil {
			after = []string{}
		}
		if _, err := patchTagMetadata(string(raw), formatTags(after), "2000-01-01T00:00:00Z"); err != nil {
			return empty, err
		}
		// Bind source identity as well as bytes; no path-only target may be signed.
		meta, _ := splitFrontmatter(string(raw))
		if meta["note_id"] != target.ObjectID || strings.Join(parseTags(meta["tags"]), "\x00") != strings.Join(source.ProtectedMetadata.Tags, "\x00") {
			return empty, collaborationError("frontmatter_unsupported", "Source metadata parsers disagree")
		}
		plan.Items = append(plan.Items, WriterTagPlanItem{ObjectID: target.ObjectID, Revision: target.Revision, Path: note.Path, Before: before, After: after})
	}
	// Recheck all selected bytes after scanning, without opening an index writer.
	for _, item := range plan.Items {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		raw, err := readWriterEditorCandidate(opened, item.Path)
		if err != nil {
			return empty, err
		}
		if collaborationRevision(string(raw)) != item.Revision {
			return empty, collaborationError("revision_conflict", "Selected source changed before the plan was returned")
		}
	}
	currentRoot, err := os.Stat(root)
	if err != nil || !os.SameFile(rootIdentity, currentRoot) {
		return empty, collaborationError("source_changed", "The authorized vault changed during planning")
	}
	plan.Digest = writerTagsPlanDigest(root, req.Writer, plan)
	wire, err := json.Marshal(plan)
	if err != nil || len(wire) > 128*1024 {
		return empty, collaborationError("body_too_large", "Tag plan exceeds its preview budget")
	}
	var payload any
	if json.Unmarshal(wire, &payload) != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterTagsPlanV1Alpha1, payload) != nil {
		return empty, collaborationError("writer_request_invalid", "Tag plan cannot be represented by the public contract")
	}
	return plan, nil
}
