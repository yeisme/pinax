package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/promptasset"
)

// Prompt asset operations: import/create/search/show/resolve/lifecycle and feedback
// import, with supporting prompt-asset facts and feedback payload decoding.
// Extracted from service.go to isolate the prompt-asset surface.

func (s *Service) PromptImport(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	return s.promptImport(ctx, req, "prompt.import")
}

func (s *Service) PromptCreate(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	return s.promptImport(ctx, req, "prompt.create")
}

func (s *Service) PromptSearch(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("prompt.search", err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection("prompt.search", err), err
	}
	assets, err := repo.Search(ctx, promptasset.SearchRequest{Query: req.Query, Domain: req.Domain, Tag: req.Tag, Lifecycle: req.Lifecycle, Limit: req.Limit})
	if err != nil {
		return errorProjection("prompt.search", err), err
	}
	projection := domain.NewProjection("prompt.search", "Prompt asset search completed.")
	projection.Facts["results"] = fmt.Sprint(len(assets))
	if req.Query != "" {
		projection.Facts["query"] = req.Query
	}
	if req.Domain != "" {
		projection.Facts["domain"] = req.Domain
	}
	for i, asset := range assets {
		prefix := fmt.Sprintf("prompt_asset.%d.", i+1)
		projection.Facts[prefix+"id"] = asset.PromptAssetID
		projection.Facts[prefix+"lifecycle"] = asset.Lifecycle
		projection.Facts[prefix+"permission"] = asset.Permission
	}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"prompt_assets": assets}
	return projection, nil
}

func (s *Service) PromptShow(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	return s.promptDetails(ctx, req, "prompt.show")
}

func (s *Service) PromptResolve(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	return s.promptDetails(ctx, req, "prompt.resolve")
}

func (s *Service) PromptLifecycle(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("prompt.lifecycle", err), err
	}
	if strings.TrimSpace(req.Ref) == "" || strings.TrimSpace(req.To) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt lifecycle requires an asset id and --to", Hint: "pinax prompt lifecycle <id> --to tested --reason <reason> --vault <vault> --json"}
		return domain.NewErrorProjection("prompt.lifecycle", err), err
	}
	if strings.TrimSpace(req.Reason) == "" {
		err := &domain.CommandError{Code: "reason_required", Message: "prompt lifecycle requires --reason", Hint: "Record why Pinax changed the lifecycle"}
		return domain.NewErrorProjection("prompt.lifecycle", err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection("prompt.lifecycle", err), err
	}
	record, err := repo.UpdateLifecycle(ctx, req.Ref, req.To)
	if err != nil {
		if promptasset.IsNotFound(err) {
			err := &domain.CommandError{Code: "prompt_asset_not_found", Message: "Prompt asset not found", Hint: fmt.Sprintf("pinax prompt search %s --vault %s --json", shellQuote(req.Ref), shellQuote(root))}
			return domain.NewErrorProjection("prompt.lifecycle", err), err
		}
		err := &domain.CommandError{Code: "prompt_lifecycle_invalid", Message: err.Error(), Hint: "Use draft, tested, accepted, promoted, or retired"}
		return domain.NewErrorProjection("prompt.lifecycle", err), err
	}
	feedbackID := lifecycleFeedbackID(record.PromptAssetID, req.To, req.Reason)
	_, _ = repo.ImportFeedback(ctx, promptasset.Feedback{FeedbackID: feedbackID, PromptAssetID: record.PromptAssetID, VersionID: record.CurrentVersionID, PromptTemplateHash: record.PromptTemplateHash, ExternalRunRef: "pinax://prompt/lifecycle", Decision: req.To, Reason: req.Reason})
	projection := domain.NewProjection("prompt.lifecycle", "Prompt asset lifecycle updated.")
	promptAssetFacts(&projection, record)
	projection.Facts["reason_recorded"] = "true"
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"prompt_asset": record, "feedback_id": feedbackID}
	return projection, nil
}

