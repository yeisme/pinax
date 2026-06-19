package publishops

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/redaction"
)

func ScanPublishTree(root string) (domain.PublishScanReport, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return domain.PublishScanReport{}, err
	}
	report := domain.PublishScanReport{}
	seen := map[string]bool{}
	err = filepath.WalkDir(cleanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(cleanRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		report.FilesScanned++
		binary := isPublishBinaryContent(body)
		hash := sha256.Sum256(body)
		for _, class := range redaction.ScanSensitiveClasses(rel) {
			report.Findings = appendScanFinding(report.Findings, seen, scanFinding(class, rel, info.Size(), hash[:], binary))
		}
		if binary {
			return nil
		}
		for _, class := range redaction.ScanSensitiveClasses(string(body)) {
			report.Findings = appendScanFinding(report.Findings, seen, scanFinding(class, rel, info.Size(), hash[:], false))
		}
		return nil
	})
	if err != nil {
		return domain.PublishScanReport{}, err
	}
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Path == report.Findings[j].Path {
			return report.Findings[i].Class < report.Findings[j].Class
		}
		return report.Findings[i].Path < report.Findings[j].Path
	})
	return report, nil
}

func isPublishBinaryContent(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	// 发布扫描把 NUL 字节或非法 UTF-8 视为二进制内容：二进制可能包含 token、cookie
	// 或 provider payload 的原始字节，不能为了诊断把片段回显到 stdout、receipt 或 evidence。
	for _, b := range body {
		if b == 0 {
			return true
		}
	}
	return !utf8.Valid(body)
}

func scanFinding(class redaction.SensitiveClass, path string, size int64, hash []byte, binary bool) domain.PublishScanFinding {
	return domain.PublishScanFinding{Class: publishViolationClassForSensitive(class), Path: path, Severity: "blocking", Message: publishScanMessage(class, binary), Size: size, SHA256: hex.EncodeToString(hash), Binary: binary}
}

func appendScanFinding(findings []domain.PublishScanFinding, seen map[string]bool, finding domain.PublishScanFinding) []domain.PublishScanFinding {
	key := string(finding.Class) + "\x00" + finding.Path
	if seen[key] {
		return findings
	}
	seen[key] = true
	return append(findings, finding)
}

func publishViolationClassForSensitive(class redaction.SensitiveClass) domain.PublishViolationClass {
	switch class {
	case redaction.SensitiveAuthorization:
		return domain.PublishViolationAuthorizationHeader
	case redaction.SensitiveCookie:
		return domain.PublishViolationCookieHeader
	case redaction.SensitiveWebhook:
		return domain.PublishViolationWebhookURL
	case redaction.SensitiveProvider:
		return domain.PublishViolationProviderPayload
	case redaction.SensitiveAbsolutePath:
		return domain.PublishViolationAbsolutePath
	case redaction.SensitivePinaxInternal:
		return domain.PublishViolationPinaxInternalRef
	case redaction.SensitivePrivateBody:
		return domain.PublishViolationPrivateBodyLeak
	default:
		return domain.PublishViolationSecretPattern
	}
}

func publishScanMessage(class redaction.SensitiveClass, binary bool) string {
	if binary {
		// 二进制命中只记录路径、大小和 hash，避免把不可审查的原始字节写入任何发布面。
		return "Publish output path matched a blocked sensitive pattern; binary content was not echoed"
	}
	return "Publish output matched a blocked sensitive pattern; content was not echoed"
}
