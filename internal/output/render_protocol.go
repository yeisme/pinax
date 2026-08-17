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
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(start); err != nil {
		return err
	}

	seq := 2
	if p.Command == "sync.logs.tail" {
		if data, ok := p.Data.(map[string]any); ok {
			if events, ok := data["events"].([]map[string]any); ok {
				for _, event := range events {
					payload := map[string]any{
						"spec_version": p.SpecVersion,
						"mode":         "events",
						"command":      p.Command,
						"type":         "progress",
						"seq":          seq,
					}
					for key, value := range event {
						switch key {
						case "type":
							payload["event_type"] = value
						case "command":
							payload["source_command"] = value
						case "seq":
							payload["timeline_seq"] = value
						default:
							payload[key] = value
						}
					}
					if err := enc.Encode(payload); err != nil {
						return err
					}
					seq++
				}
			}
		}
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
		"seq":          seq,
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
	return enc.Encode(end)
}

func renderExplain(w io.Writer, p domain.Projection) error {
	if isSyncExplainCommand(p.Command) {
		return renderSyncExplain(w, p)
	}
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

func isSyncExplainCommand(command string) bool {
	switch command {
	case "sync", "sync.all", "sync.all.pull", "sync.all.push", "sync.diff", "sync.push", "sync.pull", "sync.logs.show":
		return true
	default:
		return false
	}
}

func renderSyncExplain(w io.Writer, p domain.Projection) error {
	if _, err := fmt.Fprintf(w, "Conclusion: %s\n", defaultString(p.Summary, p.Status)); err != nil {
		return err
	}
	// Keep an evidence line in the explain contract while limiting it to the
	// already-redacted projection facts/evidence; never include note bodies or
	// provider payloads in this mode.
	evidence := p.Evidence
	if len(evidence) == 0 {
		evidence = []string{}
		for _, field := range []struct{ key, label string }{{"run_id", "run_id"}, {"revision_id", "revision_id"}, {"remote_revision", "remote_revision"}} {
			if value := p.Facts[field.key]; value != "" {
				evidence = append(evidence, field.label+"="+value)
			}
		}
	}
	if len(evidence) == 0 {
		evidence = []string{"sync projection generated"}
	}
	if _, err := fmt.Fprintf(w, "Evidence: %s\n", strings.Join(evidence, "; ")); err != nil {
		return err
	}
	if scope := p.Facts["sync.scope"]; scope != "" {
		if _, err := fmt.Fprintf(w, "Scope: %s\n", scope); err != nil {
			return err
		}
	}
	if result := p.Facts["sync.result"]; result != "" {
		if _, err := fmt.Fprintf(w, "Result: %s\n", result); err != nil {
			return err
		}
	}
	for _, field := range []struct{ label, key string }{{"Run ID", "run_id"}, {"Revision", "revision_id"}, {"Remote revision", "remote_revision"}} {
		if value := p.Facts[field.key]; value != "" {
			if _, err := fmt.Fprintf(w, "%s: %s\n", field.label, value); err != nil {
				return err
			}
		}
	}
	counts := []string{"added", "modified", "deleted", "renamed", "conflicts"}
	parts := make([]string, 0, len(counts))
	for _, key := range counts {
		if value := p.Facts["sync."+key]; value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	if len(parts) > 0 {
		if _, err := fmt.Fprintf(w, "Changes: %s\n", strings.Join(parts, ", ")); err != nil {
			return err
		}
	}
	if remote := p.Facts["remote_write"]; remote != "" {
		if _, err := fmt.Fprintf(w, "Remote write: %s\n", remote); err != nil {
			return err
		}
	}
	if local := p.Facts["local_write"]; local != "" {
		if _, err := fmt.Fprintf(w, "Local write: %s\n", local); err != nil {
			return err
		}
	}
	risk := "No conflict risk reported."
	if p.Facts["sync.conflicts"] != "" && p.Facts["sync.conflicts"] != "0" {
		risk = "Conflict review is required before the next write."
	}
	if p.Error != nil {
		risk = p.Error.Message
	}
	if _, err := fmt.Fprintf(w, "Risk: %s\n", risk); err != nil {
		return err
	}
	if len(p.Actions) > 0 {
		_, err := fmt.Fprintf(w, "Recommended next step: %s\n", p.Actions[0].Command)
		return err
	}
	_, err := fmt.Fprintln(w, "Confidence: 0.8")
	return err
}
