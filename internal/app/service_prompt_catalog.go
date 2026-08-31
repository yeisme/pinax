package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/promptasset"
	"github.com/yeisme/pinax/internal/promptbridge"
	promptrepo "github.com/yeisme/promptrepo"
)

// Federated prompt repository and catalog operations: shared user-level
// repository administration, provider-free catalog reads, and the
// rights-gated install that materializes a verified body as a Pinax-owned
// draft PromptAsset. All commands surface stable operation_id facts using the
// public promptrepo.* identity while the `command` field keeps the Pinax
// command identity. Template bodies, input values, and credential refs never
// enter projections or evidence.

const (
	promptCatalogMaxValuesBytes = 1 << 20
	promptCatalogOrigin         = "federated_catalog"
)

// importPermissions are the canonical contract permissions that grant a local
// import/copy of the template body. Anything else fails closed.
var importPermissions = map[string]struct{}{
	"local_import": {},
	"local_copy":   {},
	"copy":         {},
	"install":      {},
}

// PromptRepositoryRequest is the shared request for repository administration
// commands.
type PromptRepositoryRequest struct {
	ID            string
	Source        string
	Revision      string
	Channel       string
	Trust         string
	Locale        string
	CredentialRef string
	SyncAll       bool
	Repositories  []string
}

// PromptCatalogRequest is the shared request for federated catalog reads and
// install.
type PromptCatalogRequest struct {
	VaultPath string
	Ref       string
	Query     string
	Locale    string
	Role      string
	Selector  string
	Tags      []string
	// ValuesFile is a JSON object of input values for validate/preview; the
	// values themselves never appear in any output.
	ValuesFile string
	// RepositoryIDs restricts this command's effective repository set.
	RepositoryIDs []string
	DenyIDs       []string
	// Confirm applies an install that passed its plan gate.
	Confirm bool
	// Fork resolves a local-ID conflict by installing side-by-side.
	Fork bool
}

// WithPromptBridge overrides the federated prompt bridge (tests and embedded
// consumers); production commands lazily open the shared engine.
func (s *Service) WithPromptBridge(bridge *promptbridge.Bridge) *Service {
	s.promptBridge = bridge
	return s
}

func (s *Service) promptCatalogBridge() (*promptbridge.Bridge, error) {
	if s.promptBridge != nil {
		return s.promptBridge, nil
	}
	bridge, err := promptbridge.OpenShared(promptbridge.EngineOptions{})
	if err != nil {
		return nil, &domain.CommandError{Code: promptbridge.CodeCatalogUnavailable, Message: "Prompt repository engine is unavailable", Hint: "Check PROMPTREPO_HOME/PROMPTREPO_CACHE or run pinax prompt repository list --json"}
	}
	s.promptBridge = bridge
	return bridge, nil
}

func promptRepositoryError(command string, err error) (domain.Projection, error) {
	projection := errorProjection(command, err)
	return projection, err
}

// PromptRepositoryAdd registers a repository profile in the shared
// user-level promptrepo store.
func (s *Service) PromptRepositoryAdd(ctx context.Context, req PromptRepositoryRequest) (domain.Projection, error) {
	const command = "prompt.repository.add"
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Source) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt repository add requires an id and --source", Hint: "pinax prompt repository add <id> --source <uri> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	profile := promptrepo.RepositoryProfile{ID: strings.TrimSpace(req.ID), Source: strings.TrimSpace(req.Source), Scope: promptbridge.ScopeUser, Trust: strings.TrimSpace(req.Trust), Revision: strings.TrimSpace(req.Revision), Channel: strings.TrimSpace(req.Channel)}
	if ref := strings.TrimSpace(req.CredentialRef); ref != "" {
		profile.CredentialRef = ref
	}
	if locale := strings.TrimSpace(req.Locale); locale != "" {
		profile.PreferredLocales = []string{locale}
	}
	view, err := bridge.AddRepository(ctx, promptrepo.AddRepositoryRequest{Profile: profile})
	if err != nil {
		return promptRepositoryError(command, err)
	}
	projection := domain.NewProjection(command, "Prompt repository configured in the shared store.")
	promptRepositoryFacts(&projection, view)
	projection.Facts["operation_id"] = "promptrepo.repository.add.v1"
	projection.Actions = []domain.Action{{Name: "sync", Command: fmt.Sprintf("pinax prompt repository sync %s --json", shellQuote(view.Profile.ID))}}
	projection.Data = map[string]any{"repository": sanitizeRepositoryView(view)}
	return projection, nil
}

// PromptRepositoryList lists shared repository profiles.
func (s *Service) PromptRepositoryList(ctx context.Context, req PromptRepositoryRequest) (domain.Projection, error) {
	const command = "prompt.repository.list"
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	page, err := bridge.ListRepositories(ctx)
	if err != nil {
		return promptRepositoryError(command, err)
	}
	views := make([]promptrepo.RepositoryView, 0, len(page.Repositories))
	for _, view := range page.Repositories {
		views = append(views, sanitizeRepositoryView(view))
	}
	projection := domain.NewProjection(command, "Shared prompt repositories listed.")
	projection.Facts["results"] = fmt.Sprint(len(views))
	projection.Facts["operation_id"] = "promptrepo.repository.list.v1"
	for i, view := range views {
		prefix := fmt.Sprintf("repository.%d.", i+1)
		projection.Facts[prefix+"id"] = view.Profile.ID
		projection.Facts[prefix+"source_kind"] = view.Profile.SourceKind
		projection.Facts[prefix+"enabled"] = fmt.Sprint(view.Profile.Enabled)
		projection.Facts[prefix+"state"] = view.Health.State
	}
	if len(views) > 0 {
		projection.Actions = []domain.Action{{Name: "search", Command: `pinax prompt catalog search "中文" --json`}}
	}
	projection.Data = map[string]any{"repositories": views}
	return projection, nil
}

