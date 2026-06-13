package output

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yeisme/pinax/internal/domain"
)

func TestJournalPagerNavigation(t *testing.T) {
	loaded := []int{}
	loader := func(direction int, current domain.Projection) (domain.Projection, error) {
		loaded = append(loaded, direction)
		date := "2026-06-06"
		if direction > 0 {
			date = "2026-06-07"
		}
		if direction < 0 {
			date = "2026-06-05"
		}
		return journalProjection(date), nil
	}

	model := NewJournalPagerModel(journalProjection("2026-06-06"), loader, 80, 20)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(JournalPagerModel)
	if model.projection.Facts["date"] != "2026-06-07" || loaded[0] != 1 {
		t.Fatalf("pgdown did not load next journal: model=%#v loaded=%#v", model.projection.Facts, loaded)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(JournalPagerModel)
	if model.projection.Facts["date"] != "2026-06-05" || loaded[1] != -1 {
		t.Fatalf("pgup did not load previous journal: model=%#v loaded=%#v", model.projection.Facts, loaded)
	}
	view := model.View()
	for _, want := range []string{"daily", "2026-06-05", "PgUp", "PgDn", "q Quit", "Body 2026-06-05"} {
		if !strings.Contains(view, want) {
			t.Fatalf("journal pager view missing %q:\n%s", want, view)
		}
	}
}

func journalProjection(date string) domain.Projection {
	note := domain.Note{Title: "Daily " + date, Path: fmt.Sprintf("notes/daily/%s.md", date), Body: "Body " + date}
	projection := domain.NewProjection("daily.show", "Daily note loaded.")
	projection.Facts["period"] = "daily"
	projection.Facts["date"] = date
	projection.Facts["path"] = note.Path
	projection.Data = map[string]any{"note": note}
	return projection
}
