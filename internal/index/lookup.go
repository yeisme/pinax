package index

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"gorm.io/gorm"
)

type LookupRequest struct {
	Query string
	Scope string
	Kind  string
}

type LookupResult struct {
	Status     Status                        `json:"status"`
	Candidates []domain.VaultObjectCandidate `json:"candidates"`
}

func Lookup(root string, req LookupRequest) (LookupResult, error) {
	query := strings.TrimSpace(req.Query)
	status, db, ok, err := lookupReady(root)
	if err != nil || !ok || query == "" {
		return LookupResult{Status: status}, err
	}
	scope := lookupDefault(req.Scope, "registered")
	kind := lookupDefault(req.Kind, "all")
	q := strings.ToLower(query)
	candidates := []domain.VaultObjectCandidate{}

	if lookupScopeAllows(scope, "registered") && lookupKindAllows(kind, "note") {
		notes, texts, aliases, err := lookupNoteProjection(db)
		if err != nil {
			return LookupResult{Status: status}, err
		}
		for _, note := range notes {
			fields, score := lookupNoteRecordMatch(q, note, texts[note.Path], aliases[note.Path])
			if score == 0 {
				continue
			}
			candidates = append(candidates, domain.VaultObjectCandidate{ObjectKind: domain.VaultObjectKindNote, Path: note.Path, Title: note.Title, NoteID: note.NoteID, ManagedStatus: domain.ManagedStatusRegistered, MatchFields: fields, Score: score, IndexStatus: status.Status})
		}
	}

	if lookupScopeAllows(scope, "adoptable") && lookupKindAllows(kind, "file") {
		files := []VaultFileRecord{}
		if err := db.Where("object_kind = ? AND managed_status = ?", string(domain.VaultObjectKindNote), string(domain.ManagedStatusUnmanaged)).Order("path asc").Find(&files).Error; err != nil {
			return LookupResult{Status: status}, err
		}
		for _, file := range files {
			fields, score := lookupVaultFileMatch(q, file)
			if score == 0 {
				continue
			}
			candidates = append(candidates, domain.VaultObjectCandidate{ObjectKind: domain.VaultObjectKindFile, Path: file.Path, ManagedStatus: domain.ManagedStatusAdoptable, MatchFields: fields, Score: score, MediaType: file.MediaType, IndexStatus: status.Status})
		}
	}

	if lookupScopeAllows(scope, "assets") && lookupKindAllows(kind, "asset") {
		assets := []AssetRecord{}
		if err := db.Order("path asc").Find(&assets).Error; err != nil {
			return LookupResult{Status: status}, err
		}
		for _, asset := range assets {
			fields, score := lookupAssetRecordMatch(q, asset)
			if score == 0 {
				continue
			}
			candidates = append(candidates, domain.VaultObjectCandidate{ObjectKind: domain.VaultObjectKindAsset, Path: asset.Path, AssetID: asset.AssetID, ManagedStatus: domain.ManagedStatus(asset.ManagedStatus), MatchFields: fields, Score: score, MediaType: asset.MediaType, IndexStatus: status.Status})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Path < candidates[j].Path
	})
	return LookupResult{Status: status, Candidates: candidates}, nil
}

func lookupReady(root string) (Status, *gorm.DB, bool, error) {
	if _, err := os.Stat(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		if os.IsNotExist(err) {
			return Status{Status: "missing", Path: indexRelPath()}, nil, false, nil
		}
		return Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, nil, false, err
	}
	db, err := open(root)
	if err != nil {
		return Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, nil, false, err
	}
	if issues := indexStorageSchemaIssues(db); len(issues) > 0 {
		if err := indexSchemaReadError(db); err != nil {
			return Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, db, false, err
		}
		return Status{Status: "stale", Path: indexRelPath(), Evidence: issueEvidence(issues)}, db, false, nil
	}
	schema := metaValue(db, "schema_version")
	if schema != SchemaVersion {
		if schema == "" {
			schema = "missing"
		}
		return Status{Status: "stale", Path: indexRelPath(), SchemaVersion: schema, Evidence: []string{"schema_version=" + schema}}, db, false, nil
	}
	propertySchema := metaValue(db, "property_schema_version")
	if propertySchema != PropertySchemaVersion {
		if propertySchema == "" {
			propertySchema = "missing"
		}
		return Status{Status: "stale", Path: indexRelPath(), SchemaVersion: schema, Evidence: []string{"property_schema_version=" + propertySchema}}, db, false, nil
	}
	return Status{Status: "fresh", Path: indexRelPath(), SchemaVersion: schema}, db, true, nil
}