// PromptRepositoryShow shows one shared repository profile.
func (s *Service) PromptRepositoryShow(ctx context.Context, req PromptRepositoryRequest) (domain.Projection, error) {
	const command = "prompt.repository.show"
	return s.promptRepositoryDetails(ctx, command, req, "Prompt repository loaded.", "promptrepo.repository.show.v1")
}

// PromptRepositoryDoctor reports repository health for diagnostics.
func (s *Service) PromptRepositoryDoctor(ctx context.Context, req PromptRepositoryRequest) (domain.Projection, error) {
	const command = "prompt.repository.doctor"
	return s.promptRepositoryDetails(ctx, command, req, "Prompt repository diagnostic completed.", "promptrepo.repository.doctor.v1")
}

func (s *Service) promptRepositoryDetails(ctx context.Context, command string, req PromptRepositoryRequest, summary, operationID string) (domain.Projection, error) {
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if strings.TrimSpace(req.ID) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: command + " requires a repository id", Hint: "pinax prompt repository doctor <id> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	view, err := bridge.ShowRepository(ctx, strings.TrimSpace(req.ID))
	if err != nil {
		return promptRepositoryError(command, err)
	}
	projection := domain.NewProjection(command, summary)
	promptRepositoryFacts(&projection, view)
	projection.Facts["state"] = view.Health.State
	if view.Health.Code != "" {
		projection.Facts["health_code"] = view.Health.Code
	}
	projection.Facts["operation_id"] = operationID
	projection.Actions = []domain.Action{{Name: "sync", Command: fmt.Sprintf("pinax prompt repository sync %s --json", shellQuote(view.Profile.ID))}}
	projection.Data = map[string]any{"repository": sanitizeRepositoryView(view)}
	return projection, nil
}

// PromptRepositoryRemove removes a repository profile from the shared store.
func (s *Service) PromptRepositoryRemove(ctx context.Context, req PromptRepositoryRequest) (domain.Projection, error) {
	const command = "prompt.repository.remove"
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if strings.TrimSpace(req.ID) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt repository remove requires a repository id", Hint: "pinax prompt repository remove <id> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	if err := bridge.RemoveRepository(ctx, strings.TrimSpace(req.ID)); err != nil {
		return promptRepositoryError(command, err)
	}
	projection := domain.NewProjection(command, "Prompt repository removed from the shared store.")
	projection.Facts["repository_id"] = strings.TrimSpace(req.ID)
	projection.Facts["operation_id"] = "promptrepo.repository.remove.v1"
	projection.Data = map[string]any{"repository_id": strings.TrimSpace(req.ID)}
	return projection, nil
}

// PromptRepositoryEnable enables a repository profile.
func (s *Service) PromptRepositoryEnable(ctx context.Context, req PromptRepositoryRequest) (domain.Projection, error) {
	return s.promptRepositorySetEnabled(ctx, req, true)
}

// PromptRepositoryDisable disables a repository profile without deleting it.
func (s *Service) PromptRepositoryDisable(ctx context.Context, req PromptRepositoryRequest) (domain.Projection, error) {
	return s.promptRepositorySetEnabled(ctx, req, false)
}

func (s *Service) promptRepositorySetEnabled(ctx context.Context, req PromptRepositoryRequest, enabled bool) (domain.Projection, error) {
	command := "prompt.repository.enable"
	operationID := "promptrepo.repository.enable.v1"
	summary := "Prompt repository enabled."
	if !enabled {
		command = "prompt.repository.disable"
		operationID = "promptrepo.repository.disable.v1"
		summary = "Prompt repository disabled."
	}
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if strings.TrimSpace(req.ID) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: command + " requires a repository id", Hint: fmt.Sprintf("pinax prompt repository %s <id> --json", strings.TrimPrefix(command, "prompt.repository."))}
		return domain.NewErrorProjection(command, err), err
	}
	view, err := bridge.SetRepositoryEnabled(ctx, strings.TrimSpace(req.ID), enabled)
	if err != nil {
		return promptRepositoryError(command, err)
	}
	projection := domain.NewProjection(command, summary)
	promptRepositoryFacts(&projection, view)
	projection.Facts["operation_id"] = operationID
	projection.Data = map[string]any{"repository": sanitizeRepositoryView(view)}
	return projection, nil
}

// PromptRepositorySync refreshes repository snapshots in the shared store.
func (s *Service) PromptRepositorySync(ctx context.Context, req PromptRepositoryRequest) (domain.Projection, error) {
	const command = "prompt.repository.sync"
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if !req.SyncAll && len(req.Repositories) == 0 {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt repository sync requires repository ids or --all", Hint: "pinax prompt repository sync --all --json"}
		return domain.NewErrorProjection(command, err), err
	}
	receipt, err := bridge.SyncRepositories(ctx, promptrepo.SyncRequest{RepositoryIDs: req.Repositories, All: req.SyncAll})
	if err != nil {
		return promptRepositoryError(command, err)
	}
	synced, failed := 0, 0
	for _, result := range receipt.Results {
		if result.ErrorCode == "" && result.State != "" {
			synced++
		} else if result.ErrorCode != "" {
			failed++
		}
	}
	projection := domain.NewProjection(command, "Prompt repository snapshots synchronized.")
	projection.Facts["synced"] = fmt.Sprint(synced)
	projection.Facts["failed"] = fmt.Sprint(failed)
	projection.Facts["operation_id"] = "promptrepo.repository.sync.v1"
	projection.Actions = []domain.Action{{Name: "search", Command: `pinax prompt catalog search "中文" --json`}}
	projection.Data = map[string]any{"receipt": receipt}
	return projection, nil
}

