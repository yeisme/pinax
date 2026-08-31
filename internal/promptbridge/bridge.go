// Package promptbridge adapts Pinax to the public promptrepo SDK as a domain
// consumer. The bridge owns the Pinax-side ports, consumer capabilities, error
// mapping, panic containment, and the effective repository-set/policy
// composition; it never imports Template Registry internals, never reads
// promptrepo state files directly, and never projects template bodies, input
// values, or credentials. Application and CLI layers depend only on this
// package's ports, not on the promptrepo SDK.
package promptbridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	promptrepo "github.com/yeisme/promptrepo"
	"github.com/yeisme/promptrepo/engine"
)

// DefaultLocale is the catalog locale Pinax requests unless the command
// overrides it for the current session only.
const DefaultLocale = "zh-CN"

// Consumer identifies Pinax in promptrepo stage receipts.
const Consumer = "pinax"

// ConsumerCapabilities are the stable English capability keys Pinax declares
// to repositories and future policy providers.
var ConsumerCapabilities = []string{
	"catalog_search",
	"catalog_resolve",
	"catalog_inspect",
	"catalog_validate",
	"catalog_preview",
	"prompt_install_draft",
}

// Bridge is the Pinax application port over a promptrepo client. Optional
// SDK capabilities (inspect/validate/render/preview/contract) are resolved
// through type assertion so unsupported clients degrade with stable errors.
// All errors returned from bridge methods are *domain.CommandError with
// stable pinax codes; SDK panics are contained as provider_panic errors.
type Bridge struct {
	client promptrepo.Client
}

// New wraps a promptrepo client. The client must be the public SDK Client;
// Pinax never wraps Registry internals here.
func New(client promptrepo.Client) *Bridge {
	return &Bridge{client: client}
}

// Client exposes the underlying client for bridge-internal use only.
func (b *Bridge) Client() promptrepo.Client { return b.client }

// EngineOptions configures the shared embedded engine opened by OpenShared.
// Zero values resolve to the OS user config/cache directories shared with
// other promptrepo consumers.
type EngineOptions struct {
	ConfigRoot string
	CacheRoot  string
}

// OpenShared opens the embedded promptrepo engine over the OS user's shared
// repository store, honoring PROMPTREPO_HOME and PROMPTREPO_CACHE like other
// domain CLIs so repository profiles stay cross-CLI shared.
func OpenShared(options EngineOptions) (*Bridge, error) {
	if strings.TrimSpace(options.ConfigRoot) == "" {
		if root := strings.TrimSpace(os.Getenv("PROMPTREPO_HOME")); root != "" {
			options.ConfigRoot = root
		}
	}
	if strings.TrimSpace(options.CacheRoot) == "" {
		if root := strings.TrimSpace(os.Getenv("PROMPTREPO_CACHE")); root != "" {
			options.CacheRoot = root
		} else if options.ConfigRoot != "" {
			options.CacheRoot = filepath.Join(options.ConfigRoot, "cache")
		}
	}
	manager, err := engine.New(engine.Options{ConfigRoot: options.ConfigRoot, CacheRoot: options.CacheRoot})
	if err != nil {
		return nil, MapError(err)
	}
	return New(manager), nil
}

// Stable pinax error codes for the federated prompt surface. Machine keys and
// reason codes stay English and stable; messages never carry bodies or values.
const (
	CodeCatalogInvalidRequest   = "prompt_catalog_invalid_request"
	CodeCatalogNotFound         = "prompt_catalog_not_found"
	CodeCatalogAddressMismatch  = "prompt_catalog_address_mismatch"
	CodeCatalogDigestMismatch   = "prompt_catalog_digest_mismatch"
	CodeCatalogInputInvalid     = "prompt_catalog_input_invalid"
	CodeCatalogSelector         = "prompt_catalog_selector_unsupported"
	CodeCatalogUnavailable      = "prompt_catalog_unavailable"
	CodeCapabilityUnsupported   = "prompt_catalog_capability_unsupported"
	CodeProviderPanic           = "prompt_catalog_provider_panic"
	CodeRepositoryExists        = "prompt_repository_exists"
	CodeRepositoryNotFound      = "prompt_repository_not_found"
	CodeRepositorySourceBad     = "prompt_repository_source_unsupported"
	CodeRepositoryAuthFailed    = "prompt_repository_auth_failed"
	CodeRepositorySyncFailed    = "prompt_repository_sync_failed"
	CodeRepositoryStateLocked   = "prompt_repository_state_locked"
	CodeInstallRightsBlocked    = "prompt_install_rights_blocked"
	CodeInstallConflict         = "prompt_install_conflict"
	CodeInstallBodyUnverifiable = "prompt_install_body_unverifiable"
	CodeInstallStaticTemplate   = "prompt_install_static_template"
)

