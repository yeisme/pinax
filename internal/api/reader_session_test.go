package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// TestReaderSession covers the P05 supervised lifecycle: strict bootstrap
// frame validation, redacted stdout protocol, loopback-only readonly reader
// surface, metadata preflight isolation, EOF drain and clean shutdown with
// zero secret leakage. Fixture vaults only; no daemon is left behind.

func sessionTestVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\nAlpha zq-session-body-marker.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewService().IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	return root
}

func sessionFrame(vaultRoot string, scopes ...string) string {
	scopesJSON, _ := json.Marshal(scopes)
	return `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"binding-sess","vault_root":` + jsonString(vaultRoot) + `,"scopes":` + string(scopesJSON) + `,"readonly":true}`
}

// Pipe2 creates a kernel-buffered connected pair so protocol writes never
// block on the test's read timing (io.Pipe would deadlock the shutdown
// event).
func Pipe2() (*os.File, *os.File) {
	reader, writer, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	return reader, writer
}

func jsonString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestReaderSessionBootstrapValidation(t *testing.T) {
	t.Parallel()
	valid := sessionFrame("/tmp/vault", app.ReaderScopeMetadata)
	if _, err := ReadSessionBootstrap(strings.NewReader(valid)); err != nil {
		t.Fatalf("valid frame rejected: %v", err)
	}
	// pinax-human-reading-write-v1 W02: writer scopes enter the same
	// allowlist slot and flip the session out of readonly mode. The
	// readonly/writer correlation is enforced here at the frame level.
	writerFrame := func(scopes ...string) string {
		encoded, _ := json.Marshal(scopes)
		return `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":` + string(encoded) + `,"readonly":false}`
	}
	if _, err := ReadSessionBootstrap(strings.NewReader(writerFrame(app.WriterScopeCapture))); err != nil {
		t.Fatalf("writer capture frame rejected: %v", err)
	}
	if _, err := ReadSessionBootstrap(strings.NewReader(writerFrame(app.ReaderScopeBody, app.WriterScopeEditor))); err != nil {
		t.Fatalf("mixed reader/writer frame rejected: %v", err)
	}
	cases := []struct {
		name  string
		frame string
	}{
		{"unknown field", `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","listen_host":"0.0.0.0"}`},
		{"wrong schema version", `{"schema_version":"pinax.local_session.bootstrap.v2","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":["reader.metadata"],"readonly":true}`},
		{"mutation rejected", `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":["reader.metadata"],"readonly":false}`},
		{"writer scope with readonly rejected", `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":["writer.capture"],"readonly":true}`},
		{"unknown scope", `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":["reader.write"],"readonly":true}`},
		{"duplicate scope", `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":["reader.metadata","reader.metadata"],"readonly":true}`},
		{"empty scopes", `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":[],"readonly":true}`},
		{"short bearer", `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"short","binding_id":"b","vault_root":"/v","scopes":["reader.metadata"],"readonly":true}`},
		{"short nonce", `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"n","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":["reader.metadata"],"readonly":true}`},
		{"empty frame", ``},
		{"oversize frame", `{"schema_version":"pinax.local_session.bootstrap.v1alpha1","pad":"` + strings.Repeat("x", 17*1024) + `"}`},
	}
	for _, testCase := range cases {
		if _, err := ReadSessionBootstrap(strings.NewReader(testCase.frame)); err == nil {
			t.Errorf("frame %q unexpectedly accepted", testCase.name)
		}
	}
}

func TestReaderSessionBootstrapDeadline(t *testing.T) {
	t.Parallel()
	original := sessionBootstrapDeadline
	sessionBootstrapDeadline = 50 * time.Millisecond
	defer func() { sessionBootstrapDeadline = original }()
	// A pipe with no writer blocks forever: only the deadline can fire.
	blocked, _ := io.Pipe()
	if _, err := ReadSessionBootstrapWithDeadline(context.Background(), blocked); err == nil || !strings.Contains(err.Error(), "did not arrive") {
		t.Fatalf("deadline err = %v", err)
	}
}

