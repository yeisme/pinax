package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/pkg/localsession"
)

func TestReaderSessionRecoveryProcess(t *testing.T) {
	for _, approved := range []bool{true, false} {
		t.Run(map[bool]string{true: "approved", false: "rejected"}[approved], func(t *testing.T) {
			root := t.TempDir()
			ledger, err := operation.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			original := operation.CreateRequest{OperationID: "op-process", IdempotencyKey: "key-process", CapabilityID: "writer.daily.edit", BindingID: "rest.writer.daily.edit", PrincipalDigest: operation.IdentityDigest("old-session"), ScopeDigest: operation.IdentityDigest("old-scope"), RequestDigest: operation.IdentityDigest("old-request")}
			if _, err := ledger.CreateAccepted(t.Context(), original); err != nil {
				t.Fatal(err)
			}
			if err := ledger.Close(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(sharedBinDir, "pinax"), "api", "serve", "--session-stdin", "--session-recovery-stdin")
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stdin.Close() }()
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			defer func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			bearer := strings.Repeat("b", 64)
			nonce := strings.Repeat("n", 32)
			frame := localsession.Bootstrap{SchemaVersion: localsession.BootstrapVersion, SessionNonce: nonce, Bearer: bearer, BindingID: "binding-process", VaultRoot: root, Scopes: []string{"reader.metadata"}, Readonly: true}
			if err := json.NewEncoder(stdin).Encode(frame); err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(stdout)
			scanner.Buffer(make([]byte, 1024), localsession.MaxFrameBytes)
			if !scanner.Scan() {
				t.Fatal("missing private challenge")
			}
			binding, err := localsession.DecodeRecoveryChallenge(scanner.Bytes(), nonce, frame.BindingID)
			if err != nil {
				t.Fatal("invalid private challenge")
			}
			now := time.Now().UTC()
			grant := localsession.RecoveryLookupGrant{SchemaVersion: localsession.RecoveryGrantVersion, SessionNonce: nonce, BindingID: binding.BindingID, VaultIdentityDigest: binding.VaultIdentityDigest, OperationID: original.OperationID, CapabilityID: original.CapabilityID, OperationBindingID: original.BindingID, PrincipalDigest: original.PrincipalDigest, ScopeDigest: original.ScopeDigest, RequestDigest: original.RequestDigest, Action: "lookup", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano)}
			if !approved {
				grant.Action = "apply"
			}
			if err := json.NewEncoder(stdin).Encode(grant); err != nil {
				t.Fatal(err)
			}
			if !scanner.Scan() {
				t.Fatal("missing ready or rejection")
			}
			var event localsession.Event
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				t.Fatal("invalid event")
			}
			if approved {
				if event.Type != "ready" || event.Readonly == nil || !*event.Readonly || event.Address == "" {
					t.Fatal("recovery handshake did not preserve readonly ready")
				}
				for _, id := range []string{original.OperationID, "foreign-operation"} {
					request, err := http.NewRequestWithContext(ctx, http.MethodGet, event.Address+"/v1/reader/recovery/operation?operation_id="+id, nil)
					if err != nil {
						t.Fatal(err)
					}
					request.Header.Set("Authorization", "Bearer "+bearer)
					response, err := http.DefaultClient.Do(request)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(io.LimitReader(response.Body, 128*1024+1))
					_ = response.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					if id != original.OperationID {
						if response.StatusCode != 403 {
							t.Fatal("foreign receipt exposed")
						}
						continue
					}
					var projection struct {
						Command string         `json:"command"`
						Data    operation.View `json:"data"`
					}
					if response.StatusCode != 200 || json.Unmarshal(body, &projection) != nil || projection.Command != "reader.recovery.lookup" || projection.Data.OperationID != id || projection.Data.RequestDigest != original.RequestDigest || projection.Data.Status != operation.StatusAccepted {
						t.Fatal("new process did not return original operation evidence")
					}
				}
			} else if event.Type != "error" || event.Code != "session_recovery_invalid" || event.Address != "" {
				t.Fatal("unsafe recovery became ready")
			}
			_ = stdin.Close()
			if approved {
				if !scanner.Scan() {
					t.Fatal("missing shutdown")
				}
				if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Type != "shutdown" {
					t.Fatal("bad shutdown")
				}
			}
			err = cmd.Wait()
			waited = true
			if approved && err != nil || !approved && err == nil {
				t.Fatal("incorrect process exit")
			}
			for _, secret := range []string{bearer, root, original.PrincipalDigest, original.RequestDigest, binding.VaultIdentityDigest} {
				if strings.Contains(stderr.String(), secret) {
					t.Fatal("private material in stderr")
				}
			}
		})
	}
}

func TestReaderSessionRecoveryFlagRequiresSupervision(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, filepath.Join(sharedBinDir, "pinax"), "api", "serve", "--session-recovery-stdin").CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("session_mode_required")) {
		t.Fatal("recovery flag escaped supervised mode")
	}
}
