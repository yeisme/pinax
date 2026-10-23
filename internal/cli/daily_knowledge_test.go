package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestDailyKnowledgeRemoteCaptureStdin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PINAX_API_URL", "")
	calls := 0
	body := "中文 source\n\n```go\nprintln(1)\n```\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpc struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rpc); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		calls++
		if rpc.Method != "Pinax.Inbox.Capture" || rpc.Params["body"] != body || rpc.Params["dry_run"] != true {
			t.Errorf("capture input lost or wrong operation")
		}
		if err := json.NewEncoder(w).Encode(domain.NewProjection("inbox.capture", "Previewed.")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	args := []string{"inbox", "capture", "Fixture", "--stdin", "--dry-run", "--api-url", server.URL, "--json"}
	out, stderr, err := runCLIWithStdin(t, body, args...)
	if err != nil || stderr != "" || !json.Valid([]byte(out)) || calls != 1 {
		t.Fatalf("capture failed: %v, %s, %s", err, stderr, out)
	}
	_, _, err = runCLIWithStdin(t, body, append(args, "--body", "conflicting")...)
	if err == nil || calls != 1 {
		t.Fatal("conflicting body reached server")
	}
}

func TestDailyKnowledgeHelpAndInputFailure(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"inbox", "capture", "--help"}, "--stdin"},
		{[]string{"inbox", "review", "--help"}, "--after"},
	} {
		out, _, err := runCLIWithStdin(t, "", test.args...)
		if err != nil || !strings.Contains(out, test.want) {
			t.Fatal(out, err)
		}
	}
	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetIn(failingDailyReader{})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"inbox", "capture", "Fixture", "--stdin", "--vault", t.TempDir(), "--json"})
	if err := cmd.Execute(); err == nil || !strings.Contains(out.String(), "stdin_read_failed") {
		t.Fatal(out.String(), err)
	}
}

type failingDailyReader struct{}

func (failingDailyReader) Read([]byte) (int, error) { return 0, http.ErrBodyReadAfterClose }
