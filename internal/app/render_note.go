package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/searchops"
	"github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/notelinks"
	"github.com/yeisme/pinax/internal/records"
	noterender "github.com/yeisme/pinax/internal/render"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

type RenderNoteRequest struct {
	VaultPath        string
	NoteRef          string
	Display          string
	ResolvedRevision string
}
type NoteRefRequest struct {
	VaultPath        string
	NoteRef          string
	ResolvedRevision string
	Floating         bool
	Pick             int
}
type NoteRefResult struct {
	Ref              noterender.Reference   `json:"ref"`
	ResolvedRevision string                 `json:"resolved_revision,omitempty"`
	CurrentRevision  string                 `json:"current_revision,omitempty"`
	ResolvedAt       string                 `json:"resolved_at"`
	Stale            bool                   `json:"stale"`
	Candidates       []noterender.Reference `json:"candidates,omitempty"`
	Markdown         string                 `json:"markdown,omitempty"`
	Wikilink         string                 `json:"wikilink,omitempty"`
}

func validRenderRevision(value string) bool {
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	b, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(b) == 32
}

func parseRenderRef(value string) (string, string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "pinax" || u.Host != "note" || u.User != nil || u.Fragment != "" || !noterender.SafeID(strings.TrimPrefix(u.Path, "/")) {
		return "", "", noteProposalError("reference_invalid", "Use pinax://note/<object-id>?v=<content-revision>")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) > 1 || (len(q) == 1 && (len(q["v"]) != 1 || !validRenderRevision(q.Get("v")))) {
		return "", "", noteProposalError("reference_invalid", "Reference revision must be a pinned SHA-256 content revision")
	}
	return strings.TrimPrefix(u.Path, "/"), q.Get("v"), nil
}

func renderRecordVersion(ctx context.Context, root, id string) (uint64, error) {
	st, err := records.NewService(root).ReplayReadOnly(ctx)
	if err != nil {
		return 0, err
	}
	return st.Records[id].RecordVersion, nil
}

func renderReference(ctx context.Context, root string, note domain.Note) (noterender.Reference, error) {
	if !noterender.SafeID(note.ID) {
		return noterender.Reference{}, noteProposalError("identity_required", "Note requires a safe stable object ID")
	}
	if err := collaborationSafePath(root, note.Path); err != nil {
		return noterender.Reference{}, err
	}
	content, err := os.ReadFile(filepath.Join(root, note.Path))
	if err != nil {
		return noterender.Reference{}, err
	}
	return renderReferenceContent(ctx, root, note, string(content))
}

func renderReferenceContent(ctx context.Context, root string, note domain.Note, content string) (noterender.Reference, error) {
	actual := parseNote(note.Path, content)
	if actual.ID != note.ID {
		return noterender.Reference{}, noteProposalError("plan_stale", "Note identity changed while reading; read the current card again")
	}
	note = actual
	version, err := renderRecordVersion(ctx, root, note.ID)
	if err != nil {
		return noterender.Reference{}, err
	}
	r := noterender.Reference{ObjectID: note.ID, Title: note.Title, Version: version, Revision: collaborationRevision(content), Status: "resolved"}
	r.URI = noterender.NoteURI(r.ObjectID, r.Revision)
	if !noterender.SafeText(r.Title) {
		r.Title = "[REDACTED]"
		r.Unsafe = true
	}
	return r, nil
}

