package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func TestMCPStdoutStderrPanicIsolation(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"pinax://manifest"}}`,
	}, "\n") + "\n"
	var stdout strings.Builder
	var stderr strings.Builder
	err := ServeWithOptions(context.Background(), app.NewService(), t.TempDir(), strings.NewReader(input), &stdout, ServerOptions{
		Diagnostics: &stderr,
		Manifest: func() (domain.Projection, error) {
			panic("SECRET_PANIC_PAYLOAD")
		},
	})
	if err != nil {
		t.Fatalf("ServeWithOptions() error = %v", err)
	}
	responses := parseMCPFrameResponses(t, stdout.String())
	if len(responses) != 2 || responses[1].Error == nil || responses[1].Error.Data["legacy_code"] != "internal_error" {
		t.Fatalf("panic response = %#v", responses)
	}
	if strings.Contains(stdout.String(), "SECRET_PANIC_PAYLOAD") || strings.Contains(stderr.String(), "SECRET_PANIC_PAYLOAD") {
		t.Fatalf("panic payload leaked: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `panic recovered method="resources/read"`) {
		t.Fatalf("redacted panic diagnostic = %q", stderr.String())
	}
}

func TestMCPJSONRPCStdoutRemainsParseableAfterMalformedInput(t *testing.T) {
	t.Parallel()

	input := "{not-json}\n" + `{"jsonrpc":"2.0","id":2,"method":"initialize"}` + "\n"
	var stdout strings.Builder
	if err := Serve(context.Background(), app.NewService(), t.TempDir(), strings.NewReader(input), &stdout); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout frames = %d: %q", len(lines), stdout.String())
	}
	for _, line := range lines {
		var response Response
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("stdout contains non-JSON-RPC frame %q: %v", line, err)
		}
		if response.JSONRPC != "2.0" {
			t.Fatalf("response jsonrpc = %q", response.JSONRPC)
		}
	}
}

func TestMCPSignalContextStopsLifecycleWithoutStdoutPollution(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	var stdout strings.Builder
	done := make(chan error, 1)
	root := t.TempDir()
	go func() {
		done <- Serve(ctx, app.NewService(), root, reader, &stdout)
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not stop after signal context cancellation")
	}
	if stdout.Len() != 0 {
		t.Fatalf("signal cancellation polluted stdout: %q", stdout.String())
	}
}
