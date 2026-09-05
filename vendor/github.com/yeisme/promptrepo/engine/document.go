package engine

import (
	"context"
	"strings"

	"github.com/yeisme/promptrepo"
	"github.com/yeisme/promptrepo/source"
)

var (
	_ promptrepo.DocumentResolver = (*Manager)(nil)
	_ promptrepo.DocumentLoader   = (*Manager)(nil)
	_ promptrepo.DocumentSelector = (*Manager)(nil)
)

func (m *Manager) ResolveDocumentDescriptor(ctx context.Context, request promptrepo.ResolveDocumentDescriptorRequest) (promptrepo.ResolvedDocumentDescriptor, error) {
	details, err := m.resolveTemplateDetails(ctx, request.Ref, request.Locale, request.Role)
	if err != nil {
		return promptrepo.ResolvedDocumentDescriptor{}, err
	}
	return m.resolveDocumentDescriptorForDetails(ctx, details)
}

func (m *Manager) LoadDocument(ctx context.Context, request promptrepo.LoadDocumentRequest) (promptrepo.LoadedDocument, error) {
	details, err := m.resolveTemplateDetails(ctx, request.Ref, request.Locale, request.Role)
	if err != nil {
		return promptrepo.LoadedDocument{}, err
	}
	if details.address.Selector != "" {
		return promptrepo.LoadedDocument{}, promptrepo.NewError(promptrepo.CodeSelectorUnsupported, "LoadDocument does not consume selectors; use SelectDocument", false, nil)
	}
	return m.loadDocumentForDetails(ctx, details)
}

func (m *Manager) SelectDocument(ctx context.Context, request promptrepo.SelectDocumentRequest) (promptrepo.SelectedDocument, error) {
	details, err := m.resolveTemplateDetails(ctx, request.Ref, request.Locale, request.Role)
	if err != nil {
		return promptrepo.SelectedDocument{}, err
	}
	selector := strings.TrimSpace(request.Selector)
	if details.address.Selector != "" {
		if selector != "" && selector != details.address.Selector {
			return promptrepo.SelectedDocument{}, promptrepo.NewError(promptrepo.CodeAddressMismatch, "document request selector does not match the template address", false, nil)
		}
		selector = details.address.Selector
	}
	if selector == "" {
		return promptrepo.SelectedDocument{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "document selector is required", false, nil)
	}
	loaded, err := m.loadDocumentForDetails(ctx, details)
	if err != nil {
		return promptrepo.SelectedDocument{}, err
	}
	return promptrepo.SelectLoadedDocument(loaded, selector)
}

func (m *Manager) resolveDocumentDescriptorForDetails(ctx context.Context, details templateDetails) (promptrepo.ResolvedDocumentDescriptor, error) {
	descriptorPath, err := promptrepo.TemplateDocumentDescriptorPath(details.template)
	if err != nil {
		return promptrepo.ResolvedDocumentDescriptor{}, err
	}
	adapter, err := m.sources.Resolve(details.profile.Source)
	if err != nil {
		return promptrepo.ResolvedDocumentDescriptor{}, err
	}
	reader, ok := adapter.(source.CompanionReader)
	if !ok {
		return promptrepo.ResolvedDocumentDescriptor{}, promptrepo.NewError(promptrepo.CodeUnsupportedSourceScheme, "repository source does not support document descriptors", false, nil)
	}
	payload, err := reader.ReadCompanion(ctx, details.profile, details.resolved.Snapshot, descriptorPath, m.cacheRoot)
	if err != nil {
		if promptrepo.ErrorCode(err) == promptrepo.CodeNotFound {
			return promptrepo.ResolvedDocumentDescriptor{}, promptrepo.NewError(promptrepo.CodeDocumentDescriptorMissing, "document descriptor is not available for the resolved template", false, err)
		}
		return promptrepo.ResolvedDocumentDescriptor{}, err
	}
	document, err := promptrepo.DecodeTemplateDocumentDescriptor(payload)
	if err != nil {
		return promptrepo.ResolvedDocumentDescriptor{}, err
	}
	if err := promptrepo.ValidateTemplateDocumentDescriptorBinding(document, details.resolved.Solution, details.template); err != nil {
		return promptrepo.ResolvedDocumentDescriptor{}, err
	}
	return promptrepo.ResolvedDocumentDescriptor{
		Path: descriptorPath, Consistency: contractConsistency(details.profile.SourceKind), Document: document, Snapshot: details.resolved.Snapshot,
	}, nil
}

func (m *Manager) loadDocumentForDetails(ctx context.Context, details templateDetails) (promptrepo.LoadedDocument, error) {
	resolvedDescriptor, err := m.resolveDocumentDescriptorForDetails(ctx, details)
	if err != nil {
		return promptrepo.LoadedDocument{}, err
	}
	adapter, err := m.sources.Resolve(details.profile.Source)
	if err != nil {
		return promptrepo.LoadedDocument{}, err
	}
	payload, err := adapter.ReadTemplate(ctx, details.profile, details.resolved.Snapshot, details.template, m.cacheRoot)
	if err != nil {
		return promptrepo.LoadedDocument{}, err
	}
	loaded, err := promptrepo.DecodeDocument(resolvedDescriptor.Document, payload)
	if err != nil {
		return promptrepo.LoadedDocument{}, err
	}
	address := details.address
	address.Selector = ""
	loaded.Ref = details.resolved.Ref
	loaded.Address = promptrepo.FormatTemplateAddress(address)
	loaded.DescriptorPath = resolvedDescriptor.Path
	loaded.Consistency = resolvedDescriptor.Consistency
	loaded.Snapshot = details.resolved.Snapshot
	return loaded, nil
}
