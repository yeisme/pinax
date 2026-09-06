package index

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/index/query"
	"gorm.io/gorm"
)

func replaceNoteProjection(tx *gorm.DB, root string, note domain.Note) error {
	ctx := context.Background()
	q := query.Use(tx)
	if err := deleteNotePathProjection(q, ctx, note.Path); err != nil {
		return err
	}
	if _, err := q.AssetLinkRecord.WithContext(ctx).Where(q.AssetLinkRecord.SourcePath.Eq(note.Path)).Delete(); err != nil {
		return err
	}
	if err := q.NoteTextRecord.WithContext(ctx).Create(&NoteTextRecord{ObjectID: note.ID, NotePath: note.Path, TitleText: note.Title, BodyText: note.Body, Excerpt: excerpt(note.Body), WordCount: len(tokens(note.Body))}); err != nil {
		return err
	}
	for _, tag := range uniqueTags(note) {
		if err := q.TagRecord.WithContext(ctx).Create(&TagRecord{ObjectID: note.ID, NotePath: note.Path, Tag: tag}); err != nil {
			return err
		}
	}
	for _, token := range noteTokens(note) {
		if err := q.SearchTokenRecord.WithContext(ctx).Create(&SearchTokenRecord{ObjectID: note.ID, NotePath: note.Path, Token: token.Token, Field: token.Field, Count: token.Count, Weight: token.Weight}); err != nil {
			return err
		}
	}
	linkRecords, err := q.NoteRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	records := make([]NoteRecord, 0, len(linkRecords))
	for _, record := range linkRecords {
		records = append(records, *record)
	}
	for _, link := range noteLinks(note, resolverSnapshotFromRecords(records)) {
		if err := q.LinkRecord.WithContext(ctx).Create(&link); err != nil {
			return err
		}
	}
	for _, attachment := range noteAttachments(root, note) {
		if err := q.AttachmentRecord.WithContext(ctx).Create(&attachment); err != nil {
			return err
		}
	}
	for _, assetLink := range noteAssetLinks(root, note) {
		if err := q.AssetLinkRecord.WithContext(ctx).Create(&assetLink); err != nil {
			return err
		}
	}
	return nil
}

