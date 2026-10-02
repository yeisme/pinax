package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/yeisme/pinax/internal/markdownnote"

	goldmark "github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Human-reading relations, assets and explicit source export
// (pinax-human-reading-api-v1 P04). Targets are re-authorized per request:
// invisible targets are omitted without names or counts, assets bind to the
// note revision that referenced them, and the source export never accepts a
// path — only object ids resolved by the owner.

// ReaderLinkItem is one authorized relation edge.
type ReaderLinkItem struct {
	ObjectID string `json:"object_id"`
	Revision string `json:"revision,omitempty"`
	Anchor   string `json:"anchor,omitempty"`
}

// ReaderLinkPage is the /v1/reader/notes/{object_id}/links payload.
type ReaderLinkPage struct {
	ObjectID           string           `json:"object_id"`
	Revision           string           `json:"revision"`
	Direction          string           `json:"direction"`
	Items              []ReaderLinkItem `json:"items"`
	NextCursor         string           `json:"next_cursor,omitempty"`
	RelationsWatermark string           `json:"relations_watermark"`
	ObservedAt         string           `json:"observed_at"`
}

// ReaderLinksRequest lists relations for one pinned note revision.
type ReaderLinksRequest struct {
	VaultPath    string
	Session      ReaderSessionInfo
	ObjectID     string
	Revision     string
	Direction    string
	Limit        int
	Cursor       string
	CursorSecret []byte
}

// ReaderAssetRequest reads attachment bytes through the owner resolver.
type ReaderAssetRequest struct {
	VaultPath    string
	Session      ReaderSessionInfo
	AssetRef     string
	NoteID       string
	NoteRevision string
}

// ReaderAssetContent is the resolved asset stream descriptor.
type ReaderAssetContent struct {
	AssetRef   string        `json:"asset_ref"`
	Mime       string        `json:"mime"`
	SizeBytes  int64         `json:"size_bytes"`
	Digest     string        `json:"digest"`
	Path       string        `json:"-"`
	ReadSeeker io.ReadSeeker `json:"-"`
}

// ReaderSourceExportRequest serves the explicit markdown download.
type ReaderSourceExportRequest struct {
	VaultPath string
	Session   ReaderSessionInfo
	ObjectID  string
	Revision  string
}

// ReaderSourceExport is the export stream descriptor.
type ReaderSourceExport struct {
	ObjectID   string        `json:"object_id"`
	Revision   string        `json:"revision"`
	MediaType  string        `json:"media_type"`
	SizeBytes  int64         `json:"size_bytes"`
	Digest     string        `json:"digest"`
	ReadSeeker io.ReadSeeker `json:"-"`
}

// readerExportHeadBytes bounds the frontmatter probe for each candidate
// during export note location: sibling note bodies are never read.
const readerExportHeadBytes = 128 * 1024

// readerExportTarget is a located export candidate: identity plus the
// byte window of its parsed body inside the live file. No body bytes are
// retained during location.
type readerExportTarget struct {
	note      domain.Note // identity fields only; Body may be a truncated probe
	path      string
	bodyStart int64
	bodyEnd   int64
	digest    string // full-file sha256, streamed
}

// readerFileStream streams one byte section of an open file and closes the
// handle when the consumer is done.
type readerFileStream struct {
	section *io.SectionReader
	file    *os.File
}

func (s *readerFileStream) Read(p []byte) (int, error) { return s.section.Read(p) }
func (s *readerFileStream) Seek(off int64, whence int) (int64, error) {
	return s.section.Seek(off, whence)
}
func (s *readerFileStream) Close() error { return s.file.Close() }

type readerExportStream struct {
	reader io.ReadSeeker
	size   int64
	digest string
}