func (s *Service) resolveRenderVersion(ctx context.Context, root, id, revision string) (domain.Note, noterender.Reference, error) {
	if n, err := readRenderNote(root, id); err == nil {
		if err := collaborationSafePath(root, n.Path); err != nil {
			return domain.Note{}, noterender.Reference{}, err
		}
		content, err := os.ReadFile(filepath.Join(root, n.Path))
		if err != nil {
			return domain.Note{}, noterender.Reference{}, err
		}
		r, err := renderReferenceContent(ctx, root, n, string(content))
		if err != nil {
			return domain.Note{}, r, err
		}
		if revision == "" || r.Revision == revision {
			return parseNote(n.Path, string(content)), r, nil
		}
	}
	if revision == "" {
		return domain.Note{}, noterender.Reference{ObjectID: id, Status: "unresolved"}, nil
	}
	if err := collaborationSafePath(root, ".pinax/version/snapshots"); err != nil {
		return domain.Note{}, noterender.Reference{}, err
	}
	if err := collaborationSafePath(root, ".pinax/version/objects"); err != nil {
		return domain.Note{}, noterender.Reference{}, err
	}
	snapshots, err := loadVersionSnapshots(root, 0)
	if err != nil {
		return domain.Note{}, noterender.Reference{}, err
	}
	for _, snapshot := range snapshots {
		if err := ctx.Err(); err != nil {
			return domain.Note{}, noterender.Reference{}, err
		}
		for _, fact := range snapshot.FileFacts {
			if "sha256:"+fact.ContentHash != revision {
				continue
			}
			file, err := s.versionBackend.ReadFile(ctx, pinaxversion.ReadFileRequest{Root: root, Path: fact.Path, Revision: snapshot.SnapshotID})
			if err != nil {
				return domain.Note{}, noterender.Reference{}, err
			}
			note := parseNote(fact.Path, file.Content)
			if note.ID != id || collaborationRevision(file.Content) != revision {
				continue
			}
			events, err := records.NewService(root).HistoryReadOnly(ctx)
			if err != nil {
				return domain.Note{}, noterender.Reference{}, err
			}
			var version uint64
			for _, e := range events {
				if e.Seq <= snapshot.LedgerSeq && e.ObjectID == id {
					version++
				}
			}
			r := noterender.Reference{ObjectID: id, Title: note.Title, Version: version, Revision: revision, URI: noterender.NoteURI(id, revision), Status: "resolved"}
			if !noterender.SafeText(r.Title) {
				r.Title = "[REDACTED]"
				r.Unsafe = true
			}
			return note, r, nil
		}
	}
	return domain.Note{}, noterender.Reference{ObjectID: id, Revision: revision, URI: noterender.NoteURI(id, revision), Status: "unresolved"}, nil
}

