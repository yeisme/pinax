package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/searchops"
	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/markdownnote"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Reader access surface (pinax-human-reading-api-v1 P02): source descriptor,
// note listing and keyword search with owner-granted reader scopes. The
// surface is readonly: it never refreshes, rebuilds or writes the index, and
// error results stay distinguishable from successful empty pages.

// ReaderScope values mirror the frozen schema vocabulary. They are granted
// only by the owner; an Agent token never upgrades into them.
const (
	ReaderScopeMetadata  = catalogschema.ReaderScopeMetadata
	ReaderScopeBody      = catalogschema.ReaderScopeBody
	ReaderScopeRelations = catalogschema.ReaderScopeRelations
	ReaderScopeAssets    = catalogschema.ReaderScopeAssets
)

// ReaderSessionInfo is the verified session identity handed to the app layer
// by the HTTP transport after bearer authentication.
type ReaderSessionInfo struct {
	SessionID string
	BindingID string
	Scopes    []string
}

// HasScope reports whether the session carries a granted reader scope.
func (s ReaderSessionInfo) HasScope(scope string) bool {
	for _, granted := range s.Scopes {
		if granted == scope {
			return true
		}
	}
	return false
}

func (s ReaderSessionInfo) scopeDigest() string {
	scopes := append([]string(nil), s.Scopes...)
	sort.Strings(scopes)
	sum := sha256.Sum256([]byte(s.SessionID + "\x00" + strings.Join(scopes, ",")))
	return hex.EncodeToString(sum[:8])
}

// readerNoteStatuses lists the owner's note statuses today; unknown filter
// values are rejected instead of silently matching nothing.
var readerNoteStatuses = map[string]bool{
	"active": true, "inbox": true, "draft": true, "parked": true, "archived": true,
}

// readerScanNotes walks the vault for the human reading surface WITHOUT
// delegating to the legacy scan: hidden entries (any dot-prefixed name,
// files included) are never opened or parsed, symlinks and special files
// are never followed, and malformed trust metadata makes a note invisible
// instead of surfacing an error that echoes hidden field values or paths.
// The legacy scanNotes contract for Agent surfaces stays untouched.
func readerScanNotes(root string) ([]domain.Note, error) {
	root, err := cleanVaultPath(root)
	if err != nil {
		return nil, err
	}
	notes := make([]domain.Note, 0)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
			return nil // symlinks and special files are not reader resources
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		// The interactive source budget gates BEFORE any full load or
		// parse: a raw source beyond the budget is never read in full —
		// identity comes from a bounded head probe and the revision from
		// a streamed digest, so listings and the 413 recovery chain stay
		// available while memory stays bounded.
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil // stat raced with the walk: the file is gone
		}
		if info.Size() > int64(catalogschema.ReaderDocumentSourceMaxBytes) {
			note, ok := readerOversizedNoteIdentity(path, filepath.ToSlash(rel))
			if !ok {
				return nil
			}
			digest, digestErr := streamedReaderFileDigest(path)
			if digestErr != nil {
				return nil
			}
			note.ContentDigest = digest
			note.RawSizeBytes = info.Size()
			note.OversizedSource = true
			notes = append(notes, note)
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		meta, _, _ := markdownnote.ParseFrontmatter(content)
		if !isPinaxNoteFrontmatter(meta) {
			return nil
		}
		note := parseNote(filepath.ToSlash(rel), string(content))
		note.ContentDigest = collaborationRevision(string(content))
		note.RawSizeBytes = int64(len(content))
		if isSystemIndexNote(note) {
			return nil
		}
		signals, trustErr := domain.ParseTrustSignals(content)
		if trustErr != nil {
			// Skip silently: propagating the parse error would echo the
			// malformed field value and the file path through reader
			// responses; legacy surfaces keep their own diagnostics.
			return nil
		}
		note.Trust = &signals
		notes = append(notes, note)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	return notes, nil
}