// openReaderExportStream hashes the target file in streaming fashion (the
// revision is the full-file digest, the content digest covers the parsed
// body window) and returns a lazy file-backed reader: memory stays bounded
// regardless of body size — the HTTP window does the rest.
func openReaderExportStream(target readerExportTarget) (readerExportStream, error) {
	file, openErr := os.Open(target.path)
	if openErr != nil {
		return readerExportStream{}, openErr
	}
	fullHash := sha256.New()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return readerExportStream{}, err
	}
	if _, err := io.Copy(fullHash, file); err != nil {
		_ = file.Close()
		return readerExportStream{}, err
	}
	if "sha256:"+hex.EncodeToString(fullHash.Sum(nil)) != target.digest {
		_ = file.Close()
		return readerExportStream{}, fmt.Errorf("the target note changed while exporting")
	}
	bodyLength := target.bodyEnd - target.bodyStart
	if bodyLength < 0 {
		bodyLength = 0
	}
	bodyHash := sha256.New()
	if _, err := file.Seek(target.bodyStart, io.SeekStart); err != nil {
		_ = file.Close()
		return readerExportStream{}, err
	}
	if _, err := io.CopyN(bodyHash, file, bodyLength); err != nil && err != io.EOF {
		_ = file.Close()
		return readerExportStream{}, err
	}
	section := io.NewSectionReader(file, target.bodyStart, bodyLength)
	return readerExportStream{
		reader: &readerFileStream{section: section, file: file},
		size:   bodyLength,
		digest: "sha256:" + hex.EncodeToString(bodyHash.Sum(nil)),
	}, nil
}

// readerFrontmatterOpener reports whether the head starts a YAML
// frontmatter block under either line-ending convention.
func readerFrontmatterOpener(head []byte) bool {
	return bytes.HasPrefix(head, []byte("---\n")) || bytes.HasPrefix(head, []byte("---\r\n"))
}

// readerFrontmatterUnterminated reports whether ParseFrontmatter stopped
// without a closing delimiter.
func readerFrontmatterUnterminated(diagnostics []markdownnote.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "frontmatter_unclosed" {
			return true
		}
	}
	return false
}

// readerExportFirstNonSpaceOffset scans forward from `from` over Unicode
// whitespace (rune-aware: multi-byte space runes never split) and returns
// the offset of the first non-space byte; at EOF the whole region was
// whitespace and the offset equals the end.
func readerExportFirstNonSpaceOffset(file *os.File, from int64) int64 {
	rr := bufio.NewReader(io.NewSectionReader(file, from, 1<<50))
	consumed := int64(0)
	for {
		r, size, err := rr.ReadRune()
		if err != nil {
			return from + consumed
		}
		if !unicode.IsSpace(r) {
			return from + consumed
		}
		consumed += int64(size)
	}
}

// readerExportLastNonSpaceEnd scans [from, to) once and returns the end
// offset of the LAST non-space rune — the byte-exact TrimSpace right edge
// of the parsed body. An all-whitespace region collapses to `from`.
func readerExportLastNonSpaceEnd(file *os.File, from, to int64) int64 {
	rr := bufio.NewReader(io.NewSectionReader(file, from, to-from))
	offset := from
	lastEnd := from
	for {
		r, size, err := rr.ReadRune()
		if err != nil {
			break
		}
		if !unicode.IsSpace(r) {
			lastEnd = offset + int64(size)
		}
		offset += int64(size)
	}
	return lastEnd
}

