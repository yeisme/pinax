package promptbridge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	promptrepo "github.com/yeisme/promptrepo"
)

type fakeClient struct {
	promptrepo.Client

	listErr error
	page    promptrepo.RepositoryPage
	panics  bool
}

func (f *fakeClient) ListRepositories(context.Context, promptrepo.ListRepositoriesRequest) (promptrepo.RepositoryPage, error) {
	if f.panics {
		panic("provider exploded")
	}
	if f.listErr != nil {
		return promptrepo.RepositoryPage{}, f.listErr
	}
	return f.page, nil
}

func TestEffectiveLocaleDefaultsToEnglish(t *testing.T) {
	if got := EffectiveLocale(""); got != "en" {
		t.Fatalf("default locale = %q, want en", got)
	}
	if got := EffectiveLocale("  en  "); got != "en" {
		t.Fatalf("session locale = %q, want en", got)
	}
}

func TestEffectiveLocaleForRefPreservesExactAddress(t *testing.T) {
	ref := "promptrepo://official/general/summary@1.0.0?kind=template&locale=zh-CN&role=main"
	if got := EffectiveLocaleForRef(ref, ""); got != "zh-CN" {
		t.Fatalf("exact ref locale = %q", got)
	}
	if got := EffectiveLocaleForRef(ref, "en"); got != "en" {
		t.Fatalf("explicit override locale = %q", got)
	}
}

func TestMapErrorCoversStableCodes(t *testing.T) {
	cases := []struct {
		sdk  string
		want string
	}{
		{promptrepo.CodeNotFound, CodeRepositoryNotFound},
		{promptrepo.CodeAlreadyExists, CodeRepositoryExists},
		{promptrepo.CodeRightsBlocked, CodeInstallRightsBlocked},
		{promptrepo.CodeAddressMismatch, CodeCatalogAddressMismatch},
		{promptrepo.CodeSelectorUnsupported, CodeCatalogSelector},
		{promptrepo.CodeStateLocked, CodeRepositoryStateLocked},
		{"TOTALLY_NEW", CodeCatalogUnavailable},
	}
	for _, tc := range cases {
		err := MapError(promptrepo.NewError(tc.sdk, "stable message", false, nil))
		commandError, ok := err.(*domain.CommandError)
		if !ok {
			t.Fatalf("%s did not map to CommandError", tc.sdk)
		}
		if commandError.Code != tc.want {
			t.Fatalf("%s mapped to %s, want %s", tc.sdk, commandError.Code, tc.want)
		}
		if strings.Contains(commandError.Message, "secret") {
			t.Fatalf("message leaked payload: %s", commandError.Message)
		}
	}
	if MapError(nil) != nil {
		t.Fatal("nil error must map to nil")
	}
	if mapped := MapError(errors.New("boom")); mapped.(*domain.CommandError).Code != CodeCatalogUnavailable {
		t.Fatal("non-SDK errors map to prompt_catalog_unavailable")
	}
}

func TestBridgeContainsProviderPanic(t *testing.T) {
	bridge := New(&fakeClient{panics: true})
	_, err := bridge.ListRepositories(context.Background())
	commandError, ok := err.(*domain.CommandError)
	if !ok {
		t.Fatalf("panic escaped as %T", err)
	}
	if commandError.Code != CodeProviderPanic {
		t.Fatalf("panic code = %s, want %s", commandError.Code, CodeProviderPanic)
	}
}

func TestBridgeCapabilityDegradesStably(t *testing.T) {
	bridge := New(&fakeClient{})
	if _, err := bridge.Inspect(context.Background(), promptrepo.InspectRequest{}); err.(*domain.CommandError).Code != CodeCapabilityUnsupported {
		t.Fatal("inspect on incapable client must degrade with prompt_catalog_capability_unsupported")
	}
	if _, err := bridge.Validate(context.Background(), promptrepo.ValidateRequest{}); err.(*domain.CommandError).Code != CodeCapabilityUnsupported {
		t.Fatal("validate on incapable client must degrade with prompt_catalog_capability_unsupported")
	}
	if _, err := bridge.Preview(context.Background(), promptrepo.PreviewRequest{}); err.(*domain.CommandError).Code != CodeCapabilityUnsupported {
		t.Fatal("preview on incapable client must degrade with prompt_catalog_capability_unsupported")
	}
	if _, err := bridge.ResolveTemplateContract(context.Background(), promptrepo.ResolveTemplateContractRequest{}); err.(*domain.CommandError).Code != CodeCapabilityUnsupported {
		t.Fatal("contract on incapable client must degrade with prompt_catalog_capability_unsupported")
	}
}

func TestPreviewDiscardsRenderedBody(t *testing.T) {
	previewing := &previewingClient{}
	bridge := New(previewing)
	result, err := bridge.Preview(context.Background(), promptrepo.PreviewRequest{})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if result.RenderedBody != "" {
		t.Fatal("rendered body crossed the bridge port")
	}
	if result.RenderedDigest == "" {
		t.Fatal("rendered digest should be preserved")
	}
}

type previewingClient struct {
	promptrepo.Client
}

func (p *previewingClient) Preview(context.Context, promptrepo.PreviewRequest) (promptrepo.PreviewResult, error) {
	return promptrepo.PreviewResult{RenderedBody: "SECRET BODY", RenderedDigest: "sha256:abc", RenderedBytes: 11}, nil
}