// runSupervisedSession boots ServeSupervisedSession against a pipe stdin and
// returns the parsed ready event plus helpers to talk to the reader surface.
type supervisedHarness struct {
	ready      SessionEvent
	events     *bufio.Scanner
	bearer     string
	address    string
	closeStdin func()
	wait       func() error
}

func startSupervisedSession(t *testing.T, vaultRoot, frame string) *supervisedHarness {
	t.Helper()
	stdoutReader, stdoutWriter := Pipe2()
	stdinReader, stdinWriter := Pipe2()
	go func() { _, _ = stdinWriter.Write([]byte(frame + "\n")) }()
	// Production shape: the frame is consumed from the SAME reader the
	// session drains, byte-precisely, so the session polices the remainder.
	parsed, err := ReadSessionBootstrap(stdinReader)
	if err != nil {
		t.Fatalf("parse frame: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- ServeSupervisedSession(context.Background(), ServeSupervisedSessionOptions{
			Service: app.NewService(),
			Frame:   parsed,
			Stdin:   stdinReader,
			Stdout:  stdoutWriter,
			Stderr:  os.Stderr,
		})
	}()
	scanner := bufio.NewScanner(stdoutReader)
	if !scanner.Scan() {
		t.Fatalf("no ready event: %v", scanner.Err())
	}
	var ready SessionEvent
	if err := json.Unmarshal([]byte(scanner.Text()), &ready); err != nil || ready.Type != "ready" {
		t.Fatalf("ready event = %q err=%v", scanner.Text(), err)
	}
	return &supervisedHarness{
		ready: ready, events: scanner, bearer: parsed.Bearer,
		address:    strings.TrimPrefix(ready.Address, "http://"),
		closeStdin: func() { _ = stdinWriter.Close() },
		wait:       func() error { return <-done },
	}
}

func TestReaderSessionLifecycleAndRedaction(t *testing.T) {
	t.Parallel()
	root := sessionTestVault(t)
	frame := sessionFrame(root, app.ReaderScopeMetadata, app.ReaderScopeBody)
	harness := startSupervisedSession(t, root, frame)

	// The real ready event must satisfy the frozen machine schema that
	// consumers generate their types from.
	registry := catalogschema.Default()
	encodedEvent, _ := json.Marshal(harness.ready)
	var decodedEvent any
	if err := json.Unmarshal(encodedEvent, &decodedEvent); err != nil {
		t.Fatal(err)
	}
	if err := catalogschema.ValidatePayload(registry, catalogschema.LocalSessionEventV1Alpha1, decodedEvent); err != nil {
		t.Fatalf("ready event violates pinax.local_session.event.v1alpha1: %v", err)
	}

	// Ready event is redacted: no bearer, no vault root, readonly pinned.
	encoded, _ := json.Marshal(harness.ready)
	for _, leak := range []string{harness.bearer, root, "zq-session-body-marker"} {
		if strings.Contains(string(encoded), leak) {
			t.Fatalf("ready event leaked %q: %s", leak, encoded)
		}
	}
	if harness.ready.Readonly == nil || *harness.ready.Readonly != true {
		t.Fatalf("ready not readonly: %#v", harness.ready)
	}
	if !strings.HasPrefix(harness.ready.Address, "http://127.0.0.1:") {
		t.Fatalf("ready address = %q", harness.ready.Address)
	}
	if harness.ready.SessionID == "" || harness.ready.SessionNonce == "" || harness.ready.BindingID == "" {
		t.Fatalf("ready identity = %#v", harness.ready)
	}

	// Reader surface works with the in-memory bearer only.
	request, _ := http.NewRequest(http.MethodGet, "http://"+harness.address+"/v1/reader/source", nil)
	request.Header.Set("Authorization", "Bearer "+harness.bearer)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("source request: %v", err)
	}
	body := make([]byte, 4096)
	n, _ := response.Body.Read(body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body[:n]), "pinax.reader.source.v1alpha1") {
		t.Fatalf("source status=%d body=%s", response.StatusCode, body[:n])
	}
	if response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("source cache control = %q", response.Header.Get("Cache-Control"))
	}

	// Legacy routes stay hidden in supervised mode.
	request, _ = http.NewRequest(http.MethodGet, "http://"+harness.address+"/v1/manifest", nil)
	request.Header.Set("Authorization", "Bearer "+harness.bearer)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("legacy route exposed in supervised mode: %d", response.StatusCode)
	}

	// EOF drains and shuts down cleanly with a final event.
	harness.closeStdin()
	if err := harness.wait(); err != nil {
		t.Fatalf("supervised session: %v", err)
	}
	if !harness.events.Scan() {
		t.Fatal("missing shutdown event")
	}
	var shutdown SessionEvent
	if err := json.Unmarshal([]byte(harness.events.Text()), &shutdown); err != nil || shutdown.Type != "shutdown" {
		t.Fatalf("shutdown event = %q", harness.events.Text())
	}
}

