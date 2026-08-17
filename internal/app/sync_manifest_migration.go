package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/identity"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

const (
	syncManifestAuditSchemaVersion      = "pinax.sync_manifest_identity_audit.v1"
	syncManifestPlanSchemaVersion       = "pinax.sync_manifest_migration_plan.v1"
	syncManifestCapabilitySchemaVersion = "pinax.sync_manifest_capability.v1"
	syncManifestReceiptSchemaVersion    = "pinax.sync_manifest_promotion_receipt.v1"
)

type SyncManifestMigrationRequest struct {
	VaultPath        string
	DeviceID         string
	PlanID           string
	RemoteCapability string
	Save             bool
	Yes              bool
}

type SyncManifestIdentityIssue struct {
	Path       string `json:"path"`
	ObjectKind string `json:"object_kind"`
	Code       string `json:"code"`
}

type SyncManifestIdentityAllocation struct {
	Path       string `json:"path"`
	ObjectID   string `json:"object_id"`
	ObjectKind string `json:"object_kind"`
}

type syncManifestFileIdentityRegistry struct {
	SchemaVersion string                                 `json:"schema_version"`
	Identities    map[string]pinaxcloud.ManifestIdentity `json:"identities"`
}

const syncManifestFileIdentityRegistrySchemaVersion = "pinax.sync_file_identities.v1"

type SyncManifestIdentityAudit struct {
	SchemaVersion string                                 `json:"schema_version"`
	AuditID       string                                 `json:"audit_id"`
	Eligible      bool                                   `json:"eligible"`
	DeviceID      string                                 `json:"device_id"`
	EntryCount    int                                    `json:"entry_count"`
	ResolvedCount int                                    `json:"resolved_count"`
	Issues        []SyncManifestIdentityIssue            `json:"issues,omitempty"`
	Identities    map[string]pinaxcloud.ManifestIdentity `json:"-"`
}

type SyncManifestMigrationPlan struct {
	SchemaVersion       string                           `json:"schema_version"`
	PlanID              string                           `json:"plan_id"`
	AuditID             string                           `json:"audit_id"`
	FromVersion         string                           `json:"from_version"`
	ToVersion           string                           `json:"to_version"`
	DeviceID            string                           `json:"device_id"`
	EntryCount          int                              `json:"entry_count"`
	IdentityAllocations []SyncManifestIdentityAllocation `json:"identity_allocations,omitempty"`
	SavedPath           string                           `json:"saved_path"`
	CreatedAt           string                           `json:"created_at"`
}

type SyncManifestCapabilityState struct {
	SchemaVersion         string `json:"schema_version"`
	Status                string `json:"status"`
	ManifestVersion       string `json:"manifest_version"`
	DeviceID              string `json:"device_id"`
	PlanID                string `json:"plan_id,omitempty"`
	AuditID               string `json:"audit_id,omitempty"`
	PromotedAt            string `json:"promoted_at,omitempty"`
	FirstV2RemoteRevision string `json:"first_v2_remote_revision,omitempty"`
}

type SyncManifestPromotionReceipt struct {
	SchemaVersion    string `json:"schema_version"`
	ReceiptID        string `json:"receipt_id"`
	PlanID           string `json:"plan_id"`
	AuditID          string `json:"audit_id"`
	FromVersion      string `json:"from_version"`
	ToVersion        string `json:"to_version"`
	DeviceID         string `json:"device_id"`
	RemoteCapability string `json:"remote_capability"`
	Status           string `json:"status"`
	SavedPath        string `json:"saved_path"`
	CreatedAt        string `json:"created_at"`
}