// locateReaderNoteForExport walks the vault with the reader visibility
// rules (hidden segments, non-regular files and trust-invalid notes are
// never candidates) but probes only a bounded head of each candidate and
// never retains sibling bodies. Unreadable candidates cannot be the
// readable export target and are skipped instead of failing the export.
func locateReaderNoteForExport(root, objectID string) (readerExportTarget, bool) {
	var target readerExportTarget
	found := false
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if found {
			return filepath.SkipAll
		}
		if walkErr != nil {
			return nil // unreadable tree entries cannot host the target
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			return nil // hidden files are never read, parsed or leaked
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		candidate, ok := probeReaderNoteForExport(path, filepath.ToSlash(rel), objectID)
		if ok {
			target = candidate
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return target, found
}

// probeReaderNoteForExport reads a bounded head of one candidate, applies
// the export visibility filters, and — on an ID match — records the exact
// parsed-body byte window plus the streamed full-file digest.
func probeReaderNoteForExport(path, rel, objectID string) (readerExportTarget, bool) {
	file, openErr := os.Open(path)
	if openErr != nil {
		return readerExportTarget{}, false
	}
	defer func() { _ = file.Close() }()
	head, headErr := io.ReadAll(io.LimitReader(file, readerExportHeadBytes+1))
	if headErr != nil {
		return readerExportTarget{}, false
	}
	// The probe may cut MID-LINE: feed the parser only COMPLETE lines so a
	// truncated line (e.g. "  ---" cut out of "  ---not-a-delimiter") can
	// never be mistaken for the closing delimiter.
	truncated := int64(len(head)) > readerExportHeadBytes
	if truncated {
		if cut := bytes.LastIndexByte(head, '\n'); cut >= 0 {
			head = head[:cut+1]
		} else {
			head = nil // no newline within the probe: not a frontmatter note
		}
	}
	meta, body, diagnostics := markdownnote.ParseFrontmatter(head)
	if truncated && readerFrontmatterOpener(head) && readerFrontmatterUnterminated(diagnostics) {
		// Legal frontmatter may extend past the probe window (e.g. large
		// YAML comment blocks): fall back to one full read for THIS
		// candidate only. The owner scan reads every file in full, so
		// visibility semantics stay identical — a note the catalog can
		// see is never reported missing by the export locator.
		if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
			return readerExportTarget{}, false
		}
		full, fullErr := io.ReadAll(file)
		if fullErr != nil {
			return readerExportTarget{}, false
		}
		head = full
		meta, body, _ = markdownnote.ParseFrontmatter(head)
	}
	if !isPinaxNoteFrontmatter(meta) {
		return readerExportTarget{}, false
	}
	note := parseNote(rel, string(head))
	if isSystemIndexNote(note) || isSystemJournalNote(note) {
		return readerExportTarget{}, false
	}
	if _, trustErr := domain.ParseTrustSignals(head); trustErr != nil {
		return readerExportTarget{}, false // same silent skip as the catalog scan
	}
	if note.ID != objectID {
		return readerExportTarget{}, false
	}
	info, statErr := file.Stat()
	if statErr != nil {
		return readerExportTarget{}, false
	}
	frontLen := int64(len(head) - len(body))
	// The parsed body is the raw body section with strings.TrimSpace
	// applied (markdownnote.ParseFull and the splitFrontmatter fallback
	// agree on that). Reproduce it BYTE-EXACTLY with rune-aware forward
	// scans: Unicode whitespace (U+3000, U+00A0, ...) trims at both ends,
	// whitespace runs longer than the head probe are scanned forward, and
	// CRLF bodies keep their exact bytes (ParseFrontmatter returns the
	// raw suffix; an unterminated frontmatter yields empty metadata and is
	// filtered above).
	bodyStart := readerExportFirstNonSpaceOffset(file, frontLen)
	bodyEnd := bodyStart
	if bodyStart < info.Size() {
		bodyEnd = readerExportLastNonSpaceEnd(file, bodyStart, info.Size())
	}
	fullHash := sha256.New()
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return readerExportTarget{}, false
	}
	if _, copyErr := io.Copy(fullHash, file); copyErr != nil {
		return readerExportTarget{}, false
	}
	return readerExportTarget{
		note:      note,
		path:      path,
		bodyStart: bodyStart,
		bodyEnd:   bodyEnd,
		digest:    "sha256:" + hex.EncodeToString(fullHash.Sum(nil)),
	}, true
}