func (s *Service) PromptFeedbackImport(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("prompt.feedback.import", err), err
	}
	if strings.TrimSpace(req.From) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt feedback import requires --from", Hint: "pinax prompt feedback import --from <file> --vault <vault> --json"}
		return domain.NewErrorProjection("prompt.feedback.import", err), err
	}
	content, err := os.ReadFile(req.From)
	if err != nil {
		err := &domain.CommandError{Code: "prompt_feedback_source_unreadable", Message: "Prompt feedback source cannot be read", Hint: "Check the --from file path and retry"}
		return domain.NewErrorProjection("prompt.feedback.import", err), err
	}
	var payload promptFeedbackPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		err := &domain.CommandError{Code: "prompt_feedback_invalid", Message: err.Error(), Hint: "Provide a valid Eikona prompt usage feedback JSON file"}
		return domain.NewErrorProjection("prompt.feedback.import", err), err
	}
	feedback, commandErr := payload.toFeedback()
	if commandErr != nil {
		return domain.NewErrorProjection("prompt.feedback.import", commandErr), commandErr
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection("prompt.feedback.import", err), err
	}
	if _, err := repo.Resolve(ctx, feedback.PromptAssetID); err != nil {
		if promptasset.IsNotFound(err) {
			err := &domain.CommandError{Code: "prompt_asset_not_found", Message: "Prompt asset not found", Hint: fmt.Sprintf("pinax prompt import --from <asset-file> --vault %s --json", shellQuote(root))}
			return domain.NewErrorProjection("prompt.feedback.import", err), err
		}
		return errorProjection("prompt.feedback.import", err), err
	}
	result, err := repo.ImportFeedback(ctx, feedback)
	if err != nil {
		return errorProjection("prompt.feedback.import", err), err
	}
	projection := domain.NewProjection("prompt.feedback.import", "Prompt usage feedback imported.")
	projection.Facts["prompt_asset_id"] = result.Record.PromptAssetID
	projection.Facts["feedback_id"] = result.Record.FeedbackID
	projection.Facts["imported"] = fmt.Sprint(result.Imported)
	projection.Facts["decision"] = result.Record.Decision
	if result.Record.Decision != "" {
		projection.Actions = []domain.Action{{Name: "lifecycle", Command: fmt.Sprintf("pinax prompt lifecycle %s --to %s --reason <reason> --json", shellQuote(result.Record.PromptAssetID), shellQuote(result.Record.Decision))}}
	}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"feedback": result.Record, "imported": result.Imported}
	return projection, nil
}

func (s *Service) promptImport(ctx context.Context, req PromptAssetRequest, command string) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if strings.TrimSpace(req.From) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt import requires --from", Hint: "pinax prompt import --from <file> --vault <vault> --json"}
		if command == "prompt.create" {
			err.Message = "prompt create requires --from"
			err.Hint = "pinax prompt create --from <file> --vault <vault> --json"
		}
		return domain.NewErrorProjection(command, err), err
	}
	content, err := os.ReadFile(req.From)
	if err != nil {
		err := &domain.CommandError{Code: "prompt_asset_source_unreadable", Message: "Prompt asset source cannot be read", Hint: "Check the --from file path and retry"}
		return domain.NewErrorProjection(command, err), err
	}
	asset, err := promptasset.Load(content)
	if err != nil {
		err := &domain.CommandError{Code: "prompt_asset_invalid", Message: err.Error(), Hint: "Provide a valid yeisme.prompt_asset.v1 YAML file"}
		return domain.NewErrorProjection(command, err), err
	}
	if err := promptasset.Validate(asset); err != nil {
		err := &domain.CommandError{Code: "prompt_asset_invalid", Message: err.Error(), Hint: "Fix required prompt asset fields and retry"}
		return domain.NewErrorProjection(command, err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	record, err := repo.Create(ctx, asset)
	if err != nil {
		return errorProjection(command, err), err
	}
	projection := domain.NewProjection(command, "Prompt asset imported.")
	if command == "prompt.create" {
		projection.Summary = "Prompt asset created."
	}
	promptAssetFacts(&projection, record)
	projection.Actions = []domain.Action{{Name: "resolve", Command: fmt.Sprintf("pinax prompt resolve pinax://prompt/%s --agent", shellQuote(record.PromptAssetID))}}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"prompt_asset": record}
	return projection, nil
}