func (s *Service) NoteReference(ctx context.Context, req NoteRefRequest) (domain.Projection, error) {
	command := "note.ref"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	result := NoteRefResult{ResolvedAt: s.currentTimeUTC().Format(time.RFC3339Nano)}
	if strings.HasPrefix(req.NoteRef, "pinax:") {
		command = "note.ref.resolve"
		id, revision, err := parseRenderRef(req.NoteRef)
		if err != nil {
			return errorProjection(command, err), err
		}
		if req.ResolvedRevision != "" && (!validRenderRevision(req.ResolvedRevision) || revision != "") {
			err := noteProposalError("reference_invalid", "Pass resolved_revision only for a floating reference")
			return errorProjection(command, err), err
		}
		_, current, err := s.resolveRenderVersion(ctx, root, id, "")
		if err != nil {
			return errorProjection(command, err), err
		}
		result.CurrentRevision = current.Revision
		if revision == "" {
			revision = req.ResolvedRevision
		}
		_, result.Ref, err = s.resolveRenderVersion(ctx, root, id, revision)
		if err != nil {
			return errorProjection(command, err), err
		}
		result.ResolvedRevision = result.Ref.Revision
		result.Stale = req.ResolvedRevision != "" && result.CurrentRevision != req.ResolvedRevision
	} else {
		candidates, err := s.resolveVaultObjectFallback(root, req.NoteRef, "registered", "note", "not_checked")
		if err != nil {
			return errorProjection(command, err), err
		}
		sortResolverCandidates(candidates)
		lookup := strongestResolverResult(ResolverResult{Candidates: candidates})
		if len(lookup.Candidates) > 1 && req.Pick == 0 {
			result.Ref.Status = "ambiguous"
			for i, c := range lookup.Candidates {
				if i >= 20 {
					break
				}
				if !noterender.SafeID(c.NoteID) {
					continue
				}
				n, err := readRenderNote(root, c.NoteID)
				if err != nil {
					return errorProjection(command, err), err
				}
				ref, err := renderReference(ctx, root, n)
				if err != nil {
					return errorProjection(command, err), err
				}
				result.Candidates = append(result.Candidates, ref)
			}
		} else {
			idx := 0
			if req.Pick > 0 {
				idx = req.Pick - 1
			}
			if idx >= len(lookup.Candidates) || req.Pick < 0 {
				err := noteProposalError("note_not_found", "Select a listed candidate or run pinax search")
				return errorProjection(command, err), err
			}
			note, err := readRenderNote(root, lookup.Candidates[idx].NoteID)
			if err != nil {
				return errorProjection(command, err), err
			}
			result.Ref, err = renderReference(ctx, root, note)
			if err != nil {
				return errorProjection(command, err), err
			}
			result.ResolvedRevision, result.CurrentRevision = result.Ref.Revision, result.Ref.Revision
			if req.Floating {
				result.Ref.URI = noterender.NoteURI(note.ID, "")
			}
		}
	}
	if result.Ref.Status == "resolved" {
		title := strings.NewReplacer("[", "\\[", "]", "\\]", "\n", " ").Replace(result.Ref.Title)
		result.Markdown = "[" + title + "](" + result.Ref.URI + ")"
		result.Wikilink = "[[" + result.Ref.URI + "|" + strings.ReplaceAll(result.Ref.Title, "|", " ") + "]]"
	}
	p := domain.NewProjection(command, "Stable note reference read.")
	p.Facts["reference_status"], p.Facts["resolved_revision"], p.Facts["current_revision"], p.Facts["stale"] = result.Ref.Status, result.ResolvedRevision, result.CurrentRevision, fmt.Sprint(result.Stale)
	p.Facts["local_write"], p.Facts["remote_write"] = "false", "false"
	p.Data = result
	p.Facts["markdown_link"], p.Facts["wikilink"] = result.Markdown, result.Wikilink
	p.Facts["uri"] = result.Ref.URI
	if result.Ref.Status != "resolved" || result.Stale {
		p.Status = "partial"
		p.Actions = []domain.Action{{Name: "read_current", Command: "pinax note ref " + shellQuote(result.Ref.ObjectID)}}
	}
	return p, nil
}

func readRenderNote(root, id string) (domain.Note, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return domain.Note{}, err
	}
	var found *domain.Note
	for i := range notes {
		if notes[i].ID == id && id != "" {
			if found != nil {
				return domain.Note{}, noteProposalError("note_ref_ambiguous", "Object identity is ambiguous; inspect the record ledger")
			}
			found = &notes[i]
		}
	}
	if found == nil {
		return domain.Note{}, noteProposalError("note_not_found", "Note identity is unresolved")
	}
	return *found, nil
}

