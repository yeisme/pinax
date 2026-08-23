package syncplan

import (
	"errors"
	"fmt"
	"sort"

	"github.com/yeisme/pinax/internal/remote"
)

const PlanSchemaVersion = "pinax.cloud.sync_plan.v1"

type Direction string

const (
	DirectionDiff Direction = "diff"
	DirectionPull Direction = "pull"
	DirectionPush Direction = "push"
)

var ErrRevisionConflict = errors.New("REVISION_CONFLICT")

type Request struct {
	Direction      Direction
	Target         string
	LocalManifest  remote.Manifest
	RemoteManifest remote.Manifest
	BaseManifest   remote.Manifest
	BaseRevision   string
	RemoteRevision string
	DryRun         bool
	Yes            bool
}

type Plan struct {
	SchemaVersion    string          `json:"schema_version"`
	Status           string          `json:"status"`
	Direction        Direction       `json:"direction"`
	Target           string          `json:"target"`
	BaseRevision     string          `json:"base_revision"`
	RemoteRevision   string          `json:"remote_revision"`
	DryRun           bool            `json:"dry_run"`
	RequiresApproval bool            `json:"requires_approval"`
	RemoteWrite      bool            `json:"remote_write"`
	Operations       []Operation     `json:"operations"`
	ConflictQueue    []ConflictEntry `json:"conflict_queue,omitempty"`
}

type Operation struct {
	ObjectID       string `json:"object_id,omitempty"`
	ObjectKind     string `json:"object_kind,omitempty"`
	FromPath       string `json:"from_path,omitempty"`
	ToPath         string `json:"to_path,omitempty"`
	LocalRevision  string `json:"local_revision,omitempty"`
	RemoteRevision string `json:"remote_revision,omitempty"`
	BaseRevision   string `json:"base_object_revision,omitempty"`
	Kind           string `json:"kind"` // "upload_blob", "download_blob", "delete_local", "delete_remote", "conflict"
	Path           string `json:"path,omitempty"`
	PathHash       string `json:"path_hash,omitempty"`
	BlobID         string `json:"blob_id,omitempty"`
	// LocalBlobID is the local manifest's content-addressed blob for this
	// object (the last synced state); apply uses it as a TOCTOU guard.
	LocalBlobID string `json:"local_blob_id,omitempty"`
	// FastForward marks a download where local provably matches the common
	// base, so apply may overwrite local with the remote blob without
	// preserving a conflict copy.
	FastForward bool   `json:"fast_forward,omitempty"`
	Status      string `json:"status"`
}

type ConflictEntry struct {
	Code            string `json:"code"`
	BaseRevision    string `json:"base_revision"`
	CurrentRevision string `json:"current_revision"`
	Resolution      string `json:"resolution"`
}

func BuildPlan(req Request) (Plan, error) {
	if req.Target == "" {
		req.Target = "cloud"
	}
	if req.Direction == "" {
		req.Direction = DirectionDiff
	}
	plan := Plan{
		SchemaVersion:    PlanSchemaVersion,
		Status:           "planned",
		Direction:        req.Direction,
		Target:           req.Target,
		BaseRevision:     req.BaseRevision,
		RemoteRevision:   req.RemoteRevision,
		DryRun:           req.DryRun,
		RequiresApproval: requiresApproval(req),
		RemoteWrite:      remoteWrite(req),
	}

	if req.Direction == DirectionPush && req.BaseRevision != "" && req.BaseRevision != req.RemoteRevision {
		plan.Status = "conflict"
		plan.RemoteWrite = false
		plan.ConflictQueue = []ConflictEntry{{Code: "REVISION_CONFLICT", BaseRevision: req.BaseRevision, CurrentRevision: req.RemoteRevision, Resolution: "manual_review"}}
		return plan, ErrRevisionConflict
	}

	if objectManifestRequest(req) {
		plan.Operations = diffObjectManifests(req.BaseManifest, req.LocalManifest, req.RemoteManifest, req.Direction)
	} else {
		plan.Operations = diffManifests(req.BaseManifest, req.LocalManifest, req.RemoteManifest, req.Direction)
	}
	return plan, nil
}

