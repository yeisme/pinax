package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/semantic"
)

const KBGenerationFailureSchema = "pinax.kb.generation-failure.v1"

type KBGenerationFailure struct {
	SchemaVersion string `json:"schema_version"`
	GenerationID  string `json:"generation_id"`
	Stage         string `json:"stage"`
	Code          string `json:"code"`
	CreatedAt     string `json:"created_at"`
}

func (s *Service) kbRebuildStaged(ctx context.Context, req KBIndexRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.rebuild", err), err
	}
	backend := strings.TrimSpace(req.Backend)
	if backend == "" {
		backend = semantic.DefaultBackend
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("kb.rebuild", err), err
	}
	sourceBytes := int64(0)
	for _, note := range notes {
		sourceBytes += int64(len(note.Body))
	}
	sourceDigest := kbSourceDigest(notes)
	sourceSnapshot := kbSourceSnapshot(sourceDigest)
	generationID := strings.TrimSpace(req.GenerationID)
	if generationID == "" {
		generationID = newKBGenerationID(sourceDigest)
	}
	if !validKBToken(generationID) {
		err := &domain.CommandError{Code: "kb_generation_id_invalid", Message: "KB generation id is invalid", Hint: "Use a short URL-safe generation id"}
		return domain.NewErrorProjection("kb.rebuild", err), err
	}
	if _, err := os.Stat(kbGenerationManifestPath(root, generationID)); err == nil {
		err := &domain.CommandError{Code: "kb_generation_immutable", Message: "KB generation already exists", Hint: "Use a new generation id instead of replacing an existing candidate"}
		return domain.NewErrorProjection("kb.rebuild", err), err
	}
	var diskAdmission KBDiskAdmission
	if req.DiskAdmission != nil {
		diskAdmission, err = CheckKBDiskAdmission(root, sourceBytes, *req.DiskAdmission)
		if err != nil {
			_ = writeKBGenerationFailure(root, generationID, "disk_admission", commandErrorCode(err))
			return commandErrorProjection("kb.rebuild", err)
		}
	}
	provider, err := semantic.NewProviderForBackend(backend, req.Provider, req.Model)
	if err != nil {
		return commandErrorProjection("kb.rebuild", err)
	}
	identity, err := semantic.InspectProviderIdentity(ctx, provider.Name(), provider.Model())
	if err != nil {
		_ = writeKBGenerationFailure(root, generationID, "identity", commandErrorCode(err))
		return commandErrorProjection("kb.rebuild", err)
	}
	canary, err := provider.Embed(ctx, "pinax generation dimension canary")
	if err != nil {
		_ = writeKBGenerationFailure(root, generationID, "embedding", commandErrorCode(err))
		return commandErrorProjection("kb.rebuild", err)
	}
	dimension := len(canary)
	if dimension == 0 {
		err := &domain.CommandError{Code: "embedding_dimension_invalid", Message: "Embedding provider returned an empty vector", Hint: "Use a provider/model that returns non-empty fixed-dimension embeddings"}
		_ = writeKBGenerationFailure(root, generationID, "embedding", err.Code)
		return domain.NewErrorProjection("kb.rebuild", err), err
	}
	chunks, err := semantic.BuildChunks(ctx, notes, provider, backend)
	if err != nil {
		_ = writeKBGenerationFailure(root, generationID, "embedding", commandErrorCode(err))
		if cmdErr, ok := err.(*domain.CommandError); ok {
			return domain.NewErrorProjection("kb.rebuild", cmdErr), cmdErr
		}
		cmdErr := &domain.CommandError{Code: "embedding_provider_failed", Message: "Embedding provider failed", Hint: "Check provider credentials or use --provider fake for local validation"}
		return domain.NewErrorProjection("kb.rebuild", cmdErr), cmdErr
	}
	if err := validateStagedChunks(chunks, dimension); err != nil {
		_ = writeKBGenerationFailure(root, generationID, "validating", commandErrorCode(err))
		return commandErrorProjection("kb.rebuild", err)
	}
	if len(chunks) == 0 {
		// Preserve the stable actionable sidecar error for an empty vault when
		// LanceDB is selected. A missing sidecar is more useful than reporting
		// an empty generation before the operator has a backend to run.
		if backend == semantic.DefaultBackend {
			if _, doctorErr := semantic.Doctor(ctx, root, backend, semantic.SidecarConfig{Executable: req.SidecarExecutable, Timeout: req.SidecarTimeout}); doctorErr != nil {
				_ = writeKBGenerationFailure(root, generationID, "indexing", commandErrorCode(doctorErr))
				if cmdErr, ok := doctorErr.(*domain.CommandError); ok {
					return domain.NewErrorProjection("kb.rebuild", cmdErr), cmdErr
				}
				return errorProjection("kb.rebuild", doctorErr), doctorErr
			}
		}
		err := &domain.CommandError{Code: "kb_generation_empty", Message: "KB generation contains no indexable chunks", Hint: "Add Markdown notes before rebuilding the local semantic projection"}
		_ = writeKBGenerationFailure(root, generationID, "validating", err.Code)
		return domain.NewErrorProjection("kb.rebuild", err), err
	}
	storeURI := filepath.Join(root, ".pinax", "kb", "generations", generationID, "lancedb")
	_, err = semantic.Save(ctx, root, chunks, backend, semantic.SidecarConfig{Executable: req.SidecarExecutable, Timeout: req.SidecarTimeout, StoreURI: storeURI}, len(notes))
	if err != nil {
		_ = writeKBGenerationFailure(root, generationID, "indexing", commandErrorCode(err))
		if cmdErr, ok := err.(*domain.CommandError); ok {
			return domain.NewErrorProjection("kb.rebuild", cmdErr), cmdErr
		}
		return errorProjection("kb.rebuild", err), err
	}
	latestNotes, err := scanNotes(root)
	if err != nil {
		_ = writeKBGenerationFailure(root, generationID, "validating", "kb_source_scan_failed")
		return errorProjection("kb.rebuild", err), err
	}
	if latestDigest := kbSourceDigest(latestNotes); latestDigest != sourceDigest {
		err := &domain.CommandError{Code: "kb_source_drift", Message: "KB source changed during generation staging", Hint: "Retry rebuild after the Markdown source stops changing"}
		_ = writeKBGenerationFailure(root, generationID, "validating", err.Code)
		return domain.NewErrorProjection("kb.rebuild", err), err
	}
	manifest := KBGenerationManifest{
		SchemaVersion:       KBGenerationManifestSchema,
		GenerationID:        generationID,
		Status:              KBGenerationStatusReady,
		Protocol:            semantic.SidecarSchema,
		Backend:             backend,
		Provider:            provider.Name(),
		Model:               provider.Model(),
		BaseModelDigest:     identity.BaseModelDigest,
		ModelManifestDigest: identity.ModelManifestDigest,
		ProfileHash:         identity.ProfileHash,
		DaemonVersion:       identity.DaemonVersion,
		SourceSnapshot:      sourceSnapshot,
		SourceDigest:        sourceDigest,
		EmbeddingDim:        dimension,
		Documents:           len(notes),
		Chunks:              len(chunks),
		RowCount:            len(chunks),
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
	}
	if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
		_ = writeKBGenerationFailure(root, generationID, "manifest", commandErrorCode(err))
		return commandErrorProjection("kb.rebuild", err)
	}
	storeRel := filepath.ToSlash(filepath.Join(".pinax", "kb", "generations", generationID, "lancedb"))
	projection := domain.NewProjection("kb.rebuild", "KB semantic candidate generation staged.")
	projection.Facts["backend"] = backend
	projection.Facts["protocol"] = manifest.Protocol
	projection.Facts["provider"] = manifest.Provider
	projection.Facts["model"] = manifest.Model
	projection.Facts["model_manifest_digest"] = manifest.ModelManifestDigest
	projection.Facts["profile_hash"] = manifest.ProfileHash
	projection.Facts["daemon_version"] = manifest.DaemonVersion
	if manifest.BaseModelDigest != "" {
		projection.Facts["base_model_digest"] = manifest.BaseModelDigest
	}
	projection.Facts["embedding_dim"] = fmt.Sprint(manifest.EmbeddingDim)
	projection.Facts["documents"] = fmt.Sprint(manifest.Documents)
	projection.Facts["chunks"] = fmt.Sprint(manifest.Chunks)
	projection.Facts["row_count"] = fmt.Sprint(manifest.RowCount)
	projection.Facts["generation_id"] = manifest.GenerationID
	projection.Facts["generation_status"] = string(manifest.Status)
	projection.Facts["source_snapshot"] = manifest.SourceSnapshot
	projection.Facts["source_digest"] = manifest.SourceDigest
	projection.Facts["sync_vectors"] = "false"
	if req.DiskAdmission != nil {
		projection.Facts["disk_used_percent"] = fmt.Sprintf("%.2f", diskAdmission.UsedPercent)
		projection.Facts["disk_free_bytes"] = fmt.Sprint(diskAdmission.FreeBytes)
		projection.Facts["disk_high_water"] = fmt.Sprint(diskAdmission.HighWater)
		projection.Facts["disk_high_water_override"] = fmt.Sprint(diskAdmission.Override)
	}
	projection.Evidence = []string{storeRel, filepath.ToSlash(filepath.Join(".pinax", "kb", "generations", generationID, "generation.json"))}
	projectionData := map[string]any{
		"documents":             manifest.Documents,
		"chunks":                manifest.Chunks,
		"row_count":             manifest.RowCount,
		"backend":               manifest.Backend,
		"protocol":              manifest.Protocol,
		"provider":              manifest.Provider,
		"model":                 manifest.Model,
		"base_model_digest":     manifest.BaseModelDigest,
		"model_manifest_digest": manifest.ModelManifestDigest,
		"profile_hash":          manifest.ProfileHash,
		"daemon_version":        manifest.DaemonVersion,
		"embedding_dim":         manifest.EmbeddingDim,
		"generation_id":         manifest.GenerationID,
		"generation_status":     string(manifest.Status),
		"source_snapshot":       manifest.SourceSnapshot,
		"source_digest":         manifest.SourceDigest,
		"store_path":            storeRel,
	}
	if req.DiskAdmission != nil {
		projectionData["disk_admission"] = diskAdmission
	}
	projection.Data = projectionData
	return projection, nil
}