// readerOversizedNoteIdentity extracts the note identity (frontmatter
// fields) from a bounded head probe of a source beyond the interactive
// budget. The body is NEVER loaded: Body stays empty and the caller marks
// the note oversized. A frontmatter block that closes within the probe
// parses normally; one that runs past it still yields its identity via a
// synthetic closing delimiter (fields sit at the top of the block).
func readerOversizedNoteIdentity(path, rel string) (domain.Note, bool) {
	file, openErr := os.Open(path)
	if openErr != nil {
		return domain.Note{}, false
	}
	defer func() { _ = file.Close() }()
	head, headErr := io.ReadAll(io.LimitReader(file, readerExportHeadBytes+1))
	if headErr != nil {
		return domain.Note{}, false
	}
	truncated := int64(len(head)) > readerExportHeadBytes
	if truncated {
		if cut := bytes.LastIndexByte(head, '\n'); cut >= 0 {
			head = head[:cut+1] // only complete lines reach the parser
		} else {
			head = nil
		}
	}
	identity := head
	meta, _, diagnostics := markdownnote.ParseFrontmatter(identity)
	if len(meta) == 0 && truncated && readerFrontmatterOpener(head) {
		for _, diagnostic := range diagnostics {
			if diagnostic.Code == "frontmatter_unclosed" {
				// Identity fields live at the top: a synthetic close makes
				// the bounded head parse without reading further. The note
				// identity is then built from that closed form so ID,
				// title and status come from the real frontmatter.
				closed := append(append([]byte{}, head...), []byte("\n---\n\n")...)
				if closedMeta, _, _ := markdownnote.ParseFrontmatter(closed); len(closedMeta) > 0 {
					identity = closed
					meta = closedMeta
				}
				break
			}
		}
	}
	if !isPinaxNoteFrontmatter(meta) {
		return domain.Note{}, false
	}
	note := parseNote(rel, string(identity))
	if isSystemIndexNote(note) {
		return domain.Note{}, false
	}
	if signals, trustErr := domain.ParseTrustSignals(head); trustErr == nil {
		note.Trust = &signals
	}
	note.Body = ""
	return note, true
}