func promptRepositoryFacts(projection *domain.Projection, view promptrepo.RepositoryView) {
	projection.Facts["repository_id"] = view.Profile.ID
	projection.Facts["source_kind"] = view.Profile.SourceKind
	projection.Facts["scope"] = view.Profile.Scope
	projection.Facts["enabled"] = fmt.Sprint(view.Profile.Enabled)
	if view.Profile.Trust != "" {
		projection.Facts["trust"] = view.Profile.Trust
	}
	projection.Facts["credential_configured"] = fmt.Sprint(strings.TrimSpace(view.Profile.CredentialRef) != "")
	if view.Snapshot != nil {
		projection.Facts["snapshot_digest"] = view.Snapshot.Digest
	}
}

// sanitizeRepositoryView strips credential references and embedded userinfo
// from source URIs before anything enters a projection.
func sanitizeRepositoryView(view promptrepo.RepositoryView) promptrepo.RepositoryView {
	sanitized := view
	sanitized.Profile.CredentialRef = ""
	sanitized.Profile.Source = sanitizeSourceURI(view.Profile.Source)
	return sanitized
}

func sanitizeSourceURI(source string) string {
	scheme := strings.Index(source, "://")
	if scheme < 0 {
		return source
	}
	rest := source[scheme+3:]
	authority, path := rest, ""
	if slash := strings.Index(rest, "/"); slash >= 0 {
		authority, path = rest[:slash], rest[slash:]
	}
	if at := strings.Index(authority, "@"); at >= 0 {
		return source[:scheme+3] + "***" + authority[at:] + path
	}
	return source
}

// PromptCatalogSearch searches the federated catalog. The local
// `pinax prompt search` stays vault-only; this is the external surface.
func (s *Service) PromptCatalogSearch(ctx context.Context, req PromptCatalogRequest) (domain.Projection, error) {
	const command = "prompt.catalog.search"
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	locale := promptbridge.EffectiveLocale(req.Locale)
	result, err := bridge.Search(ctx, promptrepo.SearchRequest{Query: strings.TrimSpace(req.Query), Locale: locale, Tags: req.Tags})
	if err != nil {
		return promptRepositoryError(command, err)
	}
	result, denied, err := s.restrictCatalogSearch(ctx, bridge, req, result)
	if err != nil {
		return promptRepositoryError(command, err)
	}
	projection := domain.NewProjection(command, "Federated prompt catalog searched.")
	projection.Facts["results"] = fmt.Sprint(len(result.Results))
	projection.Facts["locale"] = locale
	projection.Facts["operation_id"] = "promptrepo.catalog.search.v1"
	projection.Facts["provider_calls"] = "0"
	projection.Facts["durable_writes"] = "0"
	if req.Query != "" {
		projection.Facts["query"] = req.Query
	}
	if denied > 0 {
		projection.Facts["scope_denied"] = fmt.Sprint(denied)
	}
	for i, card := range result.Results {
		prefix := fmt.Sprintf("catalog_result.%d.", i+1)
		projection.Facts[prefix+"ref"] = card.Ref
		projection.Facts[prefix+"title"] = card.Title
		projection.Facts[prefix+"rights"] = card.Rights
		projection.Facts[prefix+"trust"] = card.Trust
		projection.Facts[prefix+"health"] = card.Health
	}
	if len(result.Results) > 0 {
		projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax prompt catalog show %s --json", shellQuote(result.Results[0].Ref))}}
	}
	projection.Data = map[string]any{"results": result}
	return projection, nil
}

// restrictCatalogSearch filters search cards to the effective repository set
// (session pins/denies; organization and project sources arrive later through
// the same bridge port). Returns the number of scope-denied cards.
func (s *Service) restrictCatalogSearch(ctx context.Context, bridge *promptbridge.Bridge, req PromptCatalogRequest, result promptrepo.SearchResult) (promptrepo.SearchResult, int, error) {
	if len(req.RepositoryIDs) == 0 && len(req.DenyIDs) == 0 {
		return result, 0, nil
	}
	set, err := bridge.EffectiveRepositorySet(ctx, promptbridge.EffectiveScopeOptions{Session: promptbridge.SessionScope{RepositoryIDs: req.RepositoryIDs, Deny: req.DenyIDs}})
	if err != nil {
		return promptrepo.SearchResult{}, 0, err
	}
	allowed := make(map[string]struct{}, len(set.Repositories))
	for _, view := range set.Repositories {
		allowed[view.Profile.ID] = struct{}{}
	}
	filtered := promptrepo.SearchResult{RankingProfile: result.RankingProfile, RequestedLocale: result.RequestedLocale}
	denied := 0
	for _, card := range result.Results {
		repositoryID := repositoryIDFromRef(card.Ref)
		if _, ok := allowed[repositoryID]; !ok {
			denied++
			continue
		}
		filtered.Results = append(filtered.Results, card)
	}
	return filtered, denied, nil
}