func TestReaderSessionMetadataPreflightStaysIsolated(t *testing.T) {
	t.Parallel()
	root := sessionTestVault(t)

	// Preflight session: metadata only — cannot read body surfaces.
	preflight := startSupervisedSession(t, root, sessionFrame(root, app.ReaderScopeMetadata))
	request, _ := http.NewRequest(http.MethodGet, "http://"+preflight.address+"/v1/reader/search?q=zq-session-body-marker", nil)
	request.Header.Set("Authorization", "Bearer "+preflight.bearer)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 4096)
	n, _ := response.Body.Read(body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden || strings.Contains(string(body[:n]), "zq-session-body-marker") {
		t.Fatalf("preflight search status=%d body=%s", response.StatusCode, body[:n])
	}

	// Confirmed session with body scope on the same vault: reading works and
	// the preflight session's authorization never changes.
	confirmed := startSupervisedSession(t, root, sessionFrame(root, app.ReaderScopeMetadata, app.ReaderScopeBody))
	request, _ = http.NewRequest(http.MethodGet, "http://"+confirmed.address+"/v1/reader/search?q=zq-session-body-marker", nil)
	request.Header.Set("Authorization", "Bearer "+confirmed.bearer)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body2 := make([]byte, 8192)
	n, _ = response.Body.Read(body2)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body2[:n]), "note_alpha") {
		t.Fatalf("confirmed search status=%d body=%s", response.StatusCode, body2[:n])
	}

	// The preflight bearer still cannot search after confirmation elsewhere.
	request, _ = http.NewRequest(http.MethodGet, "http://"+preflight.address+"/v1/reader/search?q=zq-session-body-marker", nil)
	request.Header.Set("Authorization", "Bearer "+preflight.bearer)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("preflight upgraded after confirmation: %d", response.StatusCode)
	}

	// Cancelling the preflight exits cleanly.
	preflight.closeStdin()
	if err := preflight.wait(); err != nil {
		t.Fatalf("preflight cancel: %v", err)
	}
	confirmed.closeStdin()
	if err := confirmed.wait(); err != nil {
		t.Fatalf("confirmed shutdown: %v", err)
	}
}

func TestReaderSessionEOFDrainsInFlightRequests(t *testing.T) {
	t.Parallel()
	root := sessionTestVault(t)
	frame := sessionFrame(root, app.ReaderScopeMetadata, app.ReaderScopeBody)
	harness := startSupervisedSession(t, root, frame)

	// Fire a reader request and close stdin at the same moment: the request
	// must still complete, and the shutdown event must only be emitted after
	// draining finished.
	type result struct {
		status int
		err    error
	}
	requestDone := make(chan result, 1)
	dialed := make(chan struct{})
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, dialErr := (&net.Dialer{}).DialContext(ctx, network, addr)
			if dialErr == nil {
				close(dialed)
			}
			return conn, dialErr
		},
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	go func() {
		request, _ := http.NewRequest(http.MethodGet, "http://"+harness.address+"/v1/reader/notes?limit=1", nil)
		request.Header.Set("Authorization", "Bearer "+harness.bearer)
		response, err := client.Do(request)
		if err != nil {
			requestDone <- result{err: err}
			return
		}
		_ = response.Body.Close()
		requestDone <- result{status: response.StatusCode}
	}()
	// Wait until the connection is established and the request bytes have
	// flushed, so EOF arrives while the request is genuinely in flight and
	// Shutdown must wait for its response.
	<-dialed
	time.Sleep(100 * time.Millisecond)
	harness.closeStdin()

	outcome := <-requestDone
	if outcome.err != nil || outcome.status != http.StatusOK {
		t.Fatalf("in-flight request after EOF: status=%d err=%v", outcome.status, outcome.err)
	}
	if err := harness.wait(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if !harness.events.Scan() {
		t.Fatal("missing shutdown event")
	}
	var shutdown SessionEvent
	if err := json.Unmarshal([]byte(harness.events.Text()), &shutdown); err != nil || shutdown.Type != "shutdown" {
		t.Fatalf("shutdown event = %q", harness.events.Text())
	}
}

