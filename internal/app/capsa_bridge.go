package app

import (
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	syncTargetCapsa      = "capsa"
	syncTargetCloud      = "cloud"
	syncTargetPinaxCloud = "pinax-cloud"
)

func isCapsaSyncTarget(target string) bool {
	switch strings.TrimSpace(target) {
	case "", syncTargetCapsa, syncTargetCloud, syncTargetPinaxCloud:
		return true
	default:
		return false
	}
}

func syncOutputTarget(target string) string {
	switch strings.TrimSpace(target) {
	case syncTargetCloud:
		return syncTargetCloud
	case syncTargetPinaxCloud:
		return syncTargetCapsa
	case "", syncTargetCapsa:
		return syncTargetCapsa
	default:
		return strings.TrimSpace(target)
	}
}

func syncLegacyTarget(target string) string {
	switch strings.TrimSpace(target) {
	case syncTargetCloud, syncTargetPinaxCloud:
		return strings.TrimSpace(target)
	default:
		return ""
	}
}

func syncConfigCommand(target string) string {
	if syncOutputTarget(target) == syncTargetCloud {
		return syncTargetCloud
	}
	return syncTargetCapsa
}

func addCapsaBridgeFacts(projection *domain.Projection, requestedTarget string) {
	if projection.Facts == nil {
		projection.Facts = map[string]string{}
	}
	projection.Facts["sync_platform"] = syncTargetCapsa
	projection.Facts["state_dir"] = ".pinax/cloud"
	if legacy := syncLegacyTarget(requestedTarget); legacy != "" {
		projection.Facts["deprecated_alias"] = "true"
		projection.Facts["legacy_target"] = legacy
	}
}

func rewriteProjectionCommands(projection *domain.Projection, old, next string) {
	if old == next || old == "" || next == "" {
		return
	}
	projection.Command = strings.Replace(projection.Command, old+".", next+".", 1)
	projection.Summary = strings.ReplaceAll(projection.Summary, "Cloud", "Capsa")
	projection.Summary = strings.ReplaceAll(projection.Summary, "cloud", "Capsa")
	if projection.Error != nil {
		projection.Error.Message = strings.ReplaceAll(projection.Error.Message, "Cloud", "Capsa")
		projection.Error.Message = strings.ReplaceAll(projection.Error.Message, "cloud", "Capsa")
		projection.Error.Hint = strings.ReplaceAll(projection.Error.Hint, "pinax "+old, "pinax "+next)
		projection.Error.Hint = strings.ReplaceAll(projection.Error.Hint, "--target "+old, "--target "+next)
	}
	for i := range projection.Actions {
		projection.Actions[i].Command = strings.ReplaceAll(projection.Actions[i].Command, "pinax "+old, "pinax "+next)
		projection.Actions[i].Command = strings.ReplaceAll(projection.Actions[i].Command, "--target "+old, "--target "+next)
	}
}