// ReaderLinks returns authorized relation edges. The source revision is
// pinned; the relations watermark guards cursor staleness; invisible targets
// are excluded entirely rather than anonymized.
func (s *Service) ReaderLinks(_ context.Context, req ReaderLinksRequest) (domain.Projection, error) {
	command := "reader.links"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if !req.Session.HasScope(ReaderScopeRelations) {
		err := ReaderError("insufficient_scope", "reader.links requires the reader.relations scope", "")
		return domain.NewErrorProjection(command, err), err
	}
	objectID := strings.TrimSpace(req.ObjectID)
	revision := strings.TrimSpace(req.Revision)
	if objectID == "" || revision == "" {
		err := ReaderError("reader_request_invalid", "object_id and revision are required", "")
		return domain.NewErrorProjection(command, err), err
	}
	direction := strings.TrimSpace(req.Direction)
	if direction == "" {
		direction = "outgoing"
	}
	if direction != "outgoing" && direction != "backlinks" {
		err := ReaderError("reader_request_invalid", "direction must be outgoing or backlinks", "")
		return domain.NewErrorProjection(command, err), err
	}
	query := ReaderPageQuery{Limit: req.Limit, Cursor: req.Cursor, Sort: "relations"}
	normalized, queryErr := NormalizeReaderPageQuery(query, []string{"relations"}, "relations")
	if queryErr != nil {
		return domain.NewErrorProjection(command, queryErr), queryErr
	}

	notes, err := readerScanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes = ordinaryNotes(notes)
	note, found := findNoteByObjectID(notes, objectID)
	if !found {
		err := ReaderError("reader_object_not_found", "The requested note is not visible", "")
		return domain.NewErrorProjection(command, err), err
	}
	currentRevision := readerNoteRevision(note)
	if revision != currentRevision {
		err := ReaderError("revision_unavailable", "The requested revision is no longer available", "Re-open the note at its current revision")
		return domain.NewErrorProjection(command, err), err
	}

	items, watermark := readerRelationEdges(notes, note, direction)
	queryDigest := readerQueryDigest(ReaderPageQuery{Sort: "relations"}, "links:"+direction+":"+objectID, "")
	start := 0
	if normalized.Cursor != "" {
		cursor, cursorErr := decodeReaderCursor(normalized.Cursor, req.CursorSecret, req.Session, queryDigest, watermark)
		if cursorErr != nil {
			return domain.NewErrorProjection(command, cursorErr.(*domain.CommandError)), cursorErr
		}
		start = readerLinksCursorOffset(items, cursor)
	}
	end := start + normalized.Limit
	if start >= len(items) {
		start = len(items)
	}
	if end > len(items) {
		end = len(items)
	}
	page := ReaderLinkPage{
		ObjectID:           objectID,
		Revision:           revision,
		Direction:          direction,
		Items:              items[start:end],
		RelationsWatermark: watermark,
		ObservedAt:         readerObservedAt(s.currentTimeUTC()),
	}
	if len(page.Items) == normalized.Limit && end < len(items) && len(page.Items) > 0 {
		last := page.Items[len(page.Items)-1]
		next, err := encodeReaderCursor(readerCursor{
			Version:       1,
			SessionDigest: req.Session.scopeDigest(),
			QueryDigest:   queryDigest,
			Watermark:     watermark,
			LastSortValue: last.ObjectID + "\x00" + last.Anchor,
			LastObjectID:  last.ObjectID,
		}, req.CursorSecret, req.Session)
		if err != nil {
			return errorProjection(command, err), err
		}
		page.NextCursor = next
	}
	projection := domain.NewProjection(command, "Reader link page composed.")
	projection.Facts["items"] = fmt.Sprint(len(page.Items))
	projection.Facts["direction"] = direction
	projection.Facts["relations_watermark"] = watermark
	projection.Data = map[string]any{"links": page}
	return projection, nil
}

