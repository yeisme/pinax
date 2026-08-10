package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/app/syncops"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

const syncOutputViewSchemaVersion = "pinax.sync.output.v1"

type syncOutputViewOptions struct {
	Scope       string
	Result      string
	RemoteAfter string
	LocalAfter  string
	PathPolicy  string
}

type syncOutputView struct {
	SchemaVersion string              `json:"schema_version"`
	Direction     string              `json:"direction"`
	Scope         string              `json:"scope"`
	Result        string              `json:"result"`
	Revisions     syncOutputRevisions `json:"revisions"`
	Counts        syncOutputCounts    `json:"counts"`
	Bytes         syncOutputBytes     `json:"bytes"`
	Changes       []syncOutputChange  `json:"changes,omitempty"`
	Shown         int                 `json:"shown"`
	Total         int                 `json:"total"`
	Truncated     bool                `json:"truncated"`
}

type syncOutputRevisions struct {
	Base         string `json:"base,omitempty"`
	RemoteBefore string `json:"remote_before,omitempty"`
	RemoteAfter  string `json:"remote_after,omitempty"`
	LocalAfter   string `json:"local_after,omitempty"`
}

type syncOutputCounts struct {
	Added     int `json:"added"`
	Modified  int `json:"modified"`
	Deleted   int `json:"deleted"`
	Renamed   int `json:"renamed"`
	Conflicts int `json:"conflicts"`
	Unchanged int `json:"unchanged"`
	Total     int `json:"total"`
}

type syncOutputBytes struct {
	Uploaded   int64 `json:"uploaded"`
	Downloaded int64 `json:"downloaded"`
}

