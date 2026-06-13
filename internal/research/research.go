package research

import (
	"fmt"
	"strings"
	"time"
)

const ResearchResponseSchemaVersion = "pinax.research.response.v1"

type Adapter interface {
	Search(req ResearchRequest) (ResearchResponse, error)
}

type ResearchRequest struct {
	Topic        string   `json:"topic"`
	Limit        int      `json:"limit"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type ResearchResponse struct {
	SchemaVersion string     `json:"schema_version"`
	Provider      string     `json:"provider"`
	Topic         string     `json:"topic"`
	GeneratedAt   string     `json:"generated_at"`
	Evidence      []Evidence `json:"evidence"`
}

type Evidence struct {
	URL       string  `json:"url"`
	Title     string  `json:"title"`
	Summary   string  `json:"summary"`
	SourceID  string  `json:"source_id"`
	TrustHint float64 `json:"trust_hint,omitempty"`
}

type FakeAdapter struct {
	fixture []Evidence
}

type HermesConfig struct {
	Endpoint   string
	Capability string
}

type HermesAdapter struct {
	config   HermesConfig
	fallback Adapter
}

func NewFakeAdapter(fixture []Evidence) *FakeAdapter {
	return &FakeAdapter{fixture: fixture}
}

func (a *FakeAdapter) Search(req ResearchRequest) (ResearchResponse, error) {
	if err := validateRequest(req); err != nil {
		return ResearchResponse{}, err
	}
	fixture := append([]Evidence(nil), a.fixture...)
	if len(fixture) == 0 {
		fixture = defaultFakeEvidence(req.Topic)
	}
	if req.Limit > 0 && len(fixture) > req.Limit {
		fixture = fixture[:req.Limit]
	}
	for i := range fixture {
		if fixture[i].SourceID == "" {
			fixture[i].SourceID = "fake:research"
		}
	}
	return ResearchResponse{SchemaVersion: ResearchResponseSchemaVersion, Provider: "fake", Topic: req.Topic, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Evidence: fixture}, nil
}

func NewHermesAdapter(config HermesConfig, fallback Adapter) *HermesAdapter {
	if fallback == nil {
		fallback = NewFakeAdapter(nil)
	}
	return &HermesAdapter{config: config, fallback: fallback}
}

func (a *HermesAdapter) Search(req ResearchRequest) (ResearchResponse, error) {
	if err := validateRequest(req); err != nil {
		return ResearchResponse{}, err
	}
	if strings.TrimSpace(a.config.Endpoint) == "" {
		return a.fallback.Search(req)
	}
	// MVP adapter contract only; real Hermes HTTP integration is owned outside this package.
	return a.fallback.Search(req)
}

func validateRequest(req ResearchRequest) error {
	if strings.TrimSpace(req.Topic) == "" {
		return fmt.Errorf("research topic required")
	}
	if req.Limit < 0 || req.Limit > 50 {
		return fmt.Errorf("research limit must be between 0 and 50")
	}
	return nil
}

func defaultFakeEvidence(topic string) []Evidence {
	return []Evidence{
		{URL: "https://fake.pinax.local/research/agent-workflow", Title: topic + " agent workflow", Summary: "Fake Hermes fixture for local briefing development.", SourceID: "fake:research", TrustHint: 0.6},
		{URL: "https://fake.pinax.local/research/vault", Title: topic + " vault review", Summary: "Local-first note review and evidence workflow.", SourceID: "fake:research", TrustHint: 0.6},
	}
}