func (s *Service) RenderNote(ctx context.Context, req RenderNoteRequest) (domain.Projection, error) {
	command := "note.preview.rendered"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if req.Display == "" {
		req.Display = "card"
	}
	if req.Display != "card" && req.Display != "context" && req.Display != "body" {
		err := noteProposalError("display_invalid", "Use card, context, or explicit body depth")
		return errorProjection(command, err), err
	}
	refProjection, err := s.NoteReference(ctx, NoteRefRequest{VaultPath: root, NoteRef: req.NoteRef, ResolvedRevision: req.ResolvedRevision})
	if err != nil {
		return errorProjection(command, err), err
	}
	resolved := refProjection.Data.(NoteRefResult)
	if resolved.Ref.Status != "resolved" {
		refProjection.Command = command
		return refProjection, nil
	}
	note, ref, err := s.resolveRenderVersion(ctx, root, resolved.Ref.ObjectID, resolved.ResolvedRevision)
	if err != nil {
		return errorProjection(command, err), err
	}
	if ref.Status != "resolved" {
		refProjection.Command = command
		refProjection.Status = "partial"
		refProjection.Facts["reference_status"] = "unresolved"
		refProjection.Data = NoteRefResult{Ref: ref, ResolvedRevision: resolved.ResolvedRevision, CurrentRevision: resolved.CurrentRevision, Stale: true}
		return refProjection, nil
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	snapshot := notelinks.BuildResolverSnapshot(notes)
	_, incoming := notelinks.BuildGraph(notes)
	byPath := map[string]domain.Note{}
	for _, n := range notes {
		byPath[n.Path] = n
	}
	backlinks := []noterender.Reference{}
	for _, link := range incoming[note.Path] {
		if n, ok := byPath[link.SourcePath]; ok {
			r, err := renderReference(ctx, root, n)
			if err != nil {
				return errorProjection(command, err), err
			}
			backlinks = append(backlinks, r)
		}
	}
	manifest, err := assets.Load(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	resolve := func(target, label string) noterender.Reference {
		if strings.HasPrefix(target, "drivebridge://file/") {
			return noterender.Reference{URI: target, Title: label, Status: "external"}
		}
		if strings.HasPrefix(target, "pinax:") {
			id, rev, err := parseRenderRef(target)
			if err != nil {
				return noterender.Reference{Status: "unresolved"}
			}
			_, r, err := s.resolveRenderVersion(ctx, root, id, rev)
			if err != nil {
				return noterender.Reference{Status: "unresolved"}
			}
			return r
		}
		assetPath := path.Clean(path.Join(path.Dir(note.Path), target))
		for _, a := range manifest.Assets {
			if a.Path == assetPath || a.Path == target || "pinax-asset://"+a.ObjectID == target {
				return noterender.Reference{URI: "pinax-asset://" + a.ObjectID, ObjectID: a.ObjectID, Mime: a.MediaType, Title: label, Status: "resolved"}
			}
		}
		if u, err := url.Parse(target); err == nil && u.Scheme != "" {
			return noterender.Reference{URI: target, Title: label, Status: "literal"}
		}
		raw := notelinks.RawLink{Kind: "wiki", Raw: target, Target: target, Alias: label}
		if strings.HasSuffix(strings.Split(target, "#")[0], ".md") {
			raw.Kind = "markdown"
		}
		raw.Target, raw.Alias, raw.Heading = notelinks.SplitWikiLinkParts(target + "|" + label)
		link := notelinks.ResolveLinkTarget(note, raw, snapshot).Link
		if link.Status == "resolved" {
			if n, ok := byPath[link.TargetPath]; ok {
				r, err := renderReference(ctx, root, n)
				if err == nil {
					return r
				}
			}
		}
		return noterender.Reference{Title: label, Status: "broken"}
	}
	fences := s.buildFenceHandlers(ctx, root, note, notes)
	view := noterender.Build(noterender.Inputs{Body: note.Body, Ref: ref, Display: req.Display, Badge: noterender.Badge{ObjectID: ref.ObjectID, Version: ref.Version, Revision: ref.Revision, RenderedAt: s.currentTimeUTC().Format(time.RFC3339Nano)}, Backlinks: backlinks, Resolve: resolve, Fences: fences})
	p := domain.NewProjection(command, "Bounded rendered note projection assembled.")
	p.Data = view
	p.Facts["schema_version"], p.Facts["display"], p.Facts["body_requested"] = noterender.SchemaVersion, req.Display, fmt.Sprint(req.Display == "body")
	p.Facts["relations_source"] = "current_vault"
	p.Facts["object_id"], p.Facts["version"], p.Facts["revision"] = ref.ObjectID, fmt.Sprint(ref.Version), ref.Revision
	p.Facts["truncated"], p.Facts["dropped_unsafe"], p.Facts["stale"] = fmt.Sprint(view.Truncated), fmt.Sprint(view.DroppedUnsafe), fmt.Sprint(resolved.Stale)
	p.Facts["local_write"], p.Facts["remote_write"] = "false", "false"
	if view.Truncated || len(view.Warnings) > 0 || resolved.Stale {
		p.Status = "partial"
	}
	return p, nil
}

func (s *Service) renderDatabaseFence(root, name string, notes []domain.Note) noterender.FenceResult {
	registry, err := loadSavedViews(root)
	if err != nil {
		return noterender.FenceResult{ErrorCode: "view_unavailable", NextAction: "pinax database view list"}
	}
	view, ok := findSavedView(registry, name)
	if !ok {
		return noterender.FenceResult{ErrorCode: "view_not_found", NextAction: "pinax database view list"}
	}
	var result domain.TableResult
	if view.Query != "" {
		parse := searchops.ParseSQL
		if view.Language == "dataview" {
			parse = searchops.ParseDataview
		}
		query, err := parse(view.Query)
		if err != nil {
			return noterender.FenceResult{ErrorCode: "view_query_invalid", NextAction: "pinax database view show <name>"}
		}
		result = searchops.ExecuteQuery(ordinaryNotes(notes), query, searchops.QueryRequest{Limit: 20})
	} else {
		p, err := s.ShowView(context.Background(), ViewRequest{VaultPath: root, Name: name, Limit: 20})
		if err != nil {
			return noterender.FenceResult{ErrorCode: "view_query_failed", NextAction: "pinax database view show <name>"}
		}
		var ok bool
		result, ok = tableResultFromDatabaseViewData(p.Data.(map[string]any))
		if !ok {
			return noterender.FenceResult{ErrorCode: "view_query_failed", NextAction: "pinax database view show <name>"}
		}
	}
	display := view.Kind
	if display == "" {
		display = "table"
	}
	render, renderErr := buildDatabaseViewRender(view, result, display)
	if renderErr != nil {
		return noterender.FenceResult{ErrorCode: "view_render_failed", NextAction: "pinax database view show <name>"}
	}
	return noterender.FenceResult{Payload: render, Truncated: result.Page.HasMore}
}

func renderGraphFence(ctx context.Context, root string, note domain.Note, notes []domain.Note, arg string) noterender.FenceResult {
	hops := 1
	if strings.TrimSpace(arg) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "hops=")))
		if err != nil || n < 1 || n > 2 {
			return noterender.FenceResult{ErrorCode: "graph_depth_invalid", NextAction: "Use pinax-graph hops=1 or hops=2"}
		}
		hops = n
	}
	out, incoming := notelinks.BuildGraph(notes)
	seen := map[string]bool{note.Path: true}
	frontier := []string{note.Path}
	edges := []map[string]string{}
	edgeSeen := map[string]bool{}
	truncated := false
	for depth := 0; depth < hops; depth++ {
		next := []string{}
		for _, source := range frontier {
			links := append(append([]domain.NoteLink{}, out[source]...), incoming[source]...)
			for _, link := range links {
				if link.Status != "resolved" {
					continue
				}
				a, b := link.SourcePath, link.TargetPath
				for _, candidate := range []string{a, b} {
					if !seen[candidate] {
						if len(seen) == 50 {
							truncated = true
							continue
						}
						seen[candidate] = true
						next = append(next, candidate)
					}
				}
				if seen[a] && seen[b] {
					key := a + "\x00" + b
					if edgeSeen[key] {
						continue
					}
					edgeSeen[key] = true
					if len(edges) < 100 {
						edges = append(edges, map[string]string{"source": link.SourceNoteID, "target": link.TargetNoteID})
					} else {
						truncated = true
					}
				}
			}
		}
		sort.Strings(next)
		frontier = next
	}
	nodes := []noterender.Reference{}
	for _, n := range notes {
		if seen[n.Path] {
			r, err := renderReference(ctx, root, n)
			if err != nil {
				return noterender.FenceResult{ErrorCode: "graph_unavailable", NextAction: "pinax vault doctor"}
			}
			nodes = append(nodes, r)
		}
	}
	return noterender.FenceResult{Payload: map[string]any{"hops": hops, "nodes": nodes, "edges": edges}, Truncated: truncated}
}

