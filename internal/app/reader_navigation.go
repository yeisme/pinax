package app

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// ReaderNavigationCoverage distinguishes a complete page sequence from a
// complete corpus projection. Oversized notes never silently imply full graphs.
type ReaderNavigationCoverage struct {
	PageComplete   bool `json:"page_complete"`
	CorpusComplete bool `json:"corpus_complete"`
}

type ReaderNavigationNote struct {
	ObjectID string `json:"object_id"`
	Revision string `json:"revision"`
	Title    string `json:"title"`
}

type ReaderTag struct {
	TagID           string `json:"tag_id"`
	Path            string `json:"path"`
	ParentID        string `json:"parent_id"`
	Virtual         bool   `json:"virtual"`
	DirectCount     int    `json:"direct_count"`
	DescendantCount int    `json:"descendant_count"`
}

type ReaderTagsPage struct {
	SchemaVersion string                   `json:"schema_version"`
	BindingID     string                   `json:"binding_id"`
	Items         []ReaderTag              `json:"items"`
	NextCursor    string                   `json:"next_cursor,omitempty"`
	Watermark     string                   `json:"watermark"`
	ObservedAt    string                   `json:"observed_at"`
	TotalTags     int                      `json:"total_tags"`
	Coverage      ReaderNavigationCoverage `json:"coverage"`
}

type ReaderTagsRequest struct {
	VaultPath    string
	Session      ReaderSessionInfo
	Limit        int
	Cursor       string
	CursorSecret []byte
}

type ReaderGraphEdge struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
}

type ReaderGraphRequest struct {
	VaultPath    string
	Session      ReaderSessionInfo
	Mode         string
	ObjectID     string
	Revision     string
	Hops         int
	Limit        int
	Cursor       string
	CursorSecret []byte
}

type ReaderGraphPage struct {
	SchemaVersion string                   `json:"schema_version"`
	BindingID     string                   `json:"binding_id"`
	Mode          string                   `json:"mode"`
	ObjectID      string                   `json:"object_id"`
	Revision      string                   `json:"revision"`
	Hops          int                      `json:"hops"`
	Nodes         []ReaderNavigationNote   `json:"nodes"`
	Edges         []ReaderGraphEdge        `json:"edges"`
	NextCursor    string                   `json:"next_cursor,omitempty"`
	Watermark     string                   `json:"watermark"`
	ObservedAt    string                   `json:"observed_at"`
	TotalNodes    int                      `json:"total_nodes"`
	TotalEdges    int                      `json:"total_edges"`
	Coverage      ReaderNavigationCoverage `json:"coverage"`
}

// navigationSnapshot reuses reader visibility and the existing readonly index
// gate. Identity uniqueness and path/root identity also fence every new cursor.
func navigationSnapshot(ctx context.Context, root string, includeJournals, requireIndex bool) ([]domain.Note, string, bool, error) {
	before, err := os.Stat(root)
	if err != nil {
		return nil, "", false, ReaderError("reader_unavailable", "The authorized source is unavailable", "")
	}
	if err := ctx.Err(); err != nil {
		return nil, "", false, err
	}
	notes, err := readerScanNotes(root)
	if err != nil {
		return nil, "", false, ReaderError("reader_unavailable", "The source could not be inspected", "")
	}
	if !includeJournals {
		notes = ordinaryNotes(notes)
	}
	seen := make(map[string]bool, len(notes))
	paths := make([]string, 0, len(notes))
	complete := true
	for _, note := range notes {
		if note.ID == "" || seen[note.ID] {
			return nil, "", false, ReaderError("reader_unsupported", "Source note identities must be unique before navigation", "Repair object identities with the owner")
		}
		seen[note.ID] = true
		paths = append(paths, note.ID+"\x00"+note.Path+"\x00"+readerNoteRevision(note))
		complete = complete && !note.OversizedSource
	}
	if requireIndex {
		if _, err := readerIndexGate(root, ordinaryNotes(notes)); err != nil {
			return nil, "", false, err
		}
	}
	after, err := os.Stat(root)
	if err != nil || !os.SameFile(before, after) {
		return nil, "", false, ReaderError("source_changed", "The authorized source changed", "Rebind the source")
	}
	if err := ctx.Err(); err != nil {
		return nil, "", false, err
	}
	sort.Strings(paths)
	encoded, err := json.Marshal([]any{root, before.ModTime().UTC(), paths})
	if err != nil {
		return nil, "", false, err
	}
	return notes, collaborationRevision(string(encoded)), complete, nil
}

