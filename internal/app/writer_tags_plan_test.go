package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func tagPlanFixture(t *testing.T) (*Service, WriterTagsPlanRequest) {
	t.Helper()
	root := t.TempDir()
	targets := []WriterTagTarget{}
	for _, id := range []string{"note_b", "note_a"} {
		content := editorSourceFixture(t, root, "notes/"+id+".md", id, "\nExact private body\r\n\n")
		targets = append(targets, WriterTagTarget{ObjectID: id, Revision: collaborationRevision(content)})
	}
	return NewService(), WriterTagsPlanRequest{Writer: WriterTagsRequest{VaultPath: root, Session: ReaderSessionInfo{SessionID: "tags-session", BindingID: "tags-binding", Scopes: []string{WriterScopeEditor, ReaderScopeBody, ReaderScopeMetadata}}, OperationID: "tags-plan-original", PreviewSecret: []byte("01234567890123456789012345678901")}, Operation: "rename", OldTag: "reading", NewTag: "research/reading", Targets: targets}
}

func tagPlanTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if entry.IsDir() {
			result[rel] = "directory"
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[rel] = collaborationRevision(string(raw))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestWriterTagsPlanExactSelectionNoWrites(t *testing.T) {
	svc, req := tagPlanFixture(t)
	before := tagPlanTree(t, req.Writer.VaultPath)
	plan, err := svc.WriterTagsPlan(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
		t.Fatal("plan wrote owner state")
	}
	if plan.OperationID != req.Writer.OperationID || plan.OldTagID != readerTagID(req.OldTag) || plan.NewTagID != readerTagID(req.NewTag) || len(plan.Items) != 2 || plan.Items[0].ObjectID != "note_a" {
		t.Fatalf("wrong plan identity: %#v", plan)
	}
	for _, item := range plan.Items {
		if !reflect.DeepEqual(item.Before, []string{"reading", "中文"}) || !reflect.DeepEqual(item.After, []string{"research/reading", "中文"}) {
			t.Fatalf("wrong tag difference: %#v", item)
		}
	}
	wire, _ := json.Marshal(plan)
	for _, secret := range []string{req.Writer.VaultPath, "Exact private body", "never-return-this-field", string(req.Writer.PreviewSecret), req.Writer.Session.SessionID} {
		if strings.Contains(string(wire), secret) {
			t.Fatal("plan exposed private source/session data")
		}
	}
	// Input order does not change the approved plan identity.
	req.Targets[0], req.Targets[1] = req.Targets[1], req.Targets[0]
	again, err := svc.WriterTagsPlan(t.Context(), req)
	if err != nil || !reflect.DeepEqual(plan, again) {
		t.Fatal("equivalent selection changed plan", err)
	}
	for _, mutate := range []func(*WriterTagsRequest){func(r *WriterTagsRequest) { r.Session.SessionID = "other" }, func(r *WriterTagsRequest) { r.Session.BindingID = "other" }, func(r *WriterTagsRequest) { r.Session.Scopes = []string{WriterScopeEditor} }, func(r *WriterTagsRequest) { r.PreviewSecret = []byte("other-session-key-0123456789") }} {
		changed := req.Writer
		mutate(&changed)
		if writerTagsPlanDigest(req.Writer.VaultPath, changed, plan) == plan.Digest {
			t.Fatal("plan HMAC not bound to session")
		}
	}
	altered := plan
	altered.NewTag = "forged"
	if writerTagsPlanDigest(req.Writer.VaultPath, req.Writer, altered) == plan.Digest {
		t.Fatal("altered plan retained digest")
	}
}

func TestWriterTagsPlanDeleteAndMergeExactOnly(t *testing.T) {
	svc, req := tagPlanFixture(t)
	req.Targets = req.Targets[:1]
	req.NewTag = "中文"
	plan, err := svc.WriterTagsPlan(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Items[0].After, []string{"中文"}) {
		t.Fatal("existing destination duplicated")
	}
	req.Operation = "delete"
	req.NewTag = ""
	plan, err = svc.WriterTagsPlan(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewTagID != "" || !reflect.DeepEqual(plan.Items[0].After, []string{"中文"}) {
		t.Fatal("delete mapping incorrect")
	}
	req.OldTag = "read"
	if _, err = svc.WriterTagsPlan(t.Context(), req); err == nil {
		t.Fatal("prefix treated as exact tag")
	}
}

func TestWriterTagsPlanRejectsInputsAndSources(t *testing.T) {
	tests := map[string]func(*testing.T, *WriterTagsPlanRequest){
		"scope": func(t *testing.T, r *WriterTagsPlanRequest) {
			r.Writer.Session.Scopes = []string{WriterScopeEditor, ReaderScopeBody}
		},
		"secret":        func(t *testing.T, r *WriterTagsPlanRequest) { r.Writer.PreviewSecret = nil },
		"operation-id":  func(t *testing.T, r *WriterTagsPlanRequest) { r.Writer.OperationID = "" },
		"empty":         func(t *testing.T, r *WriterTagsPlanRequest) { r.Targets = nil },
		"duplicates":    func(t *testing.T, r *WriterTagsPlanRequest) { r.Targets = append(r.Targets, r.Targets[0]) },
		"too-many":      func(t *testing.T, r *WriterTagsPlanRequest) { r.Targets = make([]WriterTagTarget, 33) },
		"noncanonical":  func(t *testing.T, r *WriterTagsPlanRequest) { r.OldTag = "#reading" },
		"empty-layer":   func(t *testing.T, r *WriterTagsPlanRequest) { r.NewTag = "research//reading" },
		"same":          func(t *testing.T, r *WriterTagsPlanRequest) { r.NewTag = r.OldTag },
		"delete-target": func(t *testing.T, r *WriterTagsPlanRequest) { r.Operation = "delete" },
		"revision":      func(t *testing.T, r *WriterTagsPlanRequest) { r.Targets[0].Revision = collaborationRevision("changed") },
		"missing":       func(t *testing.T, r *WriterTagsPlanRequest) { r.Targets[0].ObjectID = "note_missing" },
		"daily":         func(t *testing.T, r *WriterTagsPlanRequest) { tagPlanRewrite(t, r, "kind: reference", "kind: daily") },
		"block-list": func(t *testing.T, r *WriterTagsPlanRequest) {
			tagPlanRewrite(t, r, "tags: [reading, 中文]", "tags:\n  - reading\n  - 中文")
		},
		"duplicate-key": func(t *testing.T, r *WriterTagsPlanRequest) {
			tagPlanRewrite(t, r, "tags: [reading, 中文]", "tags: [reading, 中文]\ntags: [reading]")
		},
		"identity-duplicate": func(t *testing.T, r *WriterTagsPlanRequest) {
			editorSourceFixture(t, r.Writer.VaultPath, "notes/duplicate.md", r.Targets[0].ObjectID, "other")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			svc, req := tagPlanFixture(t)
			mutate(t, &req)
			before := tagPlanTree(t, req.Writer.VaultPath)
			if _, err := svc.WriterTagsPlan(t.Context(), req); err == nil {
				t.Fatal("invalid plan accepted")
			}
			if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
				t.Fatal("rejected plan changed source")
			}
		})
	}
}

