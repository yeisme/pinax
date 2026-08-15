package index

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/index/model"
	"github.com/yeisme/pinax/internal/index/query"
	"github.com/yeisme/pinax/internal/notelinks"
	"gorm.io/gorm"
)

func reclassifyAffectedLinkEdges(tx *gorm.DB, targetKeys map[string]bool, changedPath string) error {
	if len(targetKeys) == 0 {
		return nil
	}
	ctx := context.Background()
	q := query.Use(tx)
	linkRows, err := q.LinkRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	affected := map[string]bool{}
	for _, link := range linkRows {
		if linkMatchesTargetKeys(*link, targetKeys) {
			affected[link.NotePath] = true
		}
	}
	if len(affected) == 0 {
		return nil
	}
	noteRows, err := q.NoteRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	records := make([]NoteRecord, 0, len(noteRows))
	var changed *model.NoteRecord
	for _, record := range noteRows {
		records = append(records, *record)
		if record.Path == changedPath {
			changed = record
		}
	}
	resolver := resolverSnapshotFromRecords(records)
	paths := make([]string, 0, len(affected))
	for path := range affected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		note, ok, lookupErr := indexedNoteForLinkRebuild(tx, path)
		if lookupErr != nil {
			return lookupErr
		}
		if !ok {
			continue
		}
		previousRows, err := q.LinkRecord.WithContext(ctx).Where(q.LinkRecord.NotePath.Eq(path)).Find()
		if err != nil {
			return err
		}
		previous := map[string]*model.LinkRecord{}
		for _, row := range previousRows {
			previous[linkEdgeKey(*row)] = row
		}
		if _, err := q.LinkRecord.WithContext(ctx).Where(q.LinkRecord.NotePath.Eq(path)).Delete(); err != nil {
			return err
		}
		for _, link := range noteLinks(note, resolver) {
			if changed != nil && link.Status != string(domain.LinkStatusResolved) {
				if old := previous[linkEdgeKey(link)]; old != nil && old.TargetObjectID == changed.ObjectID {
					link.TargetObjectID = changed.ObjectID
					link.TargetNoteID = changed.NoteID
					link.TargetPath = changed.Path
					link.TargetTitle = changed.Title
					link.Status = string(domain.LinkStatusResolved)
					link.Broken = false
					link.Evidence = "preserved by object_id; link rewrite review required"
				}
			}
			if err := q.LinkRecord.WithContext(ctx).Create(&link); err != nil {
				return err
			}
		}
	}
	return nil
}

func linkEdgeKey(link LinkRecord) string {
	return link.Kind + "\x00" + link.TargetRaw + "\x00" + link.TargetAlias + "\x00" + link.TargetHeading
}

func LinksByTargetObjectID(root, objectID string) ([]LinkRecord, error) {
	db, err := open(root)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	q := query.Use(db)
	rows, err := q.LinkRecord.WithContext(context.Background()).Where(q.LinkRecord.TargetObjectID.Eq(strings.TrimSpace(objectID))).Order(q.LinkRecord.NotePath, q.LinkRecord.Line).Find()
	if err != nil {
		return nil, err
	}
	links := make([]LinkRecord, 0, len(rows))
	for _, row := range rows {
		links = append(links, *row)
	}
	return links, nil
}

func indexedNoteForLinkRebuild(tx *gorm.DB, path string) (domain.Note, bool, error) {
	ctx := context.Background()
	q := query.Use(tx)
	record, err := q.NoteRecord.WithContext(ctx).Where(q.NoteRecord.Path.Eq(path)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Note{}, false, nil
		}
		return domain.Note{}, false, err
	}
	text, err := q.NoteTextRecord.WithContext(ctx).Where(q.NoteTextRecord.NotePath.Eq(path)).First()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Note{}, false, err
	}
	bodyText := ""
	if text != nil {
		bodyText = text.BodyText
	}
	return domain.Note{ID: record.NoteID, Title: record.Title, Path: record.Path, Body: bodyText, Project: record.Project, Folder: record.Folder, Kind: record.Kind, Status: record.Status, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, true, nil
}

func linkMatchesTargetKeys(link LinkRecord, targetKeys map[string]bool) bool {
	for _, value := range []string{link.Target, link.TargetRaw, link.TargetPath, link.TargetNoteID, link.TargetTitle} {
		if targetKeys[normalizeLinkTargetKey(value)] {
			return true
		}
	}
	return false
}

func mergeLinkTargetKeys(groups ...map[string]bool) map[string]bool {
	merged := map[string]bool{}
	for _, group := range groups {
		for key := range group {
			if key != "" {
				merged[key] = true
			}
		}
	}
	return merged
}

func linkTargetKeysFromRecord(record NoteRecord) map[string]bool {
	keys := map[string]bool{}
	for _, value := range []string{record.Title, record.NoteID, record.Path, strings.TrimSuffix(filepath.Base(record.Path), filepath.Ext(record.Path))} {
		key := normalizeLinkTargetKey(value)
		if key != "" {
			keys[key] = true
		}
	}
	return keys
}

func normalizeLinkTargetKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return ""
	}
	value = filepath.ToSlash(filepath.Clean(value))
	if value == "." {
		return ""
	}
	return strings.ToLower(value)
}

func noteLinks(note domain.Note, resolver notelinks.ResolverSnapshot) []LinkRecord {
	rawLinks := notelinks.ParseNoteLinks(note.Body)
	links := make([]LinkRecord, 0, len(rawLinks))
	for _, raw := range rawLinks {
		resolved := notelinks.ResolveLinkTarget(note, raw, resolver).Link
		links = append(links, linkRecordFromDomainLink(note, resolved))
	}
	return links
}

func linkRecordFromDomainLink(note domain.Note, link domain.NoteLink) LinkRecord {
	return LinkRecord{
		SourceObjectID: note.ID,
		TargetObjectID: link.TargetObjectID,
		NotePath:       note.Path,
		SourceNoteID:   link.SourceNoteID,
		Target:         link.Target,
		TargetPath:     link.TargetPath,
		Kind:           link.Kind,
		Broken:         link.Broken,
		TargetNoteID:   link.TargetNoteID,
		TargetTitle:    link.TargetTitle,
		TargetRaw:      link.TargetRaw,
		TargetAlias:    link.TargetAlias,
		TargetHeading:  link.TargetHeading,
		Status:         link.Status,
		Line:           link.Line,
		Evidence:       link.Evidence,
	}
}

func resolverSnapshotFromRecords(records []NoteRecord) notelinks.ResolverSnapshot {
	notes := make([]domain.Note, 0, len(records))
	for _, record := range records {
		notes = append(notes, domain.Note{ID: record.NoteID, Title: record.Title, Path: record.Path, Project: record.Project, Folder: record.Folder, Kind: record.Kind, Status: record.Status, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt})
	}
	return notelinks.BuildResolverSnapshot(notes)
}