func lookupNoteProjection(db *gorm.DB) ([]NoteRecord, map[string]NoteTextRecord, map[string][]string, error) {
	notes := []NoteRecord{}
	if err := db.Where("is_system = ?", false).Order("path asc").Find(&notes).Error; err != nil {
		return nil, nil, nil, err
	}
	texts := []NoteTextRecord{}
	if err := db.Find(&texts).Error; err != nil {
		return nil, nil, nil, err
	}
	textByPath := map[string]NoteTextRecord{}
	for _, text := range texts {
		textByPath[text.NotePath] = text
	}
	values := []PropertyValueRecord{}
	if err := db.Where("name IN ?", []string{"alias", "aliases"}).Find(&values).Error; err != nil {
		return nil, nil, nil, err
	}
	aliases := map[string][]string{}
	for _, value := range values {
		for _, alias := range strings.Split(value.Value, ",") {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				aliases[value.NotePath] = append(aliases[value.NotePath], alias)
			}
		}
	}
	return notes, textByPath, aliases, nil
}

func lookupNoteRecordMatch(q string, note NoteRecord, text NoteTextRecord, aliases []string) ([]domain.MatchField, int) {
	checks := []lookupFieldCheck{
		{field: domain.MatchFieldNoteID, value: note.NoteID, exact: 100, contains: 60},
		{field: domain.MatchFieldPath, value: note.Path, exact: 95, contains: 55},
		{field: domain.MatchFieldFilename, value: note.Filename, exact: 90, contains: 50},
		{field: domain.MatchFieldStem, value: note.Stem, exact: 90, contains: 50},
		{field: domain.MatchFieldTitle, value: note.Title, exact: 85, contains: 45},
	}
	for _, alias := range aliases {
		checks = append(checks, lookupFieldCheck{field: domain.MatchFieldAlias, value: alias, exact: 80, contains: 40})
	}
	checks = append(checks, lookupFieldCheck{field: domain.MatchFieldContent, value: text.BodyText, exact: 30, contains: 30})
	return lookupMatchFields(q, checks)
}

func lookupAssetRecordMatch(q string, asset AssetRecord) ([]domain.MatchField, int) {
	return lookupMatchFields(q, []lookupFieldCheck{
		{field: domain.MatchFieldAssetID, value: asset.AssetID, exact: 100, contains: 60},
		{field: domain.MatchFieldPath, value: asset.Path, exact: 95, contains: 55},
		{field: domain.MatchFieldFilename, value: asset.Filename, exact: 90, contains: 50},
		{field: domain.MatchFieldStem, value: asset.Stem, exact: 90, contains: 50},
		{field: domain.MatchFieldSHA256, value: asset.SHA256, exact: 70, contains: 35},
	})
}

func lookupVaultFileMatch(q string, file VaultFileRecord) ([]domain.MatchField, int) {
	return lookupMatchFields(q, []lookupFieldCheck{
		{field: domain.MatchFieldPath, value: file.Path, exact: 95, contains: 55},
		{field: domain.MatchFieldFilename, value: file.Filename, exact: 90, contains: 50},
		{field: domain.MatchFieldStem, value: file.Stem, exact: 90, contains: 50},
	})
}

type lookupFieldCheck struct {
	field    domain.MatchField
	value    string
	exact    int
	contains int
}

func lookupMatchFields(q string, checks []lookupFieldCheck) ([]domain.MatchField, int) {
	type fieldScore struct {
		field domain.MatchField
		score int
		order int
	}
	matched := map[domain.MatchField]fieldScore{}
	best := 0
	for order, check := range checks {
		value := strings.ToLower(strings.TrimSpace(check.value))
		if value == "" {
			continue
		}
		score := 0
		if value == q {
			score = check.exact
		} else if strings.Contains(value, q) {
			score = check.contains
		}
		if score == 0 {
			continue
		}
		current, ok := matched[check.field]
		if !ok || score > current.score {
			matched[check.field] = fieldScore{field: check.field, score: score, order: order}
		}
		if score > best {
			best = score
		}
	}
	fields := make([]fieldScore, 0, len(matched))
	for _, field := range matched {
		fields = append(fields, field)
	}
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].score != fields[j].score {
			return fields[i].score > fields[j].score
		}
		return fields[i].order < fields[j].order
	})
	out := make([]domain.MatchField, 0, len(fields))
	for _, field := range fields {
		out = append(out, field.field)
	}
	return out, best
}

func lookupDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func lookupScopeAllows(scope, target string) bool {
	switch scope {
	case "all":
		return true
	case "registered_or_adoptable":
		return target == "registered" || target == "adoptable"
	default:
		return scope == target
	}
}

func lookupKindAllows(kind, target string) bool {
	return kind == "" || kind == "all" || kind == target
}
