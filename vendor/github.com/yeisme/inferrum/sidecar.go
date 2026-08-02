package inferrum

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Sidecar protocol version shared by every Inferrum consumer.
const (
	SidecarSchema            = "inferrum.sidecar.v1"
	DefaultSidecarExecutable = "inferrum-lancedb-sidecar"
)

// SidecarConfig controls the external LanceDB sidecar process.
type SidecarConfig struct {
	Executable string
	Timeout    time.Duration
}

// SidecarClient is the domain-multiplexed sidecar protocol client. It speaks
// inferrum.sidecar.v1 over stdin/stdout JSON with the inferrum-lancedb-sidecar
// process. The sidecar treats record metadata as opaque JSON — permission
// filtering is done by the caller (via the Domain adapter), never in the sidecar.
type SidecarClient struct {
	cfg SidecarConfig
}

// NewSidecarClient creates a sidecar protocol client.
func NewSidecarClient(cfg SidecarConfig) *SidecarClient {
	return &SidecarClient{cfg: cfg}
}

type sidecarRequest struct {
	SchemaVersion  string    `json:"schema_version"`
	Op             string    `json:"op,omitempty"`
	StoreURI       string    `json:"store_uri"`
	Domain         string    `json:"domain,omitempty"`
	Table          string    `json:"table,omitempty"`
	Backend        string    `json:"backend,omitempty"`
	DistanceMetric string    `json:"distance_metric,omitempty"`
	EmbeddingDim   int       `json:"embedding_dim,omitempty"`
	Records        []Record  `json:"records,omitempty"`
	QueryVector    []float64 `json:"query_vector,omitempty"`
	AllowedIDs     []string  `json:"allowed_ids,omitempty"`
	Limit          int       `json:"limit,omitempty"`
}

type sidecarResponse struct {
	SchemaVersion string          `json:"schema_version"`
	Status        string          `json:"status"`
	Backend       string          `json:"backend,omitempty"`
	Dependency    string          `json:"dependency,omitempty"`
	StoreState    string          `json:"store_state,omitempty"`
	Rows          int             `json:"rows,omitempty"`
	Total         int             `json:"total,omitempty"`
	Hits          []Hit           `json:"hits,omitempty"`
	Error         *sidecarErrResp `json:"error,omitempty"`
}

type sidecarErrResp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Doctor checks the sidecar + LanceDB backend health for a store URI.
func (c *SidecarClient) Doctor(ctx context.Context, storeURI, domain, table string) (Status, error) {
	detailed, err := c.DoctorDetailed(ctx, storeURI, domain, table)
	return detailed.Status, err
}

// DoctorDetailed adds doctor-only readiness facts while preserving Doctor's
// existing Status projection.
func (c *SidecarClient) DoctorDetailed(ctx context.Context, storeURI, domain, table string) (DoctorStatus, error) {
	resp, err := c.run(ctx, "doctor", sidecarRequest{SchemaVersion: SidecarSchema, StoreURI: storeURI, Domain: domain, Table: table, Backend: DefaultBackend})
	if err != nil {
		return DoctorStatus{}, err
	}
	// Consumers with stronger readiness requirements validate
	// backend/dependency/store-state at their own boundary.
	return DoctorStatus{Status: Status{Backend: defaultString(resp.Backend, DefaultBackend), Available: true, Dependency: resp.Dependency}, StoreState: resp.StoreState}, nil
}

// Rebuild (re)creates the vector index from the given records.
func (c *SidecarClient) Rebuild(ctx context.Context, storeURI, domain, table string, records []Record, opts RebuildOpts) (int, error) {
	metric := opts.DistanceMetric
	if strings.TrimSpace(metric) == "" {
		metric = "cosine"
	}
	resp, err := c.run(ctx, "rebuild", sidecarRequest{SchemaVersion: SidecarSchema, StoreURI: storeURI, Domain: domain, Table: table, Backend: DefaultBackend, DistanceMetric: metric, EmbeddingDim: opts.EmbeddingDim, Records: records})
	if err != nil {
		return 0, err
	}
	return resp.Rows, nil
}

// Search runs a vector search constrained to allowedIDs.
func (c *SidecarClient) Search(ctx context.Context, storeURI, domain, table string, queryVec []float64, allowedIDs []string, limit int) ([]Hit, int, error) {
	resp, err := c.run(ctx, "search", sidecarRequest{SchemaVersion: SidecarSchema, StoreURI: storeURI, Domain: domain, Table: table, Backend: DefaultBackend, QueryVector: queryVec, AllowedIDs: allowedIDs, Limit: limit})
	if err != nil {
		return nil, 0, err
	}
	return resp.Hits, resp.Total, nil
}

