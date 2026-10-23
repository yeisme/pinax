package app

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/notelinks"
	"github.com/yeisme/pinax/internal/render"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

// Human-reading document lane (pinax-human-reading-api-v1 P03): fixed
// revision, bounded pages, continuation fragments and outline cursors. The
// note body keeps user-written paths and code verbatim; it is never passed
// through the Agent SafeText redaction. No snapshots are ever created; a
// pinned revision whose live body changed continues from existing snapshots
// when readable and fails with 409 otherwise.

// ReaderDocumentRequest serves one page of a note for human reading.
type ReaderDocumentRequest struct {
	VaultPath     string
	Session       ReaderSessionInfo
	ObjectID      string
	Revision      string
	Cursor        string
	Anchor        string
	OutlineCursor string
	CursorSecret  []byte
}

// ReaderDocumentPage is the /v1/reader/notes/{object_id}/document payload.
type ReaderDocumentPage struct {
	SchemaVersion   string                      `json:"schema_version"`
	ObjectID        string                      `json:"object_id"`
	Revision        string                      `json:"revision"`
	CurrentRevision string                      `json:"current_revision"`
	URI             string                      `json:"uri"`
	Nodes           []render.ReaderDocumentNode `json:"nodes"`
	Outline         []render.ReaderOutlineEntry `json:"outline"`
	OutlineCursor   string                      `json:"outline_cursor,omitempty"`
	NextCursor      string                      `json:"next_cursor,omitempty"`
	Complete        bool                        `json:"complete"`
	ContentBytes    int                         `json:"content_bytes"`
	ObservedAt      string                      `json:"observed_at"`
	Warnings        []map[string]string         `json:"warnings,omitempty"`
}

// readerDocumentCursor is the signed pagination state for document pages.
type readerDocumentCursor struct {
	Version       int    `json:"v"`
	SessionDigest string `json:"session"`
	Revision      string `json:"revision"`
	BlockIndex    int    `json:"block"`
	Path          string `json:"path"`
	DroppedBlocks int    `json:"dropped"`
}

func encodeReaderDocumentCursor(cursor readerDocumentCursor, secret []byte, session ReaderSessionInfo) (string, error) {
	return encodeReaderCursor(readerCursor{
		Version:       cursor.Version,
		SessionDigest: session.scopeDigest(),
		QueryDigest:   fmt.Sprintf("document:%s:%s", cursor.SessionDigest, cursor.Revision),
		Watermark:     cursor.Revision,
		LastSortValue: fmt.Sprintf("%d\x1f%d\x1f%s", cursor.BlockIndex, cursor.DroppedBlocks, cursor.Path),
		LastObjectID:  "",
	}, secret, session)
}

// decodeReaderDocumentCursor verifies signature, session and revision, then
// recovers the page position. Revision drift is surfaced as source_changed.
func decodeReaderDocumentCursor(encoded string, secret []byte, session ReaderSessionInfo, revision string) (readerDocumentCursor, error) {
	base, err := decodeReaderCursor(encoded, secret, session, fmt.Sprintf("document:%s:%s", session.scopeDigest(), revision), revision)
	if err != nil {
		if commandErr, ok := err.(*domain.CommandError); ok && commandErr.Code == "reader_cursor_expired" {
			return readerDocumentCursor{}, ReaderError("revision_unavailable", "The pinned revision changed; re-open the document at its current revision", "")
		}
		return readerDocumentCursor{}, err
	}
	parts := strings.SplitN(base.LastSortValue, "\x1f", 3)
	if len(parts) != 3 {
		return readerDocumentCursor{}, ReaderError("reader_cursor_invalid", "Reader document cursor position is malformed", "")
	}
	blockIndex, blockErr := strconv.Atoi(parts[0])
	droppedBlocks, dropErr := strconv.Atoi(parts[1])
	if blockErr != nil || dropErr != nil || blockIndex < 0 || droppedBlocks < 0 {
		return readerDocumentCursor{}, ReaderError("reader_cursor_invalid", "Reader document cursor position is malformed", "")
	}
	return readerDocumentCursor{Version: 1, SessionDigest: base.SessionDigest, Revision: revision, BlockIndex: blockIndex, Path: parts[2], DroppedBlocks: droppedBlocks}, nil
}