// readerRelationEdges resolves one direction of the enhanced link graph and
// drops every target that is not an ordinary visible note: no names, no
// counts, no shape hints about invisible objects.
func readerRelationEdges(notes []domain.Note, source domain.Note, direction string) ([]ReaderLinkItem, string) {
	outgoing, incoming := BuildEnhancedLinkGraph(notes)
	byID := make(map[string]domain.Note, len(notes))
	for _, note := range notes {
		byID[note.ID] = note
	}
	var edges []domain.NoteLink
	if direction == "backlinks" {
		edges = incoming[source.Path]
	} else {
		edges = outgoing[source.Path]
	}
	items := make([]ReaderLinkItem, 0, len(edges))
	watermarkInput := make([]string, 0, len(edges))
	// The reader relation identity is (counterpart object_id, anchor):
	// multiple links that resolve to the same target and fragment — e.g.
	// different aliases or spellings — project to ONE item so pagination
	// keys stay unique and no page ever skips a returned relation.
	seenIdentities := make(map[string]bool, len(edges))
	for _, edge := range edges {
		// For backlinks the interesting object is the linking source; for
		// outgoing edges it is the referenced target. Both re-authorize
		// against the ordinary visible note set before any name is emitted.
		counterpartPath := edge.TargetPath
		counterpartID := edge.TargetObjectID
		if direction == "backlinks" {
			counterpartPath = edge.SourcePath
			counterpartID = edge.SourceObjectID
		}
		if counterpartID == "" {
			if counterpart, ok := byID[counterpartPath]; ok {
				counterpartID = counterpart.ID
			}
		}
		if counterpartID == "" {
			continue // invisible or unresolvable counterpart: omit entirely
		}
		if _, visible := byID[counterpartID]; !visible {
			continue
		}
		// The link fragment (wiki heading) distinguishes multiple relations
		// to the same counterpart; pagination keys are object_id + anchor.
		identity := counterpartID + "\x00" + edge.TargetHeading
		if seenIdentities[identity] {
			continue // same projected relation via another alias/spelling
		}
		seenIdentities[identity] = true
		items = append(items, ReaderLinkItem{
			ObjectID: counterpartID,
			Revision: readerNoteRevision(byID[counterpartID]),
			Anchor:   edge.TargetHeading,
		})
		// The watermark binds the RESOLVED relation set — counterpart
		// identity, path, heading and status — so renames, resolution
		// changes or fragment edits move it and expire old cursors.
		watermarkInput = append(watermarkInput, strings.Join([]string{
			edge.SourcePath, edge.TargetPath, counterpartID, edge.TargetHeading, edge.Kind, edge.Status,
		}, "\x1f"))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ObjectID != items[j].ObjectID {
			return items[i].ObjectID < items[j].ObjectID
		}
		return items[i].Anchor < items[j].Anchor
	})
	sort.Strings(watermarkInput)
	sum := sha256.Sum256([]byte(strings.Join(watermarkInput, "\n")))
	return items, "sha256:" + hex.EncodeToString(sum[:12])
}

func readerLinksCursorOffset(items []ReaderLinkItem, cursor readerCursor) int {
	for index, item := range items {
		if item.ObjectID+"\x00"+item.Anchor > cursor.LastSortValue {
			return index
		}
	}
	return len(items)
}

