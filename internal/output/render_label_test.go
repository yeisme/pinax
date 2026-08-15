package output

import "testing"

// TestSummaryFactLabelGolden freezes the human-readable labels for the fact
// keys that today's summaries render. The label map may be reorganized, but
// these key→label pairs must not change without a CLI output contract review.
func TestSummaryFactLabelGolden(t *testing.T) {
	golden := map[string]string{
		"run_id":                "Run ID",
		"note_id":               "Note ID",
		"path":                  "Path",
		"applied":               "Applied",
		"conflicts":             "Conflicts",
		"sync.result":           "Sync result",
		"sync.scope":            "Sync scope",
		"sync.total":            "Sync changes",
		"sync.added":            "Added",
		"sync.modified":         "Modified",
		"sync.deleted":          "Deleted",
		"sync.bytes_uploaded":   "Bytes uploaded",
		"sync.bytes_downloaded": "Bytes downloaded",
		"workspace_id":          "Workspace ID",
		"backend_kind":          "backend kind",
		"up_to_date":            "up to date",
		"remote_write":          "Remote write",
		"dry_run":               "Dry run",
	}
	for key, want := range golden {
		if got := summaryFactLabel(key); got != want {
			t.Errorf("summaryFactLabel(%q) = %q, want %q", key, got, want)
		}
	}
	if got := summaryFactLabel("totally_unknown_key"); got != "totally unknown key" {
		t.Errorf("unknown key fallback = %q, want underscore-to-space form", got)
	}
	if got := summaryFactLabel("sync.unknown"); got != "sync.unknown" {
		t.Errorf("unknown sync key fallback = %q, want passthrough", got)
	}
}