func TestReaderSessionRejectsMissingVault(t *testing.T) {
	t.Parallel()
	stdoutReader, stdoutWriter := Pipe2()
	frame, err := ReadSessionBootstrap(strings.NewReader(sessionFrame(filepath.Join(t.TempDir(), "missing"), app.ReaderScopeMetadata)))
	if err != nil {
		t.Fatal(err)
	}
	// A failed startup emits the redacted error event AND fails with a
	// non-nil error: a successfully written error event is not success.
	err = ServeSupervisedSession(context.Background(), ServeSupervisedSessionOptions{
		Service: app.NewService(), Frame: frame,
		Stdin: strings.NewReader(""), Stdout: stdoutWriter,
	})
	if err == nil || !strings.Contains(err.Error(), "session_vault_unavailable") {
		t.Fatalf("missing vault must fail the session, got err=%v", err)
	}
	scanner := bufio.NewScanner(stdoutReader)
	if !scanner.Scan() {
		t.Fatal("no error event")
	}
	var event SessionEvent
	if err := json.Unmarshal([]byte(scanner.Text()), &event); err != nil || event.Type != "error" || event.Code != "session_vault_unavailable" {
		t.Fatalf("error event = %q", scanner.Text())
	}
}

func TestReaderSessionBootstrapErrorsAreRedacted(t *testing.T) {
	t.Parallel()
	// Unknown fields and hostile values must never be echoed back: the
	// frame is private input and diagnostics stay generic.
	leaky := []string{
		`{"schema_version":"pinax.local_session.bootstrap.v1alpha1","listen_host":"/private/zq-secret-path","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":["reader.metadata"],"readonly":true}`,
		`{"schema_version":"pinax.local_session.bootstrap.v2","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":["reader.metadata"],"readonly":true}`,
		`{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"b","vault_root":"/v","scopes":["zq-secret-scope"],"readonly":true}`,
	}
	for _, frame := range leaky {
		_, err := ReadSessionBootstrap(strings.NewReader(frame))
		if err == nil {
			t.Fatal("leaky frame accepted")
		}
		if strings.Contains(err.Error(), "zq-secret") || strings.Contains(err.Error(), "listen_host") || strings.Contains(err.Error(), "v2") {
			t.Fatalf("error echoed private input: %v", err)
		}
	}
}

func TestReaderSessionBootstrapRejectsSameLineTrailingContent(t *testing.T) {
	t.Parallel()
	// The first frame must be exactly ONE closed JSON object: trailing
	// content on the SAME line makes the frame itself illegal.
	valid := sessionFrame("/tmp/v", "reader.metadata")
	for _, tc := range []struct{ name, frame string }{
		{"trailing garbage", valid + " trailing-not-json"},
		{"second json object", valid + `{"unknown_field":true}`},
		// Closing delimiters defeat More(); strict EOF must still reject.
		{"trailing closing brace", valid + "}"},
		{"trailing closing bracket then garbage", valid + "] arbitrary-after-close"},
	} {
		if _, err := ReadSessionBootstrap(strings.NewReader(tc.frame)); err == nil {
			t.Errorf("%s bootstrap frame unexpectedly accepted", tc.name)
		}
	}
}

