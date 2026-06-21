package index

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
)

func ExtractLinkRows(notes []domain.Note) []domain.DatabaseRow {
	pathByTitle := map[string]string{}
	for _, note := range notes {
		pathByTitle[strings.ToLower(note.Title)] = note.Path
	}
	rows := []domain.DatabaseRow{}
	for _, note := range notes {
		for _, link := range noteLinks(note, pathByTitle) {
			rows = append(rows, linkRecordRow(link, domain.QuerySourceLinks))
		}
	}
	return rows
}

func ExtractBacklinkRows(notes []domain.Note) []domain.DatabaseRow {
	rows := ExtractLinkRows(notes)
	for i := range rows {
		rows[i].Source = string(domain.QuerySourceBacklinks)
	}
	return rows
}

func ExtractAssetRows(notes []domain.Note) []domain.DatabaseRow {
	byPath := map[string]map[string]bool{}
	for _, note := range notes {
		links := pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{SourceNoteID: note.ID, SourcePath: note.Path, Body: note.Body})
		for _, link := range links {
			if byPath[link.AssetPath] == nil {
				byPath[link.AssetPath] = map[string]bool{}
			}
			byPath[link.AssetPath][note.Path] = true
		}
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	rows := make([]domain.DatabaseRow, 0, len(paths))
	for _, path := range paths {
		filename := filepath.Base(path)
		stem := strings.TrimSuffix(filename, filepath.Ext(filename))
		values := map[string]domain.PropertyValue{}
		putRowValue(values, "path", domain.PropertyTypeString, path, path, "asset")
		putRowValue(values, "filename", domain.PropertyTypeString, filename, filename, "asset")
		putRowValue(values, "stem", domain.PropertyTypeString, stem, stem, "asset")
		putRowValue(values, "media_type", domain.PropertyTypeString, mediaType(path), mediaType(path), "asset")
		putRowValue(values, "linked_notes", domain.PropertyTypeNumber, strconv.Itoa(len(byPath[path])), len(byPath[path]), "asset")
		putRowValue(values, "status", domain.PropertyTypeSelect, "referenced", "referenced", "asset")
		rows = append(rows, domain.DatabaseRow{Source: string(domain.QuerySourceAssets), Values: values})
	}
	return rows
}

func linkRecordRow(link LinkRecord, source domain.QuerySource) domain.DatabaseRow {
	values := map[string]domain.PropertyValue{}
	putRowValue(values, "source_path", domain.PropertyTypeString, link.NotePath, link.NotePath, "link")
	putRowValue(values, "source_note_id", domain.PropertyTypeString, link.SourceNoteID, link.SourceNoteID, "link")
	putRowValue(values, "target", domain.PropertyTypeString, link.Target, link.Target, "link")
	putRowValue(values, "target_raw", domain.PropertyTypeString, link.TargetRaw, link.TargetRaw, "link")
	putRowValue(values, "target_path", domain.PropertyTypeString, link.TargetPath, link.TargetPath, "link")
	putRowValue(values, "status", domain.PropertyTypeSelect, link.Status, link.Status, "link")
	putRowValue(values, "kind", domain.PropertyTypeSelect, link.Kind, link.Kind, "link")
	putRowValue(values, "line", domain.PropertyTypeNumber, strconv.Itoa(link.Line), link.Line, "link")
	return domain.DatabaseRow{Source: string(source), Values: values}
}

func putRowValue(values map[string]domain.PropertyValue, name string, typ domain.PropertyType, raw string, value any, source string) {
	if raw == "" && value == nil {
		return
	}
	values[name] = domain.PropertyValue{Name: name, Type: typ, Raw: raw, Value: value, Source: source}
}
