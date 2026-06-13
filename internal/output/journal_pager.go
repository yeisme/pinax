package output

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yeisme/pinax/internal/domain"
)

type JournalLoader func(direction int, current domain.Projection) (domain.Projection, error)

type JournalPagerModel struct {
	projection domain.Projection
	loader     JournalLoader
	viewport   viewport.Model
	err        error
}

func NewJournalPagerModel(projection domain.Projection, loader JournalLoader, width, height int) JournalPagerModel {
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 28
	}
	vp := viewport.New(width, height-5)
	model := JournalPagerModel{projection: projection, loader: loader, viewport: vp}
	model.refresh()
	return model
}

func RunJournalPager(ctx context.Context, in io.Reader, out io.Writer, projection domain.Projection, loader JournalLoader) error {
	program := tea.NewProgram(
		NewJournalPagerModel(projection, loader, 100, 28),
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithoutSignalHandler(),
	)
	_, err := program.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (m JournalPagerModel) Init() tea.Cmd { return nil }

func (m JournalPagerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "pgdown", "right", "n":
			m = m.navigate(1)
			return m, nil
		case "pgup", "left", "p":
			m = m.navigate(-1)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m JournalPagerModel) View() string {
	header := journalHeaderStyle.Render(fmt.Sprintf("%s  %s", m.projection.Facts["period"], m.projection.Facts["date"]))
	path := journalMutedStyle.Render(m.projection.Facts["path"])
	footer := journalMutedStyle.Render("PgUp/← Previous  PgDn/→ Next  q Quit")
	if m.err != nil {
		footer = journalErrorStyle.Render(m.err.Error()) + "\n" + footer
	}
	return strings.Join([]string{header, path, "", m.viewport.View(), "", footer}, "\n")
}

func (m JournalPagerModel) navigate(direction int) JournalPagerModel {
	if m.loader == nil {
		return m
	}
	next, err := m.loader(direction, m.projection)
	if err != nil {
		m.err = err
		return m
	}
	m.projection = next
	m.err = nil
	m.refresh()
	return m
}

func (m *JournalPagerModel) refresh() {
	m.viewport.SetContent(journalProjectionContent(m.projection))
	m.viewport.GotoTop()
}

func journalProjectionContent(projection domain.Projection) string {
	if data, ok := projection.Data.(map[string]any); ok {
		if note, ok := data["note"].(domain.Note); ok {
			body := strings.TrimSpace(note.Body)
			if body != "" {
				return body
			}
			return note.Title
		}
	}
	return projection.Summary
}

var (
	journalHeaderStyle = lipgloss.NewStyle().Bold(true)
	journalMutedStyle  = lipgloss.NewStyle().Faint(true)
	journalErrorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
)
