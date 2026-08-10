package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

func TestBuildSyncOutputViewClassifiesGitStyleChanges(t *testing.T) {
	base := pinaxcloud.Manifest{Entries: []pinaxcloud.ManifestEntry{
		{Path: "notes/changed.md", PathHash: "hash:changed", BlobID: "blob-old", Size: 10},
		{Path: "notes/deleted.md", PathHash: "hash:deleted", BlobID: "blob-deleted", Size: 20},
		{Path: "notes/renamed.md", PathHash: "hash:renamed", BlobID: "blob-renamed", Size: 30},
	}}
	local := pinaxcloud.Manifest{Entries: []pinaxcloud.ManifestEntry{
		{Path: "notes/changed.md", PathHash: "hash:changed", BlobID: "blob-new", Size: 11},
		{Path: "notes/new.md", PathHash: "hash:new", BlobID: "blob-new-file", Size: 12},
		{Path: "notes/renamed-new.md", PathHash: "hash:renamed", BlobID: "blob-renamed", Size: 30},
	}}
	remote := pinaxcloud.Manifest{Entries: []pinaxcloud.ManifestEntry{
		{Path: "notes/changed.md", PathHash: "hash:changed", BlobID: "blob-new", Size: 11},
		{Path: "notes/remote.md", PathHash: "hash:remote", BlobID: "blob-remote", Size: 13},
		{Path: "notes/renamed.md", PathHash: "hash:renamed", BlobID: "blob-renamed", Size: 30},
	}}
	plan := syncplan.Plan{Direction: syncplan.DirectionDiff, BaseRevision: "rev-base", RemoteRevision: "rev-remote", Operations: []syncplan.Operation{
		{Kind: "upload_blob", Path: "notes/changed.md", Status: "planned"},
		{Kind: "upload_blob", Path: "notes/new.md", Status: "planned"},
		{Kind: "delete_remote", Path: "notes/deleted.md", Status: "planned"},
		{Kind: "move", FromPath: "notes/renamed.md", ToPath: "notes/renamed-new.md", Path: "notes/renamed-new.md", Status: "planned"},
		{Kind: "conflict", Path: "notes/conflict.md", Status: "planned"},
		{Kind: "download_blob", Path: "notes/remote.md", Status: "planned"},
		{Kind: "upload_manifest", Status: "planned"},
	}}

	view := buildSyncOutputView(plan, base, local, remote, syncOutputViewOptions{Scope: "remote-aware", Result: "planned", PathPolicy: "default"})
	if view.SchemaVersion != syncOutputViewSchemaVersion || view.Scope != "remote-aware" || view.Result != "planned" {
		t.Fatalf("view metadata = %#v", view)
	}
	if view.Counts.Added != 2 || view.Counts.Modified != 1 || view.Counts.Deleted != 1 || view.Counts.Renamed != 1 || view.Counts.Conflicts != 1 {
		t.Fatalf("change counts = %#v", view.Counts)
	}
	if view.Total != 6 || len(view.Changes) != 6 {
		t.Fatalf("view total = %d, changes = %d", view.Total, len(view.Changes))
	}
	if view.Bytes.Uploaded != 23 || view.Bytes.Downloaded != 13 {
		t.Fatalf("view bytes = %#v", view.Bytes)
	}
	for _, change := range view.Changes {
		if change.Code == "C" {
			if change.State != "conflict" {
				t.Fatalf("conflict change state = %#v", change)
			}
			continue
		}
		if change.State != "planned" {
			t.Fatalf("planned change state = %#v", change)
		}
	}
}

func TestBuildSyncContentDiffIsBoundedAndRedacted(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "cloud", "blob-cache"), 0o755); err != nil {
		t.Fatalf("mkdir blob cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("password: old\nkeep\n"), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "cloud", "blob-cache", "blob-old"), []byte("password: old\nremove\n"), 0o600); err != nil {
		t.Fatalf("write old blob: %v", err)
	}
	payload := buildSyncContentDiff(root, syncplan.Plan{Operations: []syncplan.Operation{{Kind: "upload_blob", Path: "notes.md"}}}, pinaxcloud.Manifest{Entries: []pinaxcloud.ManifestEntry{{Path: "notes.md", BlobID: "blob-old"}}}, pinaxcloud.Manifest{Entries: []pinaxcloud.ManifestEntry{{Path: "notes.md"}}}, pinaxcloud.Manifest{}, "default")
	if payload.FileCount != 1 || payload.Shown != 1 || len(payload.Files) != 1 {
		t.Fatalf("content diff payload = %#v", payload)
	}
	for _, line := range payload.Files[0].Hunks {
		if strings.Contains(strings.ToLower(line), "password: old") {
			t.Fatalf("content diff leaked secret line: %q", line)
		}
	}
	large := bytes.Repeat([]byte("line\n"), 20_000)
	if err := os.WriteFile(filepath.Join(root, "large.md"), large, 0o644); err != nil {
		t.Fatalf("write large local: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "cloud", "blob-cache", "blob-large"), bytes.Repeat([]byte("old\n"), 20_000), 0o600); err != nil {
		t.Fatalf("write large blob: %v", err)
	}
	bounded := buildSyncContentDiff(root, syncplan.Plan{Operations: []syncplan.Operation{{Kind: "upload_blob", Path: "large.md"}}}, pinaxcloud.Manifest{Entries: []pinaxcloud.ManifestEntry{{Path: "large.md", BlobID: "blob-large"}}}, pinaxcloud.Manifest{Entries: []pinaxcloud.ManifestEntry{{Path: "large.md"}}}, pinaxcloud.Manifest{}, "default")
	if len(bounded.Files) != 1 || bounded.Files[0].Bytes > syncContentDiffFileBytes || !bounded.Files[0].Truncated {
		t.Fatalf("content diff file bound = %#v", bounded)
	}
}

func TestBuildSyncOutputViewRedactsPathsAndAppliedState(t *testing.T) {
	local := pinaxcloud.Manifest{Entries: []pinaxcloud.ManifestEntry{{Path: "private/alpha.md", PathHash: "hash:alpha", BlobID: "blob-alpha", Size: 4}}}
	plan := syncplan.Plan{Direction: syncplan.DirectionPush, RemoteRevision: "rev-before", Operations: []syncplan.Operation{{Kind: "upload_blob", Path: "private/alpha.md", Status: "planned"}}}
	view := buildSyncOutputView(plan, pinaxcloud.Manifest{}, local, pinaxcloud.Manifest{}, syncOutputViewOptions{Scope: "cached", Result: "applied", RemoteAfter: "rev-after", LocalAfter: "rev-after", PathPolicy: "hash"})
	if len(view.Changes) != 1 || view.Changes[0].State != "applied" {
		t.Fatalf("applied view = %#v", view)
	}
	if view.Changes[0].Path == "private/alpha.md" || view.Changes[0].Path == "" {
		t.Fatalf("path was not hashed: %#v", view.Changes[0])
	}
	if view.Revisions.RemoteAfter != "rev-after" || view.Revisions.LocalAfter != "rev-after" {
		t.Fatalf("revisions = %#v", view.Revisions)
	}
}