func readerNavigationDigest(value any) string {
	encoded, _ := json.Marshal(value)
	return collaborationRevision(string(encoded))
}

func readerNavigationCursor(cursor string, secret []byte, session ReaderSessionInfo, digest, watermark string) (readerCursor, error) {
	if len(secret) < 16 || session.SessionID == "" || session.BindingID == "" {
		return readerCursor{}, ReaderError("reader_unavailable", "Navigation requires a supervised session and cursor key", "")
	}
	if len(cursor) > catalogschema.ReaderCursorMaxLength {
		return readerCursor{}, ReaderError("reader_cursor_invalid", "Reader cursor exceeds its budget", "")
	}
	if cursor == "" {
		return readerCursor{}, nil
	}
	return decodeReaderCursor(cursor, secret, session, readerNavigationDigest([]string{digest, session.BindingID}), watermark)
}

func readerNavigationNext(secret []byte, session ReaderSessionInfo, digest, watermark, position string) (string, error) {
	return encodeReaderCursor(readerCursor{Version: 1, SessionDigest: session.scopeDigest(), QueryDigest: readerNavigationDigest([]string{digest, session.BindingID}), Watermark: watermark, LastSortValue: position}, secret, session)
}

func canonicalReaderTag(raw string) (string, bool) {
	tags, err := normalizeTagsForWrite([]string{raw})
	if err != nil || len(tags) != 1 || len(tags[0]) > 512 {
		return "", false
	}
	for _, segment := range strings.Split(tags[0], "/") {
		if segment == "" {
			return "", false
		}
	}
	return tags[0], true
}

func readerTagID(path string) string {
	return "tag-v1:" + strings.TrimPrefix(collaborationRevision(path), "sha256:")
}

