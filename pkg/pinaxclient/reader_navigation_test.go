package pinaxclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestReaderNavigationClientRealServer(t *testing.T) {
	t.Parallel()
	server, bearer := readerClientServer(t, app.ReaderScopeMetadata, app.ReaderScopeRelations)
	client := writerClient(t, server.URL, bearer)
	ctx := context.Background()
	tags, err := client.ReaderTags(ctx, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if tags.BindingID != "binding-client" || tags.TotalTags != 0 || !tags.Coverage.PageComplete {
		t.Fatal("empty tag contract mismatch")
	}
	graph, err := client.ReaderGraph(ctx, ReaderGraphRequest{Mode: "source", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 || graph.Edges[0].SourceID != "note_alpha" || graph.Edges[0].TargetID != "note_beta" {
		t.Fatal("resolved graph mismatch")
	}
	var revision string
	for _, note := range graph.Nodes {
		if note.ObjectID == "note_alpha" {
			revision = note.Revision
		}
	}
	local, err := client.ReaderGraph(ctx, ReaderGraphRequest{Mode: "local", ObjectID: "note_alpha", Revision: revision, Hops: 2})
	if err != nil || len(local.Nodes) != 2 {
		t.Fatal("local graph failed", err)
	}
	daily, err := client.ReaderDaily(ctx, "2026-10-05", "Asia/Shanghai")
	if err != nil || daily.Status != "missing" || daily.Note != nil {
		t.Fatal("readonly daily lookup failed", err)
	}
	_, err = client.ReaderGraph(ctx, ReaderGraphRequest{Mode: "local", ObjectID: "note_alpha", Revision: "sha256:" + strings.Repeat("a", 64)})
	if err == nil {
		t.Fatal("stale center accepted")
	}
}

func TestReaderNavigationClientInvalidInputStaysLocal(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	client := writerClient(t, server.URL, "synthetic-fixture")
	for _, limit := range []int{-1, 201} {
		if _, err := client.ReaderTags(context.Background(), limit, ""); err == nil {
			t.Fatal("bad tag limit accepted")
		}
	}
	for _, req := range []ReaderGraphRequest{{Mode: "source", Limit: 1}, {Mode: "local"}, {Mode: "source", Hops: 1}, {Mode: "local", ObjectID: "note", Revision: "latest"}, {Mode: "wrong"}} {
		if _, err := client.ReaderGraph(context.Background(), req); err == nil {
			t.Fatal("bad graph request accepted")
		}
	}
	if _, err := client.ReaderDaily(context.Background(), "not-a-date", "UTC"); err == nil {
		t.Fatal("bad date accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid requests reached transport")
	}
}

func TestReaderNavigationClientRejectsUnsafeProjection(t *testing.T) {
	t.Parallel()
	for _, test := range []string{"unknown-schema", "missing-node", "duplicate-edge", "wrong-center", "coverage", "missing-daily-id", "daily-unknown-field"} {
		t.Run(test, func(t *testing.T) {
			data := map[string]any{"schema_version": ReaderGraphSchemaV1Alpha1, "binding_id": "binding", "mode": "source", "object_id": "", "revision": "", "hops": 0, "nodes": []any{map[string]any{"object_id": "a", "title": "A", "revision": "sha256:" + strings.Repeat("a", 64)}}, "edges": []any{}, "watermark": "sha256:watermark", "observed_at": "2026-10-05T00:00:00Z", "total_nodes": 1, "total_edges": 0, "coverage": map[string]any{"page_complete": true, "corpus_complete": true}}
			switch test {
			case "unknown-schema":
				data["schema_version"] = "future"
			case "missing-node":
				data["edges"] = []any{map[string]any{"source_id": "a", "target_id": "missing"}}
				data["total_edges"] = 1
			case "duplicate-edge":
				data["edges"] = []any{map[string]any{"source_id": "a", "target_id": "a"}, map[string]any{"source_id": "a", "target_id": "a"}}
				data["total_edges"] = 2
			case "wrong-center":
				data["object_id"] = "other"
			case "coverage":
				data["next_cursor"] = "cursor"
			case "missing-daily-id", "daily-unknown-field":
				data = map[string]any{"schema_version": ReaderDailySchemaV1Alpha1, "binding_id": "binding", "date": "2026-10-05", "timezone": "UTC", "status": "found", "note": map[string]any{"title": "Day", "revision": "sha256:" + strings.Repeat("a", 64)}, "watermark": "watermark", "observed_at": "2026-10-05T00:00:00Z"}
				if test == "daily-unknown-field" {
					data["note"].(map[string]any)["object_id"] = "day"
					data["note"].(map[string]any)["vault_path"] = "private"
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": data})
			}))
			defer server.Close()
			client := writerClient(t, server.URL, "synthetic-fixture")
			var err error
			if strings.Contains(test, "daily") {
				_, err = client.ReaderDaily(context.Background(), "2026-10-05", "UTC")
			} else {
				_, err = client.ReaderGraph(context.Background(), ReaderGraphRequest{Mode: "source"})
			}
			if err == nil {
				t.Fatal("unsafe projection accepted")
			}
		})
	}
}