func repositoryIDFromRef(ref string) string {
	trimmed := strings.TrimSpace(ref)
	scheme := strings.Index(trimmed, "://")
	if scheme < 0 {
		return ""
	}
	rest := trimmed[scheme+3:]
	if slash := strings.Index(rest, "/"); slash >= 0 {
		return rest[:slash]
	}
	return rest
}

// PromptCatalogShow shows one catalog solution without reading any body.
func (s *Service) PromptCatalogShow(ctx context.Context, req PromptCatalogRequest) (domain.Projection, error) {
	const command = "prompt.catalog.show"
	return s.promptCatalogDetails(ctx, command, "Federated prompt catalog entry shown.", "promptrepo.catalog.show.v1", req)
}

// PromptCatalogResolve resolves an exact ref or template address.
func (s *Service) PromptCatalogResolve(ctx context.Context, req PromptCatalogRequest) (domain.Projection, error) {
	const command = "prompt.catalog.resolve"
	return s.promptCatalogDetails(ctx, command, "Federated prompt catalog ref resolved.", "promptrepo.catalog.resolve.v1", req)
}

func (s *Service) promptCatalogDetails(ctx context.Context, command, summary, operationID string, req PromptCatalogRequest) (domain.Projection, error) {
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if strings.TrimSpace(req.Ref) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: command + " requires a promptrepo:// ref or template address", Hint: "pinax prompt catalog show promptrepo://<repository>/<package>/<solution>@<version> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	locale := promptbridge.EffectiveLocale(req.Locale)
	resolved, err := bridge.Resolve(ctx, promptrepo.ResolveRequest{Ref: strings.TrimSpace(req.Ref), Locale: locale})
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if err := s.ensureRepositoryAllowed(ctx, bridge, req, resolved.Snapshot.RepositoryID); err != nil {
		return promptRepositoryError(command, err)
	}
	projection := domain.NewProjection(command, summary)
	promptCatalogFacts(&projection, resolved)
	projection.Facts["operation_id"] = operationID
	projection.Facts["provider_calls"] = "0"
	projection.Facts["durable_writes"] = "0"
	projection.Actions = []domain.Action{
		{Name: "inspect", Command: fmt.Sprintf("pinax prompt catalog inspect %s --json", shellQuote(resolved.Ref))},
		{Name: "install_plan", Command: fmt.Sprintf("pinax prompt catalog install %s --json", shellQuote(resolved.Ref))},
	}
	projection.Data = map[string]any{"resolved": resolved}
	return projection, nil
}

func promptCatalogFacts(projection *domain.Projection, resolved promptrepo.ResolvedSolution) {
	projection.Facts["ref"] = resolved.Ref
	projection.Facts["repository_id"] = resolved.Snapshot.RepositoryID
	projection.Facts["locale"] = resolved.Locale
	projection.Facts["title"] = resolved.Display.Title
	projection.Facts["rights"] = resolved.Solution.Rights
	projection.Facts["trust"] = resolved.Trust
	projection.Facts["maturity"] = resolved.Solution.Maturity
	projection.Facts["version"] = resolved.Solution.Version
	projection.Facts["compatible"] = fmt.Sprint(resolved.Compatible)
	projection.Facts["snapshot_digest"] = resolved.Snapshot.Digest
	projection.Facts["solution_digest"] = resolved.Solution.Digest
}

// PromptCatalogInspect returns catalog, snapshot, and contract metadata with
// zero provider calls and zero durable writes.
func (s *Service) PromptCatalogInspect(ctx context.Context, req PromptCatalogRequest) (domain.Projection, error) {
	const command = "prompt.catalog.inspect"
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if strings.TrimSpace(req.Ref) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt catalog inspect requires a ref", Hint: "pinax prompt catalog inspect <promptrepo:// ref> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	locale := promptbridge.EffectiveLocale(req.Locale)
	contract, contractVerified := s.resolveCatalogContract(ctx, bridge, req, locale)
	result, err := bridge.Inspect(ctx, promptrepo.InspectRequest{Ref: strings.TrimSpace(req.Ref), Locale: locale, Role: req.Role, Selector: req.Selector, Contract: contract})
	if err != nil {
		return promptRepositoryError(command, err)
	}
	projection := domain.NewProjection(command, "Federated prompt template inspected without provider calls.")
	promptInspectFacts(&projection, result)
	projection.Facts["operation_id"] = "promptrepo.catalog.inspect.v1"
	projection.Facts["contract_verified"] = fmt.Sprint(contractVerified)
	projection.Facts["provider_calls"] = "0"
	projection.Facts["durable_writes"] = "0"
	projection.Data = map[string]any{"inspect": result}
	return projection, nil
}

