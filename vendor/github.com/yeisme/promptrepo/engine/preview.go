package engine

import (
	"context"
	"net/url"
	"strings"

	"github.com/yeisme/promptrepo"
	"github.com/yeisme/promptrepo/source"
)

var (
	_ promptrepo.ContractResolver = (*Manager)(nil)
	_ promptrepo.Inspector        = (*Manager)(nil)
	_ promptrepo.Previewer        = (*Manager)(nil)
	_ promptrepo.Renderer         = (*Manager)(nil)
	_ promptrepo.Validator        = (*Manager)(nil)
)

// ResolveTemplateContract loads and verifies the Registry-authored companion
// sidecar against the exact synchronized snapshot and catalog template.
func (m *Manager) ResolveTemplateContract(ctx context.Context, request promptrepo.ResolveTemplateContractRequest) (promptrepo.ResolvedTemplateContract, error) {
	details, err := m.resolveTemplateDetails(ctx, request.Ref, request.Locale, request.Role)
	if err != nil {
		return promptrepo.ResolvedTemplateContract{}, err
	}
	companionPath, err := promptrepo.TemplateContractCompanionPath(details.template)
	if err != nil {
		return promptrepo.ResolvedTemplateContract{}, err
	}
	adapter, err := m.sources.Resolve(details.profile.Source)
	if err != nil {
		return promptrepo.ResolvedTemplateContract{}, err
	}
	reader, ok := adapter.(source.CompanionReader)
	if !ok {
		return promptrepo.ResolvedTemplateContract{}, promptrepo.NewError(promptrepo.CodeUnsupportedSourceScheme, "repository source does not support template contract companions", false, nil)
	}
	payload, err := reader.ReadCompanion(ctx, details.profile, details.resolved.Snapshot, companionPath, m.cacheRoot)
	if err != nil {
		return promptrepo.ResolvedTemplateContract{}, err
	}
	document, err := promptrepo.DecodeTemplateContractDocument(payload)
	if err != nil {
		return promptrepo.ResolvedTemplateContract{}, err
	}
	if document.PackageID != details.resolved.Solution.PackageID || document.SolutionID != details.resolved.Solution.ID || document.Version != details.resolved.Solution.Version || document.Role != details.template.Role || document.Locale != details.template.Locale {
		return promptrepo.ResolvedTemplateContract{}, promptrepo.NewError(promptrepo.CodeAddressMismatch, "template contract companion identity does not match the resolved template", false, nil)
	}
	if document.TemplatePath != details.template.Path || document.TemplateDigest != details.template.Digest {
		return promptrepo.ResolvedTemplateContract{}, promptrepo.NewError(promptrepo.CodeDigestMismatch, "template contract companion does not match the resolved template body", false, nil)
	}
	consistency := contractConsistency(details.profile.SourceKind)
	return promptrepo.ResolvedTemplateContract{Path: companionPath, Consistency: consistency, Document: document, Contract: document.TemplateContract(), Snapshot: details.resolved.Snapshot}, nil
}

func contractConsistency(sourceKind string) string {
	if sourceKind == "git" {
		return promptrepo.ContractConsistencySnapshotPinned
	}
	return promptrepo.ContractConsistencyContentBound
}

type templateDetails struct {
	resolved promptrepo.ResolvedSolution
	template promptrepo.TemplateRole
	profile  promptrepo.RepositoryProfile
	address  promptrepo.TemplateAddress
}

// Validate reports input readiness without reading the template body.
func (m *Manager) Validate(ctx context.Context, request promptrepo.ValidateRequest) (promptrepo.InputValidation, error) {
	if _, err := m.resolveTemplateDetails(ctx, request.Ref, request.Locale, request.Role); err != nil {
		return promptrepo.InputValidation{}, err
	}
	if err := promptrepo.ValidateTemplateContract(request.Contract); err != nil {
		return promptrepo.InputValidation{}, err
	}
	return promptrepo.ValidateInputValues(request.Contract.Inputs, request.Values), nil
}

// Render performs strict provider-free rendering of caller-provided memory.
func (m *Manager) Render(_ context.Context, request promptrepo.RenderRequest) (promptrepo.RenderResult, error) {
	return promptrepo.RenderTemplate(request)
}