func (s *Service) SyncManifestAudit(_ context.Context, req SyncManifestMigrationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.manifest.audit", err), err
	}
	audit, err := buildSyncManifestIdentityAudit(root, req.DeviceID)
	if err != nil {
		return errorProjection("sync.manifest.audit", err), err
	}
	projection := domain.NewProjection("sync.manifest.audit", "Sync manifest identity audit completed.")
	projection.Facts["eligible"] = fmt.Sprint(audit.Eligible)
	projection.Facts["entries"] = fmt.Sprint(audit.EntryCount)
	projection.Facts["resolved"] = fmt.Sprint(audit.ResolvedCount)
	projection.Facts["issues"] = fmt.Sprint(len(audit.Issues))
	projection.Facts["writes"] = "false"
	projection.Data = map[string]any{"audit": audit}
	if audit.Eligible {
		projection.Actions = []domain.Action{{Name: "plan", Command: fmt.Sprintf("pinax sync manifest plan --save --vault %s --json", shellQuote(root))}}
	} else {
		projection.Actions = []domain.Action{{Name: "identity_audit", Command: fmt.Sprintf("pinax record identity audit --vault %s --json", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) SyncManifestPlan(ctx context.Context, req SyncManifestMigrationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.manifest.plan", err), err
	}
	audit, err := buildSyncManifestIdentityAudit(root, req.DeviceID)
	if err != nil {
		return errorProjection("sync.manifest.plan", err), err
	}
	remoteIdentities := map[string]pinaxcloud.ManifestIdentity{}
	if cloudState, loadErr := pinaxcloud.Load(root); loadErr == nil {
		if snapshot, snapshotErr := loadCloudRemoteSnapshot(ctx, root, cloudState); snapshotErr == nil && snapshot.Manifest.SchemaVersion == pinaxcloud.ManifestSchemaVersionV2 {
			for _, entry := range snapshot.Manifest.Entries {
				if identity.Classify(entry.ObjectID) == identity.IDClassCanonical {
					remoteIdentities[entry.Path] = pinaxcloud.ManifestIdentity{ObjectID: entry.ObjectID, ObjectKind: entry.ObjectKind}
				}
			}
		}
	}
	allocations := make([]SyncManifestIdentityAllocation, 0)
	for _, issue := range audit.Issues {
		if issue.Code != "object_identity_missing" {
			commandErr := &domain.CommandError{Code: "manifest_identity_migration_required", Message: "manifest v2 promotion requires canonical identity for every managed object", Hint: "Run pinax record identity audit and repair managed note or asset identity first"}
			return domain.NewErrorProjection("sync.manifest.plan", commandErr), commandErr
		}
		if remoteIdentity, ok := remoteIdentities[issue.Path]; ok {
			allocations = append(allocations, SyncManifestIdentityAllocation{Path: issue.Path, ObjectID: remoteIdentity.ObjectID, ObjectKind: remoteIdentity.ObjectKind})
			continue
		}
		allocated, allocateErr := s.allocateObjectID(identity.KindFile, root, issue.Path)
		if allocateErr != nil {
			return errorProjection("sync.manifest.plan", allocateErr), allocateErr
		}
		allocations = append(allocations, SyncManifestIdentityAllocation{Path: issue.Path, ObjectID: allocated, ObjectKind: issue.ObjectKind})
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	planID := "manifest-plan-" + shortSyncManifestDigest(audit.AuditID+"\x00"+audit.DeviceID)
	rel := filepath.ToSlash(filepath.Join(".pinax", "cloud", "manifest-migrations", planID+".json"))
	plan := SyncManifestMigrationPlan{SchemaVersion: syncManifestPlanSchemaVersion, PlanID: planID, AuditID: audit.AuditID, FromVersion: pinaxcloud.ManifestSchemaVersionV1, ToVersion: pinaxcloud.ManifestSchemaVersionV2, DeviceID: audit.DeviceID, EntryCount: audit.EntryCount, IdentityAllocations: allocations, SavedPath: rel, CreatedAt: createdAt}
	if req.Save {
		if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(rel)), plan); err != nil {
			return errorProjection("sync.manifest.plan", err), err
		}
		capability := SyncManifestCapabilityState{SchemaVersion: syncManifestCapabilitySchemaVersion, Status: "migration_planned", ManifestVersion: pinaxcloud.ManifestSchemaVersionV1, DeviceID: plan.DeviceID, PlanID: plan.PlanID, AuditID: plan.AuditID}
		if err := writeSyncManifestCapabilityState(root, capability); err != nil {
			return errorProjection("sync.manifest.plan", err), err
		}
	}
	projection := domain.NewProjection("sync.manifest.plan", "Sync manifest v2 migration plan generated.")
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["saved"] = fmt.Sprint(req.Save)
	projection.Facts["entries"] = fmt.Sprint(plan.EntryCount)
	projection.Facts["identity_allocations"] = fmt.Sprint(len(plan.IdentityAllocations))
	projection.Data = map[string]any{"plan": plan}
	if req.Save {
		projection.Evidence = []string{rel}
		projection.Actions = []domain.Action{{Name: "promote", Command: fmt.Sprintf("pinax sync manifest promote --plan %s --remote-capability v2 --vault %s --yes --json", shellQuote(plan.PlanID), shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) SyncManifestPromote(_ context.Context, req SyncManifestMigrationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.manifest.promote", err), err
	}
	if !req.Yes {
		commandErr := &domain.CommandError{Code: "approval_required", Message: "manifest v2 promotion requires --yes", Hint: "Review the saved plan, confirm every device supports manifest v2, then rerun with --yes"}
		return domain.NewErrorProjection("sync.manifest.promote", commandErr), commandErr
	}
	if strings.ToLower(strings.TrimSpace(req.RemoteCapability)) != "v2" {
		commandErr := &domain.CommandError{Code: "manifest_capability_unconfirmed", Message: "manifest v2 remote capability must be explicitly confirmed", Hint: "Use --remote-capability v2 only after every active device supports manifest v2"}
		return domain.NewErrorProjection("sync.manifest.promote", commandErr), commandErr
	}
	plan, err := readSyncManifestMigrationPlan(root, req.PlanID)
	if err != nil {
		return errorProjection("sync.manifest.promote", err), err
	}
	audit, err := buildSyncManifestIdentityAudit(root, plan.DeviceID)
	if err != nil {
		return errorProjection("sync.manifest.promote", err), err
	}
	if audit.AuditID != plan.AuditID {
		commandErr := &domain.CommandError{Code: "stale_manifest_migration_plan", Message: "manifest migration plan no longer matches the vault", Hint: "Generate and save a fresh manifest migration plan"}
		return domain.NewErrorProjection("sync.manifest.promote", commandErr), commandErr
	}
	if err := applySyncManifestIdentityAllocations(root, plan.IdentityAllocations); err != nil {
		return errorProjection("sync.manifest.promote", err), err
	}
	audit, err = buildSyncManifestIdentityAudit(root, plan.DeviceID)
	if err != nil {
		return errorProjection("sync.manifest.promote", err), err
	}
	if !audit.Eligible {
		commandErr := &domain.CommandError{Code: "manifest_identity_migration_required", Message: "manifest v2 identity allocation did not resolve every synced object", Hint: "Run a fresh sync manifest audit and repair remaining issues"}
		return domain.NewErrorProjection("sync.manifest.promote", commandErr), commandErr
	}
	if _, err := pinaxcloud.BuildManifestV2(root, plan.DeviceID, audit.Identities); err != nil {
		return errorProjection("sync.manifest.promote", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	state := SyncManifestCapabilityState{SchemaVersion: syncManifestCapabilitySchemaVersion, Status: "promoted", ManifestVersion: pinaxcloud.ManifestSchemaVersionV2, DeviceID: plan.DeviceID, PlanID: plan.PlanID, AuditID: plan.AuditID, PromotedAt: now}
	if err := writeSyncManifestCapabilityState(root, state); err != nil {
		return errorProjection("sync.manifest.promote", err), err
	}
	receiptID := "manifest-promotion-" + shortSyncManifestDigest(plan.PlanID+"\x00"+now)
	rel := filepath.ToSlash(filepath.Join(".pinax", "cloud", "manifest-migrations", "receipts", receiptID+".json"))
	receipt := SyncManifestPromotionReceipt{SchemaVersion: syncManifestReceiptSchemaVersion, ReceiptID: receiptID, PlanID: plan.PlanID, AuditID: plan.AuditID, FromVersion: plan.FromVersion, ToVersion: plan.ToVersion, DeviceID: plan.DeviceID, RemoteCapability: "v2", Status: "promoted_local", SavedPath: rel, CreatedAt: now}
	if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(rel)), receipt); err != nil {
		return errorProjection("sync.manifest.promote", err), err
	}
	projection := domain.NewProjection("sync.manifest.promote", "Sync manifest v2 promotion completed locally.")
	projection.Facts["manifest_version"] = state.ManifestVersion
	projection.Facts["promotion_status"] = state.Status
	projection.Facts["remote_write"] = "false"
	projection.Evidence = []string{syncManifestCapabilityStateRel(), rel}
	projection.Actions = []domain.Action{{Name: "push", Command: fmt.Sprintf("pinax sync push --target capsa --vault %s --yes --json", shellQuote(root))}, {Name: "rollback", Command: fmt.Sprintf("pinax sync manifest rollback --vault %s --yes --json", shellQuote(root))}}
	projection.Data = map[string]any{"capability": state, "receipt": receipt}
	return projection, nil
}

func (s *Service) SyncManifestRollback(_ context.Context, req SyncManifestMigrationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.manifest.rollback", err), err
	}
	if !req.Yes {
		commandErr := &domain.CommandError{Code: "approval_required", Message: "manifest rollback requires --yes", Hint: "Rerun with --yes before the first v2 remote write"}
		return domain.NewErrorProjection("sync.manifest.rollback", commandErr), commandErr
	}
	state, err := readSyncManifestCapabilityState(root)
	if err != nil {
		return errorProjection("sync.manifest.rollback", err), err
	}
	if strings.TrimSpace(state.FirstV2RemoteRevision) != "" {
		commandErr := &domain.CommandError{Code: "manifest_rollback_unsafe", Message: "manifest v2 was already written remotely", Hint: "Keep manifest v2 enabled and upgrade remaining devices; do not create a second v1 authoritative head"}
		return domain.NewErrorProjection("sync.manifest.rollback", commandErr), commandErr
	}
	state.Status = "rolled_back"
	state.ManifestVersion = pinaxcloud.ManifestSchemaVersionV1
	if err := writeSyncManifestCapabilityState(root, state); err != nil {
		return errorProjection("sync.manifest.rollback", err), err
	}
	projection := domain.NewProjection("sync.manifest.rollback", "Sync manifest promotion rolled back locally.")
	projection.Facts["manifest_version"] = state.ManifestVersion
	projection.Facts["remote_write"] = "false"
	projection.Evidence = []string{syncManifestCapabilityStateRel()}
	projection.Data = map[string]any{"capability": state}
	return projection, nil
}

func buildLocalCloudManifest(root string, state pinaxcloud.State) (pinaxcloud.Manifest, error) {
	capability, err := readSyncManifestCapabilityState(root)
	if err != nil && !os.IsNotExist(err) {
		return pinaxcloud.Manifest{}, err
	}
	if err == nil && capability.Status == "promoted" && capability.ManifestVersion == pinaxcloud.ManifestSchemaVersionV2 {
		deviceID := strings.TrimSpace(state.Config.DeviceID)
		if deviceID == "" {
			deviceID = capability.DeviceID
		}
		audit, auditErr := buildSyncManifestIdentityAudit(root, deviceID)
		if auditErr != nil {
			return pinaxcloud.Manifest{}, auditErr
		}
		if !audit.Eligible {
			return pinaxcloud.Manifest{}, &domain.CommandError{Code: "manifest_identity_migration_required", Message: "manifest v2 sync requires canonical identity for every synced object", Hint: "Run pinax sync manifest audit --vault <vault> --json"}
		}
		return pinaxcloud.BuildManifestV2(root, deviceID, audit.Identities)
	}
	return pinaxcloud.BuildManifest(root)
}

func buildSyncManifestIdentityAudit(root, requestedDeviceID string) (SyncManifestIdentityAudit, error) {
	manifest, err := pinaxcloud.BuildManifest(root)
	if err != nil {
		return SyncManifestIdentityAudit{}, err
	}
	deviceID := strings.TrimSpace(requestedDeviceID)
	if deviceID == "" {
		if state, loadErr := pinaxcloud.Load(root); loadErr == nil {
			deviceID = strings.TrimSpace(state.Config.DeviceID)
		}
	}
	issues := make([]SyncManifestIdentityIssue, 0)
	identities := make(map[string]pinaxcloud.ManifestIdentity, len(manifest.Entries))
	registry, registryErr := loadSyncManifestFileIdentityRegistry(root)
	if registryErr != nil {
		return SyncManifestIdentityAudit{}, registryErr
	}
	for path, fact := range registry.Identities {
		if identity.Classify(fact.ObjectID) == identity.IDClassCanonical {
			identities[path] = fact
		}
	}
	notes, err := scanNotes(root)
	if err != nil {
		return SyncManifestIdentityAudit{}, err
	}
	managedNotePaths := make(map[string]identity.IDClass, len(notes))
	for _, note := range notes {
		class := identity.Classify(note.ID)
		managedNotePaths[note.Path] = class
		if class == identity.IDClassCanonical {
			identities[note.Path] = pinaxcloud.ManifestIdentity{ObjectID: note.ID, ObjectKind: "note"}
		}
	}
	assetManifest, err := pinaxassets.Load(root)
	if err != nil {
		return SyncManifestIdentityAudit{}, err
	}
	for _, asset := range assetManifest.Assets {
		if identity.Classify(asset.ObjectID) == identity.IDClassCanonical {
			identities[asset.Path] = pinaxcloud.ManifestIdentity{ObjectID: asset.ObjectID, ObjectKind: "asset"}
		}
	}
	seenObjectIDs := map[string]string{}
	for _, entry := range manifest.Entries {
		fact, ok := identities[entry.Path]
		code := ""
		if !ok {
			if class, managed := managedNotePaths[entry.Path]; managed && class != identity.IDClassCanonical {
				code = "canonical_object_identity_required"
			} else {
				code = "object_identity_missing"
			}
		} else if previousPath, duplicate := seenObjectIDs[fact.ObjectID]; duplicate {
			code = "duplicate_object_id"
			_ = previousPath
		} else {
			seenObjectIDs[fact.ObjectID] = entry.Path
		}
		if code != "" {
			issues = append(issues, SyncManifestIdentityIssue{Path: entry.Path, ObjectKind: entry.ObjectKind, Code: code})
		}
	}
	if deviceID == "" {
		issues = append(issues, SyncManifestIdentityIssue{ObjectKind: "device", Code: "device_id_missing"})
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Path+issues[i].Code < issues[j].Path+issues[j].Code })
	auditFact := strings.Builder{}
	auditFact.WriteString(deviceID)
	for _, entry := range manifest.Entries {
		fact := identities[entry.Path]
		auditFact.WriteString("\n" + entry.Path + "\x00" + entry.BlobID + "\x00" + fact.ObjectID + "\x00" + fact.ObjectKind)
	}
	for _, issue := range issues {
		auditFact.WriteString("\nissue:" + issue.Path + ":" + issue.Code)
	}
	return SyncManifestIdentityAudit{SchemaVersion: syncManifestAuditSchemaVersion, AuditID: "manifest-audit-" + shortSyncManifestDigest(auditFact.String()), Eligible: len(issues) == 0, DeviceID: deviceID, EntryCount: len(manifest.Entries), ResolvedCount: len(manifest.Entries) - len(issues), Issues: issues, Identities: identities}, nil
}

func syncManifestFileIdentityRegistryRel() string {
	return filepath.ToSlash(filepath.Join(".pinax", "cloud", "file-identities.json"))
}

func loadSyncManifestFileIdentityRegistry(root string) (syncManifestFileIdentityRegistry, error) {
	registry := syncManifestFileIdentityRegistry{SchemaVersion: syncManifestFileIdentityRegistrySchemaVersion, Identities: map[string]pinaxcloud.ManifestIdentity{}}
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(syncManifestFileIdentityRegistryRel())))
	if os.IsNotExist(err) {
		return registry, nil
	}
	if err != nil {
		return registry, err
	}
	if err := json.Unmarshal(payload, &registry); err != nil {
		return registry, err
	}
	if registry.SchemaVersion != syncManifestFileIdentityRegistrySchemaVersion {
		return registry, fmt.Errorf("invalid_sync_file_identity_registry")
	}
	if registry.Identities == nil {
		registry.Identities = map[string]pinaxcloud.ManifestIdentity{}
	}
	return registry, nil
}

