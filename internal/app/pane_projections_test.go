package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func scanPaneEnvelopeForLeaks(t *testing.T, envelope PaneSnapshotEnvelope) {
	t.Helper()
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, banned := range []string{"/home/", "/etc/", "/tmp/", "token", "authorization", "cookie", "secret", "password", "api_key", "bearer", "://", "body"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("pane envelope leaked %q: %s", banned, raw)
		}
	}
}

func TestAssemblePaneBacklinksBoundedAndRedlined(t *testing.T) {
	t.Parallel()
	links := make([]domain.NoteLink, 0, 60)
	for i := 0; i < 55; i++ {
		links = append(links, domain.NoteLink{SourcePath: "notes/source.md", SourceNoteID: "note_src_" + strings.Repeat("a", 0) + itoa(i), SourceTitle: "Source " + itoa(i), Kind: "wikilink"})
	}
	for _, unsafe := range []string{"api_key_live", "/etc/passwd", "has space", "note/token"} {
		links = append(links, domain.NoteLink{SourcePath: "notes/bad.md", SourceNoteID: unsafe, SourceTitle: "bad", Kind: "wikilink"})
	}
	projection := domain.NewProjection("note.backlinks", "Note backlinks listed.")
	projection.Data = map[string]any{"backlinks": links}
	envelope, err := AssemblePaneBacklinks(projection, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"})
	if err != nil {
		t.Fatal(err)
	}
	entities := envelope.Payload["entities"].([]PaneEntity)
	if len(entities) != paneBacklinksLimit {
		t.Fatalf("entities = %d, want %d", len(entities), paneBacklinksLimit)
	}
	if envelope.Payload["truncated"] != true || envelope.Payload["dropped_unsafe"].(int) != 4 {
		t.Fatalf("truncated=%v dropped=%v", envelope.Payload["truncated"], envelope.Payload["dropped_unsafe"])
	}
	if envelope.Status != "ready" {
		t.Fatalf("status = %s", envelope.Status)
	}
	scanPaneEnvelopeForLeaks(t, envelope)
}

func TestAssemblePaneBacklinksEmptyAndFailedStates(t *testing.T) {
	t.Parallel()
	empty := domain.NewProjection("note.backlinks", "Note backlinks listed.")
	empty.Data = map[string]any{"backlinks": []domain.NoteLink{}}
	envelope, err := AssemblePaneBacklinks(empty, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"})
	if err != nil || envelope.Status != "ready" {
		t.Fatalf("empty backlinks must be ready: %+v err=%v", envelope, err)
	}
	if entities := envelope.Payload["entities"].([]PaneEntity); len(entities) != 0 {
		t.Fatalf("entities = %d", len(entities))
	}

	failed := domain.NewProjection("note.backlinks", "Note backlinks listed.")
	failed.Status = "failed"
	envelope, err = AssemblePaneBacklinks(failed, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"})
	if err != nil || envelope.Status != "offline" {
		t.Fatalf("failed projection must map offline: %+v err=%v", envelope, err)
	}

	roundTripped := domain.NewProjection("note.backlinks", "Note backlinks listed.")
	roundTripped.Data = map[string]any{"backlinks": []any{map[string]any{"source_note_id": "note_x"}}}
	if _, err := AssemblePaneBacklinks(roundTripped, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"}); err == nil {
		t.Fatal("round-tripped backlinks shape must fail closed")
	}
}

func TestAssemblePaneGraphSummaryTotalsAndTopK(t *testing.T) {
	t.Parallel()
	notes := []domain.Note{
		{ID: "note_a", Path: "notes/a.md"},
		{ID: "note_b", Path: "notes/b.md"},
		{ID: "note_c", Path: "notes/c.md"},
		{ID: "note_d", Path: "notes/d.md"},
	}
	links := []domain.NoteLink{
		{SourcePath: "notes/a.md", TargetNoteID: "note_b"},
		{SourcePath: "notes/b.md", TargetNoteID: "note_c"},
	}
	envelope, err := AssemblePaneGraphSummary(notes, links, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"})
	if err != nil {
		t.Fatal(err)
	}
	totals := envelope.Payload["totals"].(PaneGraphTotals)
	if totals.Nodes != 4 || totals.Edges != 2 || totals.Components != 2 || totals.Truncated {
		t.Fatalf("totals = %+v", totals)
	}
	entities := envelope.Payload["entities"].([]PaneEntity)
	if len(entities) != 3 {
		t.Fatalf("ranked nodes = %d, want 3 (a,b,c linked; d isolated degree 0)", len(entities))
	}
	if entities[0].Ref != "note_b" || entities[0].Value["degree"].(int) != 2 {
		t.Fatalf("top node = %+v, want note_b degree 2", entities[0])
	}
	scanPaneEnvelopeForLeaks(t, envelope)
}

func TestAssemblePaneGraphSummaryTopKTruncates(t *testing.T) {
	t.Parallel()
	notes := make([]domain.Note, 0, 25)
	links := make([]domain.NoteLink, 0, 24)
	for i := 0; i < 25; i++ {
		ref := "note_" + itoa(i)
		if len(ref) > 4 {
			ref = ref[:4] + itoa(i)
		}
		notes = append(notes, domain.Note{ID: ref, Path: "notes/n.md"})
		if i > 0 {
			links = append(links, domain.NoteLink{SourcePath: "notes/n.md", TargetNoteID: ref})
		}
	}
	envelope, err := AssemblePaneGraphSummary(notes, links, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"})
	if err != nil {
		t.Fatal(err)
	}
	entities := envelope.Payload["entities"].([]PaneEntity)
	if len(entities) != paneGraphTopK {
		t.Fatalf("entities = %d, want %d", len(entities), paneGraphTopK)
	}
	if envelope.Payload["totals"].(PaneGraphTotals).Truncated != true {
		t.Fatal("totals must report truncated")
	}
}

func TestAssemblePaneGraphSummaryDropsUnsafe(t *testing.T) {
	t.Parallel()
	notes := []domain.Note{
		{ID: "note_a", Path: "notes/a.md"},
		{ID: "/etc/passwd", Path: "notes/bad.md"},
	}
	envelope, err := AssemblePaneGraphSummary(notes, nil, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Payload["dropped_unsafe"].(int) != 1 {
		t.Fatalf("dropped = %v", envelope.Payload["dropped_unsafe"])
	}
	if envelope.Payload["totals"].(PaneGraphTotals).Nodes != 1 {
		t.Fatalf("unsafe note must not count as node")
	}
	scanPaneEnvelopeForLeaks(t, envelope)
}

func TestAssemblePaneHistoryBoundedAndRedlined(t *testing.T) {
	t.Parallel()
	events := make([]domain.RecordEvent, 0, 120)
	for i := 0; i < 120; i++ {
		events = append(events, domain.RecordEvent{
			EventID:         "evt_" + itoa(i),
			Kind:            domain.RecordEventNoteMetadataUpdated,
			ObjectID:        "note_hist_" + itoa(i),
			CreatedAt:       "2026-08-23T10:00:00Z",
			ContentRevision: domain.ContentRevision{Hash: "rev_" + itoa(i)},
			Path:            "notes/hist.md",
		})
	}
	events = append(events, domain.RecordEvent{Kind: domain.RecordEventNoteMetadataUpdated, ObjectID: "bearer_bad", CreatedAt: "2026-08-23T10:00:00Z"})
	projection := domain.NewProjection("records.list", "Record events listed.")
	projection.Data = map[string]any{"events": events}
	envelope, err := AssemblePaneHistory(projection, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"})
	if err != nil {
		t.Fatal(err)
	}
	timeline := envelope.Payload["timeline"].([]any)
	if len(timeline) != paneHistoryLimit {
		t.Fatalf("timeline = %d, want %d", len(timeline), paneHistoryLimit)
	}
	if envelope.Payload["truncated"] != true || envelope.Payload["dropped_unsafe"].(int) != 1 {
		t.Fatalf("truncated=%v dropped=%v", envelope.Payload["truncated"], envelope.Payload["dropped_unsafe"])
	}
	first := timeline[0].(PaneTimelineEvent)
	if first.Op != string(domain.RecordEventNoteMetadataUpdated) || first.Ref != "note_hist_0" || first.Revision != "rev_0" {
		t.Fatalf("first event = %+v", first)
	}
	scanPaneEnvelopeForLeaks(t, envelope)
}

func TestAssemblePaneHistoryFailsClosedOnShape(t *testing.T) {
	t.Parallel()
	projection := domain.NewProjection("records.list", "Record events listed.")
	projection.Data = map[string]any{"events": []any{map[string]any{"object_id": "note_x"}}}
	if _, err := AssemblePaneHistory(projection, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"}); err == nil {
		t.Fatal("round-tripped events shape must fail closed")
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}
