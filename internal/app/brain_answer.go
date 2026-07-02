package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/yeisme/pinax/internal/app/searchops"
	"github.com/yeisme/pinax/internal/domain"
)

type BrainAnswerRequest struct {
	VaultPath string
	Question  string
	Limit     int
}

func (s *Service) BrainAnswerPreview(ctx context.Context, req BrainAnswerRequest) (domain.Projection, error) {
	question := strings.TrimSpace(req.Question)
	if question == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "brain answer requires a question", Hint: "pinax brain answer <question> --vault <vault> --json"}
		return domain.NewErrorProjection("brain.answer", err), err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	searchProjection, err := s.SearchProjection(ctx, SearchRequest{VaultPath: req.VaultPath, Query: question, Limit: limit, Engine: "auto", LazyIndex: "auto"})
	if err != nil {
		return errorProjection("brain.answer", err), err
	}

	contexts := []domain.AgentContext{}
	indexStatus := searchProjection.Facts["index_status"]
	if result, ok := searchProjection.Data.(searchops.Result); ok {
		contexts = append(contexts, result.AgentContexts...)
	}
	bundle := BuildAgentBrainContextBundle(AgentBrainContextBundleRequest{
		Task:     question,
		Contexts: contexts,
		Freshness: domain.AgentBrainFreshness{
			GeneratedFrom: "search_projection",
			IndexStatus:   indexStatus,
		},
		Actions: searchProjection.Actions,
	})

	answer := domain.AgentBrainAnswer{
		SchemaVersion: domain.AgentBrainAnswerSchemaVersion,
		Answer:        brainAnswerSummary(question, bundle),
		Claims:        brainAnswerClaims(bundle.SemanticRefs),
		Sources:       brainAnswerSources(bundle),
		OpenQuestions: brainOpenQuestions(question, bundle),
		NextActions:   bundle.NextActions,
		Cost: domain.AgentBrainCost{
			CostClass:        "none",
			ProviderID:       "extractive",
			Model:            "none",
			LocalOnly:        true,
			NetworkRequired:  false,
			CredentialSource: "none",
			DryRunAvailable:  true,
		},
		BodyExposure:  "bounded_projection",
		ContextBundle: bundle,
	}
	projection := domain.NewProjection("brain.answer", "Brain answer preview generated from bounded evidence.")
	projection.Facts["schema_version"] = domain.AgentBrainAnswerSchemaVersion
	projection.Facts["claims"] = fmt.Sprint(len(answer.Claims))
	projection.Facts["sources"] = fmt.Sprint(len(answer.Sources))
	projection.Facts["cost_class"] = answer.Cost.CostClass
	projection.Facts["provider_id"] = answer.Cost.ProviderID
	projection.Facts["body_exposure"] = answer.BodyExposure
	projection.Actions = answer.NextActions
	projection.Data = answer
	for _, source := range answer.Sources {
		if source.Path != "" {
			projection.Evidence = append(projection.Evidence, source.Path)
		}
	}
	return projection, nil
}

func brainAnswerSummary(question string, bundle domain.AgentBrainContextBundle) string {
	if len(bundle.SemanticRefs) == 0 {
		return "No bounded evidence was found for: " + question
	}
	return fmt.Sprintf("Found %d bounded source(s) for: %s", len(bundle.SemanticRefs), question)
}

func brainAnswerClaims(refs []domain.AgentContextRef) []domain.AgentBrainClaim {
	claims := make([]domain.AgentBrainClaim, 0, len(refs))
	for _, ref := range refs {
		title := strings.TrimSpace(ref.Title)
		if title == "" {
			title = ref.Path
		}
		if title == "" {
			title = ref.ID
		}
		if title == "" {
			continue
		}
		claims = append(claims, domain.AgentBrainClaim{Text: "Relevant bounded source: " + title, Confidence: "source_match", Sources: []domain.AgentContextRef{ref}})
	}
	return claims
}

func brainAnswerSources(bundle domain.AgentBrainContextBundle) []domain.AgentBrainSource {
	refs := append([]domain.AgentContextRef{}, bundle.MemoryRefs...)
	refs = append(refs, bundle.SemanticRefs...)
	refs = append(refs, bundle.GraphRefs...)
	refs = append(refs, bundle.QueryRefs...)
	sources := make([]domain.AgentBrainSource, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		key := ref.Kind + "\x00" + ref.ID + "\x00" + ref.Path + "\x00" + ref.Title
		if seen[key] {
			continue
		}
		seen[key] = true
		sources = append(sources, domain.AgentBrainSource(ref))
	}
	return sources
}

func brainOpenQuestions(question string, bundle domain.AgentBrainContextBundle) []string {
	if len(bundle.SemanticRefs) == 0 && len(bundle.MemoryRefs) == 0 && len(bundle.GraphRefs) == 0 && len(bundle.QueryRefs) == 0 {
		return []string{"No bounded source matched the question: " + question}
	}
	return []string{}
}
