package app

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/markdownnote"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// WriterEditorSourceRequest selects a live, exact revision by object identity.
// Unlike source-export this projection is never built from the reader AST.
type WriterEditorSourceRequest struct {
	VaultPath string
	ObjectID  string
	Revision  string
	Session   ReaderSessionInfo
}

// WriterEditorMetadata is an informational whitelist, not writable frontmatter.
type WriterEditorMetadata struct {
	NoteID string   `json:"note_id"`
	Title  string   `json:"title"`
	Tags   []string `json:"tags"`
	Status string   `json:"status"`
	Kind   string   `json:"kind"`
}

type WriterEditorBudget struct {
	MaxSourceBytes int `json:"max_source_bytes"`
	MaxBodyBytes   int `json:"max_body_bytes"`
	SourceBytes    int `json:"source_bytes"`
	BodyBytes      int `json:"body_bytes"`
}

type WriterEditorSource struct {
	SchemaVersion        string               `json:"schema_version"`
	ObjectID             string               `json:"object_id"`
	Revision             string               `json:"revision"`
	Body                 string               `json:"body"`
	BodyDigest           string               `json:"body_digest"`
	Editable             bool                 `json:"editable"`
	ReadonlyReason       string               `json:"readonly_reason"`
	FrontmatterProtected bool                 `json:"frontmatter_protected"`
	ProtectedMetadata    WriterEditorMetadata `json:"protected_metadata"`
	Budget               WriterEditorBudget   `json:"budget"`
}

// WriterEditorSource does not initialize a vault, refresh an index, create a
// snapshot or write an operation. It reads bounded candidates through os.Root,
// rejects ambiguous identities, and checks the source digest after selection.
func (s *Service) WriterEditorSource(ctx context.Context, req WriterEditorSourceRequest) (WriterEditorSource, error) {
	return s.writerEditorSourceSelected(ctx, req, func(note domain.Note) bool {
		return !isSystemIndexNote(note) && !isSystemJournalNote(note)
	})
}

// Selection policy stays private to owner services, not a transport flag.
func (s *Service) writerEditorSourceSelected(ctx context.Context, req WriterEditorSourceRequest, visible func(domain.Note) bool) (WriterEditorSource, error) {
	var empty WriterEditorSource
	if !req.Session.HasScope(WriterScopeEditor) || !req.Session.HasScope(ReaderScopeBody) {
		return empty, collaborationError("insufficient_scope", "Editor source requires writer.editor and reader.body")
	}
	if err := catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterEditorSourceRequestV1Alpha1, map[string]any{"object_id": req.ObjectID, "revision": req.Revision}); err != nil {
		return empty, collaborationError("writer_request_invalid", "Editor source requires an object id and exact sha256 revision")
	}
	rootPath, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return empty, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return empty, collaborationError("writer_unavailable", "The authorized source is unavailable")
	}
	defer func() { _ = root.Close() }()
	var selected []byte
	var selectedMeta map[string]string
	var selectedBody string
	var selectedPath string
	matches := 0
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return collaborationError("writer_unavailable", "Source coverage could not be verified")
		}
		if path == "." {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		raw, err := readWriterEditorCandidate(root, path)
		if err != nil {
			return err
		}
		meta, body, diagnostics := markdownnote.ParseFrontmatter(raw)
		if len(diagnostics) != 0 || !isPinaxNoteFrontmatter(meta) || meta["note_id"] != req.ObjectID {
			return nil
		}
		note := domain.Note{Path: path, Kind: meta["kind"]}
		if !visible(note) {
			return nil
		}
		if _, err := domain.ParseTrustSignals(raw); err != nil {
			return nil
		}
		matches++
		if matches > 1 {
			return collaborationError("note_identity_ambiguous", "The object identity is not unique; repair it with the owner before editing")
		}
		selected, selectedMeta, selectedBody, selectedPath = raw, meta, body, path
		return nil
	})
	if err != nil {
		return empty, err
	}
	if matches == 0 {
		return empty, collaborationError("note_not_found", "The requested note is not visible")
	}
	if len(selected) > catalogschema.WriterMaxBodyBytes {
		return empty, collaborationError("body_too_large", "The source exceeds the 64 KiB editor budget; continue in read-only mode")
	}
	if !utf8.Valid(selected) || strings.ContainsRune(string(selected), 0) {
		return empty, collaborationError("body_invalid", "Editor source must be valid UTF-8 without NUL")
	}
	revision := collaborationRevision(string(selected))
	if revision != req.Revision {
		return empty, collaborationError("revision_conflict", "The requested editor revision is no longer current; reopen the note")
	}
	// Recheck the selected object after walking siblings; never silently serve
	// an older body if an external editor changed it during discovery.
	current, err := readWriterEditorCandidate(root, selectedPath)
	if err != nil {
		return empty, err
	}
	if collaborationRevision(string(current)) != revision {
		return empty, collaborationError("revision_conflict", "The source changed while opening the editor")
	}
	legacyMeta, legacyBody := splitFrontmatter(string(selected))
	editable := legacyMeta["note_id"] == req.ObjectID && legacyBody == selectedBody
	reason := "none"
	if !editable {
		reason = "frontmatter_unsupported"
	}
	tags := parseTags(selectedMeta["tags"])
	if tags == nil {
		tags = []string{}
	}
	return WriterEditorSource{
		SchemaVersion: catalogschema.WriterEditorSourceV1Alpha1,
		ObjectID:      req.ObjectID, Revision: revision, Body: selectedBody,
		BodyDigest: collaborationRevision(selectedBody), Editable: editable, ReadonlyReason: reason,
		FrontmatterProtected: true,
		ProtectedMetadata:    WriterEditorMetadata{NoteID: req.ObjectID, Title: selectedMeta["title"], Tags: tags, Status: selectedMeta["status"], Kind: selectedMeta["kind"]},
		Budget:               WriterEditorBudget{MaxSourceBytes: catalogschema.WriterMaxBodyBytes, MaxBodyBytes: catalogschema.WriterMaxBodyBytes, SourceBytes: len(selected), BodyBytes: len(selectedBody)},
	}, nil
}

// Both ancestry checks reject symlinks; os.Root also prevents a concurrent
// symlink replacement from escaping the authorized root between checks.
func readWriterEditorCandidate(root *os.Root, path string) ([]byte, error) {
	before, err := writerEditorSafeFile(root, path)
	if err != nil {
		return nil, err
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, collaborationError("writer_unavailable", "Source could not be read")
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, collaborationError("source_changed", "The selected source changed")
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(catalogschema.WriterMaxBodyBytes)+1))
	if err != nil {
		return nil, collaborationError("writer_unavailable", "Source could not be read")
	}
	after, err := writerEditorSafeFile(root, path)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(opened, after) || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) {
		return nil, collaborationError("source_changed", "The selected source changed")
	}
	return raw, nil
}

func writerEditorSafeFile(root *os.Root, path string) (fs.FileInfo, error) {
	var info fs.FileInfo
	prefix := ""
	parts := strings.Split(filepath.ToSlash(path), "/")
	for index, part := range parts {
		if part == "" || strings.HasPrefix(part, ".") {
			return nil, collaborationError("unsafe_path", "Editor source is not a visible regular file")
		}
		prefix = filepath.Join(prefix, part)
		var err error
		info, err = root.Lstat(prefix)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, collaborationError("source_changed", "Editor source ancestry changed")
		}
		if index < len(parts)-1 && !info.IsDir() || index == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, collaborationError("unsafe_path", "Editor source is not a regular file")
		}
	}
	return info, nil
}