func deleteNotePathProjection(q *query.Query, ctx context.Context, notePath string) error {
	clearers := []func() error{
		func() error {
			_, err := q.NoteTextRecord.WithContext(ctx).Where(q.NoteTextRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
		func() error {
			_, err := q.TagRecord.WithContext(ctx).Where(q.TagRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
		func() error {
			_, err := q.LinkRecord.WithContext(ctx).Where(q.LinkRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
		func() error {
			_, err := q.SearchTokenRecord.WithContext(ctx).Where(q.SearchTokenRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
		func() error {
			_, err := q.AttachmentRecord.WithContext(ctx).Where(q.AttachmentRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
	}
	for _, clear := range clearers {
		if err := clear(); err != nil {
			return err
		}
	}
	return nil
}

func deleteNoteProjection(tx *gorm.DB, path string, includeRecord bool) error {
	ctx := context.Background()
	q := query.Use(tx)
	path = filepath.ToSlash(filepath.Clean(path))
	if includeRecord {
		if _, err := q.NoteRecord.WithContext(ctx).Where(q.NoteRecord.Path.Eq(path)).Delete(); err != nil {
			return err
		}
	}
	if err := deleteNotePathProjection(q, ctx, path); err != nil {
		return err
	}
	if _, err := q.AssetLinkRecord.WithContext(ctx).Where(q.AssetLinkRecord.SourcePath.Eq(path)).Delete(); err != nil {
		return err
	}
	return nil
}

func uniqueTags(note domain.Note) []string {
	seen := map[string]bool{}
	for _, tag := range note.Tags {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
		if tag != "" {
			seen[tag] = true
		}
	}
	for _, match := range inlineTagPattern.FindAllStringSubmatch(note.Body, -1) {
		if len(match) > 2 && match[2] != "" {
			seen[match[2]] = true
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	return tags
}

func noteAttachments(root string, note domain.Note) []AttachmentRecord {
	links := pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{SourceNoteID: note.ID, SourcePath: note.Path, Body: note.Body})
	attachments := make([]AttachmentRecord, 0, len(links))
	for _, link := range links {
		_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(link.AssetPath)))
		attachments = append(attachments, AttachmentRecord{ObjectID: note.ID, NotePath: note.Path, ReferenceText: link.RawReference, TargetPath: link.AssetPath, MediaType: mediaType(link.AssetPath), Exists: statErr == nil})
	}
	return attachments
}

func noteAssetLinks(root string, note domain.Note) []AssetLinkRecord {
	links := pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{SourceNoteID: note.ID, SourcePath: note.Path, Body: note.Body})
	records := make([]AssetLinkRecord, 0, len(links))
	for _, link := range links {
		status := "resolved"
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(link.AssetPath))); err != nil {
			status = "missing"
		}
		records = append(records, AssetLinkRecord{SourceObjectID: link.SourceNoteID, AssetPath: link.AssetPath, SourceNoteID: link.SourceNoteID, SourcePath: link.SourcePath, RawReference: link.RawReference, LinkStyle: link.LinkStyle, LinkKind: link.LinkKind, Line: link.Line, Status: status, MediaType: mediaType(link.AssetPath)})
	}
	return records
}

func noteDimensionCounts(notes []domain.Note) []DimensionCountRecord {
	counts := map[string]map[string]int{
		"tag":    {},
		"group":  {},
		"folder": {},
		"kind":   {},
		"status": {},
	}
	for _, note := range notes {
		for _, tag := range uniqueTags(note) {
			counts["tag"][tag]++
		}
		counts["group"][noteProject(note)]++
		counts["folder"][strings.TrimSpace(note.Folder)]++
		counts["kind"][strings.TrimSpace(note.Kind)]++
		counts["status"][strings.TrimSpace(note.Status)]++
	}
	records := make([]DimensionCountRecord, 0)
	for dimension, values := range counts {
		for value, count := range values {
			records = append(records, DimensionCountRecord{Dimension: dimension, Value: value, Count: count})
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Dimension == records[j].Dimension {
			return records[i].Value < records[j].Value
		}
		return records[i].Dimension < records[j].Dimension
	})
	return records
}

func mediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return "image"
	case ".pdf":
		return "document"
	default:
		return "file"
	}
}

func noteProject(note domain.Note) string {
	if strings.TrimSpace(note.Project) != "" {
		return note.Project
	}
	return projectFromPath(note.Path)
}

func isSystemIndexNote(note domain.Note) bool {
	path := filepath.ToSlash(note.Path)
	if note.Kind == "index" {
		return strings.HasPrefix(path, "index/") || strings.HasPrefix(path, "notes/index/") || strings.HasPrefix(path, "notes/daily/")
	}
	if note.Kind == "daily" || note.Kind == "weekly" || note.Kind == "monthly" {
		return strings.HasPrefix(path, note.Kind+"/") || strings.HasPrefix(path, "notes/"+note.Kind+"/")
	}
	return false
}

func inferLifecycleStatus(status, kind string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "inbox", "draft", "active", "archived", "discarded":
		return s
	case "system":
		if kind == "index" {
			return "system"
		}
		return "active"
	default:
		return "active"
	}
}

func noteRecordFromDomain(note domain.Note, modifiedUnix, size int64) NoteRecord {
	filename := filepath.Base(note.Path)
	ext := filepath.Ext(filename)
	return NoteRecord{ObjectID: note.ID, Path: note.Path, NoteID: note.ID, Title: note.Title, Filename: filename, Stem: strings.TrimSuffix(filename, ext), Project: noteProject(note), Group: noteProject(note), Folder: note.Folder, Kind: note.Kind, Status: note.Status, LifecycleStatus: inferLifecycleStatus(note.Status, note.Kind), CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt, SourceHash: noteSourceHash(note), ModifiedUnix: modifiedUnix, Size: size, IsSystem: isSystemIndexNote(note), ObjectKind: string(domain.VaultObjectKindNote), ManagedStatus: string(domain.ManagedStatusRegistered), TrustTier: domain.TrustTierOf(note.Trust), StaleAfter: noteTrustStaleAfter(note), VerifiedAtLatest: noteTrustVerifiedAtLatest(note)}
}

// noteTrustStaleAfter 返回信任信号的 stale_after 原始时间戳缓存（无则空）。
func noteTrustStaleAfter(note domain.Note) string {
	if note.Trust == nil {
		return ""
	}
	return strings.TrimSpace(note.Trust.StaleAfter)
}

// noteTrustVerifiedAtLatest 返回最新 verified.at 缓存（无事件则空）。
func noteTrustVerifiedAtLatest(note domain.Note) string {
	if note.Trust == nil {
		return ""
	}
	return note.Trust.LatestVerifiedAt()
}

func noteSourceHash(note domain.Note) string {
	parts := []string{note.ID, note.Title, note.Path, strings.Join(note.Tags, ","), note.Body, note.Project, note.Folder, note.Kind, note.Status, note.CreatedAt, note.UpdatedAt, noteTrustHash(note)}
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])
}

// noteTrustHash 把信任信号纳入 source hash，使 verify/信任字段变更能触发增量重投影。
func noteTrustHash(note domain.Note) string {
	if note.Trust == nil {
		return ""
	}
	trust := note.Trust
	parts := []string{trust.Generated.By, trust.Generated.At, trust.StaleAfter}
	for _, event := range trust.Verified {
		parts = append(parts, event.By, event.At, event.Note)
	}
	return strings.Join(parts, "\x00")
}

func noteTokens(note domain.Note) []tokenRecord {
	counts := map[string]tokenRecord{}
	add := func(field, text string, weight int) {
		for _, token := range tokens(text) {
			key := field + "\x00" + token
			record := counts[key]
			record.Token = token
			record.Field = field
			record.Weight = weight
			record.Count++
			counts[key] = record
		}
	}
	add("title", note.Title, 5)
	add("tag", strings.Join(note.Tags, " "), 4)
	add("path", note.Path, 2)
	add("body", note.Body, 1)
	records := make([]tokenRecord, 0, len(counts))
	for _, record := range counts {
		records = append(records, record)
	}
	return records
}

func excerpt(body string) string {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\n", " "))
	if len(body) <= 120 {
		return body
	}
	return body[:120]
}