func applySyncManifestIdentityAllocations(root string, allocations []SyncManifestIdentityAllocation) error {
	registry, err := loadSyncManifestFileIdentityRegistry(root)
	if err != nil {
		return err
	}
	for _, allocation := range allocations {
		path := filepath.ToSlash(strings.TrimSpace(allocation.Path))
		if path == "" || identity.Classify(allocation.ObjectID) != identity.IDClassCanonical {
			return fmt.Errorf("invalid_manifest_identity_allocation")
		}
		if existing, ok := registry.Identities[path]; ok && existing.ObjectID != allocation.ObjectID {
			return &domain.CommandError{Code: "manifest_identity_collision", Message: "file identity allocation conflicts with existing registry state", Hint: "Generate a fresh manifest migration plan"}
		}
		registry.Identities[path] = pinaxcloud.ManifestIdentity{ObjectID: allocation.ObjectID, ObjectKind: allocation.ObjectKind}
	}
	return writeJSONAsset(filepath.Join(root, filepath.FromSlash(syncManifestFileIdentityRegistryRel())), registry)
}

func readSyncManifestMigrationPlan(root, planID string) (SyncManifestMigrationPlan, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" || strings.Contains(planID, "/") || strings.Contains(planID, "\\") || strings.Contains(planID, "..") {
		return SyncManifestMigrationPlan{}, &domain.CommandError{Code: "manifest_plan_required", Message: "saved manifest migration plan is required", Hint: "Run pinax sync manifest plan --save --vault <vault> --json"}
	}
	path := filepath.Join(root, ".pinax", "cloud", "manifest-migrations", planID+".json")
	payload, err := os.ReadFile(path)
	if err != nil {
		return SyncManifestMigrationPlan{}, err
	}
	var plan SyncManifestMigrationPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		return SyncManifestMigrationPlan{}, err
	}
	if plan.SchemaVersion != syncManifestPlanSchemaVersion || plan.PlanID != planID {
		return SyncManifestMigrationPlan{}, fmt.Errorf("invalid_manifest_migration_plan")
	}
	return plan, nil
}

