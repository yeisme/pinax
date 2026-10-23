package e2e

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/pkg/localsession"
	"github.com/yeisme/pinax/pkg/pinaxclient"
)

// This host is a fixture, not a production approval UI. It retains only the
// original proposal digest and authenticated session metadata for recovery.
type recoverySaveProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	stderr  *os.File
	ready   localsession.Event
	frame   localsession.Bootstrap
	client  *pinaxclient.Client
	stopped bool
}

func startRecoverySaveProcess(t *testing.T, root string, writer bool, grant *localsession.RecoveryLookupGrant) *recoverySaveProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	t.Cleanup(cancel)
	bearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	nonce = "nonce-" + nonce
	scopes := []string{app.ReaderScopeMetadata, app.ReaderScopeBody}
	if writer {
		scopes = append(scopes, app.WriterScopeEditor)
	}
	p := &recoverySaveProcess{frame: localsession.Bootstrap{SchemaVersion: localsession.BootstrapVersion, SessionNonce: nonce, Bearer: bearer, BindingID: "save-recovery-binding", VaultRoot: root, Scopes: scopes, Readonly: !writer}}
	args := []string{"api", "serve", "--session-stdin"}
	if grant != nil {
		args = append(args, "--session-recovery-stdin")
	}
	p.cmd = exec.CommandContext(ctx, filepath.Join(sharedBinDir, "pinax"), args...)
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	p.stderr, err = os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	p.cmd.Stderr = p.stderr
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = p.stdin.Close()
		if !p.stopped {
			_ = p.cmd.Process.Kill()
			_ = p.cmd.Wait()
		}
		_ = p.stderr.Close()
	})
	p.scanner = bufio.NewScanner(stdout)
	p.scanner.Buffer(make([]byte, 1024), localsession.MaxFrameBytes)
	if err := json.NewEncoder(p.stdin).Encode(p.frame); err != nil {
		t.Fatal(err)
	}
	if grant != nil {
		if !p.scanner.Scan() {
			t.Fatal("missing recovery challenge")
		}
		binding, err := localsession.DecodeRecoveryChallenge(p.scanner.Bytes(), nonce, p.frame.BindingID)
		if err != nil {
			t.Fatal("invalid challenge")
		}
		g := *grant
		g.SessionNonce = binding.SessionNonce
		g.BindingID = binding.BindingID
		g.VaultIdentityDigest = binding.VaultIdentityDigest
		now := time.Now().UTC()
		g.IssuedAt = now.Format(time.RFC3339Nano)
		g.ExpiresAt = now.Add(time.Minute).Format(time.RFC3339Nano)
		if err := json.NewEncoder(p.stdin).Encode(g); err != nil {
			t.Fatal(err)
		}
	}
	if !p.scanner.Scan() {
		t.Fatal("missing ready")
	}
	manifestRaw, err := exec.CommandContext(ctx, filepath.Join(sharedBinDir, "pinax"), "api", "manifest", "--json").Output()
	var manifest domain.Projection
	if err != nil || json.Unmarshal(manifestRaw, &manifest) != nil {
		t.Fatal("manifest unavailable")
	}
	p.ready, err = localsession.DecodeReady(p.scanner.Bytes(), nonce, p.frame.BindingID, manifest.Facts["digest"], !writer)
	if err != nil {
		t.Fatal("ready failed public validation")
	}
	p.client, err = pinaxclient.New(pinaxclient.Config{BaseURL: p.ready.Address, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (p *recoverySaveProcess) stop(t *testing.T) {
	t.Helper()
	if err := p.stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if !p.scanner.Scan() {
		t.Fatal("missing shutdown")
	}
	var event localsession.Event
	if json.Unmarshal(p.scanner.Bytes(), &event) != nil || event.Type != "shutdown" {
		t.Fatal("invalid shutdown")
	}
	err := p.cmd.Wait()
	p.stopped = true
	if err != nil {
		t.Fatal("supervised process failed")
	}
	raw, err := os.ReadFile(p.stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{p.frame.Bearer, p.frame.VaultRoot, "cross-process exact update"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private data in stderr")
		}
	}
}

func recoverySaveTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[rel] = "dir"
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New("non-regular fixture file")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReaderSessionRecoveryRealSaveAcrossProcesses(t *testing.T) {
	root := writerComponentVault(t)
	old := startRecoverySaveProcess(t, root, true, nil)
	ctx := t.Context()
	created, err := old.client.WriterDailyPropose(ctx, "2026-08-20", "UTC", "restart-create")
	if err != nil {
		t.Fatal(err)
	}
	creation, err := old.client.WriterDailyApply(ctx, created, "reviewed_proposal")
	if err != nil || creation.Status != "succeeded" {
		t.Fatal("real daily creation failed", err)
	}
	source, err := old.client.WriterDailyEditorSource(ctx, created.Date, created.Timezone, created.ObjectID, creation.RevisionAfter)
	if err != nil {
		t.Fatal(err)
	}
	requested := source.Body + "\ncross-process exact update.  \r\n"
	proposal, err := old.client.WriterDailyEditPropose(ctx, pinaxclient.WriterDailyEditRequest{Date: created.Date, Timezone: created.Timezone, ObjectID: created.ObjectID, Revision: source.Revision, OperationID: "restart-edit", Body: requested})
	if err != nil || proposal.Preview.Before != source.Body || proposal.Preview.After != requested {
		t.Fatal("exact proposal mismatch", err)
	}
	// Retain the original approved handle before apply, not from a later row.
	access := app.WriterOperationAccess(old.ready.SessionID, old.frame.BindingID, old.frame.Scopes)
	grant := localsession.RecoveryLookupGrant{SchemaVersion: localsession.RecoveryGrantVersion, OperationID: "restart-edit", CapabilityID: "writer.daily.edit", OperationBindingID: "rest.writer.daily.edit", PrincipalDigest: access.Access.PrincipalDigest, ScopeDigest: access.Access.ScopeDigest, RequestDigest: proposal.PreviewDigest, Action: "lookup"}
	var calls atomic.Int32
	observed := make(chan pinaxclient.Operation, 1)
	upstream := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/writer/journals/daily/edit:apply" {
			w.WriteHeader(400)
			return
		}
		calls.Add(1)
		request, err := http.NewRequestWithContext(r.Context(), r.Method, old.ready.Address+r.URL.Path, r.Body)
		if err != nil {
			t.Error("forward construction failed")
			w.WriteHeader(502)
			return
		}
		request.Header = r.Header.Clone()
		response, err := upstream.Do(request)
		if err != nil {
			t.Error("real process apply failed")
			w.WriteHeader(502)
			return
		}
		defer func() { _ = response.Body.Close() }()
		raw, err := io.ReadAll(io.LimitReader(response.Body, 128*1024+1))
		var result struct {
			Data pinaxclient.Operation `json:"data"`
		}
		if err != nil || response.StatusCode != 200 || json.Unmarshal(raw, &result) != nil || result.Data.Status != "succeeded" {
			t.Error("owner did not commit")
			w.WriteHeader(502)
			return
		}
		observed <- result.Data
		// Real owner has committed. Destroy only the delivery, never mock success.
		w.WriteHeader(503)
	}))
	defer proxy.Close()
	lossy, err := pinaxclient.New(pinaxclient.Config{BaseURL: proxy.URL, Token: old.frame.Bearer})
	if err != nil {
		t.Fatal(err)
	}
	_, err = lossy.WriterDailyEditApply(ctx, proposal, "reviewed_proposal")
	var unknown *pinaxclient.Error
	if !errors.As(err, &unknown) || unknown.OperationRef != "restart-edit" || !unknown.ReconcileRequired || calls.Load() != 1 {
		t.Fatal("lost response did not preserve unknown identity")
	}
	var committed pinaxclient.Operation
	select {
	case committed = <-observed:
	default:
		t.Fatal("missing test oracle for actual commit")
	}
	old.stop(t)
	before := recoverySaveTree(t, root)
	next := startRecoverySaveProcess(t, root, false, &grant)
	if next.ready.SessionID == old.ready.SessionID || next.frame.Bearer == old.frame.Bearer {
		t.Fatal("old identity reused")
	}
	handle := pinaxclient.ReaderRecoveryHandle{OperationID: grant.OperationID, CapabilityID: grant.CapabilityID, BindingID: grant.OperationBindingID, RequestDigest: grant.RequestDigest}
	recovered, err := next.client.ReaderRecoveryLookup(ctx, handle)
	if err != nil {
		t.Fatal("recovery unavailable", err)
	}
	// Apply and operation lookup deliberately use distinct schema identities.
	// Compare every remaining JSON field semantically (RawMessage whitespace
	// is transport formatting, not an operation fact).
	if committed.SchemaVersion != pinaxclient.WriterApplyReceiptSchemaV1Alpha1 || recovered.SchemaVersion != pinaxclient.OperationSchemaV1 {
		t.Fatal("receipt schema identity drift")
	}
	committed.SchemaVersion = recovered.SchemaVersion
	originalJSON, err := json.Marshal(committed)
	if err != nil {
		t.Fatal(err)
	}
	recoveredJSON, err := json.Marshal(recovered)
	if err != nil {
		t.Fatal(err)
	}
	var originalFacts, recoveredFacts any
	if json.Unmarshal(originalJSON, &originalFacts) != nil || json.Unmarshal(recoveredJSON, &recoveredFacts) != nil || !reflect.DeepEqual(originalFacts, recoveredFacts) {
		t.Fatal("original receipt facts changed across processes")
	}
	doc, err := next.client.ReaderDailyDocument(ctx, pinaxclient.ReaderDailyDocumentRequest{Date: created.Date, Timezone: created.Timezone, Document: pinaxclient.ReaderDocumentRequest{ObjectID: created.ObjectID, Revision: recovered.RevisionAfter}})
	if err != nil || doc.ObjectID != created.ObjectID || doc.Revision != recovered.RevisionAfter {
		t.Fatal("fixed document readback failed", err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(proposal.Preview.Path)))
	if err != nil || strings.Count(string(body), "cross-process exact update") != 1 || !strings.HasSuffix(string(body), requested) {
		t.Fatal("original body not saved exactly once")
	}
	if _, err := next.client.WriterDailyEditApply(ctx, proposal, "reviewed_proposal"); err == nil {
		t.Fatal("readonly recovery regained write permission")
	}
	foreign := handle
	foreign.OperationID = "restart-create"
	if _, err := next.client.ReaderRecoveryLookup(ctx, foreign); err == nil {
		t.Fatal("recovery grant widened")
	}
	oldBearerClient, err := pinaxclient.New(pinaxclient.Config{BaseURL: next.ready.Address, Token: old.frame.Bearer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oldBearerClient.ReaderRecoveryLookup(ctx, handle); err == nil {
		t.Fatal("old bearer accepted in new process")
	}
	next.stop(t)
	if !reflect.DeepEqual(before, recoverySaveTree(t, root)) || calls.Load() != 1 {
		t.Fatal("new session rewrote vault or repeated apply")
	}
}
