package app

// DriveBridge attach surface: zero-copy attach of the SAME local root or S3
// bucket/prefix that pinax.storage.v1 already points at, explicit provider
// working-copy opt-in, second-device hydrate, and doctor facts.
//
// Boundaries (pinax-drivebridge-remote-notes-v1):
//   - DriveBridge is a file plane only. Pinax keeps note identity, the
//     Markdown source of truth, proof loops, and bounded projections.
//   - A DriveBridge list of .md files is never a note projection.
//   - No command here touches Capsa sync-state or emits Capsa remote_write.
//   - .pinax/** stays CLI-authored; hydrate never rebuilds owner metadata
//     from DriveBridge bytes.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	pinaxconfig "github.com/yeisme/pinax/internal/config"
	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	"gopkg.in/yaml.v3"
)

const (
	drivebridgeBinaryName          = "drivebridge"
	drivebridgeAttachSchemaVersion = "pinax.drivebridge_attach.v1"
	drivebridgeConsumerPinax       = "pinax"
	drivebridgePurposeVault        = "vault"

	// drivebridgeContentMode* are the frozen English fact enums.
	drivebridgeContentModeNone              = "none"
	drivebridgeContentModeAdoptLocal        = "adopt_local"
	drivebridgeContentModeAdoptS3           = "adopt_s3"
	drivebridgeContentModeProviderPlaintext = "provider-plaintext"
	drivebridgeContentModeOpaqueEncrypted   = "opaque_encrypted"
)

