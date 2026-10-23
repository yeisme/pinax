package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata" // Keep IANA calendar lookup available in offline desktop packages.

	"github.com/yeisme/pinax/internal/templateengine"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

type ReaderDailyRequest struct {
	VaultPath string
	Session   ReaderSessionInfo
	Date      string
	Timezone  string
}

type ReaderDaily struct {
	SchemaVersion string                `json:"schema_version"`
	BindingID     string                `json:"binding_id"`
	Date          string                `json:"date"`
	Timezone      string                `json:"timezone"`
	Status        string                `json:"status"`
	Note          *ReaderNavigationNote `json:"note"`
	Watermark     string                `json:"watermark"`
	ObservedAt    string                `json:"observed_at"`
}

// ReaderDaily locates an existing journal without invoking DailyShow or
// ensureJournalNote: both legacy operations can create files and refresh state.
func (s *Service) ReaderDaily(ctx context.Context, req ReaderDailyRequest) (ReaderDaily, error) {
	var empty ReaderDaily
	if !req.Session.HasScope(ReaderScopeMetadata) {
		return empty, ReaderError("insufficient_scope", "Daily lookup requires reader.metadata", "")
	}
	invalid := ReaderError("reader_request_invalid", "Daily lookup requires YYYY-MM-DD and an IANA timezone", "")
	if req.Session.BindingID == "" || req.Timezone == "Local" || req.Timezone == "" || strings.HasPrefix(req.Timezone, "/") || strings.Contains(req.Timezone, "..") || len(req.Timezone) > 128 {
		return empty, invalid
	}
	zone, err := time.LoadLocation(req.Timezone)
	if err != nil {
		return empty, invalid
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil || date.Format("2006-01-02") != req.Date {
		return empty, invalid
	}
	// Midnight can be skipped by DST while the calendar date is valid. Noon
	// preserves those dates and still rejects an entirely skipped local day.
	localDay := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, zone)
	if localDay.Format("2006-01-02") != req.Date {
		return empty, invalid
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return empty, ReaderError("reader_unavailable", "The source is unavailable", "")
	}
	template, err := readerDailyTemplate(root)
	if err != nil {
		return empty, err
	}
	document, err := templateengine.ParseDocument("journal.daily", template)
	if err != nil {
		return empty, ReaderError("reader_unsupported", "The daily template metadata is invalid", "Repair the template with the owner")
	}
	// Inspect output metadata only; do not render template queries or fences.
	rel := journalPathFromPattern(document.Metadata.Output.PathPattern, "daily", req.Date)
	if !readerDailyVisiblePath(rel) {
		return empty, ReaderError("reader_unsupported", "The daily template output is not a visible note path", "")
	}
	notes, watermark, _, err := navigationSnapshot(ctx, root, true, false)
	if err != nil {
		return empty, err
	}
	recordedID, err := readerDailyRecordedIdentity(ctx, root, req.Date)
	if err != nil {
		return empty, err
	}
	page := ReaderDaily{SchemaVersion: catalogschema.ReaderDailyV1Alpha1, BindingID: req.Session.BindingID, Date: req.Date, Timezone: req.Timezone, Status: "missing", Watermark: readerNavigationDigest([]string{watermark, collaborationRevision(template), req.Date, req.Timezone, recordedID}), ObservedAt: readerObservedAt(s.currentTimeUTC())}
	candidates := map[string]bool{rel: true, filepath.ToSlash(filepath.Join("notes", "daily", req.Date+".md")): true}
	for _, note := range notes {
		if (!candidates[note.Path] && note.ID != recordedID) || note.Kind != "daily" {
			continue
		}
		if page.Note != nil || recordedID != "" && note.ID != recordedID {
			return empty, ReaderError("reader_unsupported", "Multiple daily journals match this calendar date", "Resolve the duplicate with the owner")
		}
		page.Note = &ReaderNavigationNote{ObjectID: note.ID, Revision: readerNoteRevision(note), Title: note.Title}
		page.Status = "found"
	}
	return page, nil
}

func readerDailyVisiblePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") || strings.ContainsAny(path, "{}\x00:") {
		return false
	}
	if filepath.ToSlash(filepath.Clean(path)) != path || !strings.EqualFold(filepath.Ext(path), ".md") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

// The override is an owner-authored template, not a note resource. Read only
// this fixed metadata location with a bounded descriptor; never follow links.
func readerDailyTemplate(rootPath string) (string, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return "", ReaderError("reader_unavailable", "The source is unavailable", "")
	}
	defer func() { _ = root.Close() }()
	parts := []string{".pinax", "templates", "journal.daily.md"}
	var selected os.FileInfo
	for index := range parts {
		info, err := root.Lstat(filepath.Join(parts[:index+1]...))
		if errors.Is(err, os.ErrNotExist) {
			return builtInTemplates()["journal.daily"], nil
		}
		if err != nil {
			return "", ReaderError("reader_unavailable", "Daily template could not be inspected", "")
		}
		if info.Mode()&os.ModeSymlink != 0 || index < len(parts)-1 && !info.IsDir() || index == len(parts)-1 && !info.Mode().IsRegular() {
			return "", ReaderError("reader_unsupported", "Daily template must be a regular owner file", "")
		}
		selected = info
	}
	file, err := root.Open(filepath.Join(parts...))
	if err != nil {
		return "", ReaderError("reader_unavailable", "Daily template could not be read", "")
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(selected, opened) || !opened.Mode().IsRegular() {
		return "", ReaderError("source_changed", "Daily template changed while opening", "")
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(catalogschema.WriterMaxBodyBytes)+1))
	if err != nil {
		return "", ReaderError("reader_unavailable", "Daily template could not be read", "")
	}
	if len(body) > catalogschema.WriterMaxBodyBytes {
		return "", ReaderError("reader_unsupported", "Daily template exceeds its inspection budget", "")
	}
	current, err := root.Lstat(filepath.Join(parts...))
	if err != nil || !os.SameFile(opened, current) || opened.Size() != current.Size() || !opened.ModTime().Equal(current.ModTime()) {
		return "", ReaderError("source_changed", "Daily template changed while reading", "")
	}
	return string(body), nil
}