// readerNoteAtRevision returns the note content pinned at the requested
// body revision: the live note when it matches, otherwise the newest
// existing snapshot file whose content hash equals the revision. The bool
// reports whether the pinned revision is readable at all.
func (s *Service) readerNoteAtRevision(ctx context.Context, root string, live domain.Note, revision string) (domain.Note, bool) {
	if revision == "" || readerNoteRevision(live) == revision {
		return live, true
	}
	snapshots, err := loadVersionSnapshots(root, 0)
	if err != nil {
		return domain.Note{}, false
	}
	for _, snapshot := range snapshots {
		if err := ctx.Err(); err != nil {
			return domain.Note{}, false
		}
		for _, fact := range snapshot.FileFacts {
			if fact.Path != live.Path {
				continue
			}
			// Snapshot hashes cover the whole file (frontmatter included);
			// the reader revision pins the parsed body, so the body digest
			// is verified after reading.
			file, readErr := s.versionBackend.ReadFile(ctx, pinaxversion.ReadFileRequest{Root: root, Path: fact.Path, Revision: snapshot.SnapshotID})
			if readErr != nil {
				continue
			}
			pinned := parseNote(fact.Path, file.Content)
			pinned.ContentDigest = collaborationRevision(file.Content)
			if pinned.ID != live.ID || readerNoteRevision(pinned) != revision {
				continue
			}
			return pinned, true
		}
	}
	return domain.Note{}, false
}

// ReaderDocument serves one bounded page. The first request resolves the
// current revision unless an explicit one is pinned; every later page is
// validated against the same revision without ever snapshotting.
func (s *Service) ReaderDocument(ctx context.Context, req ReaderDocumentRequest) (domain.Projection, error) {
	command := "reader.document"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if !req.Session.HasScope(ReaderScopeBody) {
		err := ReaderError("insufficient_scope", "reader.document requires the reader.body scope", "")
		return domain.NewErrorProjection(command, err), err
	}
	objectID := strings.TrimSpace(req.ObjectID)
	if objectID == "" {
		err := ReaderError("reader_request_invalid", "object_id is required", "")
		return domain.NewErrorProjection(command, err), err
	}
	if req.Cursor != "" && req.Anchor != "" {
		err := ReaderError("reader_request_invalid", "cursor and anchor are mutually exclusive", "")
		return domain.NewErrorProjection(command, err), err
	}

	notes, err := readerScanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	note, found := findNoteByObjectID(ordinaryNotes(notes), objectID)
	if !found {
		err := ReaderError("reader_object_not_found", "The requested note is not visible", "")
		return domain.NewErrorProjection(command, err), err
	}
	return s.readerDocumentSelected(ctx, req, root, note, notes)
}