// streamedReaderFileDigest hashes a whole file in streaming fashion (O(1)
// memory): the value equals collaborationRevision over the full content
// without ever materializing it.
func streamedReaderFileDigest(path string) (string, error) {
	file, openErr := os.Open(path)
	if openErr != nil {
		return "", openErr
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// readerValidateProjectFilter enforces the frozen filter contract: a
// project_id that is not one of the owner's registered project slugs is a
// 400 reader_request_invalid, never a silent empty success.
func readerValidateProjectFilter(root, projectID string) *domain.CommandError {
	if projectID == "" {
		return nil
	}
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return nil // registry unreadable: the gate path reports availability
	}
	for _, project := range registry.Projects {
		if project.Slug == projectID {
			return nil
		}
	}
	return ReaderError("reader_request_invalid", "unknown project_id filter", "Use one of the registered project slugs")
}

// ReaderError builds the reader-specific command error vocabulary.
func ReaderError(code, message, hint string) *domain.CommandError {
	return &domain.CommandError{Code: code, Message: message, Hint: hint}
}

// ReaderLimits is the frozen budget block returned by the source descriptor.
type ReaderLimits struct {
	ListDefaultLimit          int `json:"list_default_limit"`
	ListMaxLimit              int `json:"list_max_limit"`
	SnippetMaxChars           int `json:"snippet_max_chars"`
	OutlinePageMaxEntries     int `json:"outline_page_max_entries"`
	DocumentPageMaxTextBytes  int `json:"document_page_max_text_bytes"`
	DocumentPageMaxNodes      int `json:"document_page_max_nodes"`
	DocumentResponseMaxBytes  int `json:"document_response_max_bytes"`
	DocumentSourceMaxBytes    int `json:"document_source_max_bytes"`
	DocumentTotalMaxNodes     int `json:"document_total_max_nodes"`
	AssetRangeMaxBytes        int `json:"asset_range_max_bytes"`
	SourceExportRangeMaxBytes int `json:"source_export_range_max_bytes"`
}

func readerLimits() ReaderLimits {
	return ReaderLimits{
		ListDefaultLimit:          catalogschema.ReaderListDefaultLimit,
		ListMaxLimit:              catalogschema.ReaderListMaxLimit,
		SnippetMaxChars:           catalogschema.ReaderSnippetMaxChars,
		OutlinePageMaxEntries:     catalogschema.ReaderOutlinePageMaxEntries,
		DocumentPageMaxTextBytes:  catalogschema.ReaderDocumentPageMaxTextBytes,
		DocumentPageMaxNodes:      catalogschema.ReaderDocumentPageMaxNodes,
		DocumentResponseMaxBytes:  catalogschema.ReaderDocumentResponseMaxBytes,
		DocumentSourceMaxBytes:    catalogschema.ReaderDocumentSourceMaxBytes,
		DocumentTotalMaxNodes:     catalogschema.ReaderDocumentTotalMaxNodes,
		AssetRangeMaxBytes:        catalogschema.ReaderAssetRangeMaxBytes,
		SourceExportRangeMaxBytes: catalogschema.ReaderSourceExportRangeMaxBytes,
	}
}

// ReaderSourceDescriptor is the /v1/reader/source payload.
type ReaderSourceDescriptor struct {
	SchemaVersion string           `json:"schema_version"`
	SessionID     string           `json:"session_id"`
	BindingID     string           `json:"binding_id"`
	DisplayName   string           `json:"display_name"`
	Capabilities  []string         `json:"capabilities"`
	Limits        ReaderLimits     `json:"limits"`
	IndexState    ReaderIndexState `json:"index_state"`
	ObservedAt    string           `json:"observed_at"`
}

// ReaderIndexState reports index health without repairing it.
type ReaderIndexState struct {
	Status     string `json:"status"`
	Watermark  string `json:"watermark"`
	Generation int64  `json:"generation"`
}

// ReaderNoteCard is the bounded card; no body, no absolute paths.
type ReaderNoteCard struct {
	ObjectID   string   `json:"object_id"`
	Title      string   `json:"title"`
	TitleState string   `json:"title_state,omitempty"`
	Revision   string   `json:"revision"`
	URI        string   `json:"uri"`
	Status     string   `json:"status"`
	ProjectID  *string  `json:"project_id"`
	Tags       []string `json:"tags"`
	UpdatedAt  string   `json:"updated_at"`
}

// ReaderNoteList is the /v1/reader/notes payload.
type ReaderNoteList struct {
	Items          []ReaderNoteCard `json:"items"`
	NextCursor     string           `json:"next_cursor,omitempty"`
	ObservedAt     string           `json:"observed_at"`
	IndexWatermark string           `json:"index_watermark"`
	IndexState     ReaderIndexState `json:"index_state"`
}

// ReaderSearchSnippet is a per-hit escaped HTML fragment.
type ReaderSearchSnippet struct {
	ObjectID string `json:"object_id"`
	Snippet  string `json:"snippet"`
}

// ReaderSearchResults is the /v1/reader/search payload.
type ReaderSearchResults struct {
	Items          []ReaderNoteCard      `json:"items"`
	Snippets       []ReaderSearchSnippet `json:"snippets"`
	NextCursor     string                `json:"next_cursor,omitempty"`
	ObservedAt     string                `json:"observed_at"`
	IndexWatermark string                `json:"index_watermark"`
	IndexState     ReaderIndexState      `json:"index_state"`
}

// ReaderPageQuery carries the shared list/search pagination controls.
type ReaderPageQuery struct {
	ProjectID string
	Tags      []string
	Status    string
	Sort      string
	Limit     int
	Cursor    string
}

// ReaderSourceRequest is the readonly source descriptor request.
type ReaderSourceRequest struct {
	VaultPath string
	Session   ReaderSessionInfo
}

// ReaderNotesRequest lists note cards.
type ReaderNotesRequest struct {
	VaultPath    string
	Session      ReaderSessionInfo
	Query        ReaderPageQuery
	CursorSecret []byte
}

// ReaderSearchRequest runs keyword search. Query text never reaches logs.
type ReaderSearchRequest struct {
	VaultPath    string
	Session      ReaderSessionInfo
	QueryText    string
	Query        ReaderPageQuery
	CursorSecret []byte
}

// readerCursor is the signed pagination state. It binds the page position to
// the session identity, the canonical query, and the index generation.
type readerCursor struct {
	Version       int    `json:"v"`
	SessionDigest string `json:"session"`
	QueryDigest   string `json:"query"`
	Watermark     string `json:"watermark"`
	LastSortValue string `json:"last_sort"`
	LastObjectID  string `json:"last_id"`
}

func readerQueryDigest(query ReaderPageQuery, mode string, queryText string) string {
	tags := append([]string(nil), query.Tags...)
	sort.Strings(tags)
	parts := []string{
		mode,
		strings.ToLower(strings.TrimSpace(query.ProjectID)),
		strings.Join(tags, "\x00"),
		strings.ToLower(strings.TrimSpace(query.Status)),
		strings.ToLower(strings.TrimSpace(query.Sort)),
		strings.ToLower(strings.TrimSpace(queryText)),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:12])
}

func readerCursorKey(secret []byte, session ReaderSessionInfo) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(session.scopeDigest()))
	return mac.Sum(nil)
}

func encodeReaderCursor(cursor readerCursor, secret []byte, session ReaderSessionInfo) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, readerCursorKey(secret, session))
	_, _ = mac.Write(payload)
	signature := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// decodeReaderCursor rejects tampered, cross-session, cross-query or stale
