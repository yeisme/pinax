package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	// KBDefaultDiskHighWaterPercent is intentionally conservative for the
	// current server: a rebuild creates a second projection before activation.
	KBDefaultDiskHighWaterPercent  = 90
	KBDefaultGenerationBudgetBytes = int64(2 * 1024 * 1024 * 1024)
	KBDefaultEvidenceBudgetBytes   = int64(2 * 1024 * 1024 * 1024)
	KBDefaultModelCacheBudgetBytes = int64(4 * 1024 * 1024 * 1024)
)

type KBDiskAdmissionPolicy struct {
	HighWaterPercent      int
	GenerationBudgetBytes int64
	EvidenceBudgetBytes   int64
	ModelCacheBudgetBytes int64
	ModelCachePath        string
	AllowHighWater        bool
}

type KBDiskAdmission struct {
	UsedPercent        float64 `json:"used_percent"`
	FreeBytes          uint64  `json:"free_bytes"`
	TotalBytes         uint64  `json:"total_bytes"`
	HighWater          bool    `json:"high_water"`
	Override           bool    `json:"override"`
	GenerationBudget   int64   `json:"generation_budget_bytes"`
	EvidenceBudget     int64   `json:"evidence_budget_bytes"`
	ModelCacheBudget   int64   `json:"model_cache_budget_bytes"`
	GenerationBytes    uint64  `json:"generation_bytes"`
	EvidenceBytes      uint64  `json:"evidence_bytes"`
	ModelCacheBytes    uint64  `json:"model_cache_bytes,omitempty"`
	ModelCacheObserved bool    `json:"model_cache_observed"`
}

type kbFilesystemStats struct {
	Blocks     uint64
	FreeBlocks uint64
	BlockSize  uint64
}

var kbStatfs = func(path string) (kbFilesystemStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return kbFilesystemStats{}, err
	}
	return kbFilesystemStats{
		Blocks:     uint64(stat.Blocks),
		FreeBlocks: uint64(stat.Bavail),
		BlockSize:  uint64(stat.Bsize),
	}, nil
}

func DefaultKBDiskAdmissionPolicy() KBDiskAdmissionPolicy {
	return KBDiskAdmissionPolicy{
		HighWaterPercent:      KBDefaultDiskHighWaterPercent,
		GenerationBudgetBytes: KBDefaultGenerationBudgetBytes,
		EvidenceBudgetBytes:   KBDefaultEvidenceBudgetBytes,
		ModelCacheBudgetBytes: KBDefaultModelCacheBudgetBytes,
	}
}

func (p KBDiskAdmissionPolicy) normalize() KBDiskAdmissionPolicy {
	if p.HighWaterPercent <= 0 || p.HighWaterPercent >= 100 {
		p.HighWaterPercent = KBDefaultDiskHighWaterPercent
	}
	if p.GenerationBudgetBytes <= 0 {
		p.GenerationBudgetBytes = KBDefaultGenerationBudgetBytes
	}
	if p.EvidenceBudgetBytes <= 0 {
		p.EvidenceBudgetBytes = KBDefaultEvidenceBudgetBytes
	}
	if p.ModelCacheBudgetBytes <= 0 {
		p.ModelCacheBudgetBytes = KBDefaultModelCacheBudgetBytes
	}
	return p
}