// readerDocumentSelected renders an already-authorized note; callers own
// selection policy. It does not grant access to system objects by itself.
func (s *Service) readerDocumentSelected(ctx context.Context, req ReaderDocumentRequest, root string, note domain.Note, notes []domain.Note) (domain.Projection, error) {
	command, objectID := "reader.document", note.ID
	currentRevision := readerNoteRevision(note)

	// Interactive source budget: a raw source beyond the budget is never
	// parsed (the scan never even loaded it); the live revision fails fast
	// with the recovery facts. Pinned revisions still try snapshots.
	if note.OversizedSource && (req.Revision == "" || req.Revision == currentRevision) {
		commandErr := ReaderError("document_too_large", "The document exceeds the interactive reading budget; use the explicit source export instead", "")
		return readerBudgetErrorProjection(command, commandErr, objectID, currentRevision, int(note.RawSizeBytes)), commandErr
	}

	// Cursor requests must match the revision they were issued against.
	if req.Cursor != "" {
		cursor, cursorErr := decodeReaderDocumentCursor(req.Cursor, req.CursorSecret, req.Session, req.Revision)
		if cursorErr != nil {
			if commandErr, ok := cursorErr.(*domain.CommandError); ok {
				return domain.NewErrorProjection(command, commandErr), cursorErr
			}
			return errorProjection(command, cursorErr), cursorErr
		}
		pinnedNote := note
		if cursor.Revision != currentRevision {
			// Contract: a pinned revision continues from existing snapshots
			// when readable; only an unreadable original fails with 409.
			snapshotted, readable := s.readerNoteAtRevision(ctx, root, note, cursor.Revision)
			if !readable {
				err := ReaderError("revision_unavailable", "The pinned revision changed and is not recoverable from snapshots; re-open the document at its current revision", "")
				return domain.NewErrorProjection(command, err), err
			}
			pinnedNote = snapshotted
		}
		page, pageErr := s.renderReaderDocumentPage(ctx, pinnedNote, notes, currentRevision, req, cursor.BlockIndex, cursor.Path, cursor.DroppedBlocks)
		if pageErr != nil {
			if commandErr, ok := pageErr.(*domain.CommandError); ok {
				if isReaderBudgetCode(commandErr.Code) {
					return readerBudgetErrorProjection(command, commandErr, objectID, cursor.Revision, len(pinnedNote.Body)), pageErr
				}
				return domain.NewErrorProjection(command, commandErr), pageErr
			}
			return errorProjection(command, pageErr), pageErr
		}
		return readerDocumentProjection(command, req, page, len(pinnedNote.Body)), nil
	}

	// Explicit revision: the live body wins; otherwise the pinned revision
	// continues from existing snapshots, and only an unreadable original
	// fails with 409 (contract: no mixed pages, no silent snapshots).
	revision := strings.TrimSpace(req.Revision)
	pinnedNote := note
	if revision != "" && revision != currentRevision {
		snapshotted, readable := s.readerNoteAtRevision(ctx, root, note, revision)
		if !readable {
			err := ReaderError("revision_unavailable", "The requested revision is no longer available", "Re-open the note at its current revision")
			return domain.NewErrorProjection(command, err), err
		}
		pinnedNote = snapshotted
	}

	position := render.ReaderPagePosition{}
	if req.Anchor != "" {
		// Anchors bind to the serving revision: build the index from the
		// pinned body (which may differ from the live one) so old outlines
		// resolve on the old content.
		document, docErr := s.buildReaderDocumentForSession(ctx, root, pinnedNote, notes, req.Session.Scopes)
		if docErr != nil {
			servingRevision := revision
			if servingRevision == "" {
				servingRevision = currentRevision
			}
			return readerDocumentBudgetProjection(command, docErr, objectID, servingRevision, len(pinnedNote.Body))
		}
		block := render.LocateReaderAnchor(document, req.Anchor)
		if block < 0 {
			err := ReaderError("reader_request_invalid", "unknown anchor for this revision", "")
			return domain.NewErrorProjection(command, err), err
		}
		position = render.ReaderPagePosition{BlockIndex: block}
	}

	page, pageErr := s.renderReaderDocumentPage(ctx, pinnedNote, notes, currentRevision, req, position.BlockIndex, "", 0)
	if pageErr != nil {
		if commandErr, ok := pageErr.(*domain.CommandError); ok {
			if isReaderBudgetCode(commandErr.Code) {
				servingRevision := revision
				if servingRevision == "" {
					servingRevision = currentRevision
				}
				return readerBudgetErrorProjection(command, commandErr, objectID, servingRevision, len(pinnedNote.Body)), pageErr
			}
			return domain.NewErrorProjection(command, commandErr), pageErr
		}
		return errorProjection(command, pageErr), pageErr
	}
	return readerDocumentProjection(command, req, page, len(pinnedNote.Body)), nil
}