var sdkErrorCodes = map[string]string{
	promptrepo.CodeInvalidRequest:          CodeCatalogInvalidRequest,
	promptrepo.CodeNotFound:                CodeRepositoryNotFound,
	promptrepo.CodeAlreadyExists:           CodeRepositoryExists,
	promptrepo.CodeUnsupportedSourceScheme: CodeRepositorySourceBad,
	promptrepo.CodeAuthRequired:            CodeRepositoryAuthFailed,
	promptrepo.CodeAuthorizationFailed:     CodeRepositoryAuthFailed,
	promptrepo.CodeSourceFetchFailed:       CodeRepositorySyncFailed,
	promptrepo.CodeDigestMismatch:          CodeCatalogDigestMismatch,
	promptrepo.CodeStateLocked:             CodeRepositoryStateLocked,
	promptrepo.CodeStateSchemaTooNew:       CodeCatalogUnavailable,
	promptrepo.CodeIncompatible:            CodeCatalogInvalidRequest,
	promptrepo.CodeRightsBlocked:           CodeInstallRightsBlocked,
	promptrepo.CodeAddressMismatch:         CodeCatalogAddressMismatch,
	promptrepo.CodeInputRequired:           CodeCatalogInputInvalid,
	promptrepo.CodeInputUnknown:            CodeCatalogInputInvalid,
	promptrepo.CodeInputType:               CodeCatalogInputInvalid,
	promptrepo.CodeInputEnum:               CodeCatalogInputInvalid,
	promptrepo.CodeInputConstraint:         CodeCatalogInputInvalid,
	promptrepo.CodeTemplatePlaceholder:     CodeCatalogInputInvalid,
	promptrepo.CodeTemplateSyntax:          CodeCatalogInputInvalid,
	promptrepo.CodeSelectorUnsupported:     CodeCatalogSelector,
}

// MapError converts an SDK error into a stable domain.CommandError. SDK
// messages are log-safe by contract, so they are preserved verbatim; codes
// map to the pinax federated-prompt taxonomy above. Already-mapped pinax
// errors (including provider_panic) pass through unchanged.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	var commandError *domain.CommandError
	if errors.As(err, &commandError) {
		return commandError
	}
	var sdkError *promptrepo.Error
	if errors.As(err, &sdkError) {
		code, ok := sdkErrorCodes[sdkError.Code]
		if !ok {
			code = CodeCatalogUnavailable
		}
		return &domain.CommandError{Code: code, Message: sdkError.Message, Hint: "Check the repository source, exact promptrepo:// address, and retry"}
	}
	return &domain.CommandError{Code: CodeCatalogUnavailable, Message: "Prompt repository engine failed", Hint: err.Error()}
}

// guarded runs one SDK call, containing panics and mapping errors. Panics
// never escape the bridge into the CLI process.
func guarded(operation string, call func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &domain.CommandError{
				Code:    CodeProviderPanic,
				Message: fmt.Sprintf("Prompt repository operation %s failed internally", operation),
				Hint:    "Retry the command; if it persists, run pinax prompt repository doctor --json",
			}
		}
	}()
	return call()
}

// capabilityError returns the stable unsupported-capability error.
func capabilityError(capability string) error {
	return &domain.CommandError{
		Code:    CodeCapabilityUnsupported,
		Message: "Prompt repository client does not support this capability",
		Hint:    fmt.Sprintf("The shared repository engine must expose %s", capability),
	}
}

// Repository administration. These mutate only the shared user-level
// promptrepo store owned by the SDK, never Pinax vault state.

// AddRepository registers a user-scope repository profile in the shared store.
func (b *Bridge) AddRepository(ctx context.Context, request promptrepo.AddRepositoryRequest) (view promptrepo.RepositoryView, err error) {
	err = guarded("repository_add", func() error {
		view, err = b.client.AddRepository(ctx, request)
		return err
	})
	return view, MapError(err)
}

// ListRepositories lists every profile in the shared store.
func (b *Bridge) ListRepositories(ctx context.Context) (page promptrepo.RepositoryPage, err error) {
	err = guarded("repository_list", func() error {
		page, err = b.client.ListRepositories(ctx, promptrepo.ListRepositoriesRequest{})
		return err
	})
	return page, MapError(err)
}

// ShowRepository returns one repository view for show/doctor commands.
func (b *Bridge) ShowRepository(ctx context.Context, id string) (view promptrepo.RepositoryView, err error) {
	err = guarded("repository_show", func() error {
		view, err = b.client.ShowRepository(ctx, id)
		return err
	})
	return view, MapError(err)
}

// RemoveRepository deletes a profile from the shared store.
func (b *Bridge) RemoveRepository(ctx context.Context, id string) (err error) {
	return MapError(guarded("repository_remove", func() error {
		return b.client.RemoveRepository(ctx, id)
	}))
}