// generation cursors with the contract error codes.
func decodeReaderCursor(encoded string, secret []byte, session ReaderSessionInfo, queryDigest, watermark string) (readerCursor, error) {
	invalid := ReaderError("reader_cursor_invalid", "Reader cursor is malformed or bound to another session or query", "Restart the listing from the first page")
	payloadPart, signaturePart, ok := strings.Cut(encoded, ".")
	if !ok {
		return readerCursor{}, invalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return readerCursor{}, invalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(signaturePart)
	if err != nil {
		return readerCursor{}, invalid
	}
	mac := hmac.New(sha256.New, readerCursorKey(secret, session))
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return readerCursor{}, invalid
	}
	var cursor readerCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Version != 1 {
		return readerCursor{}, invalid
	}
	if cursor.SessionDigest != session.scopeDigest() || cursor.QueryDigest != queryDigest {
		return readerCursor{}, invalid
	}
	if cursor.Watermark != watermark {
		return readerCursor{}, ReaderError("reader_cursor_expired", "Index generation changed since this cursor was issued", "Re-run the query from the first page")
	}
	return cursor, nil
}

// readerIndexGate inspects the index strictly read-only (mode=ro; no migration,
// unreadable indexes surface as index_unavailable, never as an empty page.
func readerIndexGate(root string, notes []domain.Note) (noteindex.Status, error) {
	status, err := noteindex.InspectReadOnly(root, notes)
	if err != nil {
		return status, ReaderError("index_unavailable", "Reader requests require a readable local index", "Run pinax index refresh --vault <vault> as the owner")
	}
	switch status.Status {
	case "fresh", "stale", "partial":
		// partial = readable index with non-blocking inconsistencies (e.g.
		// a missing text projection row): the response surfaces
		// index_state=partial with the watermark instead of failing.
		return status, nil
	default:
		return status, ReaderError("index_unavailable", "Reader requests require an existing local index", "Run pinax index refresh --vault <vault> as the owner")
	}
}