// PromptCatalogValidate reports input readiness for a template role.
func (s *Service) PromptCatalogValidate(ctx context.Context, req PromptCatalogRequest) (domain.Projection, error) {
	const command = "prompt.catalog.validate"
	if strings.TrimSpace(req.Ref) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt catalog validate requires a ref", Hint: "pinax prompt catalog validate <ref> --values <file> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	values, projection, err := s.promptCatalogValues(command, req)
	if projection != nil {
		return *projection, err
	}
	bridge, bridgeErr := s.promptCatalogBridge()
	if bridgeErr != nil {
		return promptRepositoryError(command, bridgeErr)
	}
	locale := promptbridge.EffectiveLocale(req.Locale)
	contract, contractVerified := s.resolveCatalogContract(ctx, bridge, req, locale)
	validation, err := bridge.Validate(ctx, promptrepo.ValidateRequest{Ref: strings.TrimSpace(req.Ref), Locale: locale, Role: req.Role, Contract: contract, Values: values})
	if err != nil {
		return promptRepositoryError(command, err)
	}
	resultProjection := domain.NewProjection(command, "Federated prompt template inputs validated.")
	resultProjection.Facts["ready"] = fmt.Sprint(validation.Ready)
	resultProjection.Facts["issues"] = fmt.Sprint(len(validation.Issues))
	resultProjection.Facts["inputs"] = fmt.Sprint(len(validation.Inputs))
	resultProjection.Facts["operation_id"] = "promptrepo.catalog.validate.v1"
	resultProjection.Facts["contract_verified"] = fmt.Sprint(contractVerified)
	resultProjection.Facts["provider_calls"] = "0"
	resultProjection.Facts["durable_writes"] = "0"
	resultProjection.Actions = []domain.Action{{Name: "preview", Command: fmt.Sprintf("pinax prompt catalog preview %s --values <file> --json", shellQuote(strings.TrimSpace(req.Ref)))}}
	resultProjection.Data = map[string]any{"validation": validation}
	return resultProjection, nil
}

// PromptCatalogPreview renders in memory: provider_calls=0, durable_writes=0,
// and the rendered body never leaves the bridge.
func (s *Service) PromptCatalogPreview(ctx context.Context, req PromptCatalogRequest) (domain.Projection, error) {
	const command = "prompt.catalog.preview"
	if strings.TrimSpace(req.Ref) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt catalog preview requires a ref", Hint: "pinax prompt catalog preview <ref> --values <file> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	values, projection, err := s.promptCatalogValues(command, req)
	if projection != nil {
		return *projection, err
	}
	bridge, bridgeErr := s.promptCatalogBridge()
	if bridgeErr != nil {
		return promptRepositoryError(command, bridgeErr)
	}
	locale := promptbridge.EffectiveLocale(req.Locale)
	contract, contractVerified := s.resolveCatalogContract(ctx, bridge, req, locale)
	result, err := bridge.Preview(ctx, promptrepo.PreviewRequest{Ref: strings.TrimSpace(req.Ref), Locale: locale, Role: req.Role, Selector: req.Selector, Contract: contract, Values: values})
	if err != nil {
		return promptRepositoryError(command, err)
	}
	resultProjection := domain.NewProjection(command, "Federated prompt template previewed without provider calls.")
	promptInspectFacts(&resultProjection, result.InspectResult)
	resultProjection.Facts["ready"] = fmt.Sprint(result.Ready)
	resultProjection.Facts["rendered_digest"] = result.RenderedDigest
	resultProjection.Facts["rendered_bytes"] = fmt.Sprint(result.RenderedBytes)
	resultProjection.Facts["operation_id"] = "promptrepo.catalog.preview.v1"
	resultProjection.Facts["contract_verified"] = fmt.Sprint(contractVerified)
	resultProjection.Facts["provider_calls"] = "0"
	resultProjection.Facts["durable_writes"] = "0"
	resultProjection.Actions = []domain.Action{{Name: "install_plan", Command: fmt.Sprintf("pinax prompt catalog install %s --json", shellQuote(strings.TrimSpace(req.Ref)))}}
	resultProjection.Data = map[string]any{"preview": result}
	return resultProjection, nil
}

func promptInspectFacts(projection *domain.Projection, result promptrepo.InspectResult) {
	projection.Facts["ref"] = result.Ref
	projection.Facts["address"] = result.Address
	projection.Facts["locale"] = result.Locale
	projection.Facts["role"] = result.Role
	projection.Facts["rights"] = result.Rights
	projection.Facts["trust"] = result.Trust
	projection.Facts["digest"] = result.Digest
	projection.Facts["snapshot_digest"] = result.SnapshotDigest
	projection.Facts["solution_digest"] = result.SolutionDigest
	projection.Facts["next_action"] = result.NextAction.Kind
	if result.NextAction.Kind == "supply_inputs" && len(result.NextAction.RequiredInputs) > 0 {
		projection.Facts["missing_inputs"] = strings.Join(result.NextAction.RequiredInputs, ",")
	}
}

// resolveCatalogContract resolves the companion contract for a ref; a missing
// companion degrades to an unverified empty contract instead of failing the
// read (install still fails closed on unverified rights).
func (s *Service) resolveCatalogContract(ctx context.Context, bridge *promptbridge.Bridge, req PromptCatalogRequest, locale string) (promptrepo.TemplateContract, bool) {
	resolved, err := bridge.ResolveTemplateContract(ctx, promptrepo.ResolveTemplateContractRequest{Ref: strings.TrimSpace(req.Ref), Locale: locale, Role: req.Role})
	if err != nil {
		return promptrepo.TemplateContract{}, false
	}
	return resolved.Contract, true
}

// promptCatalogValues loads bounded input values from --values; the values
// themselves never appear in any projection or error.
func (s *Service) promptCatalogValues(command string, req PromptCatalogRequest) (map[string]any, *domain.Projection, error) {
	if strings.TrimSpace(req.ValuesFile) == "" {
		return map[string]any{}, nil, nil
	}
	content, err := os.ReadFile(req.ValuesFile)
	if err != nil {
		commandErr := &domain.CommandError{Code: "argument_invalid", Message: "prompt catalog values file cannot be read", Hint: "Check the --values file path and retry"}
		projection := domain.NewErrorProjection(command, commandErr)
		return nil, &projection, commandErr
	}
	if len(content) > promptCatalogMaxValuesBytes {
		commandErr := &domain.CommandError{Code: "argument_invalid", Message: "prompt catalog values file exceeds the size limit", Hint: "Split large inputs across runs"}
		projection := domain.NewErrorProjection(command, commandErr)
		return nil, &projection, commandErr
	}
	var values map[string]any
	if err := json.Unmarshal(content, &values); err != nil {
		commandErr := &domain.CommandError{Code: promptbridge.CodeCatalogInputInvalid, Message: "prompt catalog values file is not a JSON object", Hint: "Provide {\"name\": value} input values"}
		projection := domain.NewErrorProjection(command, commandErr)
		return nil, &projection, commandErr
	}
	if values == nil {
		values = map[string]any{}
	}
	return values, nil, nil
}

