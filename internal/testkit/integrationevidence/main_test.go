package main

import (
	"io"
	"strings"
	"testing"
)

func TestBuildConfigIncludesPublishEvidenceEntrypoint(t *testing.T) {
	config := buildConfig("test-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	if !strings.Contains(command, "./tests/e2e") || !strings.Contains(command, "TestPublishProfile") || !strings.Contains(command, "TestPublishStaticSite") || !strings.Contains(command, "TestShareLANReadOnly") {
		t.Fatalf("integration evidence command does not include publish e2e entrypoint: %s", command)
	}
	if config.ParentDir != "temp/integration-test-runs" {
		t.Fatalf("parent dir = %q", config.ParentDir)
	}
	if config.ExtraChecks["publish_static_profile"] != true {
		t.Fatalf("publish_static_profile check missing: %#v", config.ExtraChecks)
	}
	if config.ExtraChecks["publish_static_site"] != true {
		t.Fatalf("publish_static_site check missing: %#v", config.ExtraChecks)
	}
	if config.ExtraChecks["share_lan_readonly"] != true {
		t.Fatalf("share_lan_readonly check missing: %#v", config.ExtraChecks)
	}
}