// drivebridgeEnvelope mirrors the DriveBridge CLI machine envelope
// (spec_version 1.0) so failures surface the owner-side error code verbatim.
type drivebridgeEnvelope struct {
	SpecVersion string          `json:"spec_version"`
	Mode        string          `json:"mode"`
	Command     string          `json:"command"`
	Status      string          `json:"status"`
	Summary     string          `json:"summary"`
	Data        json.RawMessage `json:"data"`
	Error       *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type drivebridgeAdoptRecord struct {
	Consumer string `json:"consumer"`
	Kind     string `json:"kind"`
	Space    string `json:"space"`
	Location string `json:"location"`
	Copied   bool   `json:"copied"`
}

type drivebridgeAdoptStatusData struct {
	Count  int                      `json:"count"`
	Adopts []drivebridgeAdoptRecord `json:"adopts"`
}

type drivebridgeSpace struct {
	ID        string `json:"id"`
	Backend   string `json:"backend"`
	LocalRoot string `json:"local_root"`
	Adopt     *struct {
		Consumer string `json:"consumer"`
		Kind     string `json:"kind"`
		Location string `json:"location"`
	} `json:"adopt"`
}

type drivebridgeSpaceListData struct {
	Count  int                `json:"count"`
	Spaces []drivebridgeSpace `json:"spaces"`
}

type drivebridgeFile struct {
	Ref       string `json:"ref"`
	SpaceID   string `json:"space_id"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	Directory bool   `json:"is_directory"`
}

type drivebridgeListData struct {
	Count int               `json:"count"`
	Files []drivebridgeFile `json:"files"`
}

func drivebridgeAttachPath(root string) string {
	return filepath.Join(root, ".pinax", "drivebridge-attach.yaml")
}

func loadDrivebridgeAttach(root string) (domain.DrivebridgeAttach, bool, error) {
	b, err := os.ReadFile(drivebridgeAttachPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return domain.DrivebridgeAttach{}, false, nil
	}
	if err != nil {
		return domain.DrivebridgeAttach{}, false, err
	}
	var rec domain.DrivebridgeAttach
	if err := yaml.Unmarshal(b, &rec); err != nil {
		return domain.DrivebridgeAttach{}, false, err
	}
	if rec.SchemaVersion == "" {
		rec.SchemaVersion = drivebridgeAttachSchemaVersion
	}
	return rec, true, nil
}

func saveDrivebridgeAttach(root string, rec domain.DrivebridgeAttach) error {
	rec.SchemaVersion = drivebridgeAttachSchemaVersion
	b, err := yaml.Marshal(rec)
	if err != nil {
		return err
	}
	return atomicWriteFile(drivebridgeAttachPath(root), b, 0o644)
}

func removeDrivebridgeAttach(root string) error {
	err := os.Remove(drivebridgeAttachPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func drivebridgeInstalled() bool {
	_, err := exec.LookPath(drivebridgeBinaryName)
	return err == nil
}

// runDrivebridgeJSON executes a DriveBridge CLI call in machine mode and
// returns the parsed data envelope. Owner-side failure codes are surfaced as
// domain.CommandError with the owner code preserved.
func runDrivebridgeJSON(ctx context.Context, args ...string) (json.RawMessage, string, error) {
	cmdArgs := append([]string{"--json"}, args...)
	cmd := exec.CommandContext(ctx, drivebridgeBinaryName, cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	envelope := drivebridgeEnvelope{}
	parseErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &envelope)
	if runErr != nil {
		if parseErr == nil && envelope.Error != nil && envelope.Error.Code != "" {
			return nil, envelope.Error.Message, &domain.CommandError{
				Code:    envelope.Error.Code,
				Message: envelope.Error.Message,
				Hint:    "See drivebridge " + strings.Join(args, " ") + " output for the owner-side remediation",
			}
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = runErr.Error()
		}
		return nil, "", &domain.CommandError{
			Code:    "drivebridge_invocation_failed",
			Message: message,
			Hint:    "Check the DriveBridge CLI installation with drivebridge doctor",
		}
	}
	if parseErr != nil {
		return nil, "", &domain.CommandError{
			Code:    "drivebridge_invocation_failed",
			Message: "DriveBridge CLI output was not a valid machine envelope",
			Hint:    "Upgrade DriveBridge and retry; this Pinax version requires the storage adopt contract",
		}
	}
	if envelope.Status != "success" {
		code := "drivebridge_invocation_failed"
		message := envelope.Summary
		if envelope.Error != nil && envelope.Error.Code != "" {
			code = envelope.Error.Code
			message = envelope.Error.Message
		}
		return nil, message, &domain.CommandError{Code: code, Message: message, Hint: "Run drivebridge doctor to inspect the DriveBridge state"}
	}
	return envelope.Data, envelope.Summary, nil
}

// drivebridgeAdopts lists owner-side zero-copy adopt records.
func drivebridgeAdopts(ctx context.Context) ([]drivebridgeAdoptRecord, error) {
	data, _, err := runDrivebridgeJSON(ctx, "storage", "adopt", "status")
	if err != nil {
		return nil, err
	}
	parsed := drivebridgeAdoptStatusData{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, &domain.CommandError{Code: "drivebridge_invocation_failed", Message: "DriveBridge adopt status payload was unreadable", Hint: "Upgrade DriveBridge and retry"}
	}
	return parsed.Adopts, nil
}

// attachTargetLocation derives the adopt location string for the current
// storage profile. DriveBridge normalizes local roots to cleaned absolute
// paths and s3 to "<bucket>/<prefix>" (no leading or trailing slash).
func attachTargetLocation(root string, profile domain.StorageProfile) (kind string, location domain.DrivebridgeLocation, contentMode string, err error) {
	switch profile.Backend {
	case "s3":
		if profile.S3 == nil || strings.TrimSpace(profile.S3.Bucket) == "" {
			return "", domain.DrivebridgeLocation{}, "", &domain.CommandError{
				Code:    "s3_config_incomplete",
				Message: "S3 storage backend requires bucket and region",
				Hint:    "Rerun pinax storage set s3 --bucket <bucket> --region <region>",
			}
		}
		prefix := strings.Trim(filepath.ToSlash(strings.TrimSpace(profile.S3.Prefix)), "/")
		return "s3", domain.DrivebridgeLocation{Bucket: profile.S3.Bucket, Prefix: prefix}, drivebridgeContentModeAdoptS3, nil
	default:
		storageRoot := root
		if profile.Local != nil && strings.TrimSpace(profile.Local.Root) != "" {
			storageRoot = profile.Local.Root
		}
		abs, absErr := filepath.Abs(storageRoot)
		if absErr != nil {
			return "", domain.DrivebridgeLocation{}, "", absErr
		}
		return "local", domain.DrivebridgeLocation{Root: filepath.Clean(abs)}, drivebridgeContentModeAdoptLocal, nil
	}
}

func drivebridgeLocationString(loc domain.DrivebridgeLocation) string {
	if loc.Bucket != "" {
		return strings.Trim(loc.Bucket+"/"+loc.Prefix, "/")
	}
	return loc.Root
}

func sanitizeDrivebridgeName(value string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, strings.ToLower(value))
	if out == "" {
		return "pinax"
	}
	return out
}

// AttachDrivebridge adopts the SAME local root or S3 bucket/prefix as the
// current pinax.storage.v1 profile. Zero copy: no note bytes move, no new
// bucket is created, and .pinax/storage.json is left untouched.
func (s *Service) AttachDrivebridge(ctx context.Context, req DrivebridgeAttachRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("storage.attach_drivebridge", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("storage.attach_drivebridge", err), err
	}
	space := strings.TrimSpace(req.Space)
	if space == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "attach-drivebridge requires a DriveBridge space", Hint: "pinax storage attach-drivebridge --space <space> --vault <vault> --json"}
		return domain.NewErrorProjection("storage.attach_drivebridge", err), err
	}
	profile, err := loadStorageProfile(root)
	if err != nil {
		return errorProjection("storage.attach_drivebridge", err), err
	}
	kind, location, contentMode, err := attachTargetLocation(root, profile)
	if err != nil {
		return errorProjection("storage.attach_drivebridge", err), err
	}
	if !drivebridgeInstalled() {
		err := &domain.CommandError{
			Code:    "drivebridge_not_installed",
			Message: "DriveBridge CLI was not found on PATH",
			Hint:    "Install DriveBridge first; pinax storage set|status|doctor keep working without it",
		}
		return domain.NewErrorProjection("storage.attach_drivebridge", err), err
	}
	adopts, err := drivebridgeAdopts(ctx)
	if err != nil {
		return errorProjection("storage.attach_drivebridge", err), err
	}
	wantLocation := drivebridgeLocationString(location)
	for _, rec := range adopts {
		if rec.Space != space {
			continue
		}
		if rec.Kind == "onedrive" || rec.Kind == "gdrive" {
			// OneDrive/GDrive spaces are provider working copies and need the
			// explicit plaintext opt-in; they are never a default attach target.
			err := &domain.CommandError{
				Code:    "drivebridge_working_copy_opt_in_required",
				Message: "DriveBridge space " + space + " is a " + rec.Kind + " provider space",
				Hint:    "Run pinax storage bind-working-copy --provider " + rec.Kind + " --space " + space + " to opt in to a plaintext working copy",
			}
			return domain.NewErrorProjection("storage.attach_drivebridge", err), err
		}
		if rec.Location != wantLocation {
			err := &domain.CommandError{
				Code:    "drivebridge_location_mismatch",
				Message: "DriveBridge space " + space + " already adopts " + rec.Location + " but the current storage profile points at " + wantLocation,
				Hint:    "Attach the same location as pinax storage set, or pick a different --space",
			}
			return domain.NewErrorProjection("storage.attach_drivebridge", err), err
		}
	}
	adoptArgs := []string{"storage", "adopt", "--kind", kind, "--consumer", drivebridgeConsumerPinax, "--space", space}
	remote := strings.TrimSpace(req.Remote)
	if kind == "local" {
		adoptArgs = append(adoptArgs, "--root", location.Root)
	} else {
		if remote == "" {
			// Derive a stable rclone remote name from the storage bucket; the
			// user can override with --remote when their rclone config differs.
			remote = "pinax-s3-" + sanitizeDrivebridgeName(location.Bucket)
		}
		adoptArgs = append(adoptArgs, "--remote", remote, "--remote-path", wantLocation, "--local-root", root)
	}
	data, _, err := runDrivebridgeJSON(ctx, adoptArgs...)
	if err != nil {
		return errorProjection("storage.attach_drivebridge", err), err
	}
	adopted := drivebridgeAdoptRecord{}
	if err := json.Unmarshal(data, &adopted); err != nil || adopted.Location != wantLocation {
		err := &domain.CommandError{
			Code:    "drivebridge_location_mismatch",
			Message: "DriveBridge adopted a different location than the current storage profile",
			Hint:    "Attach the same location as pinax storage set, or pick a different --space",
		}
		return domain.NewErrorProjection("storage.attach_drivebridge", err), err
	}
	if location.Bucket != "" {
		location.Remote = remote
	}
	record := domain.DrivebridgeAttach{
		SchemaVersion:  drivebridgeAttachSchemaVersion,
		Consumer:       drivebridgeConsumerPinax,
		Space:          space,
		Kind:           kind,
		Purpose:        drivebridgePurposeVault,
		Location:       location,
		IdempotencyKey: "adopt:pinax:" + kind + ":" + space,
		ContentMode:    contentMode,
		AttachedAt:     s.now().Format(time.RFC3339),
	}
	if err := saveDrivebridgeAttach(root, record); err != nil {
		return errorProjection("storage.attach_drivebridge", err), err
	}
	appendEventWarned(root, "storage.attach_drivebridge", "success", map[string]string{"space": space, "kind": kind, "content_mode": contentMode})
	projection := domain.NewProjection("storage.attach_drivebridge", "DriveBridge 已零拷贝挂接当前存储位置。")
	projection.Facts["drivebridge_attached"] = "true"
	projection.Facts["drivebridge_space"] = space
	projection.Facts["drivebridge_kind"] = kind
	projection.Facts["drivebridge_content_mode"] = contentMode
	projection.Facts["drivebridge_location_match"] = "true"
	projection.Facts["copied"] = "false"
	projection.Data = map[string]any{"attach": record, "copied": false}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "drivebridge-attach.yaml"))}
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax storage doctor --vault %s --json", shellQuote(root))}}
	return projection, nil
}

// DetachDrivebridge removes the Pinax attach record and asks DriveBridge to
// release the adopt. Storage profile, Capsa config, and note bytes are never
// modified.
func (s *Service) DetachDrivebridge(ctx context.Context, req DrivebridgeDetachRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("storage.detach_drivebridge", err), err
	}
	record, attached, err := loadDrivebridgeAttach(root)
	if err != nil {
		return errorProjection("storage.detach_drivebridge", err), err
	}
	if !attached {
		err := &domain.CommandError{
			Code:    "drivebridge_not_attached",
			Message: "This vault has no DriveBridge attach record",
			Hint:    "Run pinax storage attach-drivebridge --space <space> first",
		}
		return domain.NewErrorProjection("storage.detach_drivebridge", err), err
	}
	projection := domain.NewProjection("storage.detach_drivebridge", "DriveBridge attach 记录已移除。")
	projection.Facts["drivebridge_attached"] = "false"
	projection.Facts["space"] = record.Space
	// Request the owner-side release. DriveBridge's current wave does not ship
	// an un-adopt command, so this best-effort request degrades to a warning;
	// a future DriveBridge release completes the release without Pinax changes.
	if _, _, releaseErr := runDrivebridgeJSON(ctx, "storage", "unadopt", "--consumer", drivebridgeConsumerPinax, "--space", record.Space); releaseErr != nil {
		projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{
			Code:    "drivebridge_release_unavailable",
			Message: "DriveBridge did not release the adopt record: " + releaseErr.Error() + ". The owner-side record is harmless (zero copy) and the storage backend is unchanged.",
		})
	}
	if err := removeDrivebridgeAttach(root); err != nil {
		return errorProjection("storage.detach_drivebridge", err), err
	}
	appendEventWarned(root, "storage.detach_drivebridge", "success", map[string]string{"space": record.Space})
	profile, err := loadStorageProfile(root)
	if err == nil {
		projection.Facts["backend"] = profile.Backend
	}
	projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax storage status --vault %s --json", shellQuote(root))}}
	return projection, nil
}

// BindWorkingCopy records the explicit opt-in for a plaintext OneDrive or
// Google Drive working copy. Nothing uploads; note add keeps writing locally.
func (s *Service) BindWorkingCopy(ctx context.Context, req DrivebridgeBindWorkingCopyRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("storage.bind_working_copy", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("storage.bind_working_copy", err), err
	}
	provider := strings.TrimSpace(req.Provider)
	if provider != "onedrive" && provider != "gdrive" {
		err := &domain.CommandError{Code: "argument_required", Message: "bind-working-copy requires --provider onedrive or --provider gdrive", Hint: "pinax storage bind-working-copy --provider onedrive --space <space> --vault <vault> --json"}
		return domain.NewErrorProjection("storage.bind_working_copy", err), err
	}
	space := strings.TrimSpace(req.Space)
	if space == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "bind-working-copy requires a DriveBridge space", Hint: "pinax storage bind-working-copy --provider " + provider + " --space <space> --vault <vault> --json"}
		return domain.NewErrorProjection("storage.bind_working_copy", err), err
	}
	if !drivebridgeInstalled() {
		err := &domain.CommandError{
			Code:    "drivebridge_not_installed",
			Message: "DriveBridge CLI was not found on PATH",
			Hint:    "Install DriveBridge and onboard the provider space (drivebridge auth --auto, then drivebridge setup)",
		}
		return domain.NewErrorProjection("storage.bind_working_copy", err), err
	}
	data, _, err := runDrivebridgeJSON(ctx, "space", "list")
	if err != nil {
		return errorProjection("storage.bind_working_copy", err), err
	}
	listed := drivebridgeSpaceListData{}
	if err := json.Unmarshal(data, &listed); err != nil {
		return errorProjection("storage.bind_working_copy", err), err
	}
	found := false
	for _, sp := range listed.Spaces {
		if sp.ID != space {
			continue
		}
		if sp.Backend != provider {
			err := &domain.CommandError{
				Code:    "drivebridge_location_mismatch",
				Message: "DriveBridge space " + space + " has backend " + sp.Backend + ", not " + provider,
				Hint:    "Bind a " + provider + " space, or onboard it with drivebridge auth --auto and drivebridge setup",
			}
			return domain.NewErrorProjection("storage.bind_working_copy", err), err
		}
		if sp.Adopt != nil && sp.Adopt.Consumer == drivebridgeConsumerPinax && (sp.Adopt.Kind == "local" || sp.Adopt.Kind == "s3") {
			err := &domain.CommandError{
				Code:    "drivebridge_location_mismatch",
				Message: "DriveBridge space " + space + " is already the adopted Pinax owner storage (" + sp.Adopt.Kind + ")",
				Hint:    "Use a separate provider space for the plaintext working copy",
			}
			return domain.NewErrorProjection("storage.bind_working_copy", err), err
		}
		found = true
		break
	}
	if !found {
		err := &domain.CommandError{
			Code:    "drivebridge_space_not_found",
			Message: "DriveBridge space " + space + " (" + provider + ") was not found",
			Hint:    "Onboard the provider with drivebridge auth --auto and drivebridge setup, then retry",
		}
		return domain.NewErrorProjection("storage.bind_working_copy", err), err
	}
	record := domain.DrivebridgeAttach{
		SchemaVersion:  drivebridgeAttachSchemaVersion,
		Consumer:       drivebridgeConsumerPinax,
		Space:          space,
		Kind:           provider,
		Purpose:        drivebridgePurposeVault,
		IdempotencyKey: "working-copy:pinax:" + provider + ":" + space,
		ContentMode:    drivebridgeContentModeProviderPlaintext,
		AttachedAt:     s.now().Format(time.RFC3339),
	}
	if err := saveDrivebridgeAttach(root, record); err != nil {
		return errorProjection("storage.bind_working_copy", err), err
	}
	appendEventWarned(root, "storage.bind_working_copy", "success", map[string]string{"space": space, "provider": provider, "content_mode": drivebridgeContentModeProviderPlaintext})
	projection := domain.NewProjection("storage.bind_working_copy", "已显式绑定网盘明文工作副本（opt-in）。")
	projection.Facts["drivebridge_attached"] = "true"
	projection.Facts["drivebridge_space"] = space
	projection.Facts["drivebridge_kind"] = provider
	projection.Facts["drivebridge_content_mode"] = drivebridgeContentModeProviderPlaintext
	projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{
		Code:    "drivebridge_provider_plaintext",
		Message: "The bound working copy stores note bytes in plaintext on " + provider + ". This is not Capsa encrypted sync and note add still writes locally first.",
	})
	projection.Data = map[string]any{"attach": record}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "drivebridge-attach.yaml"))}
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax storage doctor --vault %s --json", shellQuote(root))}}
	return projection, nil
}

// fileSHA256OrEmpty hashes a local file; missing files return "".
func fileSHA256OrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func drivebridgeStat(ctx context.Context, ref string) (drivebridgeFile, error) {
	data, _, err := runDrivebridgeJSON(ctx, "stat", "--ref", ref)
	if err != nil {
		return drivebridgeFile{}, err
	}
	file := drivebridgeFile{}
	if err := json.Unmarshal(data, &file); err != nil {
		return drivebridgeFile{}, &domain.CommandError{Code: "drivebridge_invocation_failed", Message: "DriveBridge stat payload was unreadable", Hint: "Upgrade DriveBridge and retry"}
	}
	return file, nil
}

// StorageHydrate rebuilds a working copy on a second device from the attached
// DriveBridge space, pinning observed file identity (ref/version/sha256), then
// refreshes the Pinax index. It never touches Capsa sync-state and never emits
// Capsa remote_write. Conflicts are handed to the existing repair flow.
func (s *Service) StorageHydrate(ctx context.Context, req DrivebridgeHydrateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("storage.hydrate", err), err
	}
	record, attached, err := loadDrivebridgeAttach(root)
	if err != nil {
		return errorProjection("storage.hydrate", err), err
	}
	if !attached {
		err := &domain.CommandError{
			Code:    "drivebridge_not_attached",
			Message: "This vault has no DriveBridge attach record to hydrate from",
			Hint:    "Run pinax storage attach-drivebridge --space <space> or pinax storage bind-working-copy first",
		}
		return domain.NewErrorProjection("storage.hydrate", err), err
	}
	if space := strings.TrimSpace(req.Space); space != "" && space != record.Space {
		err := &domain.CommandError{
			Code:    "drivebridge_location_mismatch",
			Message: "Requested space " + space + " does not match the attached space " + record.Space,
			Hint:    "Hydrate from the attached space, or detach and re-attach",
		}
		return domain.NewErrorProjection("storage.hydrate", err), err
	}
	if record.Kind == "local" {
		// 本机 local adopt 只在看得见该目录的机器上可管理；看不见就诚实失败，
		// 不把本机路径伪装成远端可读文件。
		if info, statErr := os.Stat(record.Location.Root); statErr != nil || !info.IsDir() {
			err := &domain.CommandError{
				Code:    "drivebridge_local_unreachable",
				Message: "The adopted local root " + record.Location.Root + " is not visible on this machine",
				Hint:    "Attach the same prefix as S3 (pinax storage set s3 + attach-drivebridge) or bind an explicit working copy; DriveBridge cannot read a client disk remotely",
			}
			return domain.NewErrorProjection("storage.hydrate", err), err
		}
	}
	if !drivebridgeInstalled() {
		err := &domain.CommandError{
			Code:    "drivebridge_not_installed",
			Message: "DriveBridge CLI was not found on PATH",
			Hint:    "Install DriveBridge first; hydrate transfers bytes through the DriveBridge file plane",
		}
		return domain.NewErrorProjection("storage.hydrate", err), err
	}
	listData, _, err := runDrivebridgeJSON(ctx, "ls", "--space", record.Space)
	if err != nil {
		return errorProjection("storage.hydrate", err), err
	}
	listed := drivebridgeListData{}
	if err := json.Unmarshal(listData, &listed); err != nil {
		return errorProjection("storage.hydrate", err), err
	}
	projection := domain.NewProjection("storage.hydrate", "已从 DriveBridge 重建工作副本。")
	projection.Facts["drivebridge_space"] = record.Space
	projection.Facts["drivebridge_kind"] = record.Kind
	downloaded, unchanged, conflicts, failures := 0, 0, 0, 0
	skipProtected := 0
	for _, file := range listed.Files {
		if file.Directory {
			continue
		}
		rel := path.Join(filepath.ToSlash(file.Dir), file.Name)
		// 远端清单可能来自共享空间或损坏状态：hydrate 只接受落在 vault
		// 根内的相对路径；绝对路径、.. 越界（如 Dir="../outside"）与
		// Windows 保留名一律跳过，绝不把字节写到根外。
		if !filepath.IsLocal(filepath.FromSlash(rel)) {
			continue
		}
		// .pinax/** 是 CLI-authored owner 元数据；hydrate 只重建用户工作副本，
		// 索引与配置由本机 Pinax 重新生成，不从文件面回放。
		if rel == ".pinax" || strings.HasPrefix(rel, ".pinax/") {
			skipProtected++
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(rel))
		if file.SHA256 != "" && fileSHA256OrEmpty(target) == file.SHA256 {
			unchanged++
			continue
		}
		if _, statErr := os.Stat(target); statErr == nil {
			// 本地存在未提交编辑：不覆盖，交给既有 repair/conflicts 流程。
			conflicts++
			projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{
				Code:    "hydrate_local_conflict",
				Message: rel + " has uncommitted local edits; DriveBridge never merges content automatically",
			})
			continue
		}
		// 钉住观察到的文件身份：传输后再次核对 version/sha256，变化则失败，
		// 不静默接受新字节。
		if _, _, err := runDrivebridgeJSON(ctx, "download", "--ref", file.Ref, "--out", target); err != nil {
			failures++
			projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{
				Code:    "hydrate_transfer_failed",
				Message: rel + ": " + err.Error(),
			})
			continue
		}
		after, statErr := drivebridgeStat(ctx, file.Ref)
		if statErr != nil || after.Version != file.Version || (file.SHA256 != "" && after.SHA256 != file.SHA256) || (file.SHA256 != "" && fileSHA256OrEmpty(target) != file.SHA256) {
			_ = os.Remove(target)
			failures++
			projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{
				Code:    "file_version_changed",
				Message: rel + " changed while hydrating; the pinned version is kept and local edits are untouched",
			})
			continue
		}
		downloaded++
	}
	projection.Facts["files_listed"] = fmt.Sprint(len(listed.Files))
	projection.Facts["files_downloaded"] = fmt.Sprint(downloaded)
	projection.Facts["files_unchanged"] = fmt.Sprint(unchanged)
	projection.Facts["files_conflict"] = fmt.Sprint(conflicts)
	projection.Facts["files_failed"] = fmt.Sprint(failures)
	projection.Facts["protected_skipped"] = fmt.Sprint(skipProtected)
	projection.Facts["remote_write"] = "false"
	if downloaded > 0 || unchanged > 0 {
		// 工作副本落地后由 Pinax 重建索引；笔记 id 仍由 Pinax 管理。
		if _, refreshErr := s.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); refreshErr != nil {
			projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{Code: "index_refresh_failed", Message: refreshErr.Error()})
		} else {
			projection.Facts["index_refreshed"] = "true"
		}
	}
	if conflicts > 0 || failures > 0 {
		projection.Status = "partial"
		projection.Actions = []domain.Action{
			{Name: "repair", Command: fmt.Sprintf("pinax repair plan --vault %s --save", shellQuote(root))},
			{Name: "conflicts", Command: fmt.Sprintf("pinax sync conflicts --vault %s --json", shellQuote(root))},
		}
	} else {
		projection.Actions = []domain.Action{{Name: "validate", Command: fmt.Sprintf("pinax vault validate --vault %s --json", shellQuote(root))}}
	}
	projection.Data = map[string]any{
		"space": record.Space, "kind": record.Kind,
		"downloaded": downloaded, "unchanged": unchanged, "conflicts": conflicts, "failures": failures,
		"protected_skipped": skipProtected, "capsa_remote_write": false,
	}
	return projection, nil
}

// capsaSyncConfigured reports whether a Capsa backend is configured for the vault.
func capsaSyncConfigured(root string) bool {
	return pinaxcloud.Doctor(root).Configured
}

// remoteAPIConfigured reports whether Remote API Mode is configured through
// PINAX_API_URL or remote.api_url in the user/project config.
func remoteAPIConfigured(root string) bool {
	if strings.TrimSpace(os.Getenv("PINAX_API_URL")) != "" {
		return true
	}
	paths := pinaxconfig.ResolvePaths(pinaxconfig.PathOptions{VaultPath: root})
	result, err := pinaxconfig.Load(pinaxconfig.LoadOptions{VaultPath: root, UserConfigPath: paths.User, ProjectConfigPath: paths.Project})
	if err != nil {
		return false
	}
	return strings.TrimSpace(result.Config.Remote.APIURL) != ""
}

// drivebridgeAttachmentFacts computes the additive English facts for storage
// status/doctor and vault doctor. Absent facts mean "not attached"; the
// content mode is always reported (none when unattached).
func drivebridgeAttachmentFacts(root string) (map[string]string, domain.DrivebridgeAttach, bool) {
	facts := map[string]string{
		"capsa_sync_configured": fmt.Sprint(capsaSyncConfigured(root)),
		"remote_api_configured": fmt.Sprint(remoteAPIConfigured(root)),
	}
	record, attached, err := loadDrivebridgeAttach(root)
	if err != nil || !attached {
		facts["drivebridge_attached"] = "false"
		facts["drivebridge_content_mode"] = drivebridgeContentModeNone
		return facts, domain.DrivebridgeAttach{}, false
	}
	facts["drivebridge_attached"] = "true"
	facts["drivebridge_space"] = record.Space
	facts["drivebridge_kind"] = record.Kind
	facts["drivebridge_content_mode"] = record.ContentMode
	switch record.ContentMode {
	case drivebridgeContentModeProviderPlaintext:
		facts["drivebridge_location_match"] = "true"
	default:
		profile, profErr := loadStorageProfile(root)
		match := false
		if profErr == nil {
			_, want, _, targetErr := attachTargetLocation(root, profile)
			if targetErr == nil {
				match = drivebridgeLocationString(want) == drivebridgeLocationString(record.Location)
			}
		}
		facts["drivebridge_location_match"] = fmt.Sprint(match)
		// Capsa 密文前缀检测：attach 的 S3 位置与 Capsa 后端是同一
		// bucket/prefix 时，对象是 Capsa 不透明密文，不是明文笔记。
		if record.Kind == "s3" {
			if state, stateErr := pinaxcloud.Load(root); stateErr == nil && state.Config.S3 != nil {
				capsaLoc := strings.Trim(state.Config.S3.Bucket+"/"+strings.Trim(filepath.ToSlash(state.Config.S3.Prefix), "/"), "/")
				if capsaLoc != "" && capsaLoc == drivebridgeLocationString(record.Location) {
					facts["drivebridge_content_mode"] = drivebridgeContentModeOpaqueEncrypted
				}
			}
		}
	}
	return facts, record, attached
}

// ParseDrivebridgeRef parses the pinned reference shape
// drivebridge://<space>/<file-id>@<version> shared with external consumers.
func ParseDrivebridgeRef(raw string) (space, fileID, version string, err error) {
	u, parseErr := url.Parse(strings.TrimSpace(raw))
	if parseErr != nil || u.Scheme != "drivebridge" || u.Host == "" {
		return "", "", "", &domain.CommandError{Code: "invalid_drivebridge_ref", Message: "DriveBridge references must look like drivebridge://<space>/<file-id>@<version>", Hint: "Pin the reference with drivebridge stat/pin and register the full URI"}
	}
	rest := strings.Trim(u.Path, "/")
	fileID, version, ok := strings.Cut(rest, "@")
	if !ok || fileID == "" || version == "" {
		return "", "", "", &domain.CommandError{Code: "invalid_drivebridge_ref", Message: "DriveBridge references must look like drivebridge://<space>/<file-id>@<version>", Hint: "The file id and pinned version are both required"}
	}
	return u.Host, fileID, version, nil
}

// drivebridgeAttachmentFactsOnly returns just the additive facts map for
// status/vault-doctor projections that do not need the attach record itself.
func drivebridgeAttachmentFactsOnly(root string) map[string]string {
	facts, _, _ := drivebridgeAttachmentFacts(root)
	return facts
}

// ConsumeDrivebridgeAsset resolves an asset's pinned drivebridge reference to
// bytes. It fails with file_version_changed when the source changed after
// pinning and never touches already-landed vault copies on failure. After the
// attach record is detached, new consumes fail; landed copies stay usable.
func (s *Service) ConsumeDrivebridgeAsset(ctx context.Context, req AssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.consume_drivebridge", err), err
	}
	asset, _, err := noteindex.FindAsset(root, req.Ref)
	if err != nil {
		asset, err = pinaxassets.Find(root, req.Ref)
		if err != nil {
			notFound := &domain.CommandError{Code: domain.ErrorCodeAssetNotFound, Message: "Asset not found", Hint: fmt.Sprintf("pinax asset list --vault %s --json", shellQuote(root))}
			return domain.NewErrorProjection("asset.consume_drivebridge", notFound), notFound
		}
	}
	pinned := asset.Drivebridge
	if pinned == nil {
		err := &domain.CommandError{Code: "drivebridge_ref_missing", Message: "Asset has no pinned DriveBridge reference", Hint: "Register one with pinax asset add <file> --register --drivebridge-ref drivebridge://<space>/<file-id>@<version>"}
		return domain.NewErrorProjection("asset.consume_drivebridge", err), err
	}
	if _, attached, attachErr := loadDrivebridgeAttach(root); attachErr != nil || !attached {
		// detach/撤销后新的跨项目读取必须失败；已落地副本不受影响。
		err := &domain.CommandError{Code: "drivebridge_not_attached", Message: "This vault has no active DriveBridge attach record; new cross-project reads are refused", Hint: "Re-attach with pinax storage attach-drivebridge; landed vault copies remain usable offline"}
		return domain.NewErrorProjection("asset.consume_drivebridge", err), err
	}
	if !drivebridgeInstalled() {
		err := &domain.CommandError{Code: "drivebridge_not_installed", Message: "DriveBridge CLI was not found on PATH", Hint: "Install DriveBridge first; consuming a pinned reference goes through the DriveBridge file plane"}
		return domain.NewErrorProjection("asset.consume_drivebridge", err), err
	}
	current, err := drivebridgeStat(ctx, pinned.FileID)
	if err != nil {
		return errorProjection("asset.consume_drivebridge", err), err
	}
	if current.Version != pinned.Version || (pinned.SHA256 != "" && current.SHA256 != pinned.SHA256) {
		err := &domain.CommandError{Code: "file_version_changed", Message: "DriveBridge file " + pinned.FileID + " changed after pinning (pinned version " + pinned.Version + ", current " + current.Version + ")", Hint: "Re-pin the new version explicitly; landed vault copies were not modified"}
		return domain.NewErrorProjection("asset.consume_drivebridge", err), err
	}
	target := filepath.Join(root, filepath.FromSlash(asset.Path))
	landed := false
	if _, statErr := os.Stat(target); statErr == nil {
		// 已落地副本只读校验：哈希不符不覆盖，交给 asset repair。
		if pinned.SHA256 != "" && fileSHA256OrEmpty(target) != pinned.SHA256 {
			err := &domain.CommandError{Code: "asset_landed_conflict", Message: "Landed vault copy of " + asset.Path + " differs from the pinned version; refusing to overwrite", Hint: "Run pinax asset repair-plan --vault <vault> to reconcile the landed copy"}
			return domain.NewErrorProjection("asset.consume_drivebridge", err), err
		}
		landed = true
	} else {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errorProjection("asset.consume_drivebridge", err), err
		}
		if _, _, err := runDrivebridgeJSON(ctx, "download", "--ref", pinned.FileID, "--out", target); err != nil {
			return errorProjection("asset.consume_drivebridge", err), err
		}
		if pinned.SHA256 != "" && fileSHA256OrEmpty(target) != pinned.SHA256 {
			_ = os.Remove(target)
			err := &domain.CommandError{Code: "file_version_changed", Message: "Downloaded bytes for " + asset.Path + " do not match the pinned sha256", Hint: "Re-pin the reference; no partial copy was kept"}
			return domain.NewErrorProjection("asset.consume_drivebridge", err), err
		}
		landed = true
	}
	appendEventWarned(root, "asset.consume_drivebridge", "success", map[string]string{"asset": asset.ID, "file_id": pinned.FileID, "version": pinned.Version})
	projection := domain.NewProjection("asset.consume_drivebridge", "DriveBridge 引用已校验并落地。")
	projection.Facts["asset"] = asset.ID
	projection.Facts["drivebridge_file_id"] = pinned.FileID
	projection.Facts["version"] = pinned.Version
	projection.Facts["landed"] = fmt.Sprint(landed)
	projection.Data = map[string]any{"asset": asset.ID, "drivebridge": pinned}
	projection.Evidence = []string{asset.Path}
	return projection, nil
}