// Inspect returns catalog and snapshot metadata only. It intentionally does
// not call a source adapter's ReadTemplate method.
func (m *Manager) Inspect(ctx context.Context, request promptrepo.InspectRequest) (promptrepo.InspectResult, error) {
	details, err := m.resolveTemplateDetails(ctx, request.Ref, request.Locale, request.Role)
	if err != nil {
		return promptrepo.InspectResult{}, err
	}
	if request.Selector != "" {
		details.address.Selector = request.Selector
		if _, err := promptrepo.ParseTemplateAddress(promptrepo.FormatTemplateAddress(details.address)); err != nil {
			return promptrepo.InspectResult{}, err
		}
	}
	if err := promptrepo.ValidateTemplateContract(request.Contract); err != nil {
		return promptrepo.InspectResult{}, err
	}
	validation := promptrepo.ValidateInputValues(request.Contract.Inputs, request.Values)
	return inspectResult(details, request.Contract, validation), nil
}

// Preview performs a bounded template read and strict in-memory substitution.
// It has no provider, run, state, index, or usage side effects.
func (m *Manager) Preview(ctx context.Context, request promptrepo.PreviewRequest) (promptrepo.PreviewResult, error) {
	details, err := m.resolveTemplateDetails(ctx, request.Ref, request.Locale, request.Role)
	if err != nil {
		return promptrepo.PreviewResult{}, err
	}
	if request.Selector != "" {
		details.address.Selector = request.Selector
		if _, err := promptrepo.ParseTemplateAddress(promptrepo.FormatTemplateAddress(details.address)); err != nil {
			return promptrepo.PreviewResult{}, err
		}
	}
	if details.address.Selector != "" {
		return promptrepo.PreviewResult{}, promptrepo.NewError(promptrepo.CodeSelectorUnsupported, "template selector preview is not supported", false, nil)
	}
	if err := promptrepo.ValidateTemplateContract(request.Contract); err != nil {
		return promptrepo.PreviewResult{}, err
	}
	validation := promptrepo.ValidateInputValues(request.Contract.Inputs, request.Values)
	inspection := inspectResult(details, request.Contract, validation)
	result := promptrepo.PreviewResult{InspectResult: inspection, ProviderCalls: false, StateWrites: false, UsageRecorded: false}
	if !result.Ready {
		return result, nil
	}
	content, err := m.ReadTemplate(ctx, promptrepo.ReadTemplateRequest{Ref: details.resolved.Ref, Locale: details.resolved.Locale, Role: details.template.Role})
	if err != nil {
		return promptrepo.PreviewResult{}, err
	}
	rendered, err := m.Render(ctx, promptrepo.RenderRequest{Template: content.Body, Inputs: request.Contract.Inputs, Values: request.Values})
	if err != nil {
		return promptrepo.PreviewResult{}, err
	}
	result.Inputs = rendered.Inputs
	result.Issues = rendered.Issues
	result.Ready = rendered.Ready
	result.RenderedBody = rendered.RenderedBody
	result.RenderedDigest = rendered.RenderedDigest
	result.RenderedBytes = rendered.RenderedBytes
	result.RenderedRunes = rendered.RenderedRunes
	result.NextAction = nextAction(result.Inputs, result.Issues, result.Ready)
	return result, nil
}

func (m *Manager) resolveTemplateDetails(ctx context.Context, rawRef, requestedLocale, requestedRole string) (templateDetails, error) {
	ref := strings.TrimSpace(rawRef)
	if ref == "" {
		return templateDetails{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "template ref is required", false, nil)
	}
	address, isAddress, err := parseAddressIfPresent(ref)
	if err != nil {
		return templateDetails{}, err
	}
	if isAddress {
		if requestedLocale != "" && requestedLocale != address.Locale {
			return templateDetails{}, promptrepo.NewError(promptrepo.CodeAddressMismatch, "template address locale does not match request", false, nil)
		}
		if requestedRole != "" && address.Role != "" && requestedRole != address.Role {
			return templateDetails{}, promptrepo.NewError(promptrepo.CodeAddressMismatch, "template address role does not match request", false, nil)
		}
		ref = promptrepo.FormatRef(address.Ref())
		requestedLocale = address.Locale
		if address.Role != "" {
			requestedRole = address.Role
		}
	}
	resolved, err := m.Resolve(ctx, promptrepo.ResolveRequest{Ref: ref, Locale: requestedLocale})
	if err != nil {
		return templateDetails{}, err
	}
	role := strings.TrimSpace(requestedRole)
	path := ""
	if isAddress {
		path = address.Path
	}
	if role == "" && path == "" {
		role = "main"
	}
	matches := make([]*promptrepo.TemplateRole, 0, 1)
	for index := range resolved.Solution.Templates {
		template := &resolved.Solution.Templates[index]
		if template.Locale != resolved.Locale || role != "" && template.Role != role || path != "" && template.Path != path {
			continue
		}
		matches = append(matches, template)
	}
	if len(matches) == 0 {
		if isAddress && address.Role != "" && address.Path != "" {
			return templateDetails{}, promptrepo.NewError(promptrepo.CodeAddressMismatch, "template address role and path do not identify the same template", false, nil)
		}
		return templateDetails{}, promptrepo.NewError(promptrepo.CodeNotFound, "prompt template role and locale were not found", false, nil)
	}
	if len(matches) > 1 {
		return templateDetails{}, promptrepo.NewError(promptrepo.CodeAddressMismatch, "template address is ambiguous", false, nil)
	}
	selected := matches[0]
	state, err := m.readState()
	if err != nil {
		return templateDetails{}, err
	}
	profile, ok := state.Profiles[resolved.Snapshot.RepositoryID]
	if !ok || !profile.Enabled {
		return templateDetails{}, promptrepo.NewError(promptrepo.CodeNotFound, "prompt repository is not enabled", false, nil)
	}
	fullAddress := promptrepo.TemplateAddress{RepositoryID: resolved.Snapshot.RepositoryID, PackageID: resolved.Solution.PackageID, SolutionID: resolved.Solution.ID, Version: resolved.Solution.Version, Kind: "template", Locale: resolved.Locale, Role: selected.Role, Path: selected.Path, Digest: selected.Digest, Snapshot: resolved.Snapshot.Digest}
	if isAddress {
		if address.Digest != "" && address.Digest != selected.Digest {
			return templateDetails{}, promptrepo.NewError(promptrepo.CodeAddressMismatch, "template address digest does not match catalog", false, nil)
		}
		if address.Snapshot != "" && address.Snapshot != resolved.Snapshot.Digest {
			return templateDetails{}, promptrepo.NewError(promptrepo.CodeAddressMismatch, "template address snapshot does not match catalog", false, nil)
		}
		fullAddress.Selector = address.Selector
	}
	return templateDetails{resolved: resolved, template: *selected, profile: profile, address: fullAddress}, nil
}

