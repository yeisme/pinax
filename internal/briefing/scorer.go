package briefing

import (
	"regexp"
	"sort"
	"strings"
)

var scoreTokenPattern = regexp.MustCompile(`[A-Za-z0-9\p{Han}]+`)

type CandidateScore struct {
	Evidence  EvidenceItem `json:"evidence"`
	Relevance float64      `json:"relevance"`
	Novelty   float64      `json:"novelty"`
	Trust     float64      `json:"trust"`
	Total     float64      `json:"total"`
}

func ScoreEvidence(recipe Recipe, ledger EvidenceLedger, vaultTexts []string) []CandidateScore {
	vaultTerms := terms(strings.Join(vaultTexts, "\n"))
	vaultText := strings.ToLower(strings.Join(vaultTexts, "\n"))
	scores := make([]CandidateScore, 0, len(ledger.Items))
	for _, item := range ledger.Items {
		candidateText := item.Title + " " + item.Summary
		relevance := relevanceScore(candidateText, vaultTerms)
		novelty := noveltyScore(item, vaultText)
		if novelty < 0.5 {
			relevance *= novelty
		}
		trust := item.TrustScore
		total := recipe.Weights.Relevance*relevance + recipe.Weights.Novelty*novelty + recipe.Weights.Trust*trust
		scores = append(scores, CandidateScore{Evidence: item, Relevance: roundScore(relevance), Novelty: roundScore(novelty), Trust: roundScore(trust), Total: roundScore(total)})
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Total == scores[j].Total {
			return scores[i].Evidence.EvidenceID < scores[j].Evidence.EvidenceID
		}
		return scores[i].Total > scores[j].Total
	})
	if recipe.Limit > 0 && len(scores) > recipe.Limit {
		scores = scores[:recipe.Limit]
	}
	return scores
}

func relevanceScore(candidate string, vaultTerms map[string]bool) float64 {
	candidateTerms := terms(candidate)
	if len(candidateTerms) == 0 || len(vaultTerms) == 0 {
		return 0
	}
	overlap := 0
	for term := range candidateTerms {
		if vaultTerms[term] {
			overlap++
		}
	}
	return float64(overlap) / float64(len(candidateTerms))
}

func noveltyScore(item EvidenceItem, vaultText string) float64 {
	title := strings.ToLower(strings.TrimSpace(item.Title))
	if title != "" && strings.Contains(vaultText, title) {
		return 0.1
	}
	return 1
}

func terms(input string) map[string]bool {
	out := map[string]bool{}
	for _, token := range scoreTokenPattern.FindAllString(strings.ToLower(input), -1) {
		if len([]rune(token)) < 2 {
			continue
		}
		out[token] = true
	}
	return out
}

func roundScore(value float64) float64 {
	return float64(int(value*1000+0.5)) / 1000
}