func (s *Service) ReaderTags(ctx context.Context, req ReaderTagsRequest) (ReaderTagsPage, error) {
	var empty ReaderTagsPage
	if !req.Session.HasScope(ReaderScopeMetadata) {
		return empty, ReaderError("insufficient_scope", "Tags require reader.metadata", "")
	}
	query, err := NormalizeReaderPageQuery(ReaderPageQuery{Limit: req.Limit, Cursor: req.Cursor}, []string{"path_asc"}, "path_asc")
	if err != nil {
		return empty, err
	}
	root, rootErr := cleanVaultPath(req.VaultPath)
	if rootErr != nil {
		return empty, ReaderError("reader_unavailable", "The source is unavailable", "")
	}
	notes, watermark, complete, loadErr := navigationSnapshot(ctx, root, false, true)
	if loadErr != nil {
		return empty, loadErr
	}
	digest := readerNavigationDigest([]any{"reader.tags", query.Limit})
	cursor, cursorErr := readerNavigationCursor(query.Cursor, req.CursorSecret, req.Session, digest, watermark)
	if cursorErr != nil {
		return empty, cursorErr
	}
	type counts struct{ direct, descendants map[string]bool }
	all := map[string]*counts{}
	for _, note := range notes {
		for _, raw := range note.Tags {
			path, ok := canonicalReaderTag(raw)
			if !ok {
				complete = false
				continue
			}
			parts := strings.Split(path, "/")
			for index := range parts {
				key := strings.Join(parts[:index+1], "/")
				if all[key] == nil {
					all[key] = &counts{direct: map[string]bool{}, descendants: map[string]bool{}}
				}
				if index == len(parts)-1 {
					all[key].direct[note.ID] = true
				} else {
					all[key].descendants[note.ID] = true
				}
			}
		}
	}
	paths := make([]string, 0, len(all))
	for path := range all {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	page := ReaderTagsPage{SchemaVersion: catalogschema.ReaderTagsV1Alpha1, BindingID: req.Session.BindingID, Items: []ReaderTag{}, Watermark: watermark, ObservedAt: readerObservedAt(s.currentTimeUTC()), TotalTags: len(paths), Coverage: ReaderNavigationCoverage{CorpusComplete: complete}}
	for _, path := range paths {
		if path <= cursor.LastSortValue {
			continue
		}
		if len(page.Items) == query.Limit {
			break
		}
		parentID := ""
		if index := strings.LastIndex(path, "/"); index >= 0 {
			parentID = readerTagID(path[:index])
		}
		count := all[path]
		page.Items = append(page.Items, ReaderTag{TagID: readerTagID(path), Path: path, ParentID: parentID, Virtual: len(count.direct) == 0, DirectCount: len(count.direct), DescendantCount: len(count.descendants)})
	}
	if len(page.Items) > 0 && page.Items[len(page.Items)-1].Path != paths[len(paths)-1] {
		page.NextCursor, loadErr = readerNavigationNext(req.CursorSecret, req.Session, digest, watermark, page.Items[len(page.Items)-1].Path)
		if loadErr != nil {
			return empty, loadErr
		}
	}
	page.Coverage.PageComplete = page.NextCursor == ""
	return page, nil
}

func normalizeReaderGraph(req ReaderGraphRequest) (ReaderGraphRequest, error) {
	invalid := ReaderError("reader_request_invalid", "Graph requires source mode or a fixed local object/revision with 1-2 hops", "")
	if req.Mode != "source" && req.Mode != "local" {
		return req, invalid
	}
	if req.Limit == 0 {
		req.Limit = catalogschema.ReaderGraphMaxNodes
	}
	if req.Limit < 2 || req.Limit > catalogschema.ReaderGraphMaxNodes {
		return req, invalid
	}
	if req.Mode == "source" {
		if req.ObjectID != "" || req.Revision != "" || req.Hops != 0 {
			return req, invalid
		}
	} else {
		if req.Hops == 0 {
			req.Hops = 1
		}
		if req.Hops < 1 || req.Hops > 2 || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterEditorSourceRequestV1Alpha1, map[string]any{"object_id": req.ObjectID, "revision": req.Revision}) != nil {
			return req, invalid
		}
	}
	return req, nil
}

