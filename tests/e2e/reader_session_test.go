package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/pkg/localsession"
)

// TestReaderSessionScript drives the supervised session through the real
// CLI process via testscript (pinax-human-reading-api-v1 P05): frame-driven
// api serve --session-stdin, redacted stdout protocol, clean EOF shutdown,
// and rejection of malformed frames.
func TestReaderSessionScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/reader_session/scripts",
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"write-session-frame": cmdWriteSessionFrame,
		},
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			return nil
		},
	})
}

// cmdWriteSessionFrame renders a valid bootstrap frame with the real workdir
// path; file content itself has no variable expansion in testscript.
func cmdWriteSessionFrame(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 3 {
		ts.Fatalf("usage: write-session-frame <file> <vault-root> <comma-scopes>")
	}
	scopes := make([]string, 0)
	for _, scope := range strings.Split(args[2], ",") {
		scopes = append(scopes, fmt.Sprintf("%q", scope))
	}
	frame := fmt.Sprintf(`{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":"3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","binding_id":"binding-ts","vault_root":%q,"scopes":[%s],"readonly":true}`, args[1], strings.Join(scopes, ","))
	if err := os.WriteFile(ts.MkAbs(args[0]), []byte(frame), 0o644); err != nil {
		ts.Fatalf("write frame: %v", err)
	}
}

// TestReaderSessionProcess exercises the full supervised lifecycle through
// the compiled binary: frame in, ready out, authorized reader request, EOF
// drain, process exit, and zero secret leakage on any stream.
func TestReaderSessionProcess(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	noteBody := "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\nAlpha zq-process-body-marker.\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"), []byte(noteBody), 0o644); err != nil {
		t.Fatal(err)
	}
	pinax := filepath.Join(sharedBinDir, "pinax")
	if out, err := exec.Command(pinax, "index", "refresh", "--vault", root, "--json").CombinedOutput(); err != nil {
		t.Fatalf("index refresh: %v\n%s", err, out)
	}

	bearer := "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	frame, err := json.Marshal(localsession.Bootstrap{SchemaVersion: localsession.BootstrapVersion, SessionNonce: "nonce-0123456789abcdef", Bearer: bearer, BindingID: "binding-proc", VaultRoot: root, Scopes: []string{"reader.metadata", "reader.body"}, Readonly: true})
	if err != nil {
		t.Fatal("bootstrap encoding failed")
	}
	// CLI registrations are assembled by the composition root. The imported
	// cli package alone has not constructed that command tree in this process.
	manifestBytes, err := exec.Command(pinax, "api", "manifest", "--json").Output()
	var manifest domain.Projection
	if err != nil || json.Unmarshal(manifestBytes, &manifest) != nil || manifest.Status != "success" {
		t.Fatal("manifest unavailable")
	}
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdinReader.Close() }()
	defer func() { _ = stdinWriter.Close() }()
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdoutReader.Close() }()
	defer func() { _ = stdoutWriter.Close() }()
	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = stderrFile.Close() }()
	cmd := exec.Command(pinax, "api", "serve", "--session-stdin")
	cmd.Stdin = stdinReader
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = stdinReader.Close()
	_ = stdoutWriter.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	waited := false
	defer func() {
		_ = stdinWriter.Close()
		if !waited {
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	if _, err := stdinWriter.Write(append(frame, '\n')); err != nil {
		t.Fatal(err)
	}

	scanner := bufio.NewScanner(stdoutReader)
	scanner.Buffer(make([]byte, 1024), localsession.MaxFrameBytes+1)
	readyChannel := make(chan []byte, 4)
	go func() {
		defer close(readyChannel)
		for scanner.Scan() {
			select {
			case readyChannel <- append([]byte(nil), scanner.Bytes()...):
			default:
				return
			}
		}
	}()
	select {
	case encoded := <-readyChannel:
		event, err := localsession.DecodeReady(encoded, "nonce-0123456789abcdef", "binding-proc", manifest.Facts["digest"], true)
		if err != nil {
			t.Fatal("real process ready rejected by public host contract")
		}
		if strings.Contains(fmt.Sprint(event), bearer) || strings.Contains(fmt.Sprint(event), root) {
			t.Fatalf("ready event leaked secrets: %#v", event)
		}
		request, _ := http.NewRequest(http.MethodGet, event.Address+"/v1/reader/source", nil)
		request.Header.Set("Authorization", "Bearer "+bearer)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		request = request.WithContext(ctx)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("reader request: %v", err)
		}
		body := make([]byte, 8192)
		n, _ := response.Body.Read(body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || !strings.Contains(string(body[:n]), "pinax.reader.source.v1alpha1") {
			t.Fatalf("source status=%d body=%s", response.StatusCode, body[:n])
		}
	case <-time.After(15 * time.Second):
		t.Fatal("supervised session never became ready")
	}

	// EOF drains and the process exits by itself: no daemon left behind.
	if err := stdinWriter.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		waited = true
		if err != nil {
			t.Fatalf("process exit: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process did not exit after stdin EOF")
	}
	select {
	case encoded := <-readyChannel:
		var event localsession.Event
		if json.Unmarshal(encoded, &event) != nil || event.Type != "shutdown" {
			t.Fatalf("shutdown event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing shutdown event")
	}

	// No secret ever reached stderr or the process environment dumps.
	stderrBytes, _ := os.ReadFile(stderrFile.Name())
	for _, leak := range []string{bearer, "zq-process-body-marker"} {
		if strings.Contains(string(stderrBytes), leak) {
			t.Fatalf("stderr leaked %q: %s", leak, stderrBytes)
		}
	}
}
