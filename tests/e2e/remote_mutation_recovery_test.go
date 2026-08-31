package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/pkg/pinaxclient"
)

func TestRemoteMutationRecoveryInboxResponseLossProcess(t *testing.T) {
	vault := t.TempDir()
	server := startRecoveryAPIServer(t, vault)
	proxy, forwardedMutations := startResponseLossProxy(t, server.baseURL)
	identity := pinaxclient.MutationIdentity{OperationID: "op_e2e_inbox_response_loss", IdempotencyKey: "idem_e2e_inbox_response_loss"}
	request := pinaxclient.RPCRequest{Method: "Pinax.Inbox.Capture", Params: map[string]any{
		"title": "Process recovery note", "body": "response loss fixture", "yes": true,
		"operation_id": identity.OperationID, "idempotency_key": identity.IdempotencyKey,
	}}
	client, err := pinaxclient.New(pinaxclient.Config{BaseURL: proxy.URL})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := client.CallMutationRPC(context.Background(), request, identity)
	if err != nil || projection.Facts["operation_status"] != "succeeded" || projection.Facts["recovered_from_operation"] != "true" {
		t.Fatalf("recovered projection = %#v err=%v", projection, err)
	}
	recovered, err := client.Operation(context.Background(), identity.OperationID)
	if err != nil || recovered.Status != "succeeded" || recovered.OperationID != identity.OperationID || recovered.ReceiptRef == "" || recovered.ResourceRef == "" {
		t.Fatalf("recovered operation = %#v err=%v", recovered, err)
	}
	if *forwardedMutations != 1 {
		t.Fatalf("response-loss client forwarded %d mutations", *forwardedMutations)
	}
	var result struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(recovered.Result, &result); err != nil || result.Path == "" {
		t.Fatalf("operation result=%s err=%v", recovered.Result, err)
	}
	if _, err := os.Stat(filepath.Join(vault, filepath.FromSlash(result.Path))); err != nil {
		t.Fatalf("recovered note missing: %v", err)
	}
	if events := countOperationRecordEvents(t, vault, identity.OperationID); events != 1 {
		t.Fatalf("response-loss mutation appended %d operation-bound record events", events)
	}
	if _, err := os.Stat(filepath.Join(vault, ".pinax", "api", "operations.sqlite")); err != nil {
		t.Fatalf("operation ledger missing: %v", err)
	}
}

