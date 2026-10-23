package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/notelinks"
	"github.com/yeisme/pinax/internal/render"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Internal application projection; no public binding is registered yet.
type ReaderRichAnnotation struct {
	Anchor      string   `json:"anchor"`
	PageAnchors []string `json:"page_anchors"`
	Kind        string   `json:"kind"`
	Source      string   `json:"source,omitempty"`
	CalloutKind string   `json:"callout_kind,omitempty"`
	Title       string   `json:"title,omitempty"`
	Fold        string   `json:"fold,omitempty"`
}
type ReaderRichDocumentPage struct {
	SchemaVersion string                 `json:"schema_version"`
	Document      ReaderDocumentPage     `json:"document"`
	Annotations   []ReaderRichAnnotation `json:"annotations"`
}

// Cursor keys are domain-separated from legacy document/outline cursors and
// bound to the selected object and source binding, even for identical bytes.
func richDocumentCursorKey(req ReaderDocumentRequest) []byte {
	mac := hmac.New(sha256.New, req.CursorSecret)
	_, _ = mac.Write([]byte("pinax.rich-document.v1\x00" + req.Session.BindingID + "\x00" + req.ObjectID))
	return mac.Sum(nil)
}

func (s *Service) ReaderRichDocument(ctx context.Context, req ReaderDocumentRequest) (domain.Projection, error) {
	return s.readerRichDocument(ctx, req, false)
}