func (s *Service) promptDetails(ctx context.Context, req PromptAssetRequest, command string) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if strings.TrimSpace(req.Ref) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: command + " requires a prompt asset id or URI", Hint: "pinax prompt show <id> --vault <vault> --json"}
		if command == "prompt.resolve" {
			err.Hint = "pinax prompt resolve pinax://prompt/<id> --vault <vault> --agent"
		}
		return domain.NewErrorProjection(command, err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	details, err := repo.Details(ctx, req.Ref)
	if err != nil {
		if promptasset.IsNotFound(err) {
			err := &domain.CommandError{Code: "prompt_asset_not_found", Message: "Prompt asset not found", Hint: fmt.Sprintf("pinax prompt search %s --vault %s --json", shellQuote(req.Ref), shellQuote(root))}
			return domain.NewErrorProjection(command, err), err
		}
		return errorProjection(command, err), err
	}
	projection := domain.NewProjection(command, "Prompt asset details read.")
	if command == "prompt.resolve" {
		projection.Summary = "Prompt asset resolved."
	}
	promptAssetFacts(&projection, details.Asset)
	projection.Actions = []domain.Action{{Name: "resolve", Command: fmt.Sprintf("pinax prompt resolve pinax://prompt/%s --agent", shellQuote(details.Asset.PromptAssetID))}}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"prompt_asset": details.Asset, "version": details.Version, "source_refs": details.SourceRefs}
	return projection, nil
}

func promptAssetFacts(projection *domain.Projection, asset noteindex.PromptAssetRecord) {
	projection.Facts["prompt_asset_id"] = asset.PromptAssetID
	projection.Facts["version"] = asset.CurrentVersionID
	projection.Facts["lifecycle"] = asset.Lifecycle
	projection.Facts["permission"] = asset.Permission
	projection.Facts["domain"] = asset.Domain
	if asset.OwnerProject != "" {
		projection.Facts["owner_project"] = asset.OwnerProject
	}
}

type promptFeedbackPayload struct {
	SchemaVersion      string   `json:"schema_version"`
	FeedbackID         string   `json:"feedback_id"`
	PromptAssetID      string   `json:"prompt_asset_id"`
	VersionID          string   `json:"version_id"`
	PromptTemplateHash string   `json:"prompt_template_hash"`
	ExternalRunRef     string   `json:"external_run_ref"`
	Decision           string   `json:"decision"`
	Reason             string   `json:"reason"`
	ArtifactRefs       []string `json:"artifact_refs"`
}

func (p promptFeedbackPayload) toFeedback() (promptasset.Feedback, *domain.CommandError) {
	if strings.TrimSpace(p.FeedbackID) == "" {
		return promptasset.Feedback{}, &domain.CommandError{Code: "prompt_feedback_invalid", Message: "feedback_id is required", Hint: "Add feedback_id to the feedback record"}
	}
	if strings.TrimSpace(p.PromptAssetID) == "" {
		return promptasset.Feedback{}, &domain.CommandError{Code: "prompt_feedback_invalid", Message: "prompt_asset_id is required", Hint: "Add prompt_asset_id to the feedback record"}
	}
	return promptasset.Feedback{FeedbackID: p.FeedbackID, PromptAssetID: p.PromptAssetID, VersionID: p.VersionID, PromptTemplateHash: p.PromptTemplateHash, ExternalRunRef: p.ExternalRunRef, Decision: p.Decision, Reason: p.Reason, ArtifactRefs: p.ArtifactRefs}, nil
}
