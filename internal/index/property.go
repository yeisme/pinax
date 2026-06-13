package index

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

func ExtractPropertyRows(notes []domain.Note) []domain.DatabaseRow {
	rows := make([]domain.DatabaseRow, 0, len(notes))
	for _, note := range notes {
		rows = append(rows, domain.DatabaseRow{Source: string(domain.QuerySourceNotes), Note: note, Values: ExtractProperties(note)})
	}
	return rows
}

func ExtractProperties(note domain.Note) map[string]domain.PropertyValue {
	values := map[string]domain.PropertyValue{}
	put := func(name string, typ domain.PropertyType, raw string, value any, source string) {
		if raw == "" && value == nil {
			return
		}
		values[name] = domain.PropertyValue{Name: name, Type: typ, Raw: raw, Value: value, Source: source}
	}
	put("title", domain.PropertyTypeString, note.Title, note.Title, "system")
	put("path", domain.PropertyTypeString, note.Path, note.Path, "system")
	put("note_id", domain.PropertyTypeString, note.ID, note.ID, "system")
	put("status", domain.PropertyTypeSelect, note.Status, note.Status, "frontmatter")
	put("kind", domain.PropertyTypeSelect, note.Kind, note.Kind, "frontmatter")
	put("project", domain.PropertyTypeSelect, note.Project, note.Project, "frontmatter")
	put("folder", domain.PropertyTypeString, note.Folder, note.Folder, "frontmatter")
	put("created_at", domain.PropertyTypeDate, note.CreatedAt, note.CreatedAt, "frontmatter")
	put("updated_at", domain.PropertyTypeDate, note.UpdatedAt, note.UpdatedAt, "frontmatter")
	if len(note.Tags) > 0 {
		put("tags", domain.PropertyTypeList, strings.Join(note.Tags, ","), note.Tags, "system")
	}
	for key, raw := range note.Frontmatter {
		if _, exists := values[key]; exists || strings.TrimSpace(raw) == "" {
			continue
		}
		typ, value := inferPropertyValue(raw)
		put(key, typ, raw, value, "frontmatter")
	}
	for key, raw := range inlineProperties(note.Body) {
		typ, value := inferPropertyValue(raw)
		put(key, typ, raw, value, "inline")
	}
	return values
}

func inlineProperties(body string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "::")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsAny(key, " \t") {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func inferPropertyValue(raw string) (domain.PropertyType, any) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "[[") && strings.Contains(trimmed, "]]") {
		return domain.PropertyTypeLink, strings.TrimSuffix(strings.TrimPrefix(trimmed, "[["), "]]")
	}
	if b, err := strconv.ParseBool(strings.ToLower(trimmed)); err == nil {
		return domain.PropertyTypeBoolean, b
	}
	if n, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return domain.PropertyTypeNumber, n
	}
	if _, err := time.Parse("2006-01-02", trimmed); err == nil {
		return domain.PropertyTypeDate, trimmed
	}
	if strings.Contains(trimmed, ",") {
		parts := strings.Split(trimmed, ",")
		items := make([]string, 0, len(parts))
		for _, part := range parts {
			if item := strings.TrimSpace(part); item != "" {
				items = append(items, item)
			}
		}
		return domain.PropertyTypeList, items
	}
	return domain.PropertyTypeString, trimmed
}

func InferPropertyDefinitions(rows []domain.DatabaseRow) []domain.PropertyDefinition {
	stats := map[string]map[domain.PropertyType]int{}
	samples := map[string]map[string]bool{}
	source := map[string]string{}
	for _, row := range rows {
		for name, value := range row.Values {
			if stats[name] == nil {
				stats[name] = map[domain.PropertyType]int{}
			}
			stats[name][value.Type]++
			if samples[name] == nil {
				samples[name] = map[string]bool{}
			}
			if len(samples[name]) < 3 && value.String() != "" {
				samples[name][value.String()] = true
			}
			if source[name] == "" {
				source[name] = value.Source
			}
		}
	}
	defs := make([]domain.PropertyDefinition, 0, len(stats))
	for name, counts := range stats {
		typ := domain.PropertyTypeMixed
		if len(counts) == 1 {
			for only := range counts {
				typ = only
			}
		}
		sampleValues := make([]string, 0, len(samples[name]))
		for sample := range samples[name] {
			sampleValues = append(sampleValues, sample)
		}
		sort.Strings(sampleValues)
		defs = append(defs, domain.PropertyDefinition{Name: name, Type: typ, Source: source[name], Count: propertyCount(counts), Samples: sampleValues})
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

func propertyCount(counts map[domain.PropertyType]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}
