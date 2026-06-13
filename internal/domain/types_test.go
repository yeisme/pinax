package domain

import (
	"encoding/json"
	"testing"
)

func TestLinkKindValidation(t *testing.T) {
	for _, kind := range []string{"wiki", "markdown"} {
		if !IsValidLinkKind(kind) {
			t.Fatalf("expected %q to be valid link kind", kind)
		}
	}
	for _, kind := range []string{"", "external", "Wiki", "MARKDOWN"} {
		if IsValidLinkKind(kind) {
			t.Fatalf("expected %q to be invalid link kind", kind)
		}
	}
}

func TestLinkStatusValidation(t *testing.T) {
	for _, status := range []string{"resolved", "broken", "ambiguous", "external", "ignored"} {
		if !IsValidLinkStatus(status) {
			t.Fatalf("expected %q to be valid link status", status)
		}
	}
	for _, status := range []string{"", "unknown", "Resolved"} {
		if IsValidLinkStatus(status) {
			t.Fatalf("expected %q to be invalid link status", status)
		}
	}
}

func TestNoteLinkJSONIncludesExtendedFields(t *testing.T) {
	link := NoteLink{
		SourcePath:    "notes/a.md",
		SourceTitle:   "A",
		Target:        "B",
		TargetPath:    "notes/b.md",
		TargetTitle:   "B",
		Kind:          "wiki",
		Broken:        false,
		SourceNoteID:  "note_a",
		TargetNoteID:  "note_b",
		TargetRaw:     "B",
		TargetHeading: "Section",
		Status:        "resolved",
		Line:          5,
		Evidence:      "wiki link resolved by title",
		Candidates:    []NoteLinkCandidate{{Path: "notes/b.md", Title: "B", NoteID: "note_b"}},
	}
	b, err := json.Marshal(link)
	if err != nil {
		t.Fatalf("marshal note link: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"source_path":"notes/a.md"`,
		`"source_note_id":"note_a"`,
		`"target_note_id":"note_b"`,
		`"target_heading":"Section"`,
		`"status":"resolved"`,
		`"line":5`,
		`"evidence":"wiki link resolved by title"`,
		`"candidates":[{"path":"notes/b.md","title":"B","note_id":"note_b"}]`,
	} {
		if !contains(s, want) {
			t.Fatalf("JSON missing %q:\n%s", want, s)
		}
	}
}

func TestNoteLinkBackwardCompat(t *testing.T) {
	// 旧代码只设置 Kind 和 Broken，不设置 Status。
	link := NoteLink{Kind: "wiki", Broken: true}
	if link.Status != "" {
		t.Fatalf("expected empty status for legacy note link")
	}
	b, _ := json.Marshal(link)
	if !contains(string(b), `"broken":true`) {
		t.Fatalf("expected broken=true in JSON: %s", string(b))
	}
}

func TestNoteGraphProjectionJSON(t *testing.T) {
	p := NoteGraphProjection{
		Engine:      "index",
		IndexStatus: "fresh",
		TotalNotes:  10,
		TotalLinks:  25,
		Resolved:    20,
		Broken:      3,
		Ambiguous:   1,
		Ignored:     1,
		Orphans:     2,
		Facts:       map[string]string{"vault": "/tmp/test"},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal graph projection: %v", err)
	}
	s := string(b)
	for _, want := range []string{`"engine":"index"`, `"total_notes":10`, `"resolved":20`, `"broken":3`, `"ambiguous":1`} {
		if !contains(s, want) {
			t.Fatalf("JSON missing %q:\n%s", want, s)
		}
	}
}

func TestStableErrorCodeConstants(t *testing.T) {
	codes := map[string]string{
		string(ErrorCodeVaultObjectRefAmbiguous):        "vault_object_ref_ambiguous",
		string(ErrorCodeVersionReadUnavailable):         "version_read_unavailable",
		string(ErrorCodeVersionChangedPathsUnavailable): "version_changed_paths_unavailable",
		string(ErrorCodeAssetNotFound):                  "asset_not_found",
		string(ErrorCodeAssetRefAmbiguous):              "asset_ref_ambiguous",
		string(ErrorCodeAssetPayloadForbidden):          "asset_payload_forbidden",
	}
	for got, want := range codes {
		if got != want {
			t.Fatalf("error code = %q, want %q", got, want)
		}
	}
}

func TestAssetDomainModelsJSON(t *testing.T) {
	asset := Asset{ID: "asset_abc", Path: "assets/diagram.png", Filename: "diagram.png", Stem: "diagram", Extension: "png", MediaType: "image/png", Size: 128, SHA256: "abc123", ManagedStatus: ManagedStatusManaged, CreatedAt: "2026-06-08T00:00:00Z", UpdatedAt: "2026-06-08T00:00:00Z"}
	manifest := AssetManifest{SchemaVersion: "pinax.assets.v1", Assets: []Asset{asset}}
	link := AssetLink{AssetID: "asset_abc", AssetPath: "assets/diagram.png", SourceNoteID: "note_a", SourcePath: "notes/a.md", RawReference: "![Diagram](../assets/diagram.png)", LinkStyle: "markdown", LinkKind: "embed", Line: 7, Status: "resolved"}
	verification := AssetVerification{Asset: asset, Status: "verified", SHA256: "abc123"}
	plan := AssetOperationPlan{PlanID: "asset-plan-1", AssetID: "asset_abc", Path: "assets/diagram.png", Operation: "remove", Risk: "high", RequiresSnapshot: true, Operations: []PlanOperation{{Kind: "asset_remove", Path: "assets/diagram.png", Reason: "unused", Status: "planned"}}}
	b, err := json.Marshal(map[string]any{"manifest": manifest, "link": link, "verification": verification, "plan": plan})
	if err != nil {
		t.Fatalf("marshal asset domain models: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"schema_version":"pinax.assets.v1"`,
		`"id":"asset_abc"`,
		`"path":"assets/diagram.png"`,
		`"filename":"diagram.png"`,
		`"media_type":"image/png"`,
		`"size":128`,
		`"sha256":"abc123"`,
		`"managed_status":"managed"`,
		`"source_note_id":"note_a"`,
		`"source_path":"notes/a.md"`,
		`"raw_reference":"![Diagram](../assets/diagram.png)"`,
		`"link_style":"markdown"`,
		`"link_kind":"embed"`,
		`"line":7`,
		`"status":"verified"`,
		`"plan_id":"asset-plan-1"`,
		`"operation":"remove"`,
		`"risk":"high"`,
		`"requires_snapshot":true`,
		`"kind":"asset_remove"`,
	} {
		if !contains(s, want) {
			t.Fatalf("JSON missing %q:\n%s", want, s)
		}
	}
}

func TestVersionDomainModelsJSON(t *testing.T) {
	status := VersionStatus{
		Backend: "local",
		Capabilities: VersionCapabilities{
			SnapshotSupported:     true,
			ChangedPathsSupported: true,
			ReadAtRevision:        true,
			DiffSupported:         true,
		},
		WorktreeState:   "clean",
		CurrentRevision: "local-1",
		LastSnapshotID:  "local-0",
		LastSnapshotAt:  "2026-06-08T00:00:00Z",
	}
	snapshot := VersionSnapshot{
		SnapshotID:  "local-1",
		Backend:     "local",
		Message:     "checkpoint",
		CreatedAt:   "2026-06-08T00:00:00Z",
		Files:       2,
		Bytes:       128,
		ContentHash: "abc123",
		Evidence:    []string{".pinax/version/snapshots/local-1.json"},
	}
	changed := ChangedPath{Path: "notes/a.md", ChangeKind: "modified", ObjectKind: VaultObjectKindNote, ContentHash: "abc123", SizeBytes: 128, Evidence: []string{"snapshot:local-1"}}
	diff := DiffSummary{BaseRevision: "local-0", TargetRevision: "local-1", FilesChanged: 1, Additions: 2, Deletions: 1, ChangedPaths: []ChangedPath{changed}}
	versionedFile := VersionedFile{Path: "notes/a.md", Revision: "local-1", Backend: "local", ContentHash: "abc123", SizeBytes: 128, Content: "# A\n", Evidence: []string{"snapshot:local-1"}}
	b, err := json.Marshal(map[string]any{"status": status, "snapshot": snapshot, "diff": diff, "file": versionedFile})
	if err != nil {
		t.Fatalf("marshal version domain models: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"backend":"local"`,
		`"snapshot_supported":true`,
		`"changed_paths_supported":true`,
		`"read_at_revision_supported":true`,
		`"diff_supported":true`,
		`"worktree_state":"clean"`,
		`"snapshot_id":"local-1"`,
		`"content_hash":"abc123"`,
		`"change_kind":"modified"`,
		`"object_kind":"note"`,
		`"size_bytes":128`,
		`"base_revision":"local-0"`,
		`"target_revision":"local-1"`,
		`"files_changed":1`,
		`"additions":2`,
		`"deletions":1`,
		`"content":"# A\n"`,
	} {
		if !contains(s, want) {
			t.Fatalf("JSON missing %q:\n%s", want, s)
		}
	}
}

func TestVaultObjectResolverDomainJSON(t *testing.T) {
	candidate := VaultObjectCandidate{
		ObjectKind:    VaultObjectKindNote,
		Path:          "notes/yeisme.md",
		Title:         "Yeisme",
		NoteID:        "note_yeisme",
		ManagedStatus: ManagedStatusRegistered,
		MatchFields:   []MatchField{MatchFieldStem, MatchFieldTitle},
		Score:         95,
		IndexStatus:   "fresh",
	}
	facts := ResolverFacts{
		Query:       "yeisme",
		Scope:       "registered_or_adoptable",
		Candidates:  1,
		MatchField:  MatchFieldStem,
		IndexStatus: "fresh",
	}
	b, err := json.Marshal(map[string]any{"candidate": candidate, "resolver": facts})
	if err != nil {
		t.Fatalf("marshal resolver domain models: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"object_kind":"note"`,
		`"path":"notes/yeisme.md"`,
		`"managed_status":"registered"`,
		`"match_fields":["stem","title"]`,
		`"score":95`,
		`"index_status":"fresh"`,
		`"query":"yeisme"`,
		`"scope":"registered_or_adoptable"`,
		`"candidates":1`,
		`"match_field":"stem"`,
	} {
		if !contains(s, want) {
			t.Fatalf("JSON missing %q:\n%s", want, s)
		}
	}
	if contains(s, "asset_id") || contains(s, "media_type") {
		t.Fatalf("empty optional candidate fields leaked into JSON:\n%s", s)
	}
}

func TestDomainModelJSONContractsTableDriven(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		want    []string
		notWant []string
	}{
		{
			name:    "resolver candidate omits empty optional fields",
			value:   VaultObjectCandidate{ObjectKind: VaultObjectKindFile, Path: "notes/draft.md", ManagedStatus: ManagedStatusAdoptable, MatchFields: []MatchField{MatchFieldFilename}, Score: 70},
			want:    []string{`"object_kind":"file"`, `"path":"notes/draft.md"`, `"managed_status":"adoptable"`, `"match_fields":["filename"]`, `"score":70`},
			notWant: []string{`"title"`, `"note_id"`, `"asset_id"`, `"media_type"`, `"index_status"`},
		},
		{
			name:    "version status defaults keep capability keys",
			value:   VersionStatus{Backend: "none", Capabilities: VersionCapabilities{}, WorktreeState: "unavailable"},
			want:    []string{`"backend":"none"`, `"capabilities":{"snapshot_supported":false`, `"changed_paths_supported":false`, `"read_at_revision_supported":false`, `"diff_supported":false`, `"worktree_state":"unavailable"`},
			notWant: []string{`"current_revision"`, `"last_snapshot_id"`, `"last_snapshot_at"`},
		},
		{
			name:    "asset manifest preserves empty assets array",
			value:   AssetManifest{SchemaVersion: "pinax.assets.v1", Assets: []Asset{}},
			want:    []string{`"schema_version":"pinax.assets.v1"`, `"assets":[]`},
			notWant: []string{`"asset_id"`, `"sha256"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal %s: %v", tc.name, err)
			}
			s := string(b)
			for _, want := range tc.want {
				if !contains(s, want) {
					t.Fatalf("JSON missing %q:\n%s", want, s)
				}
			}
			for _, notWant := range tc.notWant {
				if contains(s, notWant) {
					t.Fatalf("JSON unexpectedly contains %q:\n%s", notWant, s)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
