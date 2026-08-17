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

// capsaSummaryPhrases maps the exact legacy-cloud phrases that the capsa alias
// surface rebrands. Replacing whole phrases (instead of the word "cloud"
// anywhere) keeps legitimate text — vault names, provider messages, paths —
// from being corrupted.
var capsaSummaryPhrases = []string{
	"S3 direct cloud backend configured.",
	"Rclone direct cloud backend configured.",
	"Cloud backend configured.",
	"Cloud backend status read.",
	"Cloud device session logged out.",
	"Cloud backend diagnostics passed.",
	"s3 cloud backend configuration is incomplete",
	"rclone cloud backend configuration is incomplete",
	"cloud login configuration is incomplete",
}

func rewriteProjectionCommands(projection *domain.Projection, old, next string) {
	if old == next || old == "" || next == "" {
		return
	}
	projection.Command = strings.Replace(projection.Command, old+".", next+".", 1)
	projection.Summary = rebrandCapsaPhrases(projection.Summary)
	if projection.Error != nil {
		projection.Error.Message = rebrandCapsaPhrases(projection.Error.Message)
		projection.Error.Hint = strings.ReplaceAll(projection.Error.Hint, "pinax "+old, "pinax "+next)
		projection.Error.Hint = strings.ReplaceAll(projection.Error.Hint, "--target "+old, "--target "+next)
	}
	for i := range projection.Actions {
		projection.Actions[i].Command = strings.ReplaceAll(projection.Actions[i].Command, "pinax "+old, "pinax "+next)
		projection.Actions[i].Command = strings.ReplaceAll(projection.Actions[i].Command, "--target "+old, "--target "+next)
	}
}

func rebrandCapsaPhrases(text string) string {
	for _, phrase := range capsaSummaryPhrases {
		rebranded := strings.ReplaceAll(strings.ReplaceAll(phrase, "Cloud", "Capsa"), "cloud", "Capsa")
		text = strings.ReplaceAll(text, phrase, rebranded)
	}
	return text
}
