package domain

import (
	"encoding/json"
	"testing"
)

func TestProjectBoardDomainModelsJSON(t *testing.T) {
	board := ProjectBoard{
		SchemaVersion:    ProjectBoardSchemaVersion,
		ProjectSlug:      "research",
		Title:            "研究看板",
		GeneratedAt:      "2026-06-08T00:00:00Z",
		SourceSnapshotID: "board-snap-1",
		Columns:          []BoardColumn{{ID: "next", Name: "下一步", Order: 20, WIPLimit: 5}},
		Items: []BoardItem{{
			ItemID:       "item_abc123",
			Title:        "实现看板 projection",
			Column:       "next",
			SourceKind:   BoardItemSourceNote,
			NoteID:       "note_abc123",
			Path:         "project-board.md",
			Project:      "research",
			Tags:         []string{"pinax", "planning"},
			Status:       "active",
			Priority:     "high",
			Due:          "2026-06-10",
			EvidenceRefs: []string{"project-board.md"},
			Writable:     true,
		}},
		Warnings: []ProjectBoardWarning{{Code: "unknown_board_column", Message: "未知看板列", Path: "legacy.md"}},
		Facts:    ProjectBoardFacts{TotalItems: 1, Next: 1, IndexStatus: "fresh", Engine: "index", WritableItems: 1},
	}
	display := NoteDisplay{
		NoteID:            "note_abc123",
		Title:             "实现看板 projection",
		Path:              "project-board.md",
		Display:           NoteDisplayCard,
		Exposure:          NoteExposureAgent,
		Project:           "research",
		BoardColumn:       "next",
		Kind:              "task",
		Status:            "active",
		Tags:              []string{"pinax"},
		UpdatedAt:         "2026-06-08T00:00:00Z",
		Excerpt:           "先做只读 projection",
		LinksCount:        1,
		BacklinksCount:    2,
		AttachmentsCount:  0,
		RelatedCount:      3,
		RedactionWarnings: []string{"body_omitted"},
	}

	b, err := json.Marshal(map[string]any{"board": board, "note": display})
	if err != nil {
		t.Fatalf("marshal project board models: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"schema_version":"pinax.project_board.v1"`,
		`"project_slug":"research"`,
		`"source_snapshot_id":"board-snap-1"`,
		`"wip_limit":5`,
		`"source_kind":"note"`,
		`"evidence_refs":["project-board.md"]`,
		`"writable":true`,
		`"warnings":[{"code":"unknown_board_column"`,
		`"index_status":"fresh"`,
		`"display":"card"`,
		`"exposure":"agent"`,
		`"board_column":"next"`,
		`"related_count":3`,
		`"redaction_warnings":["body_omitted"]`,
	} {
		if !contains(s, want) {
			t.Fatalf("JSON missing %q:\n%s", want, s)
		}
	}
}