// renderReaderDocumentPage parses, budget-checks and paginates one page.
func (s *Service) renderReaderDocumentPage(ctx context.Context, note domain.Note, notes []domain.Note, liveRevision string, req ReaderDocumentRequest, blockIndex int, path string, droppedBlocks int) (ReaderDocumentPage, error) {
	document, err := s.buildReaderDocumentForSession(ctx, req.VaultPath, note, notes, req.Session.Scopes)
	if err != nil {
		if budgetErr, ok := err.(*render.ReaderBudgetError); ok {
			code := "document_budget_exceeded"
			if budgetErr.Code == "document_too_large" {
				code = "document_too_large"
			}
			return ReaderDocumentPage{}, ReaderError(code, "The document exceeds the interactive reading budget; use the explicit source export instead", "")
		}
		return ReaderDocumentPage{}, ReaderError("reader_unavailable", "The document could not be parsed", "")
	}
	// NOTE: asset ref resolution runs exactly once inside
	// buildReaderDocumentForServing; resolving again here would duplicate
	// the generated image nodes and double-count them against the budget.
	revision := strings.TrimSpace(req.Revision)
	if revision == "" {
		revision = liveRevision
	}
	page := ReaderDocumentPage{
		SchemaVersion:   "pinax.reader.document.v1alpha1",
		ObjectID:        note.ID,
		Revision:        revision,
		CurrentRevision: liveRevision,
		URI:             "pinax://note/" + note.ID + "?v=" + revision,
		Nodes:           nil,
		Complete:        false,
		ContentBytes:    0,
		ObservedAt:      readerObservedAt(s.currentTimeUTC()),
	}

	rendered := render.PaginateReaderDocument(document, render.ReaderPagePosition{BlockIndex: blockIndex, Path: path, DroppedBlocks: droppedBlocks})
	page.Nodes = rendered.Nodes
	if page.Nodes == nil {
		page.Nodes = []render.ReaderDocumentNode{}
	}
	page.ContentBytes = rendered.ContentBytes
	page.Complete = rendered.Complete
	page.Warnings = readerDocumentWarnings(rendered.Warnings)
	if rendered.Next != nil {
		cursor, err := encodeReaderDocumentCursor(readerDocumentCursor{
			Version:       1,
			SessionDigest: req.Session.scopeDigest(),
			Revision:      revision,
			BlockIndex:    rendered.Next.BlockIndex,
			Path:          rendered.Next.Path,
			DroppedBlocks: rendered.Next.DroppedBlocks,
		}, req.CursorSecret, req.Session)
		if err != nil {
			return ReaderDocumentPage{}, err
		}
		page.NextCursor = cursor
	}

	outlineOffset := 0
	if req.OutlineCursor != "" {
		var outlineErr *domain.CommandError
		outlineOffset, outlineErr = decodeReaderOutlineCursor(req.OutlineCursor, req.CursorSecret, req.Session, revision)
		if outlineErr != nil {
			return ReaderDocumentPage{}, outlineErr
		}
	}
	entries, next := render.ReaderOutlinePage(document, outlineOffset)
	if entries == nil {
		entries = []render.ReaderOutlineEntry{}
	}
	page.Outline = entries
	if next > 0 {
		outlineCursor, err := encodeReaderCursor(readerCursor{
			Version:       1,
			SessionDigest: req.Session.scopeDigest(),
			QueryDigest:   "outline:" + revision,
			Watermark:     revision,
			LastSortValue: strconv.Itoa(next),
		}, req.CursorSecret, req.Session)
		if err != nil {
			return ReaderDocumentPage{}, err
		}
		page.OutlineCursor = outlineCursor
	}
	return page, nil
}

// decodeReaderOutlineCursor validates signature, session and revision
// before returning the outline offset; tampered or cross-revision cursors
// fail closed instead of silently restarting at the first outline page.
func decodeReaderOutlineCursor(encoded string, secret []byte, session ReaderSessionInfo, revision string) (int, *domain.CommandError) {
	invalid := ReaderError("reader_cursor_invalid", "outline cursor is malformed or bound to another session or revision", "Restart the outline from its first page")
	base, err := decodeReaderCursor(encoded, secret, session, "outline:"+revision, revision)
	if err != nil {
		return 0, invalid
	}
	offset, parseErr := strconv.Atoi(strings.TrimSpace(base.LastSortValue))
	if parseErr != nil || offset < 0 {
		return 0, invalid
	}
	return offset, nil
}

