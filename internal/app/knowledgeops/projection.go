package knowledgeops

import (
	"path/filepath"
	"sort"
	"strings"
)

const (
	SchemaVersion = "pinax.knowledge_projection.v1"

	EmptyReasonAllowlistEmpty = "allowlist_empty"
	EmptyReasonNoMatch        = "no_dual_condition_match"

	EntryKindNote      = "note"
	EntryKindTombstone = "tombstone"

	AllowMarkerKey   = "knowledge_export"
	AllowMarkerValue = "allow"
)

// Candidate is a path-allowlisted note the facade already opened. Non-allowlisted
// note bodies never become candidates.
type Candidate struct {
	Path        string
	NoteID      string
	Title       string
	Frontmatter map[string]string
	Digest      string
	ChangedAt   string
}

type Refs struct {
	NoteID string     `json:"note_id,omitempty"`
	Path   string     `json:"path"`
	Source string     `json:"source"`
	Chunks []ChunkRef `json:"chunks,omitempty"`
}

type ChunkRef struct {
	ChunkID string `json:"chunk_id"`
	Digest  string `json:"digest,omitempty"`
}

type Permission struct {
	Export     string `json:"export"`
	Visibility string `json:"visibility"`
}

type Citation struct {
	Title  string `json:"title,omitempty"`
	Path   string `json:"path"`
	NoteID string `json:"note_id,omitempty"`
}

type Freshness struct {
	ContentDigest string `json:"content_digest"`
	ChangedAt     string `json:"changed_at,omitempty"`
}

type Revocation struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type Entry struct {
	Kind       string      `json:"kind"`
	Refs       Refs        `json:"refs"`
	Digest     string      `json:"digest"`
	Permission Permission  `json:"permission"`
	Citation   Citation    `json:"citation"`
	Freshness  Freshness   `json:"freshness"`
	Revocation *Revocation `json:"revocation,omitempty"`
}

type Audit struct {
	AllowlistPaths       int `json:"allowlist_paths"`
	PathAllowlisted      int `json:"path_allowlisted"`
	Included             int `json:"included"`
	OmittedMissingMarker int `json:"omitted_missing_marker"`
	Tombstones           int `json:"tombstones"`
	Unchanged            int `json:"unchanged"`
	ReadNoteBodies       int `json:"read_note_bodies"`
}

type Package struct {
	SchemaVersion string  `json:"schema_version"`
	ExportedAt    string  `json:"exported_at"`
	EmptyReason   string  `json:"empty_reason,omitempty"`
	Entries       []Entry `json:"entries"`
	Audit         Audit   `json:"audit"`
}

func NormalizeAllowlist(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		path := normalizeRelPath(raw)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func ParseAllowlistFile(content string) []string {
	lines := strings.Split(content, "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		paths = append(paths, line)
	}
	return NormalizeAllowlist(paths)
}

func PathAllowed(rel string, allowlist []string) bool {
	rel = normalizeRelPath(rel)
	if rel == "" || len(allowlist) == 0 {
		return false
	}
	for _, allowed := range allowlist {
		if pathMatches(rel, allowed) {
			return true
		}
	}
	return false
}

func HasAllowMarker(frontmatter map[string]string) bool {
	if len(frontmatter) == 0 {
		return false
	}
	value := strings.TrimSpace(strings.ToLower(frontmatter[AllowMarkerKey]))
	switch value {
	case AllowMarkerValue, "true", "yes", "1":
		return true
	default:
		return false
	}
}

func SelectCurrent(candidates []Candidate) (entries []Entry, omittedMissingMarker int) {
	entries = make([]Entry, 0, len(candidates))
	for _, candidate := range candidates {
		if !HasAllowMarker(candidate.Frontmatter) {
			omittedMissingMarker++
			continue
		}
		entries = append(entries, BuildNoteEntry(candidate))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Refs.Path < entries[j].Refs.Path })
	return entries, omittedMissingMarker
}

