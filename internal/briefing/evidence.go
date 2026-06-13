package briefing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const EvidenceLedgerSchemaVersion = "pinax.briefing.evidence.v1"

type EvidenceLedger struct {
	SchemaVersion string         `json:"schema_version"`
	CreatedAt     string         `json:"created_at"`
	Items         []EvidenceItem `json:"items"`
	Duplicates    int            `json:"duplicates"`
}

type EvidenceItem struct {
	EvidenceID   string  `json:"evidence_id"`
	SourceID     string  `json:"source_id"`
	URL          string  `json:"url"`
	CanonicalURL string  `json:"canonical_url"`
	Title        string  `json:"title"`
	Summary      string  `json:"summary"`
	PublishedAt  string  `json:"published_at,omitempty"`
	TrustHint    float64 `json:"trust_hint,omitempty"`
	TrustScore   float64 `json:"trust_score"`
}

func WriteEvidence(root string, items []EvidenceItem) (EvidenceLedger, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return EvidenceLedger{}, err
	}
	ledger := BuildEvidenceLedger(items)
	path := filepath.Join(root, ".pinax", "briefing", "evidence.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return EvidenceLedger{}, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return EvidenceLedger{}, err
	}
	for _, item := range ledger.Items {
		b, err := json.Marshal(item)
		if err != nil {
			_ = file.Close()
			return EvidenceLedger{}, err
		}
		if _, err := file.Write(append(b, '\n')); err != nil {
			_ = file.Close()
			return EvidenceLedger{}, err
		}
	}
	if err := file.Close(); err != nil {
		return EvidenceLedger{}, err
	}
	return ledger, nil
}

func BuildEvidenceLedger(items []EvidenceItem) EvidenceLedger {
	seen := map[string]bool{}
	out := make([]EvidenceItem, 0, len(items))
	duplicates := 0
	for _, item := range items {
		item.CanonicalURL = CanonicalURL(item.URL)
		key := item.CanonicalURL
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(item.Title))
		}
		if seen[key] {
			duplicates++
			continue
		}
		seen[key] = true
		item.EvidenceID = evidenceID(key)
		item.TrustScore = SourceTrust(item.SourceID, item.TrustHint)
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CanonicalURL < out[j].CanonicalURL })
	return EvidenceLedger{SchemaVersion: EvidenceLedgerSchemaVersion, CreatedAt: time.Now().UTC().Format(time.RFC3339), Items: out, Duplicates: duplicates}
}

func SourceTrust(sourceID string, hint float64) float64 {
	if hint > 0 {
		if hint > 1 {
			return 1
		}
		return hint
	}
	switch {
	case strings.HasPrefix(sourceID, "user:"):
		return 1
	case strings.HasPrefix(sourceID, "hermes:"):
		return 0.8
	case strings.HasPrefix(sourceID, "fake:"):
		return 0.6
	default:
		return 0.4
	}
}

func CanonicalURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimSpace(raw)
	}
	q := u.Query()
	for key := range q {
		if strings.HasPrefix(strings.ToLower(key), "utm_") {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	u.Fragment = ""
	return u.String()
}

func evidenceID(key string) string {
	h := sha256.Sum256([]byte(key))
	return "ev_" + hex.EncodeToString(h[:])[:16]
}

func FakeEvidence(recipe Recipe) []EvidenceItem {
	sourceID := "fake:default"
	if len(recipe.Sources) > 0 {
		sourceID = recipe.Sources[0].ID
	}
	topic := strings.TrimSpace(recipe.Topic)
	if topic == "" {
		topic = "AI research"
	}
	return []EvidenceItem{
		{SourceID: sourceID, URL: "https://fake.pinax.local/hot/agent-workflow", Title: topic + " for agent workflow", Summary: "Local markdown notes, agent workflow, tooling and review queue practices."},
		{SourceID: sourceID, URL: "https://fake.pinax.local/hot/research", Title: topic + " research digest", Summary: "New evidence and source trust signals for daily briefing."},
		{SourceID: sourceID, URL: "https://fake.pinax.local/hot/ops", Title: "Operational note hygiene", Summary: "Vault maintenance, metadata completeness and daily review."},
	}
}