type syncOutputChange struct {
	Code      string `json:"code"`
	State     string `json:"state"`
	Operation string `json:"operation"`
	Side      string `json:"side"`
	Path      string `json:"path,omitempty"`
	FromPath  string `json:"from_path,omitempty"`
	ToPath    string `json:"to_path,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

// buildSyncOutputView derives a stable, redacted presentation view from the
// existing sync plan and manifests. The plan remains the authoritative machine
// payload; this view is additive and exists so all renderers can share the
// same Git-style change classification.
func buildSyncOutputView(plan syncplan.Plan, base, local, remote pinaxcloud.Manifest, opts syncOutputViewOptions) syncOutputView {
	view := syncOutputView{
		SchemaVersion: syncOutputViewSchemaVersion,
		Direction:     string(plan.Direction),
		Scope:         defaultSyncOutputScope(opts.Scope),
		Result:        defaultSyncOutputResult(opts.Result, plan),
		Revisions: syncOutputRevisions{
			Base:         plan.BaseRevision,
			RemoteBefore: plan.RemoteRevision,
			RemoteAfter:  opts.RemoteAfter,
			LocalAfter:   opts.LocalAfter,
		},
	}

	baseEntries := syncManifestEntries(base)
	localEntries := syncManifestEntries(local)
	remoteEntries := syncManifestEntries(remote)
	changes := make([]syncOutputChange, 0, len(plan.Operations))
	changedKeys := make(map[string]struct{})
	for _, operation := range plan.Operations {
		if isManifestOperation(operation.Kind) {
			continue
		}
		code := syncOutputChangeCode(operation, baseEntries, localEntries, remoteEntries)
		if code == "" {
			continue
		}
		path, fromPath, toPath := operation.Path, operation.FromPath, operation.ToPath
		if path == "" {
			path = toPath
		}
		entry := syncOutputEntryForOperation(operation, localEntries, remoteEntries)
		change := syncOutputChange{
			Code:      code,
			State:     syncOutputChangeState(code, view.Result),
			Operation: operation.Kind,
			Side:      syncOutputChangeSide(operation.Kind),
			Path:      syncops.RedactPath(path, opts.PathPolicy),
			FromPath:  syncops.RedactPath(fromPath, opts.PathPolicy),
			ToPath:    syncops.RedactPath(toPath, opts.PathPolicy),
			SizeBytes: entry.Size,
		}
		changes = append(changes, change)
		changedKeys[syncOutputEntryKey(operation, entry)] = struct{}{}
		switch code {
		case "A":
			view.Counts.Added++
		case "M":
			view.Counts.Modified++
		case "D":
			view.Counts.Deleted++
		case "R":
			view.Counts.Renamed++
		case "C":
			view.Counts.Conflicts++
		}
		switch operation.Kind {
		case "upload_blob":
			view.Bytes.Uploaded += entry.Size
		case "download_blob":
			view.Bytes.Downloaded += entry.Size
		}
	}

	view.Counts.Unchanged = syncOutputUnchangedCount(baseEntries, localEntries, remoteEntries, changedKeys)
	view.Counts.Total = len(changes) + view.Counts.Unchanged
	sort.SliceStable(changes, func(i, j int) bool {
		left := syncOutputFirstNonEmpty(changes[i].Path, changes[i].ToPath, changes[i].FromPath)
		right := syncOutputFirstNonEmpty(changes[j].Path, changes[j].ToPath, changes[j].FromPath)
		if left == right {
			return changes[i].Code < changes[j].Code
		}
		return left < right
	})
	view.Changes = changes
	view.Total = len(changes)
	view.Shown = len(changes)
	return view
}

func defaultSyncOutputScope(scope string) string {
	if strings.TrimSpace(scope) != "" {
		return scope
	}
	return "cached"
}

func syncOutputScope(remoteChecked bool) string {
	if remoteChecked {
		return "remote-aware"
	}
	return "cached"
}

func defaultSyncOutputResult(result string, plan syncplan.Plan) string {
	if strings.TrimSpace(result) != "" {
		return result
	}
	if plan.RequiresApproval {
		return "planned"
	}
	return "planned"
}

type syncOutputManifestEntry struct {
	Key        string
	Path       string
	ObjectID   string
	BlobID     string
	RevisionID string
	Size       int64
}

func syncManifestEntries(manifest pinaxcloud.Manifest) map[string]syncOutputManifestEntry {
	entries := make(map[string]syncOutputManifestEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		key := entry.ObjectID
		if key == "" {
			key = syncOutputFirstNonEmpty(entry.PathHash, entry.Path)
		}
		if key == "" {
			continue
		}
		entries[key] = syncOutputManifestEntry{Key: key, Path: entry.Path, ObjectID: entry.ObjectID, BlobID: entry.BlobID, RevisionID: entry.RevisionID, Size: entry.Size}
	}
	return entries
}

func syncOutputEntryForOperation(operation syncplan.Operation, local, remote map[string]syncOutputManifestEntry) syncOutputManifestEntry {
	if operation.ObjectID != "" {
		if entry, ok := local[operation.ObjectID]; ok {
			return entry
		}
		if entry, ok := remote[operation.ObjectID]; ok {
			return entry
		}
	}
	for _, entries := range []map[string]syncOutputManifestEntry{local, remote} {
		for _, entry := range entries {
			if entry.Path == operation.Path || entry.Path == operation.ToPath || entry.Path == operation.FromPath {
				return entry
			}
		}
	}
	return syncOutputManifestEntry{}
}

func syncOutputChangeCode(operation syncplan.Operation, base, local, remote map[string]syncOutputManifestEntry) string {
	switch operation.Kind {
	case "move":
		return "R"
	case "conflict", "revision_conflict", "path_collision":
		return "C"
	case "delete_local", "delete_remote":
		return "D"
	case "upload_blob", "download_blob":
		entry := syncOutputEntryForOperation(operation, local, remote)
		key := syncOutputEntryKey(operation, entry)
		if key == "" {
			key = operation.Path
		}
		if _, ok := base[key]; ok {
			return "M"
		}
		return "A"
	default:
		return ""
	}
}

func syncOutputEntryKey(operation syncplan.Operation, entry syncOutputManifestEntry) string {
	if operation.ObjectID != "" {
		return operation.ObjectID
	}
	if entry.Key != "" {
		return entry.Key
	}
	return operation.Path
}

func syncOutputChangeState(code, result string) string {
	if code == "C" || result == "conflict" {
		return "conflict"
	}
	switch result {
	case "applied", "up_to_date":
		return "applied"
	case "failed", "partial":
		return "failed"
	default:
		return "planned"
	}
}

func syncOutputChangeSide(kind string) string {
	switch kind {
	case "upload_blob", "delete_remote":
		return "local"
	case "download_blob", "delete_local":
		return "remote"
	default:
		return "both"
	}
}

func syncOutputUnchangedCount(base, local, remote map[string]syncOutputManifestEntry, changed map[string]struct{}) int {
	seen := make(map[string]struct{})
	for key, left := range local {
		right, ok := remote[key]
		if !ok || left.BlobID != right.BlobID || left.RevisionID != right.RevisionID {
			continue
		}
		if _, ok := changed[key]; ok {
			continue
		}
		seen[key] = struct{}{}
	}
	if len(seen) > 0 {
		return len(seen)
	}
	// A cached first-run plan has no remote manifest. Preserve a useful total by
	// counting unchanged base entries only when both sides are unavailable.
	if len(remote) == 0 && len(local) == 0 {
		return len(base)
	}
	return 0
}

func syncOutputFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isManifestOperation(kind string) bool {
	return kind == "upload_manifest" || kind == "download_manifest"
}

func syncOutputViewMap(view syncOutputView) map[string]any {
	return map[string]any{
		"sync_view": view,
	}
}

func syncOutputFactCounts(view syncOutputView) map[string]string {
	return map[string]string{
		"sync.result":           view.Result,
		"sync.scope":            view.Scope,
		"sync.added":            fmt.Sprint(view.Counts.Added),
		"sync.modified":         fmt.Sprint(view.Counts.Modified),
		"sync.deleted":          fmt.Sprint(view.Counts.Deleted),
		"sync.renamed":          fmt.Sprint(view.Counts.Renamed),
		"sync.conflicts":        fmt.Sprint(view.Counts.Conflicts),
		"sync.unchanged":        fmt.Sprint(view.Counts.Unchanged),
		"sync.total":            fmt.Sprint(view.Counts.Total),
		"sync.bytes_uploaded":   fmt.Sprint(view.Bytes.Uploaded),
		"sync.bytes_downloaded": fmt.Sprint(view.Bytes.Downloaded),
	}
}

func attachSyncOutputView(projectionFacts map[string]string, data map[string]any, view syncOutputView) {
	for key, value := range syncOutputFactCounts(view) {
		projectionFacts[key] = value
	}
	for key, value := range syncOutputViewMap(view) {
		data[key] = value
	}
}

func buildSyncMetadataDiff(view syncOutputView) map[string]any {
	return map[string]any{
		"format": "unified",
		"lines": []string{
			"--- local/base",
			"+++ remote/head",
			"- revision: " + syncOutputFirstNonEmpty(view.Revisions.Base, "(none)"),
			"+ revision: " + syncOutputFirstNonEmpty(view.Revisions.RemoteAfter, view.Revisions.RemoteBefore, "(none)"),
			fmt.Sprintf("@@ changes %d @@", len(view.Changes)),
		},
		"change_count": len(view.Changes),
	}
}
