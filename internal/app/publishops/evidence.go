package publishops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/redaction"
)

const PublishEvidenceSchemaVersion = "pinax.publish_evidence.v1"

type PublishEvidenceSurface struct {
	Name string
	Text string
	Root string
}

type PublishEvidenceSurfaceReport struct {
	Name         string                      `json:"name"`
	Kind         string                      `json:"kind"`
	FilesScanned int                         `json:"files_scanned,omitempty"`
	Findings     []domain.PublishScanFinding `json:"findings,omitempty"`
}

type PublishEvidenceReport struct {
	SchemaVersion string                         `json:"schema_version"`
	Surfaces      []PublishEvidenceSurfaceReport `json:"surfaces"`
	FindingsCount int                            `json:"findings_count"`
}

func WriteRedactedEvidence(path string, surfaces []PublishEvidenceSurface) (PublishEvidenceReport, error) {
	report := PublishEvidenceReport{SchemaVersion: PublishEvidenceSchemaVersion, Surfaces: make([]PublishEvidenceSurfaceReport, 0, len(surfaces))}
	for _, surface := range surfaces {
		surfaceReport, err := scanEvidenceSurface(surface)
		if err != nil {
			return PublishEvidenceReport{}, err
		}
		report.FindingsCount += len(surfaceReport.Findings)
		report.Surfaces = append(report.Surfaces, surfaceReport)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return PublishEvidenceReport{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return PublishEvidenceReport{}, err
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return PublishEvidenceReport{}, err
	}
	return report, nil
}

func scanEvidenceSurface(surface PublishEvidenceSurface) (PublishEvidenceSurfaceReport, error) {
	name := strings.TrimSpace(surface.Name)
	if name == "" {
		name = "surface"
	}
	if strings.TrimSpace(surface.Root) != "" {
		scan, err := ScanPublishTree(surface.Root)
		if err != nil {
			return PublishEvidenceSurfaceReport{}, err
		}
		return PublishEvidenceSurfaceReport{Name: name, Kind: "tree", FilesScanned: scan.FilesScanned, Findings: scan.Findings}, nil
	}
	findings := make([]domain.PublishScanFinding, 0)
	seen := map[string]bool{}
	hash := sha256.Sum256([]byte(surface.Text))
	for _, class := range redaction.ScanSensitiveClasses(surface.Text) {
		finding := domain.PublishScanFinding{Class: publishViolationClassForSensitive(class), Path: name, Severity: "blocking", Message: "Publish evidence surface matched a blocked sensitive pattern; content was not echoed", Size: int64(len(surface.Text)), SHA256: hex.EncodeToString(hash[:])}
		findings = appendScanFinding(findings, seen, finding)
	}
	return PublishEvidenceSurfaceReport{Name: name, Kind: "text", Findings: findings}, nil
}