// ensureRepositoryAllowed checks the effective repository set when session
// pins or denies are present.
func (s *Service) ensureRepositoryAllowed(ctx context.Context, bridge *promptbridge.Bridge, req PromptCatalogRequest, repositoryID string) error {
	if len(req.RepositoryIDs) == 0 && len(req.DenyIDs) == 0 {
		return nil
	}
	set, err := bridge.EffectiveRepositorySet(ctx, promptbridge.EffectiveScopeOptions{Session: promptbridge.SessionScope{RepositoryIDs: req.RepositoryIDs, Deny: req.DenyIDs}})
	if err != nil {
		return err
	}
	for _, view := range set.Repositories {
		if view.Profile.ID == repositoryID {
			return nil
		}
	}
	return &domain.CommandError{Code: promptbridge.CodeCatalogNotFound, Message: "Repository is outside the effective repository set", Hint: "Check session --repository pins and scope denies, then retry"}
}

// PromptCatalogInstall materializes a rights-permitted verified template as a
// Pinax-owned draft PromptAsset. Without --yes it returns the install plan;
// preview-only, no-copy, or unknown-rights templates fail closed.
func (s *Service) PromptCatalogInstall(ctx context.Context, req PromptCatalogRequest) (domain.Projection, error) {
	const command = "prompt.catalog.install"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if strings.TrimSpace(req.Ref) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt catalog install requires a ref", Hint: "pinax prompt catalog install <promptrepo:// ref> --yes --json"}
		return domain.NewErrorProjection(command, err), err
	}
	bridge, err := s.promptCatalogBridge()
	if err != nil {
		return promptRepositoryError(command, err)
	}
	locale := promptbridge.EffectiveLocale(req.Locale)

	// Exact resolve first: the plan must bind to one exact snapshot.
	resolved, err := bridge.Resolve(ctx, promptrepo.ResolveRequest{Ref: strings.TrimSpace(req.Ref), Locale: locale})
	if err != nil {
		return promptRepositoryError(command, err)
	}
	if err := s.ensureRepositoryAllowed(ctx, bridge, req, resolved.Snapshot.RepositoryID); err != nil {
		return promptRepositoryError(command, err)
	}

	// Rights gate: solution rights must not block, and the verified contract
	// must grant a local import/copy permission.
	rights := strings.ToLower(strings.TrimSpace(resolved.Solution.Rights))
	if rights == "blocked" || rights == "prohibited" {
		err := &domain.CommandError{Code: promptbridge.CodeInstallRightsBlocked, Message: "Template rights block local import", Hint: "The catalog entry is preview-only; use pinax prompt catalog preview instead"}
		return domain.NewErrorProjection(command, err), err
	}
	contract, verified := s.resolveCatalogContract(ctx, bridge, req, locale)
	if !verified {
		err := &domain.CommandError{Code: promptbridge.CodeInstallRightsBlocked, Message: "Template contract is unavailable; rights cannot be verified", Hint: "Sync the repository and retry; unknown rights never install"}
		return domain.NewErrorProjection(command, err), err
	}
	if !grantsImportPermission(contract.Permissions) {
		err := &domain.CommandError{Code: promptbridge.CodeInstallRightsBlocked, Message: "Template contract does not grant local import", Hint: "The template is preview-only; request an import permission from the repository owner"}
		return domain.NewErrorProjection(command, err), err
	}
	if len(contract.Inputs) == 0 {
		// The local yeisme.prompt_asset.v1 schema requires at least one
		// variable; static templates fail closed instead of bypassing it.
		err := &domain.CommandError{Code: promptbridge.CodeInstallStaticTemplate, Message: "Template contract declares no inputs; the local prompt asset schema requires at least one variable", Hint: "Use pinax prompt catalog preview for static templates until the contract declares inputs"}
		return domain.NewErrorProjection(command, err), err
	}

	// Verified body read: digest-bound to the resolved catalog template.
	role := promptCatalogRole(req, resolved)
	body, templateDigest, err := s.readVerifiedTemplateBody(ctx, bridge, resolved, role, locale)
	if err != nil {
		return promptRepositoryError(command, err)
	}
	bodyHash := promptCatalogBodyHash(body)

	// Conflict plan before any write: the same local ID with a different
	// digest keeps the existing version unless --fork installs side-by-side.
	localID := promptCatalogLocalID(resolved)
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return promptRepositoryError(command, err)
	}
	existing, existingErr := repo.Resolve(ctx, localID)
	switch {
	case existingErr == nil:
		if existing.PromptTemplateHash == bodyHash {
			projection := domain.NewProjection(command, "Prompt asset already installed from this template.")
			promptAssetFacts(&projection, existing)
			projection.Facts["operation_id"] = "promptrepo.catalog.install.v1"
			projection.Facts["already_installed"] = "true"
			projection.Facts["provider_calls"] = "0"
			projection.Facts["durable_writes"] = "0"
			projection.Data = map[string]any{"prompt_asset_id": localID, "template_digest": templateDigest}
			return projection, nil
		}
		if !req.Fork {
			conflictErr := &domain.CommandError{Code: promptbridge.CodeInstallConflict, Message: "Local prompt asset already exists with different content", Hint: "Keep the existing version, or rerun with --fork to install side-by-side"}
			return promptInstallConflict(command, localID, existing, templateDigest), conflictErr
		}
		localID = localID + "_fork_" + bodyHash[:12]
	case !promptasset.IsNotFound(existingErr):
		return promptRepositoryError(command, existingErr)
	}

	if !req.Confirm {
		projection := domain.NewProjection(command, "Prompt catalog install planned.")
		projection.Facts["prompt_asset_id"] = localID
		projection.Facts["lifecycle"] = "draft"
		projection.Facts["permission"] = permissionFromLicense(contract.License)
		projection.Facts["template_digest"] = templateDigest
		projection.Facts["contract_digest"] = contract.Digest
		projection.Facts["rights"] = resolved.Solution.Rights
		projection.Facts["operation_id"] = "promptrepo.catalog.install.v1"
		projection.Facts["provider_calls"] = "0"
		projection.Facts["durable_writes"] = "0"
		projection.Actions = []domain.Action{{Name: "install", Command: fmt.Sprintf("pinax prompt catalog install %s --yes --vault %s --json", shellQuote(resolved.Ref), shellQuote(root))}}
		projection.Data = map[string]any{"plan": map[string]any{"prompt_asset_id": localID, "lifecycle": "draft", "template_digest": templateDigest, "contract_digest": contract.Digest}}
		return projection, nil
	}

	// Stage receipt first: install provenance requires the shared-store
	// receipt, so a receipt failure fails the install closed.
	receipt, err := bridge.Stage(ctx, promptrepo.StageRequest{ResolveRequest: promptrepo.ResolveRequest{Ref: resolved.Ref, Locale: resolved.Locale}, Consumer: promptbridge.Consumer})
	if err != nil {
		return promptRepositoryError(command, err)
	}

	asset := promptasset.Asset{
		SchemaVersion:  promptasset.SchemaVersion,
		ID:             localID,
		Title:          resolved.Display.Title,
		Domain:         resolved.Solution.Category,
		Lifecycle:      "draft",
		Permission:     permissionFromLicense(contract.License),
		Tags:           append(append([]string{}, resolved.Solution.Tags...), promptCatalogOrigin),
		PromptTemplate: body,
		SourceRefs:     promptCatalogSourceRefs(resolved, contract, templateDigest, receipt),
	}
	asset.Variables, asset.Constraints = contractVariables(contract, locale)
	if strings.TrimSpace(resolved.Display.Usage) != "" {
		asset.ReviewGuidance = resolved.Display.Usage
	} else {
		asset.ReviewGuidance = "Imported from the federated catalog; review before promotion."
	}
	record, err := repo.Create(ctx, asset)
	if err != nil {
		return promptRepositoryError(command, err)
	}

	projection := domain.NewProjection(command, "Federated prompt template installed as a local draft.")
	promptAssetFacts(&projection, record)
	projection.Facts["operation_id"] = "promptrepo.catalog.install.v1"
	projection.Facts["lifecycle"] = "draft"
	projection.Facts["provider_calls"] = "0"
	projection.Facts["durable_writes"] = "1"
	projection.Facts["remote_writes"] = "0"
	projection.Facts["stage_receipt_id"] = receipt.ID
	projection.Actions = []domain.Action{{Name: "resolve", Command: fmt.Sprintf("pinax prompt resolve pinax://prompt/%s --agent", shellQuote(record.PromptAssetID))}}
	projection.Evidence = []string{".pinax/index.sqlite"}
	projection.Data = map[string]any{"prompt_asset": record, "source_refs": asset.SourceRefs, "stage_receipt_id": receipt.ID}
	return projection, nil
}