func BuildNoteEntry(candidate Candidate) Entry {
	path := normalizeRelPath(candidate.Path)
	chunkID := path
	if candidate.NoteID != "" {
		chunkID = candidate.NoteID
	}
	return Entry{
		Kind: EntryKindNote,
		Refs: Refs{
			NoteID: candidate.NoteID,
			Path:   path,
			Source: "pinax.note",
			Chunks: []ChunkRef{{ChunkID: chunkID, Digest: candidate.Digest}},
		},
		Digest: candidate.Digest,
		Permission: Permission{
			Export:     "allowlisted",
			Visibility: "allowlisted",
		},
		Citation: Citation{
			Title:  candidate.Title,
			Path:   path,
			NoteID: candidate.NoteID,
		},
		Freshness: Freshness{
			ContentDigest: candidate.Digest,
			ChangedAt:     candidate.ChangedAt,
		},
	}
}

func BuildTombstone(prior Entry) Entry {
	tombstone := prior
	tombstone.Kind = EntryKindTombstone
	tombstone.Revocation = &Revocation{Status: "tombstone", Reason: "deleted"}
	tombstone.Permission = Permission{Export: "revoked", Visibility: "allowlisted"}
	return tombstone
}

func DigestDiff(current []Entry, prior *Package) (entries []Entry, unchanged, tombstones int) {
	priorNotes := priorNoteIndex(prior)
	currentByPath := make(map[string]Entry, len(current))
	entries = make([]Entry, 0, len(current)+len(priorNotes))
	for _, entry := range current {
		currentByPath[entry.Refs.Path] = entry
		previous, ok := priorNotes[entry.Refs.Path]
		if ok && previous.Digest == entry.Digest && previous.Kind == EntryKindNote {
			unchanged++
			continue
		}
		entries = append(entries, entry)
	}
	for path, previous := range priorNotes {
		if _, stillPresent := currentByPath[path]; stillPresent {
			continue
		}
		entries = append(entries, BuildTombstone(previous))
		tombstones++
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Refs.Path < entries[j].Refs.Path })
	return entries, unchanged, tombstones
}

func AssemblePackage(exportedAt string, allowlist []string, pathAllowlisted, readBodies int, current []Entry, omittedMissingMarker int, prior *Package) Package {
	audit := Audit{
		AllowlistPaths:       len(allowlist),
		PathAllowlisted:      pathAllowlisted,
		OmittedMissingMarker: omittedMissingMarker,
		ReadNoteBodies:       readBodies,
	}
	entries := current
	if prior != nil {
		var unchanged, tombstones int
		entries, unchanged, tombstones = DigestDiff(current, prior)
		audit.Unchanged = unchanged
		audit.Tombstones = tombstones
	}
	included := 0
	for _, entry := range entries {
		if entry.Kind == EntryKindNote {
			included++
		}
	}
	audit.Included = included
	pkg := Package{
		SchemaVersion: SchemaVersion,
		ExportedAt:    exportedAt,
		Entries:       entries,
		Audit:         audit,
	}
	if len(allowlist) == 0 {
		pkg.EmptyReason = EmptyReasonAllowlistEmpty
		pkg.Entries = []Entry{}
		return pkg
	}
	if len(entries) == 0 && audit.Unchanged == 0 {
		pkg.EmptyReason = EmptyReasonNoMatch
	}
	if pkg.Entries == nil {
		pkg.Entries = []Entry{}
	}
	return pkg
}

func priorNoteIndex(prior *Package) map[string]Entry {
	out := make(map[string]Entry)
	if prior == nil {
		return out
	}
	for _, entry := range prior.Entries {
		if entry.Kind != EntryKindNote {
			continue
		}
		path := normalizeRelPath(entry.Refs.Path)
		if path == "" {
			continue
		}
		out[path] = entry
	}
	return out
}

func pathMatches(rel, allowed string) bool {
	if rel == allowed {
		return true
	}
	prefix := strings.TrimSuffix(allowed, "/")
	if prefix == "" {
		return false
	}
	return strings.HasPrefix(rel, prefix+"/")
}

func normalizeRelPath(raw string) string {
	value := strings.TrimSpace(filepath.ToSlash(raw))
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	if value == "." || value == ".." || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		return ""
	}
	return strings.TrimSuffix(value, "/")
}
