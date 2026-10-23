package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWriterEditTagsExactSourceAndReplay(t *testing.T) {
	root := readerHTTPVault(t)
	path := filepath.Join(root, "notes/alpha.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	original := strings.Replace(string(raw), "status: active\n", "status: active\ncustom: {nested: [one, two]}\nquoted: 'keep' # exact\ntags: [old] # labels\n", 1)
	original = strings.Replace(original, "\n---\n\nAlpha", "\n---\n\n\n\nAlpha", 1)
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	server, bearer, reader := newEditorTestServer(t, root)
	for _, step := range []struct {
		op    string
		tags  []string
		after string
	}{{"add", []string{"new"}, "new, old"}, {"remove", []string{"old"}, "new"}, {"set", []string{}, ""}} {
		change, _ := json.Marshal(map[string]any{"action": "tags", "operation_id": "tag-" + step.op, "note_ref": "notes/alpha.md", "tag_operation": step.op, "tags": step.tags})
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		status, envelope := writerPost(t, server, bearer, "/v1/writer/proposals", proposeBody(string(change)))
		if status != http.StatusOK {
			t.Fatalf("propose %s: %d %v", step.op, status, envelope)
		}
		preview := writerData(t, envelope)
		if preview["after"] != step.after {
			t.Fatalf("preview after=%v", preview["after"])
		}
		untouched, _ := os.ReadFile(path)
		if string(untouched) != string(before) {
			t.Fatal("propose wrote source")
		}
		resolved := preview["change"].(map[string]any)
		digest := preview["preview_digest"].(string)
		body := applyBodyFromPreview(resolved, digest, "reviewed_proposal")
		status, _ = writerPost(t, server, reader, "/v1/writer/proposals:apply", body)
		if status != http.StatusForbidden {
			t.Fatalf("reader apply=%d", status)
		}
		status, receipt := writerPost(t, server, bearer, "/v1/writer/proposals:apply", body)
		if status != http.StatusOK || writerFact(t, receipt, "operation_status") != "succeeded" {
			t.Fatalf("apply=%d %v", status, receipt)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.SplitN(string(after), "\n---\n", 2)[1] != strings.SplitN(original, "\n---\n", 2)[1] {
			t.Fatal("body bytes changed")
		}
		for _, line := range []string{"custom: {nested: [one, two]}\n", "quoted: 'keep' # exact\n", " # labels\n"} {
			if !strings.Contains(string(after), line) {
				t.Fatalf("lost source %q", line)
			}
		}
		status, replay := writerPost(t, server, bearer, "/v1/writer/proposals:apply", body)
		if status != http.StatusOK || !reflect.DeepEqual(writerData(t, receipt), writerData(t, replay)) {
			t.Fatal("original receipt changed")
		}
		final, _ := os.ReadFile(path)
		if string(final) != string(after) {
			t.Fatal("replay rewrote source")
		}
	}
}