func CheckKBDiskAdmission(root string, sourceBytes int64, policy KBDiskAdmissionPolicy) (KBDiskAdmission, error) {
	policy = policy.normalize()
	stats, err := kbStatfs(root)
	if err != nil || stats.Blocks == 0 || stats.BlockSize == 0 {
		return KBDiskAdmission{}, &domain.CommandError{Code: "kb_disk_stat_failed", Message: "Unable to inspect KB disk capacity", Hint: "Check the vault filesystem before rebuilding; no generation was started"}
	}
	total := stats.Blocks * stats.BlockSize
	free := stats.FreeBlocks * stats.BlockSize
	used := total - minUint64(total, free)
	usedPercent := (float64(used) / float64(total)) * 100
	highWater := usedPercent >= float64(policy.HighWaterPercent)
	result := KBDiskAdmission{
		UsedPercent:      math.Round(usedPercent*100) / 100,
		FreeBytes:        free,
		TotalBytes:       total,
		HighWater:        highWater,
		Override:         highWater && policy.AllowHighWater,
		GenerationBudget: policy.GenerationBudgetBytes,
		EvidenceBudget:   policy.EvidenceBudgetBytes,
		ModelCacheBudget: policy.ModelCacheBudgetBytes,
	}
	generationBytes, err := kbDirectoryBytes(filepath.Join(root, ".pinax", "kb", "generations"))
	if err != nil {
		return KBDiskAdmission{}, &domain.CommandError{Code: "kb_generation_usage_failed", Message: "Unable to inspect KB generation usage", Hint: "Check the local KB projection directory before rebuilding"}
	}
	evidenceBytes, err := kbDirectoryBytes(filepath.Join(root, ".pinax", "kb", "evaluations"))
	if err != nil {
		return KBDiskAdmission{}, &domain.CommandError{Code: "kb_evidence_usage_failed", Message: "Unable to inspect KB evidence usage", Hint: "Check the local KB evaluation directory before rebuilding"}
	}
	activationReceiptBytes, err := kbDirectoryBytes(filepath.Join(root, ".pinax", "kb", "activation-receipts"))
	if err != nil {
		return KBDiskAdmission{}, &domain.CommandError{Code: "kb_evidence_usage_failed", Message: "Unable to inspect KB evidence usage", Hint: "Check the local KB activation receipt directory before rebuilding"}
	}
	evidenceBytes += activationReceiptBytes
	result.GenerationBytes = generationBytes
	result.EvidenceBytes = evidenceBytes
	if strings.TrimSpace(policy.ModelCachePath) != "" {
		result.ModelCacheObserved = true
		modelCacheBytes, usageErr := kbDirectoryBytes(policy.ModelCachePath)
		if usageErr != nil {
			return KBDiskAdmission{}, &domain.CommandError{Code: "kb_model_cache_usage_failed", Message: "Unable to inspect the configured embedding model cache", Hint: "Check OLLAMA_MODELS or remove the cache path from the KB disk policy"}
		}
		result.ModelCacheBytes = modelCacheBytes
	}
	if generationBytes > uint64(policy.GenerationBudgetBytes) || (sourceBytes > 0 && uint64(sourceBytes) > uint64(policy.GenerationBudgetBytes)-minUint64(generationBytes, uint64(policy.GenerationBudgetBytes))) {
		return result, &domain.CommandError{Code: "kb_generation_budget_exceeded", Message: "KB generation storage exceeds its configured budget", Hint: "Prune old candidates or raise the local generation budget before rebuilding"}
	}
	if evidenceBytes > uint64(policy.EvidenceBudgetBytes) {
		return result, &domain.CommandError{Code: "kb_evidence_budget_exceeded", Message: "KB evaluation evidence exceeds its configured budget", Hint: "Prune old evaluation receipts before rebuilding or evaluating"}
	}
	if result.ModelCacheObserved && result.ModelCacheBytes > uint64(policy.ModelCacheBudgetBytes) {
		return result, &domain.CommandError{Code: "kb_model_cache_budget_exceeded", Message: "Embedding model cache exceeds its configured budget", Hint: "Remove unused local Ollama models or raise the configured model cache budget"}
	}
	if highWater && !policy.AllowHighWater {
		return result, &domain.CommandError{
			Code:    "kb_disk_high_water",
			Message: fmt.Sprintf("KB rebuild refused at %.2f%% disk usage", result.UsedPercent),
			Hint:    "Free disk space or rerun with --allow-disk-high-water after confirming the bounded generation budget",
		}
	}
	// A source-size check prevents a tiny free filesystem from entering a
	// staging operation that cannot finish. It is deliberately conservative and
	// does not claim to predict LanceDB's exact footprint.
	if sourceBytes > 0 && uint64(sourceBytes) > free && !policy.AllowHighWater {
		return result, &domain.CommandError{Code: "kb_disk_budget_exceeded", Message: "KB source exceeds currently free disk capacity", Hint: "Free disk space before rebuilding the semantic projection"}
	}
	return result, nil
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func kbDirectoryBytes(root string) (uint64, error) {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return uint64(info.Size()), nil
	}
	var total uint64
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		fileInfo, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		size := uint64(fileInfo.Size())
		if ^uint64(0)-total < size {
			total = ^uint64(0)
			return filepath.SkipDir
		}
		total += size
		return nil
	})
	return total, err
}