func validateStagedChunks(chunks []semantic.Chunk, dimension int) error {
	if dimension <= 0 {
		return &domain.CommandError{Code: "embedding_dimension_invalid", Message: "Embedding dimension is invalid", Hint: "Use one exact provider/model with a non-empty fixed dimension"}
	}
	seen := make(map[string]struct{}, len(chunks))
	for _, chunk := range chunks {
		if chunk.ChunkID == "" || chunk.EmbeddingDim != dimension || len(chunk.Vector) != dimension || !validKBRelativeCitation(chunk.VaultPath) || strings.TrimSpace(chunk.Preview) == "" {
			return &domain.CommandError{Code: "kb_generation_validation_failed", Message: "KB staged chunk validation failed", Hint: "Check chunk ids, dimensions, relative citations, and bounded previews"}
		}
		for _, value := range chunk.Vector {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return &domain.CommandError{Code: "kb_generation_validation_failed", Message: "KB staged chunk vector contains a non-finite value", Hint: "Use a provider that returns finite numeric embeddings"}
			}
		}
		if _, exists := seen[chunk.ChunkID]; exists {
			return &domain.CommandError{Code: "kb_generation_validation_failed", Message: "KB staged chunk ids are not unique", Hint: "Rebuild after checking source paths and chunking"}
		}
		seen[chunk.ChunkID] = struct{}{}
	}
	return nil
}

