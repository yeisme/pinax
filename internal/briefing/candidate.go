package briefing

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	notebody "github.com/yeisme/pinax/internal/notes"
)

const ReviewQueueSchemaVersion = "pinax.briefing.review_queue.v1"

type ReviewQueue struct {
	SchemaVersion string            `json:"schema_version"`
	CreatedAt     string            `json:"created_at"`
	Items         []ReviewQueueItem `json:"items"`
}

type ReviewQueueItem struct {
	CandidateID string  `json:"candidate_id"`
	EvidenceID  string  `json:"evidence_id"`
	Title       string  `json:"title"`
	Path        string  `json:"path"`
	Score       float64 `json:"score"`
	Status      string  `json:"status"`
}

type GeneratedCandidate struct {
	CandidateID string `json:"candidate_id"`
	Path        string `json:"path"`
	Body        string `json:"body"`
}

func BuildCandidateNotes(recipe Recipe, scores []CandidateScore, backlinks []string) (ReviewQueue, []GeneratedCandidate) {
	items := make([]ReviewQueueItem, 0, len(scores))
	candidates := make([]GeneratedCandidate, 0, len(scores))
	for _, score := range scores {
		candidateID := "brief_" + safeSlug(score.Evidence.EvidenceID+"-"+score.Evidence.Title)
		rel := filepath.ToSlash(filepath.Join("notes", "briefing", candidateID+".md"))
		body := notebody.RenderBriefingCandidateMarkdown(notebody.BriefingCandidate{Title: score.Evidence.Title, URL: score.Evidence.CanonicalURL, Summary: score.Evidence.Summary, Topic: recipe.Topic, Tags: recipe.Output.Tags, Backlinks: backlinks})
		items = append(items, ReviewQueueItem{CandidateID: candidateID, EvidenceID: score.Evidence.EvidenceID, Title: score.Evidence.Title, Path: rel, Score: score.Total, Status: "pending_review"})
		candidates = append(candidates, GeneratedCandidate{CandidateID: candidateID, Path: rel, Body: body})
	}
	return ReviewQueue{SchemaVersion: ReviewQueueSchemaVersion, CreatedAt: time.Now().UTC().Format(time.RFC3339), Items: items}, candidates
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func safeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "candidate"
	}
	if len(value) > 64 {
		value = value[:64]
		value = strings.Trim(value, "-")
	}
	return value
}