func (s *Service) readerRichDocument(ctx context.Context, req ReaderDocumentRequest, composed bool) (domain.Projection, error) {
	command := "reader.rich_document"
	if composed {
		command = "reader.composed_document"
	}
	fail := func(code, message string) (domain.Projection, error) {
		err := ReaderError(code, message, "")
		return domain.NewErrorProjection(command, err), err
	}
	if err := ctx.Err(); err != nil {
		return errorProjection(command, err), err
	}
	if !req.Session.HasScope(ReaderScopeBody) {
		return fail("insufficient_scope", "Rich documents require reader.body")
	}
	if req.Session.SessionID == "" || req.Session.BindingID == "" || len(req.CursorSecret) < 16 {
		return fail("reader_unavailable", "Rich documents require a supervised session and cursor key")
	}
	if req.ObjectID == "" || len(req.ObjectID) > 256 || strings.TrimSpace(req.ObjectID) != req.ObjectID || len(req.Revision) != 71 || !strings.HasPrefix(req.Revision, "sha256:") {
		return fail("reader_request_invalid", "Rich documents require a fixed object and revision")
	}
	for _, ch := range req.Revision[7:] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return fail("reader_request_invalid", "Invalid fixed revision")
		}
	}
	if len(req.Cursor) > 2048 || len(req.OutlineCursor) > 2048 || len(req.Anchor) > 512 || (req.Cursor != "" && req.Anchor != "") {
		return fail("reader_request_invalid", "Invalid rich document position")
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes, watermark, _, err := navigationSnapshot(ctx, root, false, false)
	if err != nil {
		return errorProjection(command, err), err
	}
	live, found := findNoteByObjectID(notes, req.ObjectID)
	if !found {
		return fail("reader_object_not_found", "The requested note is not visible")
	}
	if live.OversizedSource && readerNoteRevision(live) == req.Revision {
		return fail("document_too_large", "Source exceeds the interactive document budget")
	}
	note, readable := s.readerNoteAtRevision(ctx, root, live, req.Revision)
	if !readable {
		return fail("revision_unavailable", "The requested revision is not available")
	}
	var rich render.ReaderRichDocument
	var composition *readerComposition
	if composed {
		composition = &readerComposition{ctx: ctx, root: root, notes: notes, snapshot: notelinks.BuildResolverSnapshot(notes), origins: map[string]ReaderNodeOrigin{}, placeholders: map[string]string{}}
		rich, _, err = composition.build(note, "", map[string]domain.Note{note.ID: note}, "")
	} else {
		rich, err = render.BuildReaderRichDocument(note.Body)
		if err == nil {
			err = resolveReaderAssetRefs(root, note, &rich.Document)
		}
		if err == nil {
			err = resolveReaderNoteRefs(note, notes, &rich.Document)
		}
	}
	if err != nil {
		return readerDocumentBudgetProjection(command, err, req.ObjectID, req.Revision, len(note.Body))
	}
	// Expanded occurrences share the same annotation budget, not a budget per note.
	payloadBytes := 0
	for _, annotation := range rich.Annotations {
		payloadBytes += len(annotation.Source) + len(annotation.Title)
	}
	if len(rich.Annotations) > catalogschema.ReaderRichMaxAnnotations || payloadBytes > catalogschema.ReaderRichTotalPayloadBytes {
		return fail("document_budget_exceeded", "Composed annotations exceed the document budget")
	}
	// Resolution can add nodes from visible targets. Bind the corpus watermark
	// so a changed target cannot silently shift a continuation position.
	cursorRequest := req
	cursorRequest.CursorSecret = []byte(readerNavigationDigest([]any{req.CursorSecret, watermark}))
	if composed {
		cursorRequest.CursorSecret = []byte(readerNavigationDigest([]any{"composed-document.v1", cursorRequest.CursorSecret}))
	}
	secret := richDocumentCursorKey(cursorRequest)
	position := render.ReaderPagePosition{}
	if req.Cursor != "" {
		cursor, err := decodeReaderDocumentCursor(req.Cursor, secret, req.Session, req.Revision)
		if err != nil {
			return errorProjection(command, err), err
		}
		position = render.ReaderPagePosition{BlockIndex: cursor.BlockIndex, Path: cursor.Path, DroppedBlocks: cursor.DroppedBlocks}
	} else if req.Anchor != "" {
		index := render.LocateReaderAnchor(rich.Document, req.Anchor)
		if index < 0 {
			return fail("reader_request_invalid", "Unknown anchor for the fixed revision")
		}
		position.BlockIndex = index
	}
	rendered := render.PaginateReaderDocument(rich.Document, position)
	page := ReaderDocumentPage{SchemaVersion: "pinax.reader.document.v1alpha1", ObjectID: req.ObjectID, Revision: req.Revision, CurrentRevision: readerNoteRevision(live), URI: "pinax://note/" + req.ObjectID + "?v=" + req.Revision, Nodes: rendered.Nodes, Complete: rendered.Complete, ContentBytes: rendered.ContentBytes, ObservedAt: readerObservedAt(s.currentTimeUTC()), Warnings: readerDocumentWarnings(rendered.Warnings)}
	if rendered.Next != nil {
		page.NextCursor, err = encodeReaderDocumentCursor(readerDocumentCursor{Version: 1, SessionDigest: req.Session.scopeDigest(), Revision: req.Revision, BlockIndex: rendered.Next.BlockIndex, Path: rendered.Next.Path, DroppedBlocks: rendered.Next.DroppedBlocks}, secret, req.Session)
		if err != nil {
			return errorProjection(command, err), err
		}
	}
	offset := 0
	if req.OutlineCursor != "" {
		var e *domain.CommandError
		offset, e = decodeReaderOutlineCursor(req.OutlineCursor, secret, req.Session, req.Revision)
		if e != nil {
			return domain.NewErrorProjection(command, e), e
		}
	}
	entries, next := render.ReaderOutlinePage(rich.Document, offset)
	if entries == nil {
		entries = []render.ReaderOutlineEntry{}
	}
	page.Outline = entries
	if next > 0 {
		page.OutlineCursor, err = encodeReaderCursor(readerCursor{Version: 1, SessionDigest: req.Session.scopeDigest(), QueryDigest: "outline:" + req.Revision, Watermark: req.Revision, LastSortValue: strconv.Itoa(next)}, secret, req.Session)
		if err != nil {
			return errorProjection(command, err), err
		}
	}
	result := ReaderRichDocumentPage{SchemaVersion: catalogschema.ReaderRichDocumentV1Alpha1, Document: page, Annotations: []ReaderRichAnnotation{}}
	// The legacy paginator can emit descendants without their quote wrapper.
	// Match exact registered descendant anchors, never guess fragment prefixes.
	for _, annotation := range rich.Annotations {
		owned := map[string]bool{}
		var descendants func([]render.ReaderDocumentNode, bool)
		descendants = func(nodes []render.ReaderDocumentNode, inside bool) {
			for _, node := range nodes {
				active := inside || node.Anchor == annotation.Anchor
				if active {
					owned[node.Anchor] = true
				}
				descendants(node.Children, active)
			}
		}
		descendants(rich.Document.Blocks, false)
		pageAnchors := []string{}
		var match func([]render.ReaderDocumentNode)
		match = func(nodes []render.ReaderDocumentNode) {
			for _, node := range nodes {
				if owned[node.Anchor] || (node.BlockAnchor != "" && owned[node.BlockAnchor]) {
					pageAnchors = append(pageAnchors, node.Anchor)
				}
				match(node.Children)
			}
		}
		match(page.Nodes)
		sort.Strings(pageAnchors)
		if len(pageAnchors) > 0 {
			result.Annotations = append(result.Annotations, ReaderRichAnnotation{Anchor: annotation.Anchor, PageAnchors: pageAnchors, Kind: annotation.Kind, Source: annotation.Source, CalloutKind: annotation.CalloutKind, Title: annotation.Title, Fold: annotation.Fold})
		}
	}
	projection := domain.NewProjection(command, "Fixed rich document page composed without executing fences.")
	if err := validateRichDocumentPage(result); err != nil {
		return fail("reader_unavailable", "Rich document projection violated its contract")
	}
	projection.Data = result
	if composition != nil {
		composedPage, err := composition.page(result)
		if err != nil {
			return fail("reader_unavailable", "Composed projection violated its contract")
		}
		projection.Data = composedPage
	}
	encoded, err := json.Marshal(projection)
	if err != nil || len(encoded) > 1024*1024 {
		return fail("document_budget_exceeded", "Rich document response exceeds the wire budget")
	}
	_, currentWatermark, _, err := navigationSnapshot(ctx, root, false, false)
	if err != nil {
		return errorProjection(command, err), err
	}
	if currentWatermark != watermark {
		return fail("source_changed", "Visible source changed while composing the rich page")
	}
	if err := ctx.Err(); err != nil {
		return errorProjection(command, err), err
	}
	return projection, nil
}
