package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

// ProviderIdentity is the bounded model identity that Pinax may persist in a
// generation manifest. It contains digests and versions only; provider JSON,
// model files and vectors never cross this boundary.
type ProviderIdentity struct {
	BaseModelDigest     string
	ModelManifestDigest string
	ProfileHash         string
	DaemonVersion       string
}

// InspectProviderIdentity resolves the exact local Ollama tag and the
// normalized derived profile. Non-Ollama providers get a deterministic local
// identity marker; they are not contacted by this helper.
func InspectProviderIdentity(ctx context.Context, name, model string) (ProviderIdentity, error) {
	provider, err := NewProviderForBackend(DefaultBackend, name, model)
	if err != nil {
		return ProviderIdentity{}, err
	}
	modelName := provider.Model()
	if provider.Name() != "ollama" {
		seed := provider.Name() + "\x00" + modelName
		return ProviderIdentity{
			ModelManifestDigest: digestToken(seed),
			ProfileHash:         digestToken("profile:" + seed),
			DaemonVersion:       "not_applicable",
		}, nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OLLAMA_HOST")), "/")
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	client := &http.Client{Timeout: 5 * time.Second}
	var version struct {
		Version string `json:"version"`
	}
	if err := ollamaJSON(ctx, client, http.MethodGet, baseURL+"/api/version", nil, &version); err != nil {
		return ProviderIdentity{}, err
	}
	var tags struct {
		Models []struct {
			Name   string `json:"name"`
			Model  string `json:"model"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if err := ollamaJSON(ctx, client, http.MethodGet, baseURL+"/api/tags", nil, &tags); err != nil {
		return ProviderIdentity{}, err
	}
	derivedDigest := ""
	for _, tag := range tags.Models {
		if tag.Name == modelName || tag.Model == modelName {
			derivedDigest = normalizeDigest(tag.Digest)
			break
		}
	}
	if derivedDigest == "" {
		return ProviderIdentity{}, &domain.CommandError{Code: "provider_model_missing", Message: "ollama exact embedding model is not installed", Hint: "Install the exact model tag before staging a generation"}
	}
	showBody, _ := json.Marshal(map[string]string{"name": modelName})
	var show struct {
		Parameters string `json:"parameters"`
		Details    struct {
			ParentModel string `json:"parent_model"`
		} `json:"details"`
	}
	if err := ollamaJSON(ctx, client, http.MethodPost, baseURL+"/api/show", showBody, &show); err != nil {
		return ProviderIdentity{}, err
	}
	baseName := strings.TrimSpace(show.Details.ParentModel)
	if configured := strings.TrimSpace(os.Getenv("PINAX_KB_OLLAMA_BASE_MODEL")); configured != "" {
		baseName = configured
	}
	if baseName == "" && modelName == "pinax-qwen3-embedding:lowmem" {
		baseName = "qwen3-embedding:0.6b"
	}
	baseDigest := ""
	for _, tag := range tags.Models {
		if baseName != "" && (tag.Name == baseName || tag.Model == baseName) {
			baseDigest = normalizeDigest(tag.Digest)
			break
		}
	}
	profileSeed := "model=" + modelName + "\nparameters=" + normalizeOllamaParameters(show.Parameters)
	if profile := strings.TrimSpace(os.Getenv("PINAX_KB_OLLAMA_PROFILE")); profile != "" {
		profileSeed += "\nprofile=" + profile
	}
	return ProviderIdentity{
		BaseModelDigest:     baseDigest,
		ModelManifestDigest: derivedDigest,
		ProfileHash:         digestToken(profileSeed),
		DaemonVersion:       strings.TrimSpace(version.Version),
	}, nil
}

func normalizeOllamaParameters(raw string) string {
	lines := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		lines = append(lines, strings.Join(fields, " "))
	}
	sort.Strings(lines)
	if len(lines) > 0 {
		return strings.Join(lines, " ")
	}
	return strings.Join(strings.Fields(raw), " ")
}

func ollamaJSON(ctx context.Context, client *http.Client, method, endpoint string, body []byte, out any) error {
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return &domain.CommandError{Code: "provider_request_failed", Message: "ollama identity request could not be created", Hint: "Check the loopback Ollama endpoint"}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return ollamaUnavailable()
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerRequestFailed("ollama", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &domain.CommandError{Code: "provider_response_invalid", Message: "ollama identity response was invalid", Hint: "Inspect the exact model tag and retry"}
	}
	return nil
}

func normalizeDigest(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "sha256:") {
		return value
	}
	return "sha256:" + value
}

func digestToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
