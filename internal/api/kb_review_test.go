package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestKBReviewProjectionsAreBoundedGETOnly(t *testing.T) {
	root := t.TempDir()
	server := NewServer(app.NewService(), root)

	overview := httptest.NewRecorder()
	server.Handler().ServeHTTP(overview, httptest.NewRequest(http.MethodGet, "/v1/kb/review/overview", nil))
	if overview.Code != http.StatusOK {
		t.Fatalf("overview status=%d body=%s", overview.Code, overview.Body.String())
	}
	if !strings.Contains(overview.Body.String(), `"command":"kb.review.overview"`) || strings.Contains(overview.Body.String(), root) || strings.Contains(overview.Body.String(), `"body"`) {
		t.Fatalf("overview leaked or missed bounded projection: %s", overview.Body.String())
	}

	sources := httptest.NewRecorder()
	server.Handler().ServeHTTP(sources, httptest.NewRequest(http.MethodGet, "/v1/kb/review/sources?limit=10", nil))
	if sources.Code != http.StatusOK || !strings.Contains(sources.Body.String(), `"command":"kb.review.sources"`) {
		t.Fatalf("sources status=%d body=%s", sources.Code, sources.Body.String())
	}

	for _, path := range []string{"/v1/kb/review/overview", "/v1/kb/review/sources"} {
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, nil))
		if res.Code != http.StatusMethodNotAllowed || !strings.Contains(res.Body.String(), `"code":"method_not_allowed"`) {
			t.Fatalf("POST %s status=%d body=%s", path, res.Code, res.Body.String())
		}
	}

	capabilities := httptest.NewRecorder()
	server.Handler().ServeHTTP(capabilities, httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil))
	for _, want := range []string{"kb.review.overview", "kb.review.sources", "/v1/kb/review/overview", "/v1/kb/review/sources"} {
		if !strings.Contains(capabilities.Body.String(), want) {
			t.Fatalf("capabilities missing %q: %s", want, capabilities.Body.String())
		}
	}
}

func TestKBReviewSourcesReturnsRelativeMetadataOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	body := "---\nschema_version: pinax.note.v1\nnote_id: note_review\ntitle: Review Note\nkind: reference\nstatus: active\n---\n\nprivate body must not cross the review projection\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "review.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	server := NewServer(app.NewService(), root)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/kb/review/sources", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"source_ref":"notes/review.md"`) {
		t.Fatalf("sources response status=%d body=%s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), root) || strings.Contains(res.Body.String(), "private body") || strings.Contains(res.Body.String(), `"body"`) {
		t.Fatalf("sources response leaked unsafe data: %s", res.Body.String())
	}
}

func TestKBReviewEvaluationRoutesAreBoundedGETOnly(t *testing.T) {
	root := t.TempDir()
	suiteDir := filepath.Join(root, ".pinax", "kb", "evaluation-suites")
	if err := os.MkdirAll(suiteDir, 0o700); err != nil {
		t.Fatalf("mkdir suite dir: %v", err)
	}
	suite := `{"schema_version":"pinax.kb.evaluation-suite.v1","suite_id":"local","version":"v1","questions":[{"question_id":"q-1","query":"where is the local model?","expected_citations":["notes/model.md"]}]}`
	if err := os.WriteFile(filepath.Join(suiteDir, "local.json"), []byte(suite), 0o600); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	if _, err := app.WriteKBEvaluationReceipt(root, app.KBEvaluationReceipt{
		SchemaVersion:          app.KBEvaluationReceiptSchema,
		RunID:                  "run-1",
		Status:                 "failed",
		GenerationID:           "gen-1",
		SourceSnapshot:         "snapshot-1",
		SourceDigest:           "sha256:source",
		Provider:               "ollama",
		Model:                  "pinax-qwen3-embedding-lowmem",
		ModelManifestDigest:    "sha256:model",
		ProfileHash:            "sha256:profile",
		EmbeddingDim:           1024,
		SuiteID:                "local",
		SuiteVersion:           "v1",
		GateConfigHash:         "sha256:gate",
		GenerationManifestHash: "sha256:manifest",
		Metrics:                app.KBEvaluationMetrics{DatasetStatus: app.KBEvaluationDatasetInsufficient, TotalQuestions: 1, K: 5, StatusCounts: map[string]int{"no_hit": 1}},
		CreatedAt:              "2026-08-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	server := NewServer(app.NewService(), root)

	for _, tc := range []struct {
		path    string
		command string
	}{
		{path: "/v1/kb/review/evaluation-suites", command: "kb.review.evaluation_suites"},
		{path: "/v1/kb/review/evaluation-suites/local/questions", command: "kb.review.evaluation_questions"},
		{path: "/v1/kb/review/runs/run-1", command: "kb.review.run"},
	} {
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"command":"`+tc.command+`"`) {
			t.Fatalf("GET %s status=%d body=%s", tc.path, res.Code, res.Body.String())
		}
		if strings.Contains(res.Body.String(), root) || strings.Contains(res.Body.String(), `"body"`) {
			t.Fatalf("GET %s leaked unsafe data: %s", tc.path, res.Body.String())
		}
	}
	for _, path := range []string{
		"/v1/kb/review/evaluation-suites",
		"/v1/kb/review/evaluation-suites/local/questions",
		"/v1/kb/review/runs/run-1",
	} {
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, nil))
		if res.Code != http.StatusMethodNotAllowed || !strings.Contains(res.Body.String(), `"code":"method_not_allowed"`) {
			t.Fatalf("POST %s status=%d body=%s", path, res.Code, res.Body.String())
		}
	}

	capabilities := httptest.NewRecorder()
	server.Handler().ServeHTTP(capabilities, httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil))
	for _, want := range []string{
		"kb.review.evaluation_suites",
		"kb.review.evaluation_questions",
		"kb.review.run",
		"/v1/kb/review/evaluation-suites",
		"/v1/kb/review/runs/",
	} {
		if !strings.Contains(capabilities.Body.String(), want) {
			t.Fatalf("capabilities missing %q: %s", want, capabilities.Body.String())
		}
	}
}