func requiresApproval(req Request) bool {
	return (req.Direction == DirectionPush || req.Direction == DirectionPull) && !req.DryRun && !req.Yes
}

func remoteWrite(req Request) bool {
	return req.Direction == DirectionPush && !req.DryRun && req.Yes
}

func objectManifestRequest(req Request) bool {
	if req.LocalManifest.SchemaVersion == remote.ManifestSchemaVersionV2 || req.RemoteManifest.SchemaVersion == remote.ManifestSchemaVersionV2 || req.BaseManifest.SchemaVersion == remote.ManifestSchemaVersionV2 {
		return true
	}
	for _, manifest := range []remote.Manifest{req.LocalManifest, req.RemoteManifest, req.BaseManifest} {
		for _, entry := range manifest.Entries {
			if entry.ObjectID != "" {
				return true
			}
		}
	}
	return false
}

func diffObjectManifests(base, local, rem remote.Manifest, dir Direction) []Operation {
	baseByID := objectEntries(base)
	localByID := objectEntries(local)
	remoteByID := objectEntries(rem)
	localByPath := objectPaths(local)
	remoteByPath := objectPaths(rem)
	operations := []Operation{}
	collisionPaths := map[string]bool{}
	for path, localEntry := range localByPath {
		if remoteEntry, ok := remoteByPath[path]; ok && localEntry.ObjectID != remoteEntry.ObjectID {
			operations = append(operations, Operation{Kind: "path_collision", Path: path, FromPath: localEntry.Path, ToPath: remoteEntry.Path, ObjectID: localEntry.ObjectID, ObjectKind: localEntry.ObjectKind, LocalRevision: localEntry.RevisionID, RemoteRevision: remoteEntry.RevisionID, Status: "planned"})
			collisionPaths[path] = true
		}
	}
	ids := map[string]bool{}
	for id := range baseByID {
		ids[id] = true
	}
	for id := range localByID {
		ids[id] = true
	}
	for id := range remoteByID {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, id := range ordered {
		baseEntry, hasBase := baseByID[id]
		localEntry, hasLocal := localByID[id]
		remoteEntry, hasRemote := remoteByID[id]
		if hasLocal && collisionPaths[localEntry.Path] {
			continue
		}
		if hasRemote && collisionPaths[remoteEntry.Path] {
			continue
		}
		if hasLocal && hasRemote {
			if hasBase && localEntry.RevisionID != baseEntry.RevisionID && remoteEntry.RevisionID != baseEntry.RevisionID && localEntry.RevisionID != remoteEntry.RevisionID {
				operations = append(operations, objectOperation("revision_conflict", localEntry, remoteEntry, localEntry.Path))
				continue
			}
			if localEntry.Path != remoteEntry.Path {
				fromPath, toPath := remoteEntry.Path, localEntry.Path
				if hasBase {
					if localEntry.Path == baseEntry.Path {
						fromPath, toPath = localEntry.Path, remoteEntry.Path
					}
					if remoteEntry.Path == baseEntry.Path {
						fromPath, toPath = remoteEntry.Path, localEntry.Path
					}
				}
				entry := localEntry
				if toPath == remoteEntry.Path {
					entry = remoteEntry
				}
				operations = append(operations, Operation{Kind: "move", ObjectID: id, ObjectKind: entry.ObjectKind, Path: toPath, FromPath: fromPath, ToPath: toPath, BlobID: entry.BlobID, LocalRevision: localEntry.RevisionID, RemoteRevision: remoteEntry.RevisionID, BaseRevision: baseEntry.RevisionID, Status: "planned"})
				continue
			}
			if localEntry.RevisionID == remoteEntry.RevisionID || localEntry.BlobID == remoteEntry.BlobID {
				continue
			}
			if dir == DirectionPush || dir == DirectionDiff {
				operations = append(operations, objectOperation("upload_blob", localEntry, remoteEntry, localEntry.Path))
			} else {
				op := objectOperation("download_blob", localEntry, remoteEntry, remoteEntry.Path)
				// A local copy that provably matches the common base has no
				// diverged content to preserve: the pull may fast-forward the
				// remote blob without leaving a conflict copy. Proof needs
				// either equal content-addressed blobs or non-empty matching
				// revision ids; empty v1 revision ids alone prove nothing.
				op.BaseRevision = baseEntry.RevisionID
				op.LocalBlobID = localEntry.BlobID
				op.FastForward = hasBase && (localEntry.BlobID == baseEntry.BlobID || (localEntry.RevisionID != "" && localEntry.RevisionID == baseEntry.RevisionID))
				operations = append(operations, op)
			}
			continue
		}
		if hasLocal {
			kind := "upload_blob"
			if hasBase {
				localUnchanged := localEntry.RevisionID == baseEntry.RevisionID || localEntry.BlobID == baseEntry.BlobID
				if localUnchanged && dir != DirectionPush {
					kind = "delete_local"
				} else if !localUnchanged {
					kind = "revision_conflict"
				} else {
					kind = "delete_remote"
				}
			} else if dir == DirectionPull {
				kind = "delete_local"
			}
			operations = append(operations, Operation{Kind: kind, ObjectID: id, ObjectKind: localEntry.ObjectKind, Path: localEntry.Path, BlobID: localEntry.BlobID, LocalRevision: localEntry.RevisionID, BaseRevision: baseEntry.RevisionID, Status: "planned"})
			continue
		}
		if hasRemote {
			kind := "download_blob"
			if hasBase {
				remoteUnchanged := remoteEntry.RevisionID == baseEntry.RevisionID || remoteEntry.BlobID == baseEntry.BlobID
				if remoteUnchanged && dir != DirectionPull {
					kind = "delete_remote"
				} else if !remoteUnchanged {
					kind = "revision_conflict"
				} else {
					kind = "download_blob"
				}
			} else if dir == DirectionPush {
				kind = "delete_remote"
			}
			operations = append(operations, Operation{Kind: kind, ObjectID: id, ObjectKind: remoteEntry.ObjectKind, Path: remoteEntry.Path, BlobID: remoteEntry.BlobID, RemoteRevision: remoteEntry.RevisionID, BaseRevision: baseEntry.RevisionID, Status: "planned"})
		}
	}
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Path == operations[j].Path {
			return operations[i].Kind < operations[j].Kind
		}
		return operations[i].Path < operations[j].Path
	})
	return operations
}