func promptInstallConflict(command, localID string, existing noteindex.PromptAssetRecord, templateDigest string) domain.Projection {
	projection := domain.NewErrorProjection(command, &domain.CommandError{Code: promptbridge.CodeInstallConflict, Message: "Local prompt asset already exists with different content", Hint: "Keep the existing version, or rerun with --fork to install side-by-side"})
	projection.Facts["conflict"] = "true"
	projection.Facts["prompt_asset_id"] = localID
	projection.Facts["existing_version"] = existing.CurrentVersionID
	projection.Facts["existing_digest"] = existing.PromptTemplateHash
	projection.Facts["incoming_digest"] = templateDigest
	projection.Facts["available_plans"] = "keep-existing,side-by-side,fork-local,reject"
	projection.Facts["selected_plan"] = "reject"
	projection.Data = map[string]any{"conflict": map[string]any{
		"prompt_asset_id":    localID,
		"existing_version":   existing.CurrentVersionID,
		"existing_digest":    existing.PromptTemplateHash,
		"incoming_digest":    templateDigest,
		"available_plans":    []string{"keep-existing", "side-by-side", "fork-local", "reject"},
		"selected_plan":      "reject",
		"existing_unchanged": true,
	}}
	return projection
}

// readVerifiedTemplateBody reads the template body and re-binds it to the
// resolved digest: a body whose digest differs from the catalog never
// installs.
func (s *Service) readVerifiedTemplateBody(ctx context.Context, bridge *promptbridge.Bridge, resolved promptrepo.ResolvedSolution, role, locale string) (string, string, error) {
	content, err := bridge.ReadTemplate(ctx, promptrepo.ReadTemplateRequest{Ref: resolved.Ref, Locale: locale, Role: role})
	if err != nil {
		return "", "", err
	}
	templateDigest := content.Digest
	for _, template := range resolved.Solution.Templates {
		if template.Role == content.Role && template.Locale == content.Locale {
			templateDigest = template.Digest
			break
		}
	}
	if content.Digest != templateDigest {
		return "", "", &domain.CommandError{Code: promptbridge.CodeInstallBodyUnverifiable, Message: "Template body digest does not match the catalog", Hint: "Sync the repository and retry the install"}
	}
	return content.Body, content.Digest, nil
}

