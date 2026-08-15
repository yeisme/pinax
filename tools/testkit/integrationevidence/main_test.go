package main

import (
	"io"
	"strings"
	"testing"
)

func TestBuildConfigIncludesPublishEvidenceEntrypoint(t *testing.T) {
	config := buildConfig("test-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	if !strings.Contains(command, "./tests/e2e") || !strings.Contains(command, "TestPublishProfile") || !strings.Contains(command, "TestPublishStaticSite") || !strings.Contains(command, "TestPublishDoc") || !strings.Contains(command, "TestShareLANReadOnly") {
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
	if config.ExtraChecks["publish_doc"] != true {
		t.Fatalf("publish_doc check missing: %#v", config.ExtraChecks)
	}
	if config.ExtraChecks["share_lan_readonly"] != true {
		t.Fatalf("share_lan_readonly check missing: %#v", config.ExtraChecks)
	}
}

func TestBuildIdentityConfigUsesRequiredEvidenceDirectoryAndEntrypoint(t *testing.T) {
	config := buildConfigForProfile("identity", "identity-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	if !strings.Contains(command, "IdentityFirstTwoDeviceKernel") || !strings.Contains(command, "ManifestMigration") {
		t.Fatalf("identity evidence command = %s", command)
	}
	if config.ParentDir != "temp/integration-test-runs" || config.ExtraChecks["canonical_object_identity"] != true {
		t.Fatalf("config = %#v", config)
	}
}

func TestBuildIdentityBenchmarkProfile(t *testing.T) {
	config := buildConfigForProfile("identity-benchmark", "benchmark-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"-bench", "Identity|IndexV2|LinkGraph|ObjectSync", "-benchmem"} {
		if !strings.Contains(command, required) {
			t.Fatalf("benchmark command %q missing %q", command, required)
		}
	}
	if config.ExtraChecks["link_graph_100k"] != true {
		t.Fatalf("benchmark checks = %#v", config.ExtraChecks)
	}
}
