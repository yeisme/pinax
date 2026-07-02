package domain

import "testing"

func TestPublishProfileDefaultsAreSafe(t *testing.T) {
	profile := NewDefaultPublishProfile("public", PublishTargetGitHubPages, PublishRendererHugo)

	if profile.SchemaVersion != PublishProfileSchemaVersion {
		t.Fatalf("schema version = %q", profile.SchemaVersion)
	}
	if profile.Name != "public" || profile.Target != PublishTargetGitHubPages || profile.Renderer != PublishRendererHugo {
		t.Fatalf("unexpected profile identity: %#v", profile)
	}
	if profile.BodyPolicy != PublishBodyPolicyPublishedNotesOnly {
		t.Fatalf("body policy = %q", profile.BodyPolicy)
	}
	if !profile.Safety.BlockSecrets || !profile.Safety.BlockPrivateBodies || !profile.Safety.BlockPinaxInternals {
		t.Fatalf("unsafe safety defaults: %#v", profile.Safety)
	}
	if got := profile.Site.Theme.Value; got != "builtin:pinax-encyclopedia" {
		t.Fatalf("theme = %q", got)
	}
	if profile.Deploy.Mode != PublishDeployModeNone {
		t.Fatalf("deploy mode = %q", profile.Deploy.Mode)
	}
}

func TestPublishStableEnumValues(t *testing.T) {
	cases := map[string]string{
		"target pages":         string(PublishTargetGitHubPages),
		"target wiki":          string(PublishTargetGitHubWiki),
		"renderer pinax-web":   string(PublishRendererPinaxWeb),
		"renderer hugo legacy": string(PublishRendererHugo),
		"renderer none":        string(PublishRendererNone),
		"body policy":          string(PublishBodyPolicyPublishedNotesOnly),
		"violation":            string(PublishViolationAuthorizationHeader),
	}
	for name, got := range cases {
		if got == "" {
			t.Fatalf("%s is empty", name)
		}
	}
}