func objectEntries(manifest remote.Manifest) map[string]remote.ManifestEntry {
	out := map[string]remote.ManifestEntry{}
	for _, entry := range manifest.Entries {
		if entry.ObjectID != "" {
			out[entry.ObjectID] = entry
		}
	}
	return out
}
func objectPaths(manifest remote.Manifest) map[string]remote.ManifestEntry {
	out := map[string]remote.ManifestEntry{}
	for _, entry := range manifest.Entries {
		if entry.Path != "" {
			out[entry.Path] = entry
		}
	}
	return out
}
func objectOperation(kind string, local, remoteEntry remote.ManifestEntry, path string) Operation {
	entry := local
	if entry.ObjectID == "" {
		entry = remoteEntry
	}
	return Operation{Kind: kind, ObjectID: entry.ObjectID, ObjectKind: entry.ObjectKind, Path: path, BlobID: entry.BlobID, LocalRevision: local.RevisionID, RemoteRevision: remoteEntry.RevisionID, Status: "planned"}
}

func diffManifests(base, local, rem remote.Manifest, dir Direction) []Operation {
	baseMap := make(map[string]remote.ManifestEntry)
	localMap := make(map[string]remote.ManifestEntry)
	remoteMap := make(map[string]remote.ManifestEntry)

	for _, e := range base.Entries {
		baseMap[e.Path] = e
	}
	for _, e := range local.Entries {
		localMap[e.Path] = e
	}
	for _, e := range rem.Entries {
		remoteMap[e.Path] = e
	}

	allPaths := make(map[string]bool)
	for p := range baseMap {
		allPaths[p] = true
	}
	for p := range localMap {
		allPaths[p] = true
	}
	for p := range remoteMap {
		allPaths[p] = true
	}

	var ops []Operation

	for path := range allPaths {
		b, hasBase := baseMap[path]
		l, hasLocal := localMap[path]
		r, hasRemote := remoteMap[path]

		// 3-way diff logic
		if hasBase {
			if !hasLocal && !hasRemote {
				// deleted in both
				continue
			}
			if !hasLocal && hasRemote {
				if r.BlobID == b.BlobID {
					// deleted locally, untouched remotely -> delete remote
					if dir == DirectionPush || dir == DirectionDiff {
						ops = append(ops, Operation{Kind: "delete_remote", Path: path, PathHash: r.PathHash, Status: "planned"})
					}
				} else {
					// deleted locally, changed remotely -> conflict
					if dir == DirectionPull || dir == DirectionDiff {
						ops = append(ops, Operation{Kind: "conflict", Path: path, BlobID: r.BlobID, Status: "planned"})
					}
				}
				continue
			}
			if hasLocal && !hasRemote {
				if l.BlobID == b.BlobID {
					// deleted remotely, untouched locally -> delete local
					if dir == DirectionPull || dir == DirectionDiff {
						ops = append(ops, Operation{Kind: "delete_local", Path: path, PathHash: l.PathHash, Status: "planned"})
					}
				} else {
					// deleted remotely, changed locally -> conflict
					if dir == DirectionPull || dir == DirectionDiff {
						// remote deleted it, but local changed it. We keep local as conflict? Or push it?
						ops = append(ops, Operation{Kind: "conflict", Path: path, BlobID: l.BlobID, Status: "planned"})
					}
				}
				continue
			}
			// hasLocal && hasRemote
			if l.BlobID == r.BlobID {
				// identical, no op
				continue
			}
			if l.BlobID == b.BlobID && r.BlobID != b.BlobID {
				// changed remotely, untouched locally -> download; blob equality
				// proves local matches base, so apply may fast-forward without
				// a conflict copy (TOCTOU-guarded against post-plan edits).
				if dir == DirectionPull || dir == DirectionDiff {
					ops = append(ops, Operation{Kind: "download_blob", Path: path, PathHash: r.PathHash, BlobID: r.BlobID, LocalBlobID: l.BlobID, FastForward: true, Status: "planned"})
				}
				continue
			}
			if r.BlobID == b.BlobID && l.BlobID != b.BlobID {
				// changed locally, untouched remotely -> upload
				if dir == DirectionPush || dir == DirectionDiff {
					ops = append(ops, Operation{Kind: "upload_blob", Path: path, PathHash: l.PathHash, BlobID: l.BlobID, Status: "planned"})
				}
				continue
			}
			// Both changed independently -> conflict
			if dir == DirectionPull || dir == DirectionDiff {
				ops = append(ops, Operation{Kind: "conflict", Path: path, BlobID: r.BlobID, Status: "planned"})
			}
		} else {
			// not in base (newly added)
			if hasLocal && !hasRemote {
				if dir == DirectionPush || dir == DirectionDiff {
					ops = append(ops, Operation{Kind: "upload_blob", Path: path, PathHash: l.PathHash, BlobID: l.BlobID, Status: "planned"})
				}
			} else if !hasLocal && hasRemote {
				if dir == DirectionPull || dir == DirectionDiff {
					ops = append(ops, Operation{Kind: "download_blob", Path: path, PathHash: r.PathHash, BlobID: r.BlobID, Status: "planned"})
				}
			} else {
				// both added
				if l.BlobID != r.BlobID {
					if dir == DirectionPull || dir == DirectionDiff {
						ops = append(ops, Operation{Kind: "conflict", Path: path, BlobID: r.BlobID, Status: "planned"})
					}
				}
			}
		}
	}

	sort.Slice(ops, func(i, j int) bool {
		return ops[i].Path < ops[j].Path
	})

	switch dir {
	case DirectionPush:
		ops = append(ops, Operation{Kind: "upload_manifest", Status: "planned"})
	case DirectionPull:
		ops = append(ops, Operation{Kind: "download_manifest", Status: "planned"})
	}

	return ops
}

func ConflictError(plan Plan) error {
	if len(plan.ConflictQueue) == 0 {
		return nil
	}
	return fmt.Errorf("%w: base %s current %s", ErrRevisionConflict, plan.BaseRevision, plan.RemoteRevision)
}