func (c *SidecarClient) run(ctx context.Context, op string, req sidecarRequest) (sidecarResponse, error) {
	executable := strings.TrimSpace(c.cfg.Executable)
	if executable == "" {
		executable = DefaultSidecarExecutable
	}
	timeout := c.cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	payload, err := json.Marshal(req)
	if err != nil {
		return sidecarResponse{}, err
	}
	cmd := exec.CommandContext(ctx, executable, op)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout limitedBuffer
	var stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	configureSidecarProcess(cmd)
	runErr := cmd.Run()
	cleanupSidecarProcess(cmd)
	if stdout.overflow || stderr.overflow || stdout.Len() >= maxSidecarOutputBytes || stderr.Len() >= maxSidecarOutputBytes {
		return sidecarResponse{}, fmt.Errorf("inferrum_sidecar_output_limit: LanceDB sidecar output exceeded limit")
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return sidecarResponse{}, fmt.Errorf("inferrum_sidecar_timeout: LanceDB sidecar timed out")
		}
		var execErr *exec.Error
		if errors.Is(runErr, exec.ErrNotFound) || errors.As(runErr, &execErr) || os.IsNotExist(runErr) {
			return sidecarResponse{}, fmt.Errorf("inferrum_sidecar_unavailable: install inferrum-lancedb-sidecar or configure inferrum.sidecar.executable")
		}
		return sidecarResponse{}, fmt.Errorf("inferrum_sidecar_failed: %s", sanitizeSidecarStderr(stderr.String()))
	}
	var resp sidecarResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return sidecarResponse{}, fmt.Errorf("inferrum_sidecar_protocol_invalid: LanceDB sidecar returned invalid JSON")
	}
	if resp.SchemaVersion != SidecarSchema {
		return sidecarResponse{}, fmt.Errorf("inferrum_sidecar_protocol_invalid: unsupported sidecar schema version")
	}
	if resp.Status == "failed" {
		code := "inferrum_sidecar_failed"
		message := "LanceDB sidecar failed"
		if resp.Error != nil {
			if strings.TrimSpace(resp.Error.Code) != "" {
				code = resp.Error.Code
			}
			if strings.TrimSpace(resp.Error.Message) != "" {
				message = resp.Error.Message
			}
		}
		return sidecarResponse{}, fmt.Errorf("%s: %s", code, message)
	}
	if resp.Status != "success" || resp.Error != nil {
		return sidecarResponse{}, fmt.Errorf("inferrum_sidecar_protocol_invalid: LanceDB sidecar returned malformed status")
	}
	return resp, nil
}

const maxSidecarOutputBytes = 64 << 10

type limitedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *limitedBuffer) Write(input []byte) (int, error) {
	if b.Len()+len(input) >= maxSidecarOutputBytes {
		remaining := maxSidecarOutputBytes - b.Len()
		if remaining > 0 {
			_, _ = b.Buffer.Write(input[:remaining])
		}
		b.overflow = true
		return len(input), errors.New("sidecar output limit")
	}
	return b.Buffer.Write(input)
}

var sidecarSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)authorization:\s*bearer\s+\S+`),
	regexp.MustCompile(`(?i)bearer\s+\S+`),
	regexp.MustCompile(`(?i)raw\s+prompt[^\n]*`),
	regexp.MustCompile(`(?i)hidden\s+prompt[^\n]*`),
	regexp.MustCompile(`(?i)provider\s+payload[^\n]*`),
	regexp.MustCompile(`(?i)secret[-_a-z0-9]*`),
}

// sanitizeSidecarStderr scrubs secret patterns and control characters from
// sidecar stderr before surfacing it in an error message.
func sanitizeSidecarStderr(input string) string {
	out := strings.TrimSpace(input)
	for _, pattern := range sidecarSecretPatterns {
		out = pattern.ReplaceAllString(out, "[redacted]")
	}
	out = strings.ReplaceAll(out, "\n", " ")
	if len(out) > 512 {
		out = out[:512] + "..."
	}
	if out == "" {
		return "Run inferrum doctor --json to inspect the sidecar"
	}
	return out
}