func readerDocumentProjection(command string, req ReaderDocumentRequest, page ReaderDocumentPage, sourceSizeBytes int) domain.Projection {
	projection := domain.NewProjection(command, "Reader document page composed.")
	projection.Facts["object_id"] = page.ObjectID
	projection.Facts["revision"] = page.Revision
	projection.Facts["current_revision"] = page.CurrentRevision
	projection.Facts["complete"] = fmt.Sprint(page.Complete)
	projection.Facts["content_bytes"] = fmt.Sprint(page.ContentBytes)
	projection.Facts["source_size_bytes"] = fmt.Sprint(sourceSizeBytes)
	projection.Data = map[string]any{"document": page}
	return projection
}

// readerDocumentBudgetProjection maps render budget errors to the contract.
// The error details carry machine-recoverable fields: the fixed revision,
// the source size in bytes, and the available source-export action.
func readerDocumentBudgetProjection(command string, err error, objectID, revision string, sourceSizeBytes int) (domain.Projection, error) {
	if budgetErr, ok := err.(*render.ReaderBudgetError); ok {
		code := "document_budget_exceeded"
		if budgetErr.Code == "document_too_large" {
			code = "document_too_large"
		}
		commandErr := ReaderError(code, "The document exceeds the interactive reading budget; use the explicit source export instead", "")
		projection := domain.NewErrorProjection(command, commandErr)
		projection.Facts["revision"] = revision
		projection.Facts["source_size_bytes"] = strconv.Itoa(sourceSizeBytes)
		projection.Actions = []domain.Action{{Name: "source_export", Command: "GET /v1/reader/notes/" + objectID + "/source?revision=" + revision}}
		return projection, commandErr
	}
	return domain.NewErrorProjection(command, &domain.CommandError{Code: "reader_unavailable", Message: "Reader document failed"}), err
}

// readerBudgetErrorProjection enriches any budget command error returned by
// the page renderer with the same machine-recoverable details.
func readerBudgetErrorProjection(command string, commandErr *domain.CommandError, objectID, revision string, sourceSizeBytes int) domain.Projection {
	projection := domain.NewErrorProjection(command, commandErr)
	projection.Facts["revision"] = revision
	projection.Facts["source_size_bytes"] = strconv.Itoa(sourceSizeBytes)
	projection.Actions = []domain.Action{{Name: "source_export", Command: "GET /v1/reader/notes/" + objectID + "/source?revision=" + revision}}
	return projection
}

func isReaderBudgetCode(code string) bool {
	return code == "document_too_large" || code == "document_budget_exceeded"
}

func readerDocumentWarnings(warnings []string) []map[string]string {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]map[string]string, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, map[string]string{"code": warning, "message": warning})
	}
	return out
}

func findNoteByObjectID(notes []domain.Note, objectID string) (domain.Note, bool) {
	for _, note := range notes {
		if note.ID == objectID {
			return note, true
		}
	}
	return domain.Note{}, false
}

// readerWikiAssetPattern matches wiki-style embeds and links in body text.
var readerWikiAssetPattern = regexp.MustCompile(`!?\[\[([^]|]+)(?:\|[^\]]*)?\]\]`)