// SetRepositoryEnabled toggles a profile without deleting it.
func (b *Bridge) SetRepositoryEnabled(ctx context.Context, id string, enabled bool) (view promptrepo.RepositoryView, err error) {
	err = guarded("repository_enable", func() error {
		view, err = b.client.SetRepositoryEnabled(ctx, id, enabled)
		return err
	})
	return view, MapError(err)
}

// SyncRepositories refreshes snapshots for the requested repositories.
func (b *Bridge) SyncRepositories(ctx context.Context, request promptrepo.SyncRequest) (receipt promptrepo.SyncReceipt, err error) {
	err = guarded("repository_sync", func() error {
		receipt, err = b.client.SyncRepositories(ctx, request)
		return err
	})
	return receipt, MapError(err)
}

// Catalog reads. Result types never contain template bodies: solution cards
// and resolved solutions carry roles, locales, paths, and digests only.

// Search searches the federated catalog across enabled repositories.
func (b *Bridge) Search(ctx context.Context, request promptrepo.SearchRequest) (result promptrepo.SearchResult, err error) {
	err = guarded("catalog_search", func() error {
		result, err = b.client.Search(ctx, request)
		return err
	})
	return result, MapError(err)
}

// Resolve resolves an exact ref or address into a solution projection.
func (b *Bridge) Resolve(ctx context.Context, request promptrepo.ResolveRequest) (resolved promptrepo.ResolvedSolution, err error) {
	err = guarded("catalog_resolve", func() error {
		resolved, err = b.client.Resolve(ctx, request)
		return err
	})
	return resolved, MapError(err)
}

// ReadTemplate reads the verified template body. It is install-gated: the
// bridge exposes it only to the application install service after rights and
// digest checks, and the body never reaches projections or evidence.
func (b *Bridge) ReadTemplate(ctx context.Context, request promptrepo.ReadTemplateRequest) (content promptrepo.TemplateContent, err error) {
	err = guarded("catalog_read_template", func() error {
		content, err = b.client.ReadTemplate(ctx, request)
		return err
	})
	return content, MapError(err)
}

// Stage records a consumer receipt in the shared store for install provenance.
func (b *Bridge) Stage(ctx context.Context, request promptrepo.StageRequest) (receipt promptrepo.StageReceipt, err error) {
	err = guarded("catalog_stage", func() error {
		receipt, err = b.client.Stage(ctx, request)
		return err
	})
	return receipt, MapError(err)
}

// Inspect returns catalog, snapshot, and contract metadata without reading
// the template body and without any provider call.
func (b *Bridge) Inspect(ctx context.Context, request promptrepo.InspectRequest) (result promptrepo.InspectResult, err error) {
	inspector, ok := b.client.(promptrepo.Inspector)
	if !ok {
		return promptrepo.InspectResult{}, capabilityError("inspect")
	}
	err = guarded("catalog_inspect", func() error {
		result, err = inspector.Inspect(ctx, request)
		return err
	})
	return result, MapError(err)
}

// Validate reports input readiness for a template role without reading the
// body and without any provider call.
func (b *Bridge) Validate(ctx context.Context, request promptrepo.ValidateRequest) (validation promptrepo.InputValidation, err error) {
	validator, ok := b.client.(promptrepo.Validator)
	if !ok {
		return promptrepo.InputValidation{}, capabilityError("validate")
	}
	err = guarded("catalog_validate", func() error {
		validation, err = validator.Validate(ctx, request)
		return err
	})
	return validation, MapError(err)
}

// Preview performs a bounded template read plus strict in-memory rendering.
// RenderedBody is discarded here; only digests and counts cross the port.
func (b *Bridge) Preview(ctx context.Context, request promptrepo.PreviewRequest) (result promptrepo.PreviewResult, err error) {
	previewer, ok := b.client.(promptrepo.Previewer)
	if !ok {
		return promptrepo.PreviewResult{}, capabilityError("preview")
	}
	err = guarded("catalog_preview", func() error {
		result, err = previewer.Preview(ctx, request)
		result.RenderedBody = ""
		return err
	})
	return result, MapError(err)
}

// ResolveTemplateContract loads the Registry-authored companion contract
// bound to the exact snapshot and template.
func (b *Bridge) ResolveTemplateContract(ctx context.Context, request promptrepo.ResolveTemplateContractRequest) (contract promptrepo.ResolvedTemplateContract, err error) {
	resolver, ok := b.client.(promptrepo.ContractResolver)
	if !ok {
		return promptrepo.ResolvedTemplateContract{}, capabilityError("template_contract")
	}
	err = guarded("catalog_contract", func() error {
		contract, err = resolver.ResolveTemplateContract(ctx, request)
		return err
	})
	return contract, MapError(err)
}

// EffectiveLocale resolves the locale for a command: session overrides win,
// otherwise the Pinax zh-CN default.
func EffectiveLocale(sessionLocale string) string {
	if trimmed := strings.TrimSpace(sessionLocale); trimmed != "" {
		return trimmed
	}
	return DefaultLocale
}