func TestWriterTagsPlanCancelledAndLinkedSource(t *testing.T) {
	svc, req := tagPlanFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before := tagPlanTree(t, req.Writer.VaultPath)
	if _, err := svc.WriterTagsPlan(ctx, req); err == nil {
		t.Fatal("cancelled plan accepted")
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
		t.Fatal("cancelled plan wrote state")
	}
	path := filepath.Join(req.Writer.VaultPath, "notes", req.Targets[0].ObjectID+".md")
	outside := filepath.Join(t.TempDir(), "outside.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WriterTagsPlan(t.Context(), req); err == nil {
		t.Fatal("linked source admitted")
	}
	after, err := os.ReadFile(outside)
	if err != nil || string(after) != string(raw) {
		t.Fatal("outside source changed")
	}
}

func TestWriterTagsPlanAggregateSourceBudget(t *testing.T) {
	svc, req := tagPlanFixture(t)
	req.Targets = nil
	for i := 0; i < 18; i++ {
		id := fmt.Sprintf("note_budget_%d", i)
		content := editorSourceFixture(t, req.Writer.VaultPath, "notes/"+id+".md", id, strings.Repeat("a", 60*1024))
		req.Targets = append(req.Targets, WriterTagTarget{ObjectID: id, Revision: collaborationRevision(content)})
	}
	before := tagPlanTree(t, req.Writer.VaultPath)
	if _, err := svc.WriterTagsPlan(t.Context(), req); err == nil {
		t.Fatal("aggregate budget ignored")
	} else {
		requireEditorSourceError(t, err, "body_too_large")
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
		t.Fatal("budget refusal wrote source")
	}
}

func tagPlanRewrite(t *testing.T, req *WriterTagsPlanRequest, old, next string) {
	t.Helper()
	target := &req.Targets[0]
	path := filepath.Join(req.Writer.VaultPath, "notes", target.ObjectID+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(string(raw), old, next, 1)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	target.Revision = collaborationRevision(content)
}