func validKBRelativeCitation(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsRune(raw, '\x00') {
		return false
	}
	normalized := strings.ReplaceAll(raw, `\`, "/")
	if filepath.IsAbs(raw) || path.IsAbs(normalized) || strings.Contains(normalized, ":") || normalized == "." {
		return false
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}

func kbSourceDigest(notes []domain.Note) string {
	hash := sha256.New()
	for _, note := range notes {
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\x00", note.ID, note.Path, note.UpdatedAt, note.Body)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func kbSourceSnapshot(sourceDigest string) string {
	clean := strings.TrimPrefix(sourceDigest, "sha256:")
	if len(clean) > 16 {
		clean = clean[:16]
	}
	return "snapshot-" + clean
}

func newKBGenerationID(sourceDigest string) string {
	clean := strings.TrimPrefix(sourceDigest, "sha256:")
	if len(clean) > 12 {
		clean = clean[:12]
	}
	return "gen-" + time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + clean
}

func commandErrorCode(err error) string {
	if cmdErr, ok := err.(*domain.CommandError); ok && cmdErr.Code != "" {
		return cmdErr.Code
	}
	return "kb_generation_failed"
}

func writeKBGenerationFailure(root, generationID, stage, code string) error {
	if !validKBToken(generationID) {
		return nil
	}
	failure := KBGenerationFailure{SchemaVersion: KBGenerationFailureSchema, GenerationID: generationID, Stage: stage, Code: code, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	return atomicKBJSONWrite(filepath.Join(kbRoot(root), "generations", generationID, "failure.json"), failure)
}

func resolveKBGenerationForSearch(root string, req KBIndexRequest) (*KBGenerationManifest, string, error) {
	if strings.TrimSpace(req.StoreURI) != "" {
		return nil, filepath.Clean(req.StoreURI), nil
	}
	generationID := strings.TrimSpace(req.GenerationID)
	if generationID == "" {
		descriptor, err := ReadKBActivationDescriptor(root)
		if err != nil {
			return nil, "", err
		}
		if descriptor.Active == nil {
			return nil, "", &domain.CommandError{Code: "kb_generation_unavailable", Message: "KB has no active generation", Hint: "Run pinax kb evaluate --suite <suite> --generation <candidate-id> before activation"}
		}
		generationID = descriptor.Active.GenerationID
	}
	manifest, err := ReadKBGenerationManifest(root, generationID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", &domain.CommandError{Code: "kb_generation_unavailable", Message: "Pinned KB generation is unavailable", Hint: "Rebuild the candidate or refresh the activation descriptor"}
		}
		return nil, "", err
	}
	if manifest.Status != KBGenerationStatusReady && manifest.Status != KBGenerationStatusActive {
		return nil, "", &domain.CommandError{Code: "kb_generation_unavailable", Message: "Pinned KB generation is not searchable", Hint: "Use a ready candidate or activate a passed generation"}
	}
	return &manifest, filepath.Join(root, ".pinax", "kb", "generations", generationID, "lancedb"), nil
}