// ReaderAsset resolves attachment bytes for a note-bound asset reference.
// The note revision pins the referencing content; the asset must still match
// its recorded content identity; symlinks, hidden files and path escapes are
// rejected before any byte is read.
func (s *Service) ReaderAsset(ctx context.Context, req ReaderAssetRequest) (domain.Projection, ReaderAssetContent, error) {
	command := "reader.asset"
	content := ReaderAssetContent{}
	projectionFor := func(err error) domain.Projection {
		if commandErr, ok := err.(*domain.CommandError); ok {
			return domain.NewErrorProjection(command, commandErr)
		}
		return errorProjection(command, err)
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return projectionFor(err), content, err
	}
	if !req.Session.HasScope(ReaderScopeAssets) {
		err := ReaderError("insufficient_scope", "reader.asset requires the reader.assets scope", "")
		return projectionFor(err), content, err
	}
	assetRef := strings.TrimSpace(req.AssetRef)
	noteID := strings.TrimSpace(req.NoteID)
	noteRevision := strings.TrimSpace(req.NoteRevision)
	if assetRef == "" || noteID == "" || noteRevision == "" {
		err := ReaderError("reader_request_invalid", "asset_ref, note_id and note_revision are required", "")
		return projectionFor(err), content, err
	}

	manifest, err := pinaxassets.Load(root)
	if err != nil {
		err = ReaderError("reader_unavailable", "The asset manifest is not readable", "")
		return projectionFor(err), content, err
	}
	var asset domain.Asset
	found := false
	for _, candidate := range manifest.Assets {
		if candidate.ID == assetRef {
			asset = candidate
			found = true
			break
		}
	}
	if !found {
		err := ReaderError("reader_object_not_found", "The requested asset is not visible", "")
		return projectionFor(err), content, err
	}

	// The note must exist, be pinned at the requested revision, and actually
	// reference this asset at that content.
	notes, err := readerScanNotes(root)
	if err != nil {
		return projectionFor(err), content, err
	}
	note, ok := findNoteByObjectID(ordinaryNotes(notes), noteID)
	if !ok {
		err := ReaderError("reader_object_not_found", "The requested note is not visible", "")
		return projectionFor(err), content, err
	}
	if current := readerNoteRevision(note); noteRevision != current {
		// Pinned note revisions continue from existing snapshots; only an
		// unreadable original fails, mirroring the document contract.
		pinned, readable := s.readerNoteAtRevision(ctx, root, note, noteRevision)
		if !readable {
			err := ReaderError("revision_unavailable", "The note revision does not match its current content", "")
			return projectionFor(err), content, err
		}
		note = pinned
	}
	if !noteReferencesAsset(note, asset.Path) {
		err := ReaderError("reader_object_not_found", "The asset does not belong to this note at this revision", "")
		return projectionFor(err), content, err
	}

	file, size, openErr := openReaderAssetFile(root, asset.Path)
	if openErr != nil {
		err := ReaderError("reader_object_not_found", "The asset content is unavailable", "")
		return projectionFor(err), content, err
	}
	// Content identity is verified from the opened descriptor itself: the
	// full stream is hashed and compared against the manifest SHA256, so a
	// same-size replacement (or a check-to-open symlink swap) cannot ride the
	// recorded digest. The stream is rewound before it is handed out.
	identity, verified := verifyReaderAssetIdentity(file, asset.SHA256)
	if !verified || identity != size {
		_ = file.Close()
		err := ReaderError("reader_object_not_found", "The asset content identity no longer matches", "")
		return projectionFor(err), content, err
	}
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		_ = file.Close()
		err := ReaderError("reader_object_not_found", "The asset content is unavailable", "")
		return projectionFor(err), content, err
	}
	content = ReaderAssetContent{
		AssetRef:   asset.ID,
		Mime:       readerAssetMime(asset),
		SizeBytes:  size,
		Digest:     "sha256:" + asset.SHA256,
		Path:       asset.Path,
		ReadSeeker: file,
	}
	projection := domain.NewProjection(command, "Reader asset resolved.")
	projection.Facts["asset_ref"] = asset.ID
	projection.Facts["size_bytes"] = fmt.Sprint(size)
	projection.Data = map[string]any{
		"asset": map[string]any{
			"asset_ref":    asset.ID,
			"mime":         content.Mime,
			"size_bytes":   size,
			"digest":       content.Digest,
			"availability": "available",
		},
	}
	return projection, content, nil
}

// noteReferencesAsset checks the note's local attachment references at the
// pinned content; external references never bind vault assets.
func noteReferencesAsset(note domain.Note, assetPath string) bool {
	links := pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{
		SourceNoteID: note.ID,
		SourcePath:   note.Path,
		Body:         note.Body,
	})
	for _, link := range links {
		// Wiki embeds (![[...]]) are accepted here; markdown link ownership
		// is decided exclusively by the AST destination path below so the
		// raw-body regex can never grant a wrong-prefix match.
		if link.LinkStyle != "wiki" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(link.AssetPath), strings.TrimSpace(assetPath)) {
			return true
		}
	}
	// Markdown link/image destinations are taken from the SAME AST parser
	// family the document renderer uses (goldmark): titles, angle brackets,
	// reference-style definitions and balanced-paren filenames all resolve
	// exactly as discovery did, instead of divergent raw-body regexes.
	for _, destination := range markdownDestinations(note.Body) {
		if resolved, ok := resolveAssetDestination(note, destination); ok {
			if strings.EqualFold(strings.TrimSpace(resolved), strings.TrimSpace(assetPath)) {
				return true
			}
		}
	}
	return false
}