func TestOperationReconcileFolderFinalizeCrashProcess(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	service := app.NewService()
	if _, err := service.CreateFolder(ctx, app.FolderOperationRequest{VaultPath: vault, Path: "spaces/source", Purpose: "notes"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "spaces", "source", "note.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.VersionSnapshot(ctx, app.SnapshotRequest{VaultPath: vault, Message: "before process finalize crash"}); err != nil {
		t.Fatal(err)
	}
	preview, err := service.RenameFolder(ctx, app.FolderOperationRequest{
		VaultPath: vault, Path: "spaces/source", TargetPath: "spaces/target", DryRun: true, RequireSnapshot: true,
	})
	if err != nil || preview.Facts["revision_before"] == "" || preview.Facts["snapshot_id"] == "" {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	identity := pinaxclient.MutationIdentity{OperationID: "op_e2e_folder_finalize_crash", IdempotencyKey: "idem_e2e_folder_finalize_crash"}
	digest, err := operation.CanonicalDigest(map[string]any{
		"capability_id": "folder.rename", "binding_id": "rpc.folder.rename",
		"source_path": "spaces/source", "target_path": "spaces/target",
		"expected_revision": preview.Facts["revision_before"],
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, _ := json.Marshal(map[string]string{
		"source_path": "spaces/source", "target_path": "spaces/target",
		"snapshot_id": preview.Facts["snapshot_id"], "revision_before": preview.Facts["revision_before"],
	})
	store, err := operation.Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAccepted(ctx, operation.CreateRequest{
		OperationID: identity.OperationID, IdempotencyKey: identity.IdempotencyKey,
		CapabilityID: "folder.rename", BindingID: "rpc.folder.rename", RequestDigest: digest,
		PrincipalDigest: operation.IdentityDigest("api-principal", "no-auth-loopback"),
		ScopeDigest:     operation.IdentityDigest("pinax-vault", filepath.Clean(vault)),
		RevisionBefore:  preview.Facts["revision_before"],
	})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if _, err := store.StartApplyingWithOutcome(ctx, identity.OperationID, operation.Outcome{Result: prepared}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	_ = store.Close()

	command := exec.Command(filepath.Join(sharedBinDir, "pinax"), "folder", "rename", "spaces/source", "spaces/target", "--vault", vault, "--yes", "--expected-revision", preview.Facts["revision_before"], "--json")
	command.Env = isolatedProcessEnv(t)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture rename failed: %v\n%s", err, output)
	}
	server := startRecoveryAPIServer(t, vault)
	client, err := pinaxclient.New(pinaxclient.Config{BaseURL: server.baseURL})
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := client.ReconcileOperation(ctx, identity.OperationID)
	if err != nil || reconciled.Status != "succeeded" || reconciled.RevisionAfter == "" || reconciled.ReceiptRef == "" {
		t.Fatalf("reconciled=%#v err=%v", reconciled, err)
	}
	if _, err := os.Stat(filepath.Join(vault, "spaces", "source")); !os.IsNotExist(err) {
		t.Fatalf("source still exists after one rename: %v", err)
	}
	if info, err := os.Stat(filepath.Join(vault, "spaces", "target")); err != nil || !info.IsDir() {
		t.Fatalf("target missing after reconcile: info=%v err=%v", info, err)
	}
	shown, err := client.Operation(ctx, identity.OperationID)
	if err != nil || shown.Status != "succeeded" || shown.RevisionAfter != reconciled.RevisionAfter {
		t.Fatalf("shown=%#v err=%v", shown, err)
	}
}

type recoveryAPIProcess struct {
	baseURL string
	command *exec.Cmd
	done    chan error
	stop    sync.Once
	stderr  *bytes.Buffer
}

func startRecoveryAPIServer(t *testing.T, vault string) *recoveryAPIProcess {
	t.Helper()
	command := exec.Command(filepath.Join(sharedBinDir, "pinax"), "api", "serve", "--vault", vault, "--port", "0", "--allow-write", "--no-auth", "--events")
	command.Env = isolatedProcessEnv(t)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	process := &recoveryAPIProcess{command: command, done: make(chan error, 1), stderr: stderr}
	go func() { process.done <- command.Wait() }()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var event map[string]any
			if json.Unmarshal(scanner.Bytes(), &event) == nil && event["type"] == "ready" {
				if apiURL, _ := event["url"].(string); apiURL != "" {
					ready <- apiURL
					return
				}
			}
		}
		ready <- ""
	}()
	select {
	case apiURL := <-ready:
		if apiURL == "" {
			_ = command.Process.Kill()
			t.Fatalf("API process ended before ready: %s", stderr.String())
		}
		process.baseURL = apiURL
	case err := <-process.done:
		t.Fatalf("API process failed before ready: %v stderr=%s", err, stderr.String())
	case <-time.After(10 * time.Second):
		_ = command.Process.Kill()
		t.Fatalf("API process readiness timed out: %s", stderr.String())
	}
	t.Cleanup(func() { process.shutdown() })
	return process
}

func (p *recoveryAPIProcess) shutdown() {
	p.stop.Do(func() {
		if p.command.Process != nil {
			_ = p.command.Process.Signal(os.Interrupt)
		}
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
			if p.command.Process != nil {
				_ = p.command.Process.Kill()
			}
			<-p.done
		}
	})
}

func isolatedProcessEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "PINAX_API_URL=", "NO_COLOR=1")
}

func startResponseLossProxy(t *testing.T, ownerURL string) (*httptest.Server, *int) {
	t.Helper()
	forwardedMutations := 0
	dropFirstMutation := true
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		upstreamRequest, err := http.NewRequestWithContext(context.Background(), request.Method, ownerURL+request.URL.RequestURI(), request.Body)
		if err != nil {
			http.Error(w, "proxy request failed", http.StatusBadGateway)
			return
		}
		upstreamRequest.Header = request.Header.Clone()
		response, err := http.DefaultClient.Do(upstreamRequest)
		if err != nil {
			http.Error(w, "owner request failed", http.StatusBadGateway)
			return
		}
		payload, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			http.Error(w, "owner response failed", http.StatusBadGateway)
			return
		}
		if request.URL.Path == "/v1/rpc" {
			forwardedMutations++
			if dropFirstMutation {
				dropFirstMutation = false
				connection, _, hijackErr := w.(http.Hijacker).Hijack()
				if hijackErr == nil {
					_ = connection.Close()
				}
				return
			}
		}
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(proxy.Close)
	return proxy, &forwardedMutations
}

func countOperationRecordEvents(t *testing.T, root, operationID string) int {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(root, ".pinax", "records", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	for scanner.Scan() {
		var event struct {
			IdempotencyKey string `json:"idempotency_key"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.IdempotencyKey == "operation:"+operationID {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return count
}
