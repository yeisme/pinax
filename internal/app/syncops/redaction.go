package syncops

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/redaction"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

func NormalizePathPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "hash", "hashed":
		return "hash"
	case "omit", "omitted", "none":
		return "omitted"
	default:
		return "default"
	}
}

func SanitizeOperations(ops []syncplan.Operation, policy string) []syncplan.Operation {
	if len(ops) == 0 {
		return nil
	}
	policy = NormalizePathPolicy(policy)
	out := make([]syncplan.Operation, 0, len(ops))
	for _, op := range ops {
		op.Path = RedactPath(op.Path, policy)
		if policy == "hash" && op.PathHash == "" && op.Path != "" {
			op.PathHash = op.Path
		}
		if policy == "omitted" {
			op.PathHash = ""
		}
		out = append(out, op)
	}
	return out
}

func SanitizePlan(plan syncplan.Plan, policy string) syncplan.Plan {
	plan.Operations = SanitizeOperations(plan.Operations, policy)
	return plan
}

func RedactPath(pathValue, policy string) string {
	pathValue = filepath.ToSlash(strings.TrimSpace(pathValue))
	if pathValue == "" {
		return ""
	}
	switch NormalizePathPolicy(policy) {
	case "hash":
		sum := sha256.Sum256([]byte(pathValue))
		return "path_sha256:" + hex.EncodeToString(sum[:])
	case "omitted":
		return ""
	default:
		return SanitizeString(pathValue)
	}
}

func SanitizeString(value string) string {
	value = redaction.Cloud(value)
	for _, marker := range []string{"Authorization", "Cookie", "provider payload", "provider stderr", "NOTICE:", "CRITICAL:", "rclone copyto failed"} {
		value = strings.ReplaceAll(value, marker, "[REDACTED]")
	}
	return value
}
