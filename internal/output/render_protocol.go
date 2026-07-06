package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

func renderEvents(w io.Writer, p domain.Projection) error {
	start := map[string]any{
		"spec_version": p.SpecVersion,
		"mode":         "events",
		"command":      p.Command,
		"type":         "start",
		"seq":          1,
	}
	endType := "end"
	if p.Status == "failed" {
		endType = "error"
	}
	end := map[string]any{
		"spec_version": p.SpecVersion,
		"mode":         "events",
		"command":      p.Command,
		"type":         endType,
		"seq":          2,
		"status":       p.Status,
		"summary":      p.Summary,
	}
	if len(p.Facts) > 0 {
		end["facts"] = p.Facts
	}
	if len(p.Actions) > 0 {
		end["actions"] = p.Actions
	}
	if len(p.Evidence) > 0 {
		end["evidence"] = p.Evidence
	}
	if p.Error != nil {
		end["error"] = p.Error
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(start); err != nil {
		return err
	}
	return enc.Encode(end)
}

func renderExplain(w io.Writer, p domain.Projection) error {
	if _, err := fmt.Fprintf(w, "Conclusion: %s\n", defaultString(p.Summary, p.Status)); err != nil {
		return err
	}
	evidence := p.Evidence
	if len(evidence) == 0 && len(p.Facts) > 0 {
		keys := make([]string, 0, len(p.Facts))
		for key := range p.Facts {
			keys = append(keys, key)
		}
		sortFactKeys(keys)
		for _, key := range keys {
			evidence = append(evidence, key+"="+p.Facts[key])
		}
	}
	if len(evidence) == 0 {
		evidence = []string{"Command projection generated"}
	}
	if _, err := fmt.Fprintf(w, "Evidence: %s\n", strings.Join(evidence, "; ")); err != nil {
		return err
	}
	if p.Error != nil {
		if _, err := fmt.Fprintf(w, "Risk: %s\n", p.Error.Message); err != nil {
			return err
		}
	}
	if len(p.Actions) > 0 {
		if _, err := fmt.Fprintf(w, "Recommended next step: %s\n", p.Actions[0].Command); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "Confidence: 0.8")
	return err
}