func syncManifestCapabilityStateRel() string {
	return filepath.ToSlash(filepath.Join(".pinax", "cloud", "manifest-capability.json"))
}

func readSyncManifestCapabilityState(root string) (SyncManifestCapabilityState, error) {
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(syncManifestCapabilityStateRel())))
	if err != nil {
		return SyncManifestCapabilityState{}, err
	}
	var state SyncManifestCapabilityState
	if err := json.Unmarshal(payload, &state); err != nil {
		return SyncManifestCapabilityState{}, err
	}
	if state.SchemaVersion != syncManifestCapabilitySchemaVersion {
		return SyncManifestCapabilityState{}, fmt.Errorf("invalid_manifest_capability_state")
	}
	return state, nil
}

func writeSyncManifestCapabilityState(root string, state SyncManifestCapabilityState) error {
	return writeJSONAsset(filepath.Join(root, filepath.FromSlash(syncManifestCapabilityStateRel())), state)
}

func recordFirstV2RemoteRevision(root, revision string) error {
	if strings.TrimSpace(revision) == "" {
		return nil
	}
	state, err := readSyncManifestCapabilityState(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if state.ManifestVersion != pinaxcloud.ManifestSchemaVersionV2 || state.FirstV2RemoteRevision != "" {
		return nil
	}
	state.FirstV2RemoteRevision = strings.TrimSpace(revision)
	return writeSyncManifestCapabilityState(root, state)
}

func shortSyncManifestDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}