// markdownDestinations parses body with goldmark (Table/TaskList, matching
// the reader renderer) and collects every Link/Image/AutoLink destination
// string. Raw HTML is not parsed.
func markdownDestinations(body string) []string {
	parser := goldmark.New(goldmark.WithExtensions(extension.Table, extension.TaskList))
	document := parser.Parser().Parse(text.NewReader([]byte(body)))
	destinations := make([]string, 0, 8)
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch typed := node.(type) {
		case *ast.Link:
			destinations = append(destinations, string(typed.Destination))
		case *ast.Image:
			destinations = append(destinations, string(typed.Destination))
		case *ast.AutoLink:
			source := []byte(body)
			destinations = append(destinations, string(typed.URL(source)))
		}
		return ast.WalkContinue, nil
	})
	return destinations
}

// verifyReaderAssetIdentity hashes the whole stream and reports both the
// match result and the actual byte count read.
func verifyReaderAssetIdentity(file *os.File, expectedHex string) (int64, bool) {
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return -1, false
	}
	return size, strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), strings.TrimPrefix(strings.TrimSpace(expectedHex), "sha256:"))
}

// openReaderAssetFile enforces the owner-resolved path: no symlinked or
// replaced files, no hidden components, no escape from the vault root.
// Size comes from the opened descriptor's own stat, not a second lookup.
func openReaderAssetFile(root, relative string) (*os.File, int64, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relative)))
	if clean == "" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, ".pinax/") {
		return nil, 0, fmt.Errorf("asset path escapes the vault")
	}
	for _, segment := range strings.Split(clean, "/") {
		if strings.HasPrefix(segment, ".") || segment == "" {
			return nil, 0, fmt.Errorf("asset path contains a hidden segment")
		}
	}
	absolute := filepath.Join(root, filepath.FromSlash(clean))
	resolvedRoot, rootErr := filepath.EvalSymlinks(root)
	if rootErr != nil {
		resolvedRoot = root
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, 0, fmt.Errorf("asset path is a symlink")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return nil, 0, err
	}
	if resolved != resolvedRoot && !strings.HasPrefix(resolved+string(filepath.Separator), resolvedRoot+string(filepath.Separator)) {
		return nil, 0, fmt.Errorf("asset directory escapes the vault root")
	}
	file, err := os.Open(absolute)
	if err != nil {
		return nil, 0, err
	}
	// Size and mode come from the opened descriptor: a swap between the
	// path checks above and this open is still caught by the digest
	// verification performed on this same descriptor.
	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	if stat.IsDir() || !stat.Mode().IsRegular() {
		_ = file.Close()
		return nil, 0, fmt.Errorf("asset path is not a regular file")
	}
	return file, stat.Size(), nil
}

func readerAssetMime(asset domain.Asset) string {
	if asset.MediaType != "" {
		return asset.MediaType
	}
	return "application/octet-stream"
}