func renderBoardFence(root, arg string, notes []domain.Note) noterender.FenceResult {
	parts := strings.SplitN(arg, "/", 2)
	project, err := findProject(root, parts[0])
	if err != nil {
		return noterender.FenceResult{ErrorCode: "board_not_found", NextAction: "pinax project list"}
	}
	var workspace *domain.ProjectWorkspace
	if len(parts) == 2 {
		slug, e := validateSubprojectSlug(parts[1])
		if e != nil {
			return noterender.FenceResult{ErrorCode: "board_not_found", NextAction: "pinax project workspace list"}
		}
		w, loadErr := loadProjectWorkspace(root, project.Slug, slug)
		if loadErr != nil {
			return noterender.FenceResult{ErrorCode: "board_not_found", NextAction: "pinax project workspace list"}
		}
		workspace = &w
	}
	columns, err := loadProjectBoardColumns(root, project.Slug, workspaceSubproject(workspace))
	if err != nil {
		return noterender.FenceResult{ErrorCode: "board_unavailable", NextAction: "pinax project board show <project>"}
	}
	// Reuse the domain projection without opening the optional SQLite index.
	board := buildProjectBoard(root, project, workspace, columns, ordinaryNotes(notes), domain.NoteDisplayCard, "scan", "not_checked", false)
	return noterender.FenceResult{Payload: board}
}