func parseAddressIfPresent(raw string) (promptrepo.TemplateAddress, bool, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return promptrepo.TemplateAddress{}, false, promptrepo.NewError(promptrepo.CodeInvalidRequest, "template ref is invalid", false, err)
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return promptrepo.TemplateAddress{}, false, promptrepo.NewError(promptrepo.CodeInvalidRequest, "template ref query is invalid", false, err)
	}
	for _, key := range []string{"kind", "role", "path", "selector", "digest", "snapshot"} {
		if _, ok := values[key]; ok {
			address, err := promptrepo.ParseTemplateAddress(raw)
			return address, true, err
		}
	}
	return promptrepo.TemplateAddress{}, false, nil
}

func inspectResult(details templateDetails, contract promptrepo.TemplateContract, validation promptrepo.InputValidation) promptrepo.InspectResult {
	result := promptrepo.InspectResult{
		Origin: details.profile.SourceKind, Ref: details.resolved.Ref, Address: promptrepo.FormatTemplateAddress(details.address),
		Version: details.resolved.Solution.Version, Digest: details.template.Digest, SolutionDigest: details.resolved.Solution.Digest,
		SnapshotDigest: details.resolved.Snapshot.Digest, Locale: details.resolved.Locale, Role: details.template.Role,
		Rights: details.resolved.Solution.Rights, Trust: details.resolved.Trust, Maturity: details.resolved.Solution.Maturity, Tags: append([]string{}, details.resolved.Solution.Tags...),
		Capabilities: append([]string{}, details.resolved.Solution.Capabilities...), Title: details.resolved.Display.Title,
		Summary: details.resolved.Display.Summary, Usage: details.resolved.Display.Usage, Inputs: validation.Inputs,
		Contract: contract, Ready: validation.Ready, Issues: append([]promptrepo.InputIssue{}, validation.Issues...),
	}
	if strings.EqualFold(result.Rights, "blocked") || strings.EqualFold(result.Rights, "prohibited") {
		result.Ready = false
		result.Issues = append(result.Issues, promptrepo.InputIssue{Code: promptrepo.CodeRightsBlocked, Message: "template rights block use"})
	}
	result.NextAction = nextAction(result.Inputs, result.Issues, result.Ready)
	return result
}

func nextAction(inputs []promptrepo.InputStatus, issues []promptrepo.InputIssue, ready bool) promptrepo.NextAction {
	if ready {
		return promptrepo.NextAction{Kind: "preview"}
	}
	for _, issue := range issues {
		if issue.Code == promptrepo.CodeRightsBlocked {
			return promptrepo.NextAction{Kind: "blocked"}
		}
	}
	missing := make([]string, 0)
	for _, input := range inputs {
		if input.Status == "missing" && input.Definition.Required {
			missing = append(missing, input.Definition.Name)
		}
	}
	return promptrepo.NextAction{Kind: "supply_inputs", RequiredInputs: missing}
}