// buildReaderDocumentForServing builds the renderer projection with the
// owner fence handlers and runs the owner asset resolution with its
// budget/anchor validation, so every serving path (anchor location, page
// rendering) sees the identical document.
// buildReaderDocumentForSession scopes the fence handlers to the session's
// grants so a body-only reader cannot obtain relations or cross-note
// metadata projections through fence payloads.
func (s *Service) buildReaderDocumentForSession(ctx context.Context, root string, note domain.Note, notes []domain.Note, scopes []string) (render.ReaderDocument, error) {
	document, err := render.BuildReaderDocumentWithFences(note.Body, s.buildScopedFenceHandlers(ctx, root, note, notes, scopes))
	if err != nil {
		return document, err
	}
	if resolveErr := resolveReaderAssetRefs(root, note, &document); resolveErr != nil {
		if _, ok := resolveErr.(*render.ReaderBudgetError); ok {
			return document, resolveErr
		}
		return document, nil // non-budget resolver failures stay non-fatal
	}
	if noteRefErr := resolveReaderNoteRefs(note, notes, &document); noteRefErr != nil {
		if _, ok := noteRefErr.(*render.ReaderBudgetError); ok {
			return document, noteRefErr
		}
		return document, nil // non-budget resolver failures stay non-fatal
	}
	return document, nil
}

// readerWikiNotePattern matches non-embed wiki note links in body text.
var readerWikiNotePattern = regexp.MustCompile(`(!?)\[\[([^|\]]+)(?:\|([^\]]+))?\]\]`)

// resolveReaderNoteRefs gives in-body note references their owner-resolved
// semantics via the notelinks resolver (note_id, path, exact title with
// ambiguity handling, frontmatter aliases/stems): wiki links and markdown
// .md links to visible notes become typed link children carrying stable
// pinax://note URIs. Inline-code tokens (Tokens ranges), fenced code and
// fence nodes are never resolved; ambiguous or invisible targets stay plain
// text without existence leaks.
func resolveReaderNoteRefs(source domain.Note, notes []domain.Note, document *render.ReaderDocument) error {
	return resolveReaderNoteRefsWithBudget(source, notes, document, &readerHeadingRefs{})
}