func promptCatalogRole(req PromptCatalogRequest, resolved promptrepo.ResolvedSolution) string {
	if role := strings.TrimSpace(req.Role); role != "" {
		return role
	}
	for _, template := range resolved.Solution.Templates {
		if template.Locale == resolved.Locale && template.Role == "main" {
			return "main"
		}
	}
	for _, template := range resolved.Solution.Templates {
		if template.Locale == resolved.Locale {
			return template.Role
		}
	}
	return "main"
}

// promptCatalogLocalID derives the deterministic local prompt asset ID from
// the resolved package and solution.
func promptCatalogLocalID(resolved promptrepo.ResolvedSolution) string {
	normalize := func(value string) string {
		return strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(value)))
	}
	return "catalog_" + normalize(resolved.Solution.PackageID) + "_" + normalize(resolved.Solution.ID)
}

func promptCatalogBodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func grantsImportPermission(permissions []string) bool {
	for _, permission := range permissions {
		if _, ok := importPermissions[strings.TrimSpace(strings.ToLower(permission))]; ok {
			return true
		}
	}
	return false
}

// permissionFromLicense maps a verified contract license onto the local
// permission taxonomy; anything unmapped stays unknown.
func permissionFromLicense(license string) string {
	normalized := strings.ToLower(strings.TrimSpace(license))
	switch {
	case normalized == "":
		return "unknown"
	case strings.Contains(normalized, "mit"), strings.Contains(normalized, "apache"), strings.Contains(normalized, "bsd"), strings.Contains(normalized, "isc"), strings.Contains(normalized, "public domain"):
		return "public"
	case strings.Contains(normalized, "internal"):
		return "internal"
	default:
		return "unknown"
	}
}

// contractVariables maps verified contract inputs onto prompt asset variables
// and human-readable constraints.
func contractVariables(contract promptrepo.TemplateContract, locale string) (map[string]promptasset.Variable, []string) {
	variables := make(map[string]promptasset.Variable, len(contract.Inputs))
	var constraints []string
	for _, input := range contract.Inputs {
		variable := promptasset.Variable{Type: string(input.Type), Required: input.Required}
		if text := input.Descriptions[locale]; text != "" {
			variable.Description = text
		} else if text := input.Labels[locale]; text != "" {
			variable.Description = text
		}
		variables[input.Name] = variable
		if text := constraintText(input); text != "" {
			constraints = append(constraints, input.Name+": "+text)
		}
	}
	return variables, constraints
}

func constraintText(input promptrepo.InputDefinition) string {
	var parts []string
	if len(input.Enum) > 0 {
		values := make([]string, 0, len(input.Enum))
		for _, value := range input.Enum {
			values = append(values, fmt.Sprint(value))
		}
		parts = append(parts, "one of "+strings.Join(values, "|"))
	}
	if input.Min != nil && input.Max != nil {
		parts = append(parts, fmt.Sprintf("range %v..%v", *input.Min, *input.Max))
	} else if input.Min != nil {
		parts = append(parts, fmt.Sprintf("min %v", *input.Min))
	} else if input.Max != nil {
		parts = append(parts, fmt.Sprintf("max %v", *input.Max))
	}
	if input.MinLength != nil && input.MaxLength != nil {
		parts = append(parts, fmt.Sprintf("length %d..%d", *input.MinLength, *input.MaxLength))
	} else if input.MaxLength != nil {
		parts = append(parts, fmt.Sprintf("max length %d", *input.MaxLength))
	}
	if input.Regex != "" {
		parts = append(parts, "pattern "+input.Regex)
	}
	return strings.Join(parts, ", ")
}

// promptCatalogSourceRefs builds the provenance refs: exact template ref,
// snapshot/template digests, contract digest, and the stage receipt.
func promptCatalogSourceRefs(resolved promptrepo.ResolvedSolution, contract promptrepo.TemplateContract, templateDigest string, receipt promptrepo.StageReceipt) []promptasset.SourceRef {
	refs := []promptasset.SourceRef{{
		URI:      resolved.Ref,
		Label:    "promptrepo.template",
		Evidence: "snapshot=" + resolved.Snapshot.Digest + ";template=" + templateDigest,
	}}
	if contract.Digest != "" {
		refs = append(refs, promptasset.SourceRef{URI: "promptrepo://contract/" + strings.TrimPrefix(contract.Digest, "sha256:"), Label: "promptrepo.template_contract", Evidence: "license=" + contract.License})
	}
	refs = append(refs, promptasset.SourceRef{URI: "promptrepo://stage/" + receipt.ID, Label: "promptrepo.stage_receipt", Evidence: "consumer=" + promptbridge.Consumer + ";rights=" + resolved.Solution.Rights})
	return refs
}