func syncManifestRemoteWriteGate(root string) error {
	state, err := readSyncManifestCapabilityState(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if state.Status == "migration_planned" {
		return &domain.CommandError{Code: "manifest_promotion_pending", Message: "saved manifest v2 migration plan is pending", Hint: "Promote the saved plan or roll it back before automatic remote writes"}
	}
	return nil
}

func negotiateSyncManifestCapability(localManifest, remoteManifest pinaxcloud.Manifest) error {
	if len(remoteManifest.Entries) == 0 && len(remoteManifest.Deletes) == 0 {
		return nil
	}
	localVersion := strings.TrimSpace(localManifest.SchemaVersion)
	remoteVersion := strings.TrimSpace(remoteManifest.SchemaVersion)
	if localVersion == remoteVersion {
		return nil
	}
	if localVersion == pinaxcloud.ManifestSchemaVersionV2 && remoteVersion == pinaxcloud.ManifestSchemaVersionV1 {
		return &domain.CommandError{Code: "manifest_capability_mismatch", Message: "remote authoritative head still uses manifest v1", Hint: "Upgrade every active device and explicitly coordinate manifest v2 promotion before writing remotely"}
	}
	if localVersion == pinaxcloud.ManifestSchemaVersionV1 && remoteVersion == pinaxcloud.ManifestSchemaVersionV2 {
		return &domain.CommandError{Code: "manifest_v2_migration_required", Message: "remote authoritative head already uses manifest v2", Hint: "Run pinax sync manifest audit, plan --save, and promote before syncing this device"}
	}
	return &domain.CommandError{Code: "manifest_capability_unknown", Message: "sync manifest capability negotiation failed", Hint: "Inspect the remote manifest schema and avoid remote writes until every device agrees"}
}
