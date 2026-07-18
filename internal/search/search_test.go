package search

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestNotesNativeSearchReturnsStablePathOrder(t *testing.T) {
	notes := []domain.Note{
		{Path: "zeta.md", Title: "Zeta", Body: "needle"},
		{Path: "alpha.md", Title: "Alpha", Body: "needle"},
		{Path: "middle.md", Title: "Middle", Body: "needle"},
	}

	result := Notes(context.Background(), "/tmp/vault", "needle", notes)
	if result.Engine != "native" {
		t.Fatalf("engine = %q", result.Engine)
	}
	if len(result.Notes) != 3 {
		t.Fatalf("notes = %#v", result.Notes)
	}
	if result.Notes[0].Path != "alpha.md" || result.Notes[1].Path != "middle.md" || result.Notes[2].Path != "zeta.md" {
		t.Fatalf("notes not sorted by path: %#v", result.Notes)
	}
}

func TestNotesNativeSearchHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Notes(ctx, "/tmp/vault", "needle", []domain.Note{{Path: "alpha.md", Body: "needle"}})
	if len(result.Notes) != 0 {
		t.Fatalf("canceled search returned notes: %#v", result.Notes)
	}
}
