package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/glamour"
)

func renderMarkdownBody(w io.Writer, body string, opts RenderOptions) error {
	body = strings.TrimRight(body, "\n")
	if strings.TrimSpace(body) == "" {
		return nil
	}
	if !opts.Markdown.Enabled {
		_, err := fmt.Fprintln(w, body)
		return err
	}
	style := markdownStyle(opts)
	width := opts.Width
	if width <= 0 {
		width = 100
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(width))
	if err != nil {
		return err
	}
	rendered, err := renderer.Render(body)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, strings.TrimRight(rendered, "\n"))
	return err
}

func markdownStyle(opts RenderOptions) string {
	style := strings.ToLower(strings.TrimSpace(opts.Markdown.Style))
	switch style {
	case "ascii", "dark", "light", "notty":
		return style
	case "auto", "":
		if summaryColorEnabledWithOptions(io.Discard, opts) {
			return "dark"
		}
		return "ascii"
	default:
		return "ascii"
	}
}