func (s *Service) ReaderGraph(ctx context.Context, request ReaderGraphRequest) (ReaderGraphPage, error) {
	var empty ReaderGraphPage
	if !request.Session.HasScope(ReaderScopeMetadata) || !request.Session.HasScope(ReaderScopeRelations) {
		return empty, ReaderError("insufficient_scope", "Graph requires reader.metadata and reader.relations", "")
	}
	req, err := normalizeReaderGraph(request)
	if err != nil {
		return empty, err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return empty, ReaderError("reader_unavailable", "The source is unavailable", "")
	}
	notes, watermark, complete, err := navigationSnapshot(ctx, root, false, true)
	if err != nil {
		return empty, err
	}
	byID := make(map[string]domain.Note, len(notes))
	for _, note := range notes {
		byID[note.ID] = note
	}
	if req.Mode == "local" {
		note, ok := byID[req.ObjectID]
		if !ok {
			return empty, ReaderError("reader_object_not_found", "The graph center is not visible", "")
		}
		if readerNoteRevision(note) != req.Revision {
			return empty, ReaderError("revision_unavailable", "The graph center revision changed", "Reopen the current revision")
		}
	}
	digest := readerNavigationDigest([]any{"reader.graph", req.Mode, req.ObjectID, req.Revision, req.Hops, req.Limit})
	cursor, err := readerNavigationCursor(req.Cursor, req.CursorSecret, req.Session, digest, watermark)
	if err != nil {
		return empty, err
	}
	outgoing, _ := BuildEnhancedLinkGraph(notes)
	edges := []ReaderGraphEdge{}
	seen := map[string]bool{}
	adjacent := map[string][]string{}
	for _, links := range outgoing {
		for _, edge := range links {
			if edge.Status != string(domain.LinkStatusResolved) {
				continue
			}
			if _, ok := byID[edge.SourceObjectID]; !ok {
				continue
			}
			if _, ok := byID[edge.TargetObjectID]; !ok {
				continue
			}
			key := edge.SourceObjectID + "\x00" + edge.TargetObjectID
			if seen[key] {
				continue
			}
			seen[key] = true
			edges = append(edges, ReaderGraphEdge{SourceID: edge.SourceObjectID, TargetID: edge.TargetObjectID})
			adjacent[edge.SourceObjectID] = append(adjacent[edge.SourceObjectID], edge.TargetObjectID)
			adjacent[edge.TargetObjectID] = append(adjacent[edge.TargetObjectID], edge.SourceObjectID)
		}
	}
	selected := map[string]bool{}
	if req.Mode == "source" {
		for id := range byID {
			selected[id] = true
		}
	} else {
		selected[req.ObjectID] = true
		frontier := []string{req.ObjectID}
		for depth := 0; depth < req.Hops; depth++ {
			next := []string{}
			for _, id := range frontier {
				for _, neighbor := range adjacent[id] {
					if !selected[neighbor] {
						selected[neighbor] = true
						next = append(next, neighbor)
					}
				}
			}
			frontier = next
		}
	}
	filtered := edges[:0]
	connected := map[string]bool{}
	for _, edge := range edges {
		if selected[edge.SourceID] && selected[edge.TargetID] {
			filtered = append(filtered, edge)
			connected[edge.SourceID] = true
			connected[edge.TargetID] = true
		}
	}
	edges = filtered
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].SourceID != edges[j].SourceID {
			return edges[i].SourceID < edges[j].SourceID
		}
		return edges[i].TargetID < edges[j].TargetID
	})
	isolated := []string{}
	for id := range selected {
		if !connected[id] {
			isolated = append(isolated, id)
		}
	}
	sort.Strings(isolated)
	offset := 0
	if req.Cursor != "" {
		offset, err = strconv.Atoi(cursor.LastSortValue)
		if err != nil || offset < 0 || offset >= len(edges)+len(isolated) {
			return empty, ReaderError("reader_cursor_invalid", "Graph cursor position is invalid", "")
		}
	}
	page := ReaderGraphPage{SchemaVersion: catalogschema.ReaderGraphV1Alpha1, BindingID: req.Session.BindingID, Mode: req.Mode, ObjectID: req.ObjectID, Revision: req.Revision, Hops: req.Hops, Nodes: []ReaderNavigationNote{}, Edges: []ReaderGraphEdge{}, Watermark: watermark, ObservedAt: readerObservedAt(s.currentTimeUTC()), TotalNodes: len(selected), TotalEdges: len(edges), Coverage: ReaderNavigationCoverage{CorpusComplete: complete}}
	pageIDs := map[string]bool{}
	for offset < len(edges) {
		edge := edges[offset]
		candidates := map[string]bool{edge.SourceID: true, edge.TargetID: true}
		additional := 0
		for id := range candidates {
			if !pageIDs[id] {
				additional++
			}
		}
		if len(pageIDs)+additional > req.Limit || len(page.Edges) >= catalogschema.ReaderGraphMaxEdges {
			break
		}
		for id := range candidates {
			pageIDs[id] = true
		}
		page.Edges = append(page.Edges, edge)
		offset++
	}
	for offset >= len(edges) && offset < len(edges)+len(isolated) && len(pageIDs) < req.Limit {
		pageIDs[isolated[offset-len(edges)]] = true
		offset++
	}
	ids := make([]string, 0, len(pageIDs))
	for id := range pageIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		note := byID[id]
		page.Nodes = append(page.Nodes, ReaderNavigationNote{ObjectID: id, Revision: readerNoteRevision(note), Title: note.Title})
	}
	if offset < len(edges)+len(isolated) {
		page.NextCursor, err = readerNavigationNext(req.CursorSecret, req.Session, digest, watermark, strconv.Itoa(offset))
		if err != nil {
			return empty, err
		}
	}
	page.Coverage.PageComplete = page.NextCursor == ""
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return page, nil
}