func resolveReaderNoteRefsWithBudget(source domain.Note, notes []domain.Note, document *render.ReaderDocument, headingRefs *readerHeadingRefs) error {
	snapshot := notelinks.BuildResolverSnapshot(notes)
	claim := func(base string, topIndex int) string {
		anchor := base
		for suffix := 2; ; suffix++ {
			if _, exists := document.AnchorIndex[anchor]; !exists {
				break
			}
			anchor = fmt.Sprintf("%s-%d", base, suffix)
		}
		document.AnchorIndex[anchor] = topIndex
		return anchor
	}
	var resolutionErr error
	resolvedTarget := func(raw notelinks.RawLink) (domain.Note, bool) {
		if raw.Target == "" && raw.Heading != "" {
			return source, true
		}
		result := notelinks.ResolveLinkTarget(source, raw, snapshot)
		if result.Link.Status != string(domain.LinkStatusResolved) || result.Link.TargetObjectID == "" {
			return domain.Note{}, false // ambiguous/broken/unresolvable stay text
		}
		if result.Link.TargetObjectID == source.ID && raw.Heading != "" {
			// The serving source may be a pinned snapshot, not the live entry.
			return source, true
		}
		for _, candidate := range notes {
			if candidate.ID == result.Link.TargetObjectID && candidate.ID != source.ID {
				return candidate, true
			}
		}
		return domain.Note{}, false
	}
	// insideToken reports whether byte offset falls in an inline-code span.
	insideToken := func(tokens [][2]int, offset int) bool {
		for _, token := range tokens {
			if token[0] <= offset && offset < token[1] {
				return true
			}
		}
		return false
	}
	var walk func(node *render.ReaderDocumentNode, topIndex int) bool
	walk = func(node *render.ReaderDocumentNode, topIndex int) bool {
		// Markdown link refs resolve through the owner resolver (relative
		// path semantics included).
		if node.Type == "link" && node.Ref != "" {
			cleanTarget, heading := notelinks.NormalizeMarkdownLinkTarget(node.Ref)
			if strings.HasPrefix(node.Ref, "#") {
				heading = strings.TrimPrefix(node.Ref, "#")
			}
			raw := notelinks.RawLink{Kind: "markdown", Target: cleanTarget, Heading: heading}
			if target, ok := resolvedTarget(raw); ok {
				uri, found, err := headingRefs.uri(target, heading)
				if err != nil {
					resolutionErr = err
					return false
				}
				if found {
					node.Ref = uri
				} else if heading != "" {
					node.Type = "unsupported"
					node.Ref = ""
				}
			}
		}
		// Wiki resolution applies to block-text contexts: paragraphs,
		// headings, table bodies (and their continuations). Code/fence
		// contexts stay literal; inline-code spans (Tokens) are excluded.
		textContext := node.Type == "paragraph" || node.Type == "heading" ||
			node.Type == "table" || (node.Type == "continuation" && node.OriginalType == "table")
		if textContext && node.Text != "" {
			originalChildren := len(node.Children)
			for _, match := range readerWikiNotePattern.FindAllStringSubmatchIndex(node.Text, -1) {
				groups := matchToGroups(node.Text, match)
				if groups[1] == "!" {
					continue // embeds belong to the asset resolver
				}
				if insideToken(node.Tokens, match[4]) {
					continue // inline code literal, never a note ref
				}
				inner := groups[2]
				if groups[3] != "" {
					inner += "|" + groups[3]
				}
				wikiTarget, wikiAlias, wikiHeading := notelinks.SplitWikiLinkParts(inner)
				raw := notelinks.RawLink{Kind: "wiki", Raw: groups[0], Target: wikiTarget, Alias: wikiAlias, Heading: wikiHeading}
				target, ok := resolvedTarget(raw)
				if !ok {
					continue
				}
				uri, found, err := headingRefs.uri(target, wikiHeading)
				if err != nil {
					resolutionErr = err
					return false
				}
				if !found {
					continue
				}
				label := strings.TrimSpace(wikiAlias)
				if label == "" {
					label = target.Title
				}
				document.NodeCount++
				if document.NodeCount >= render.ReaderTotalMaxNodes {
					return false // explicit budget error, never silent
				}
				node.Children = append(node.Children, render.ReaderDocumentNode{
					Type:   "link",
					Anchor: claim(fmt.Sprintf("%s-noteref-%d", node.Anchor, len(node.Children)+1), topIndex),
					Text:   label,
					Ref:    uri,
				})
			}
			for index := 0; index < originalChildren; index++ {
				if !walk(&node.Children[index], topIndex) {
					return false
				}
			}
			return true
		}
		for index := range node.Children {
			if !walk(&node.Children[index], topIndex) {
				return false
			}
		}
		return true
	}
	for topIndex := range document.Blocks {
		if !walk(&document.Blocks[topIndex], topIndex) {
			if resolutionErr != nil {
				return resolutionErr
			}
			return &render.ReaderBudgetError{Code: "document_budget_exceeded"}
		}
	}
	return nil
}

// matchToGroups extracts the full match plus the (!), target and alias
// capture groups from a FindAllStringSubmatchIndex result.
func matchToGroups(text string, match []int) [4]string {
	var groups [4]string
	groups[0] = text[match[0]:match[1]]
	for index := 1; index < len(match)/2; index++ {
		if match[index*2] < 0 {
			continue
		}
		groups[index] = text[match[index*2]:match[index*2+1]]
	}
	return groups
}