// buildFenceHandlers assembles the owner-registered read-only fence map
// shared by the legacy rendered-note projection and the human reading
// document surface. Handlers only read vault projections; they never
// execute note code.
func (s *Service) buildFenceHandlers(ctx context.Context, root string, note domain.Note, notes []domain.Note) map[string]func(string) noterender.FenceResult {
	// Legacy rendered-note contract: the full registered set, unscoped. Only
	// the human reading session path filters via buildScopedFenceHandlers.
	return s.buildFenceHandlersUnscoped(ctx, root, note, notes)
}

// fenceHandlerScopes maps each owner fence to the reader scope its payload
// requires: graph projects relations; board/timeline/database-view project
// other objects' metadata. A session without the scope never runs the
// handler — the fence source stays inert code instead of leaking cross-note
// projections through a body-only grant.
var fenceHandlerScopes = map[string]string{
	"pinax-graph":         ReaderScopeRelations,
	"pinax-board":         ReaderScopeMetadata,
	"pinax-timeline":      ReaderScopeMetadata,
	"pinax-database-view": ReaderScopeMetadata,
}

// buildScopedFenceHandlers filters the registered handlers down to those
// the session's scopes authorize.
func (s *Service) buildScopedFenceHandlers(ctx context.Context, root string, note domain.Note, notes []domain.Note, scopes []string) map[string]func(string) noterender.FenceResult {
	granted := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		granted[scope] = true
	}
	allowed := make(map[string]func(string) noterender.FenceResult)
	for name, handler := range s.buildFenceHandlersUnscoped(ctx, root, note, notes) {
		if required, ok := fenceHandlerScopes[name]; ok && !granted[required] {
			continue
		}
		allowed[name] = handler
	}
	return allowed
}

func (s *Service) buildFenceHandlersUnscoped(ctx context.Context, root string, note domain.Note, notes []domain.Note) map[string]func(string) noterender.FenceResult {
	fences := map[string]func(string) noterender.FenceResult{}
	fences["pinax-database-view"] = func(arg string) noterender.FenceResult { return s.renderDatabaseFence(root, arg, notes) }
	fences["pinax-graph"] = func(arg string) noterender.FenceResult { return renderGraphFence(ctx, root, note, notes, arg) }
	fences["pinax-board"] = func(arg string) noterender.FenceResult {
		return renderBoardFence(root, arg, notes)
	}
	fences["pinax-timeline"] = func(_ string) noterender.FenceResult {
		events, err := records.NewService(root).HistoryReadOnly(ctx)
		if err != nil {
			return noterender.FenceResult{ErrorCode: "timeline_unavailable", NextAction: "pinax record history <object-id>"}
		}
		rows := []map[string]any{}
		for _, e := range events {
			if e.ObjectID == note.ID {
				rows = append(rows, map[string]any{"op": e.Kind, "ref": note.ID, "revision": e.ContentRevision.Hash, "time": e.CreatedAt})
			}
		}
		truncated := len(rows) > 100
		if truncated {
			rows = rows[len(rows)-100:]
		}
		return noterender.FenceResult{Payload: rows, Truncated: truncated}
	}
	return fences
}