// readerWatermark binds the index projection generation to the actual note
// content identities, so an edit-and-refresh that keeps the note count
// constant still expires every outstanding cursor.
func readerWatermark(status noteindex.Status, notes []domain.Note) string {
	identities := make([]string, 0, len(notes))
	for _, note := range notes {
		// The fingerprint covers listing-relevant metadata, not just body
		// identity: title, tags, status, project and timestamps drive
		// ordering and filtering, so a metadata-only edit must still move
		// the watermark and expire outstanding cursors.
		tags := append([]string(nil), note.Tags...)
		sort.Strings(tags)
		identities = append(identities, strings.Join([]string{
			note.ID,
			readerNoteRevision(note),
			note.Title,
			note.Status,
			note.Project,
			strings.Join(tags, ","),
			strings.TrimSpace(note.UpdatedAt),
			strings.TrimSpace(note.CreatedAt),
		}, "\x1f"))
	}
	sort.Strings(identities)
	sum := sha256.Sum256([]byte(status.Status + "\x00" + status.SchemaVersion + "\x00" + strings.Join(identities, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:12])
}

func readerObservedAt(now time.Time) string {
	return now.UTC().Format(time.RFC3339)
}

// readerIndexStateStatus maps the internal index status onto the frozen
// pinax.reader.source.v1alpha1 enum (ready|partial|stale|index_unavailable).
func readerIndexStateStatus(status string) string {
	switch status {
	case "fresh":
		return "ready"
	case "partial", "stale":
		return status
	default:
		return "index_unavailable"
	}
}

// ReaderSource describes the authorized reading surface for this session.
func (s *Service) ReaderSource(_ context.Context, req ReaderSourceRequest) (domain.Projection, error) {
	command := "reader.source"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if !req.Session.HasScope(ReaderScopeMetadata) {
		err := ReaderError("insufficient_scope", "reader.source requires the reader.metadata scope", "")
		return domain.NewErrorProjection(command, err), err
	}
	notes, err := readerScanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	status, gateErr := readerIndexGate(root, ordinaryNotes(notes))
	descriptor := ReaderSourceDescriptor{
		SchemaVersion: catalogschema.ReaderSourceV1Alpha1,
		SessionID:     req.Session.SessionID,
		BindingID:     req.Session.BindingID,
		DisplayName:   readerDisplayName(root),
		Capabilities:  readerGrantedCapabilities(req.Session),
		Limits:        readerLimits(),
		IndexState:    ReaderIndexState{Status: readerIndexStateStatus(status.Status), Watermark: readerWatermark(status, ordinaryNotes(notes)), Generation: int64(status.Notes)},
		ObservedAt:    readerObservedAt(s.currentTimeUTC()),
	}
	if gateErr != nil {
		projection := domain.NewErrorProjection(command, gateErr.(*domain.CommandError))
		projection.Data = map[string]any{"source": descriptor}
		return projection, gateErr
	}
	projection := domain.NewProjection(command, "Reader source descriptor composed.")
	projection.Facts["session_id"] = req.Session.SessionID
	projection.Facts["binding_id"] = req.Session.BindingID
	projection.Facts["index_status"] = status.Status
	projection.Data = map[string]any{"source": descriptor}
	return projection, nil
}

// readerGrantedCapabilities intersects session scopes with the frozen scope
// vocabulary; unimplemented grants stay unadvertised until their lane lands.
func readerGrantedCapabilities(session ReaderSessionInfo) []string {
	capabilities := make([]string, 0, 4)
	for _, scope := range []string{ReaderScopeMetadata, ReaderScopeBody, ReaderScopeRelations, ReaderScopeAssets} {
		if session.HasScope(scope) {
			capabilities = append(capabilities, scope)
		}
	}
	return capabilities
}

func readerDisplayName(root string) string {
	name := strings.TrimSpace(filepath.Base(strings.TrimSpace(root)))
	if name == "" || name == "." || name == "/" {
		return "Pinax vault"
	}
	return name
}

// ReaderNotes lists bounded note cards with stable keyset ordering and
// opaque cursors. Read-only: no index writes, no refresh, no vault mutation.
func (s *Service) ReaderNotes(_ context.Context, req ReaderNotesRequest) (domain.Projection, error) {
	command := "reader.notes"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if !req.Session.HasScope(ReaderScopeMetadata) {
		err := ReaderError("insufficient_scope", "reader.notes requires the reader.metadata scope", "")
		return domain.NewErrorProjection(command, err), err
	}
	query, queryErr := NormalizeReaderPageQuery(req.Query, []string{"updated_desc", "created_desc", "title_asc"}, "updated_desc")
	if queryErr != nil {
		return domain.NewErrorProjection(command, queryErr), queryErr
	}
	if projectErr := readerValidateProjectFilter(root, query.ProjectID); projectErr != nil {
		return domain.NewErrorProjection(command, projectErr), projectErr
	}
	notes, err := readerScanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes = ordinaryNotes(notes)
	status, err := readerIndexGate(root, notes)
	if err != nil {
		return domain.NewErrorProjection(command, err.(*domain.CommandError)), err
	}
	watermark := readerWatermark(status, notes)
	notes = filterReaderNotes(notes, query)
	sortReaderNotes(notes, query.Sort)

	queryDigest := readerQueryDigest(query, "notes", "")
	var cursor readerCursor
	if query.Cursor != "" {
		cursor, err = decodeReaderCursor(query.Cursor, req.CursorSecret, req.Session, queryDigest, watermark)
		if err != nil {
			return domain.NewErrorProjection(command, err.(*domain.CommandError)), err
		}
	}
	page := readerNotesPage(notes, cursor, query.Sort, query.Limit)
	items := make([]ReaderNoteCard, 0, len(page))
	for _, note := range page {
		items = append(items, readerNoteCard(note))
	}
	list := ReaderNoteList{
		Items:          items,
		ObservedAt:     readerObservedAt(s.currentTimeUTC()),
		IndexWatermark: watermark,
		// Freshness is explicit: consumers can tell a current index from a
		// stale one without decoding the opaque watermark.
		IndexState: ReaderIndexState{Status: readerIndexStateStatus(status.Status), Watermark: watermark, Generation: int64(status.Notes)},
	}
	if len(page) == query.Limit && len(notes) > 0 {
		last := page[len(page)-1]
		next, err := encodeReaderCursor(readerCursor{
			Version:       1,
			SessionDigest: req.Session.scopeDigest(),
			QueryDigest:   queryDigest,
			Watermark:     watermark,
			LastSortValue: readerSortValue(last, query.Sort),
			LastObjectID:  last.ID,
		}, req.CursorSecret, req.Session)
		if err != nil {
			return errorProjection(command, err), err
		}
		list.NextCursor = next
	}
	projection := domain.NewProjection(command, "Reader note page composed.")
	projection.Facts["items"] = fmt.Sprint(len(list.Items))
	projection.Facts["index_watermark"] = watermark
	projection.Facts["sort"] = query.Sort
	projection.Data = map[string]any{"notes": list}
	return projection, nil
}

// ReaderSearch runs keyword search against the loaded notes. No model, no
// lazy rebuild, and the query text never enters facts or evidence.
func (s *Service) ReaderSearch(_ context.Context, req ReaderSearchRequest) (domain.Projection, error) {
	command := "reader.search"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if !req.Session.HasScope(ReaderScopeBody) {
		err := ReaderError("insufficient_scope", "reader.search requires the reader.body scope", "")
		return domain.NewErrorProjection(command, err), err
	}
	queryText := strings.TrimSpace(req.QueryText)
	if queryText == "" || len([]rune(queryText)) > catalogschema.ReaderQueryMaxChars {
		err := ReaderError("reader_request_invalid", "search query must be 1-512 characters after trimming", "")
		return domain.NewErrorProjection(command, err), err
	}
	req.Query.Sort = "relevance_desc"
	query, queryErr := NormalizeReaderPageQuery(req.Query, []string{"relevance_desc"}, "relevance_desc")
	if queryErr != nil {
		return domain.NewErrorProjection(command, queryErr), queryErr
	}
	if projectErr := readerValidateProjectFilter(root, query.ProjectID); projectErr != nil {
		return domain.NewErrorProjection(command, projectErr), projectErr
	}
	notes, err := readerScanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes = ordinaryNotes(notes)
	status, err := readerIndexGate(root, notes)
	if err != nil {
		return domain.NewErrorProjection(command, err.(*domain.CommandError)), err
	}
	watermark := readerWatermark(status, notes)
	notes = filterReaderNotes(notes, query)

	// Keyword match over title/tags/body with stable relevance ordering; no
	// model calls, no external service, no fallback scanning beyond the
	// already-loaded readonly note set.
	hits := readerKeywordMatch(notes, queryText)

	queryDigest := readerQueryDigest(query, "search", queryText)
	var cursor readerCursor
	if query.Cursor != "" {
		cursor, err = decodeReaderCursor(query.Cursor, req.CursorSecret, req.Session, queryDigest, watermark)
		if err != nil {
			return domain.NewErrorProjection(command, err.(*domain.CommandError)), err
		}
	}
	page := readerHitsPage(hits, cursor, query.Limit)
	items := make([]ReaderNoteCard, 0, len(page))
	snippets := make([]ReaderSearchSnippet, 0, len(page))
	for _, hit := range page {
		items = append(items, readerNoteCard(hit.note))
		snippets = append(snippets, ReaderSearchSnippet{
			ObjectID: hit.note.ID,
			Snippet:  readerSnippet(hit.note, queryText),
		})
	}
	results := ReaderSearchResults{
		Items:          items,
		Snippets:       snippets,
		ObservedAt:     readerObservedAt(s.currentTimeUTC()),
		IndexWatermark: watermark,
		IndexState:     ReaderIndexState{Status: readerIndexStateStatus(status.Status), Watermark: watermark, Generation: int64(status.Notes)},
	}
	if len(page) == query.Limit && len(hits) > 0 {
		last := page[len(page)-1]
		next, err := encodeReaderCursor(readerCursor{
			Version:       1,
			SessionDigest: req.Session.scopeDigest(),
			QueryDigest:   queryDigest,
			Watermark:     watermark,
			LastSortValue: strconv.Itoa(last.score),
			LastObjectID:  last.note.ID,
		}, req.CursorSecret, req.Session)
		if err != nil {
			return errorProjection(command, err), err
		}
		results.NextCursor = next
	}
	projection := domain.NewProjection(command, "Reader search page composed.")
	projection.Facts["items"] = fmt.Sprint(len(results.Items))
	projection.Facts["index_watermark"] = watermark
	// Deliberately no query fact/evidence: search text never enters records.
	projection.Data = map[string]any{"search": results}
	return projection, nil
}

// NormalizeReaderPageQuery validates and normalizes shared reader page
// controls; it is shared by the HTTP layer and app-level callers.
func NormalizeReaderPageQuery(query ReaderPageQuery, allowedSorts []string, defaultSort string) (ReaderPageQuery, *domain.CommandError) {
	invalid := func(message string) *domain.CommandError {
		return ReaderError("reader_request_invalid", message, "")
	}
	for _, tag := range query.Tags {
		if strings.TrimSpace(tag) == "" {
			return query, invalid("tag filters must be non-empty")
		}
	}
	if len(query.Tags) > 16 {
		return query, invalid("at most 16 tag filters are allowed")
	}
	if status := strings.TrimSpace(query.Status); status != "" {
		if !readerNoteStatuses[status] {
			return query, invalid("unknown note status filter")
		}
		query.Status = status
	}
	sortKey := strings.TrimSpace(query.Sort)
	if sortKey == "" {
		sortKey = defaultSort
	}
	allowed := false
	for _, candidate := range allowedSorts {
		if sortKey == candidate {
			allowed = true
			break
		}
	}
	if !allowed {
		return query, invalid("unsupported sort")
	}
	query.Sort = sortKey
	if query.Limit == 0 {
		query.Limit = catalogschema.ReaderListDefaultLimit
	}
	if query.Limit < 1 || query.Limit > catalogschema.ReaderListMaxLimit {
		return query, invalid("limit must be between 1 and 200")
	}
	query.ProjectID = strings.TrimSpace(query.ProjectID)
	query.Cursor = strings.TrimSpace(query.Cursor)
	return query, nil
}

func filterReaderNotes(notes []domain.Note, query ReaderPageQuery) []domain.Note {
	filtered := make([]domain.Note, 0, len(notes))
	for _, note := range notes {
		if query.Status != "" && note.Status != query.Status {
			continue
		}
		if query.ProjectID != "" && note.Project != query.ProjectID {
			continue
		}
		blocked := false
		for _, tag := range query.Tags {
			if !noteHasTag(note, tag) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		filtered = append(filtered, note)
	}
	return filtered
}

func noteHasTag(note domain.Note, tag string) bool {
	for _, candidate := range note.Tags {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(tag)) {
			return true
		}
	}
	return false
}

// sortReaderNotes orders notes by the requested key with object_id ascending
// as the stable tiebreaker, so keyset cursors never skip or repeat items.
func sortReaderNotes(notes []domain.Note, sortKey string) {
	sort.SliceStable(notes, func(i, j int) bool {
		left, right := notes[i], notes[j]
		switch sortKey {
		case "created_desc":
			if li, rj := readerTimeValue(left.CreatedAt), readerTimeValue(right.CreatedAt); li != rj {
				return li > rj
			}
		case "title_asc":
			if lt, rt := strings.ToLower(left.Title), strings.ToLower(right.Title); lt != rt {
				return lt < rt
			}
		default: // updated_desc
			if li, rj := readerTimeValue(left.UpdatedAt), readerTimeValue(right.UpdatedAt); li != rj {
				return li > rj
			}
		}
		return left.ID < right.ID
	})
}

func readerTimeValue(value string) int64 {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed.Unix()
}

func readerSortValue(note domain.Note, sortKey string) string {
	switch sortKey {
	case "created_desc":
		return strings.TrimSpace(note.CreatedAt)
	case "title_asc":
		return strings.ToLower(note.Title)
	default:
		return strings.TrimSpace(note.UpdatedAt)
	}
}

// readerNotesPage returns up to limit notes strictly after the cursor
// position in the already-sorted slice. When the cursor's exact anchor was
// removed between pages, pagination continues from the first item that is
// still after the anchor position (no silent restart from page one).
func readerNotesPage(notes []domain.Note, cursor readerCursor, sortKey string, limit int) []domain.Note {
	start := 0
	if cursor.LastObjectID != "" {
		start = len(notes)
		for index, note := range notes {
			if readerNoteAfterAnchor(note, cursor, sortKey) {
				start = index
				break
			}
		}
	}
	end := start + limit
	if start >= len(notes) {
		return nil
	}
	if end > len(notes) {
		end = len(notes)
	}
	return notes[start:end]
}

// readerNoteAfterAnchor reports whether the note sorts strictly after the
// cursor anchor under the active ordering (sort key first, id ascending as
// the stable tiebreaker).
func readerNoteAfterAnchor(note domain.Note, cursor readerCursor, sortKey string) bool {
	switch sortKey {
	case "created_desc":
		item, anchor := readerTimeValue(note.CreatedAt), readerTimeAnchor(cursor.LastSortValue)
		if item != anchor {
			return item < anchor
		}
	case "title_asc":
		if value := strings.ToLower(note.Title); value != cursor.LastSortValue {
			return value > cursor.LastSortValue
		}
	default: // updated_desc
		item, anchor := readerTimeValue(note.UpdatedAt), readerTimeAnchor(cursor.LastSortValue)
		if item != anchor {
			return item < anchor
		}
	}
	return note.ID > cursor.LastObjectID
}

func readerTimeAnchor(value string) int64 {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed.Unix()
}

// readerHitsPage paginates relevance-ordered hits; equal scores order by
// object_id ascending so the keyset anchor stays deterministic.
func readerHitsPage(hits []readerSearchHit, cursor readerCursor, limit int) []readerSearchHit {
	start := 0
	if cursor.LastObjectID != "" {
		anchorScore, _ := strconv.Atoi(cursor.LastSortValue)
		start = len(hits)
		for index, hit := range hits {
			if hit.score < anchorScore || (hit.score == anchorScore && hit.note.ID > cursor.LastObjectID) {
				start = index
				break
			}
		}
	}
	end := start + limit
	if start >= len(hits) {
		return nil
	}
	if end > len(hits) {
		end = len(hits)
	}
	return hits[start:end]
}

func readerNoteCard(note domain.Note) ReaderNoteCard {
	revision := readerNoteRevision(note)
	card := ReaderNoteCard{
		ObjectID:   note.ID,
		Title:      note.Title,
		TitleState: "present",
		Revision:   revision,
		URI:        "pinax://note/" + note.ID + "?v=" + revision,
		Status:     note.Status,
		Tags:       append([]string(nil), note.Tags...),
		UpdatedAt:  strings.TrimSpace(note.UpdatedAt),
	}
	if strings.TrimSpace(note.Title) == "" {
		card.TitleState = "unavailable"
	}
	if note.Project != "" {
		project := note.Project
		card.ProjectID = &project
	}
	if card.Tags == nil {
		card.Tags = []string{}
	}
	return card
}

func readerNoteRevision(note domain.Note) string {
	// Prefer the full-content digest so pinax://note revisions interop with
	// the legacy note.ref/render resolver (same collaboration algorithm).
	if note.ContentDigest != "" {
		return note.ContentDigest
	}
	sum := sha256.Sum256([]byte(note.Body))
	return "sha256:" + hex.EncodeToString(sum[:])
}

type readerSearchHit struct {
	note  domain.Note
	score int
}

// readerKeywordMatch scores title > tags > body containment. It reads only
// note content already loaded by the readonly scan.
func readerKeywordMatch(notes []domain.Note, query string) []readerSearchHit {
	needle := strings.ToLower(query)
	hits := make([]readerSearchHit, 0)
	for _, note := range notes {
		score := 0
		if strings.Contains(strings.ToLower(note.Title), needle) {
			score += 10
		}
		for _, tag := range note.Tags {
			if strings.Contains(strings.ToLower(tag), needle) {
				score += 5
				break
			}
		}
		if strings.Contains(strings.ToLower(note.Body), needle) {
			score += 1
		}
		if score > 0 {
			hits = append(hits, readerSearchHit{note: note, score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].note.ID < hits[j].note.ID
	})
	return hits
}

// readerSnippet extracts a bounded plaintext window; prefers frontmatter
// summaries, then the first keyword window from the body. The result is
// HTML-escaped AND bounded in escaped runes, so the frozen 320-char wire
// budget holds for ampersand-heavy content too.
func readerSnippet(note domain.Note, query string) string {
	if value := searchShowSnippet(note); value != "" && strings.Contains(strings.ToLower(value), strings.ToLower(query)) {
		return readerEscapeSnippet(boundedReaderSnippet(value))
	}
	return readerEscapeSnippet(boundedReaderSnippet(searchops.FirstSnippet(note.Body, query)))
}

func boundedReaderSnippet(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	runes := []rune(value)
	if len(runes) <= catalogschema.ReaderSnippetMaxChars*5 {
		// Generous raw cap; the escaped bound below enforces the wire limit.
		return value
	}
	return string(runes[:catalogschema.ReaderSnippetMaxChars*5])
}

// readerEscapeSnippet returns safe HTML-escaped text bounded to the frozen
// snippet budget AFTER escaping; entities are never severed mid-sequence
// because whole runes are escaped one at a time.
func readerEscapeSnippet(value string) string {
	var builder strings.Builder
	remaining := catalogschema.ReaderSnippetMaxChars
	for _, r := range value {
		if remaining <= 0 {
			break
		}
		escaped := readerSnippetEscaper.Replace(string(r))
		escapedRunes := len([]rune(escaped))
		if escapedRunes > remaining {
			break
		}
		builder.WriteString(escaped)
		remaining -= escapedRunes
	}
	return builder.String()
}

var readerSnippetEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")

// GenerateReaderBearer creates a 256-bit reader bearer secret for session
// bootstrap. It is only handed to the in-memory session store.
func GenerateReaderBearer() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
