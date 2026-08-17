package app

import (
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestRewriteProjectionCommandsPreservesLegitCloudText(t *testing.T) {
	t.Parallel()
	projection := domain.NewProjection("cloud.status", "Cloud backend status read. Vault: Cloud Notes at s3://cloud-storage-bucket")
	projection.Error = &domain.CommandError{Message: "provider said cloud token scope insufficient"}
	rewriteProjectionCommands(&projection, syncTargetCloud, syncTargetCapsa)
	if !strings.Contains(projection.Summary, "Capsa backend status read") {
		t.Fatalf("known phrase not rebranded: %s", projection.Summary)
	}
	if !strings.Contains(projection.Summary, "Vault: Cloud Notes") || !strings.Contains(projection.Summary, "s3://cloud-storage-bucket") {
		t.Fatalf("legitimate cloud text corrupted: %s", projection.Summary)
	}
	if strings.Contains(projection.Error.Message, "Capsa token") {
		t.Fatalf("provider message corrupted: %s", projection.Error.Message)
	}
}
