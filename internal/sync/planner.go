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
	Kind     string `json:"kind"` // "upload_blob", "download_blob", "delete_local", "delete_remote", "conflict"
	Path     string `json:"path,omitempty"`
	PathHash string `json:"path_hash,omitempty"`
	BlobID   string `json:"blob_id,omitempty"`
	Status   string `json:"status"`
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

	plan.Operations = diffManifests(req.BaseManifest, req.LocalManifest, req.RemoteManifest, req.Direction)
	return plan, nil
}

func requiresApproval(req Request) bool {
	return (req.Direction == DirectionPush || req.Direction == DirectionPull) && !req.DryRun && !req.Yes
}

func remoteWrite(req Request) bool {
	return req.Direction == DirectionPush && !req.DryRun && req.Yes
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
				// changed remotely, untouched locally -> download
				if dir == DirectionPull || dir == DirectionDiff {
					ops = append(ops, Operation{Kind: "download_blob", Path: path, PathHash: r.PathHash, BlobID: r.BlobID, Status: "planned"})
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
