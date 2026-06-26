package memory

import (
	"sort"
	"strings"
	"time"
)

type Candidate struct {
	Record  Record
	FTSRank int
}

type SignalBreakdown struct {
	KeywordFTS      bool   `json:"keyword_fts,omitempty"`
	KeywordField    string `json:"keyword_field,omitempty"`
	SourceKind      string `json:"source_kind,omitempty"`
	SourceAuthority int    `json:"source_authority,omitempty"`
	Confidence      int    `json:"confidence,omitempty"`
	Freshness       int    `json:"freshness,omitempty"`
	TaskFitness     int    `json:"task_fitness,omitempty"`
}

type ScoredCandidate struct {
	Record       Record
	RecallReason string
	Score        int
	Signals      SignalBreakdown
}

type Scorer struct {
	Now time.Time
}

func (s Scorer) Score(filter RecallFilter, candidates []Candidate, ftsRank map[string]int, limit int) []RecallHit {
	if limit <= 0 {
		limit = 8
	}
	now := s.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if ftsRank == nil {
		ftsRank = map[string]int{}
	}
	terms := tokenize(filter.Query)
	superseded := map[string]bool{}
	for _, candidate := range candidates {
		if id := strings.TrimSpace(candidate.Record.SupersedesID); id != "" {
			superseded[id] = true
		}
	}

	scored := make([]RecallHit, 0, len(candidates))
	for _, candidate := range candidates {
		record := candidate.Record
		if superseded[record.ID] {
			continue
		}
		score := 0
		signals := SignalBreakdown{}
		reasons := []string{"status:" + record.Status}
		if strings.TrimSpace(filter.Entity) != "" {
			score += 30
			reasons = append(reasons, "entity_match:"+strings.ToLower(strings.TrimSpace(filter.Entity)))
		}
		if strings.TrimSpace(filter.Type) != "" {
			score += 10
			reasons = append(reasons, "type:"+record.Type)
		}
		rank := candidate.FTSRank
		if rank == 0 {
			rank = ftsRank[record.ID]
		}
		if rank > 0 {
			score += 40 + rank
			signals.KeywordFTS = true
			reasons = append(reasons, "keyword:fts", "keyword_match:fts")
		}
		field, fieldScore := keywordFieldSignal(record, terms)
		if fieldScore > 0 {
			score += fieldScore
			signals.KeywordField = field
			reasons = append(reasons, "field:"+field)
		} else if len(terms) > 0 && rank == 0 {
			continue
		}
		if record.SourceURI != "" {
			kind := sourceKind(record.SourceURI)
			authority := sourceAuthority(kind)
			score += authority
			signals.SourceKind = kind
			signals.SourceAuthority = authority
			reasons = append(reasons, "source:"+kind)
		}
		confidence := confidenceWeight(record.Confidence)
		if confidence > 0 {
			score += confidence
			signals.Confidence = confidence
			reasons = append(reasons, "confidence:"+strings.ToLower(defaultString(record.Confidence, "confirmed")))
		}
		freshness := freshnessWeight(record, now)
		if freshness > 0 {
			score += freshness
			signals.Freshness = freshness
			reasons = append(reasons, "freshness:recent")
		}
		fitness := taskFitness(record, terms)
		if fitness > 0 {
			score += fitness
			signals.TaskFitness = fitness
			reasons = append(reasons, "task_fitness")
		}
		scored = append(scored, RecallHit{Record: record, RecallReason: strings.Join(reasons, " + "), Score: score, Signals: signals})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		if scored[i].Signals.SourceAuthority != scored[j].Signals.SourceAuthority {
			return scored[i].Signals.SourceAuthority > scored[j].Signals.SourceAuthority
		}
		if !scored[i].Record.CreatedAt.Equal(scored[j].Record.CreatedAt) {
			return scored[i].Record.CreatedAt.After(scored[j].Record.CreatedAt)
		}
		return scored[i].Record.ID < scored[j].Record.ID
	})

	collapsed := make([]RecallHit, 0, len(scored))
	seenSubjectPredicate := map[string]bool{}
	for _, hit := range scored {
		key := strings.ToLower(strings.TrimSpace(hit.Record.Subject)) + "\x00" + strings.ToLower(strings.TrimSpace(hit.Record.Predicate))
		if hit.Record.Status == StatusConfirmed && key != "\x00" {
			if seenSubjectPredicate[key] {
				continue
			}
			seenSubjectPredicate[key] = true
		}
		collapsed = append(collapsed, hit)
		if len(collapsed) >= limit {
			break
		}
	}
	return collapsed
}

func keywordFieldSignal(record Record, terms []string) (string, int) {
	if len(terms) == 0 {
		return "", 0
	}
	fields := []struct {
		name  string
		value string
		score int
	}{
		{"predicate", record.Predicate, 30},
		{"object", record.Object, 28},
		{"subject", record.Subject, 24},
		{"body", record.Body, 12},
	}
	for _, field := range fields {
		value := strings.ToLower(field.value)
		if value == "" {
			continue
		}
		for _, term := range terms {
			if strings.Contains(value, term) {
				return field.name, field.score
			}
		}
	}
	return "", 0
}

func sourceAuthority(kind string) int {
	switch kind {
	case "openspec":
		return 20
	case "docs":
		return 15
	case "github_actions":
		return 12
	case "file":
		return 5
	default:
		return 1
	}
}

func confidenceWeight(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "certain", "high":
		return 10
	case "confirmed", "medium", "normal":
		return 5
	case "low", "unknown":
		return 1
	case "":
		return 5
	default:
		return 1
	}
}

func freshnessWeight(record Record, now time.Time) int {
	if record.Type != TypeEvent && record.Type != TypeTask {
		return 0
	}
	age := now.Sub(record.CreatedAt)
	switch {
	case age < 0:
		return 0
	case age <= 24*time.Hour:
		return 10
	case age <= 7*24*time.Hour:
		return 6
	default:
		return 0
	}
}

func taskFitness(record Record, terms []string) int {
	if len(terms) == 0 {
		return 0
	}
	topics := map[string]bool{"release": true, "test": true, "provider": true, "cloud": true, "kb": true, "memory": true}
	haystack := strings.ToLower(strings.Join([]string{record.Type, record.Subject, record.Predicate, record.Object, record.Body, record.SourceURI}, " "))
	for _, term := range terms {
		if topics[term] && strings.Contains(haystack, term) {
			return 6
		}
	}
	return 0
}