func TestReaderSessionRejectsTrailingBootstrapInput(t *testing.T) {
	t.Parallel()
	// Multi-line trailing content stays in stdin after the byte-precise
	// frame read: the supervised session must fail (error event + non-nil
	// error), never accept it with a graceful ready → shutdown.
	for _, tc := range []struct{ name, extra string }{
		{"trailing garbage", "trailing-not-json"},
		{"second json object", `{"unknown_field":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := sessionTestVault(t)
			frame := sessionFrame(root, app.ReaderScopeMetadata)
			stdoutReader, stdoutWriter := Pipe2()
			stdinReader, stdinWriter := Pipe2()
			go func() {
				_, _ = stdinWriter.Write([]byte(frame + "\n" + tc.extra + "\n"))
			}()
			parsed, err := ReadSessionBootstrap(stdinReader)
			if err != nil {
				t.Fatalf("parse frame: %v", err)
			}
			done := make(chan error, 1)
			go func() {
				done <- ServeSupervisedSession(context.Background(), ServeSupervisedSessionOptions{
					Service: app.NewService(),
					Frame:   parsed,
					Stdin:   stdinReader,
					Stdout:  stdoutWriter,
					Stderr:  os.Stderr,
				})
			}()
			scanner := bufio.NewScanner(stdoutReader)
			if !scanner.Scan() {
				t.Fatalf("no ready event: %v", scanner.Err())
			}
			var ready SessionEvent
			if err := json.Unmarshal([]byte(scanner.Text()), &ready); err != nil || ready.Type != "ready" {
				t.Fatalf("ready event = %q err=%v", scanner.Text(), err)
			}
			if !scanner.Scan() {
				t.Fatalf("no event after ready: %v", scanner.Err())
			}
			var terminal SessionEvent
			if err := json.Unmarshal([]byte(scanner.Text()), &terminal); err != nil {
				t.Fatalf("terminal event = %q err=%v", scanner.Text(), err)
			}
			if terminal.Type != "error" || terminal.Code != "session_bootstrap_invalid" {
				t.Fatalf("terminal event = %#v", terminal)
			}
			if err := <-done; err == nil {
				t.Fatal("session with trailing bootstrap input exited without error")
			}
		})
	}
}

func TestReaderSessionIDsUniquePerSession(t *testing.T) {
	t.Parallel()
	// The same binding started twice (within the same second, different
	// bearers/nonces) must produce DIFFERENT session ids.
	root := sessionTestVault(t)
	first := startSupervisedSession(t, root, sessionFrame(root, "reader.metadata", "reader.body"))
	second := startSupervisedSession(t, root, sessionFrame(root, "reader.metadata", "reader.body"))
	if first.ready.SessionID == second.ready.SessionID {
		t.Fatalf("two sessions for the same binding share session_id %q", first.ready.SessionID)
	}
	first.closeStdin()
	second.closeStdin()
	if err := first.wait(); err != nil {
		t.Fatal(err)
	}
	if err := second.wait(); err != nil {
		t.Fatal(err)
	}
}

func TestReaderSessionRejectsNonVaultDirectory(t *testing.T) {
	t.Parallel()
	// A plain directory without the .pinax marker is not an initialized
	// vault: startup fails safely with a redacted error (never boots a
	// listener over a non-vault root, never auto-initializes it).
	plain := t.TempDir()
	stdoutReader, stdoutWriter := Pipe2()
	frame, err := ReadSessionBootstrap(strings.NewReader(sessionFrame(plain, app.ReaderScopeMetadata)))
	if err != nil {
		t.Fatal(err)
	}
	err = ServeSupervisedSession(context.Background(), ServeSupervisedSessionOptions{
		Service: app.NewService(), Frame: frame,
		Stdin: strings.NewReader(""), Stdout: stdoutWriter,
	})
	if err == nil || !strings.Contains(err.Error(), "session_vault_invalid") {
		t.Fatalf("non-vault directory must fail the session, got err=%v", err)
	}
	scanner := bufio.NewScanner(stdoutReader)
	if !scanner.Scan() {
		t.Fatal("no error event")
	}
	var event SessionEvent
	if err := json.Unmarshal([]byte(scanner.Text()), &event); err != nil || event.Type != "error" || event.Code != "session_vault_invalid" {
		t.Fatalf("error event = %q err=%v", scanner.Text(), err)
	}
}