func resolveReaderAssetRefs(root string, note domain.Note, document *render.ReaderDocument) error {
	manifest, err := pinaxassets.Load(root)
	if err != nil {
		return nil
	}
	assetByPath := make(map[string]pinaxassets.Asset, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		assetByPath[strings.ToLower(strings.TrimSpace(filepathSlash(asset.Path)))] = asset
	}
	// Resolve each unique destination through the OWNER reference parser
	// (assets.ExtractLinks on a synthetic reference): it normalizes safe
	// relative paths (../), URL-decodes, and strips fragment/query — the
	// same semantics as the legacy attachment chain, with no duplicate
	// resolver logic here.
	targetCache := map[string]string{}
	assetRef := func(target string) string {
		if ref, cached := targetCache[target]; cached {
			return ref
		}
		ref := ""
		if resolved, ok := resolveAssetDestination(note, target); ok {
			if asset, found := assetByPath[strings.ToLower(resolved)]; found {
				ref = "pinax-asset://" + asset.ID
			}
		}
		targetCache[target] = ref
		return ref
	}
	claimAssetAnchor := func(base string, topIndex int) string {
		anchor := base
		for suffix := 2; ; suffix++ {
			if _, exists := document.AnchorIndex[anchor]; !exists {
				break
			}
			anchor = fmt.Sprintf("%s-%d", base, suffix)
		}
		document.AnchorIndex[anchor] = topIndex
		return anchor
	}
	overBudget := false
	var process func(node *render.ReaderDocumentNode, topIndex int)
	process = func(node *render.ReaderDocumentNode, topIndex int) {
		if overBudget {
			return
		}
		if (node.Type == "image" || node.Type == "link") && node.Ref != "" {
			// Markdown attachment links ([下载](attachments/spec.pdf)) get
			// the same opaque-ref resolution as embeds, so consumers can
			// hand the ref straight to the asset endpoint.
			if strings.HasPrefix(node.Ref, "pinax-asset://") || strings.HasPrefix(node.Ref, "pinax://note/") {
				// Already resolved by an earlier pass.
			} else if ref := assetRef(node.Ref); ref != "" {
				node.Ref = ref
			}
		}
		// Wiki embeds inside BLOCK TEXT become additional resolved image
		// children; the original text stays untouched. Code/fence contexts
		// stay literal and inline-code spans (Tokens) are excluded, so
		// `![[asset]]` literals never become live image refs.
		textContext := node.Type == "paragraph" || node.Type == "heading" ||
			node.Type == "table" || (node.Type == "continuation" && node.OriginalType == "table")
		if textContext && node.Text != "" {
			for _, match := range readerWikiAssetPattern.FindAllStringSubmatchIndex(node.Text, -1) {
				inner := node.Text[match[2]:match[3]]
				// Skip inline-code literals.
				if len(node.Tokens) > 0 {
					skip := false
					for _, token := range node.Tokens {
						if token[0] <= match[2] && match[2] < token[1] {
							skip = true
							break
						}
					}
					if skip {
						continue
					}
				}
				target := strings.TrimSpace(strings.SplitN(inner, "#", 2)[0])
				if target == "" || strings.HasSuffix(strings.ToLower(target), ".md") {
					continue
				}
				if ref := assetRef(target); ref != "" {
					document.NodeCount++
					if document.NodeCount >= render.ReaderTotalMaxNodes {
						// Generated nodes obey the same total budget as the
						// renderer's own nodes.
						overBudget = true
						return
					}
					node.Children = append(node.Children, render.ReaderDocumentNode{
						Type:   "image",
						Anchor: claimAssetAnchor(fmt.Sprintf("%s-asset-%d", node.Anchor, len(node.Children)+1), topIndex),
						Ref:    ref,
					})
				}
			}
		}
		for index := range node.Children {
			process(&node.Children[index], topIndex)
		}
	}
	for topIndex := range document.Blocks {
		process(&document.Blocks[topIndex], topIndex)
		if overBudget {
			break
		}
	}
	if overBudget || document.NodeCount >= render.ReaderTotalMaxNodes {
		return &render.ReaderBudgetError{Code: "document_budget_exceeded"}
	}
	return nil
}

func filepathSlash(value string) string {
	return strings.ReplaceAll(value, "\\", "/")
}

// resolveAssetDestination normalizes one reference destination through the
// owner attachment parser (assets.ExtractLinks): safe relative paths,
// URL-encoded targets and markdown fragment/query suffixes follow the exact
// legacy semantics. Returns the resolved vault-relative asset path.
func resolveAssetDestination(note domain.Note, target string) (string, bool) {
	links := pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{
		SourceNoteID: note.ID,
		SourcePath:   note.Path,
		Body:         "![](<" + target + ">)",
	})
	if len(links) == 0 || links[0].AssetPath == "" {
		return "", false
	}
	return links[0].AssetPath, true
}
