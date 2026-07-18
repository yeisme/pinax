package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/output"
)

func TestSyncLogFollowEmitterRendersSafeStreamingModes(t *testing.T) {
	event := map[string]any{"seq": 2, "type": "sync.file", "run_id": "sync_1", "direction": "push", "kind": "upload_blob", "path": "notes/live.md", "status": "success", "backend_kind": "embedded", "ts": "2026-07-14T03:00:00Z"}

	var summary bytes.Buffer
	if err := newSyncLogFollowEmitter(&summary, output.ModeSummary)(event); err != nil {
		t.Fatalf("summary emit: %v", err)
	}
	for _, want := range []string{"sync.file", "push", "upload_blob", "notes/live.md", "success"} {
		if !strings.Contains(summary.String(), want) {
			t.Fatalf("summary missing %q: %s", want, summary.String())
		}
	}

	var agent bytes.Buffer
	agentEmit := newSyncLogFollowEmitter(&agent, output.ModeAgent)
	if err := agentEmit(event); err != nil {
		t.Fatalf("agent emit: %v", err)
	}
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=sync.logs.tail", "event.2.type=sync.file", "event.2.kind=upload_blob", "event.2.path=notes/live.md"} {
		if !strings.Contains(agent.String(), want) {
			t.Fatalf("agent missing %q: %s", want, agent.String())
		}
	}

	var events bytes.Buffer
	if err := newSyncLogFollowEmitter(&events, output.ModeEvents)(event); err != nil {
		t.Fatalf("events emit: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(events.Bytes(), &payload); err != nil {
		t.Fatalf("events output is not NDJSON: %v\n%s", err, events.String())
	}
	if payload["command"] != "sync.logs.tail" || payload["type"] != "progress" || payload["event_type"] != "sync.file" || payload["path"] != "notes/live.md" {
		t.Fatalf("events payload = %#v", payload)
	}
}