// ReaderSourceExport serves the explicit human download of a note's markdown
// source. Only object ids resolve content; no path or output directory is
// accepted, and the bytes stream straight from the pinned revision body.
func (s *Service) ReaderSourceExport(ctx context.Context, req ReaderSourceExportRequest) (domain.Projection, ReaderSourceExport, error) {
	command := "reader.source_export"
	content := ReaderSourceExport{}
	projectionFor := func(err error) domain.Projection {
		if commandErr, ok := err.(*domain.CommandError); ok {
			return domain.NewErrorProjection(command, commandErr)
		}
		return errorProjection(command, err)
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return projectionFor(err), content, err
	}
	if !req.Session.HasScope(ReaderScopeBody) {
		err := ReaderError("insufficient_scope", "reader.source_export requires the reader.body scope", "")
		return projectionFor(err), content, err
	}
	objectID := strings.TrimSpace(req.ObjectID)
	revision := strings.TrimSpace(req.Revision)
	if objectID == "" || revision == "" {
		err := ReaderError("reader_request_invalid", "object_id and revision are required", "")
		return projectionFor(err), content, err
	}
	if strings.Contains(objectID, "/") || strings.Contains(revision, "/") || strings.Contains(objectID, "..") || strings.Contains(revision, "..") {
		err := ReaderError("reader_request_invalid", "object ids and revisions are not paths", "")
		return projectionFor(err), content, err
	}
	// Bounded targeted resolution: the export reads only a bounded head of
	// each candidate file (never full bodies of sibling notes) and then
	// streams the target from the file itself — memory stays bounded by
	// the request window, not by the vault or the target body.
	target, found := locateReaderNoteForExport(root, objectID)
	if !found {
		err := ReaderError("reader_object_not_found", "The requested note is not visible", "")
		return projectionFor(err), content, err
	}
	if revision != target.digest {
		// The explicit export is the recovery path for over-budget pinned
		// revisions: continue from existing snapshots when readable, and
		// only fail when the original is truly unreachable.
		live := target.note
		live.ContentDigest = target.digest
		pinned, readable := s.readerNoteAtRevision(ctx, root, live, revision)
		if !readable {
			err := ReaderError("revision_unavailable", "The requested revision is no longer available", "Re-open the note at its current revision")
			return projectionFor(err), content, err
		}
		body := pinned.Body
		sum := sha256.Sum256([]byte(body))
		content = ReaderSourceExport{
			ObjectID:   objectID,
			Revision:   revision,
			MediaType:  "text/markdown; charset=utf-8",
			SizeBytes:  int64(len(body)),
			Digest:     "sha256:" + hex.EncodeToString(sum[:]),
			ReadSeeker: stringsReader(body),
		}
	} else {
		stream, streamErr := openReaderExportStream(target)
		if streamErr != nil {
			err := ReaderError("reader_unavailable", "The requested note could not be read", "")
			return projectionFor(err), content, err
		}
		content = ReaderSourceExport{
			ObjectID:   objectID,
			Revision:   revision,
			MediaType:  "text/markdown; charset=utf-8",
			SizeBytes:  stream.size,
			Digest:     stream.digest,
			ReadSeeker: stream.reader,
		}
	}
	projection := domain.NewProjection(command, "Reader source export resolved.")
	projection.Facts["object_id"] = objectID
	projection.Facts["revision"] = revision
	projection.Facts["size_bytes"] = fmt.Sprint(content.SizeBytes)
	projection.Data = map[string]any{"export": map[string]any{
		"object_id":  objectID,
		"revision":   revision,
		"media_type": content.MediaType,
		"size_bytes": content.SizeBytes,
		"digest":     content.Digest,
	}}
	_ = catalogschema.ReaderSourceExportRangeMaxBytes
	return projection, content, nil
}

type stringReadSeeker struct {
	reader io.Reader
	source string
}

func stringsReader(source string) io.ReadSeeker {
	return &stringReadSeeker{source: source}
}

func (s *stringReadSeeker) Read(p []byte) (int, error) {
	if s.reader == nil {
		s.reader = strings.NewReader(s.source)
	}
	return s.reader.Read(p)
}

func (s *stringReadSeeker) Seek(offset int64, whence int) (int64, error) {
	if s.reader != nil {
		if seeker, ok := s.reader.(io.Seeker); ok {
			return seeker.Seek(offset, whence)
		}
	}
	s.reader = strings.NewReader(s.source)
	seeker := s.reader.(io.Seeker)
	return seeker.Seek(offset, whence)
}
