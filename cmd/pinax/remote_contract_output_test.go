package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
)

func TestManifestReadinessOperationOutputContractsAcrossModes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("PINAX_API_URL", "")
	vault := t.TempDir()
	operationID := "op_output_modes"
	seedOutputContractOperation(t, vault, operationID)

	commands := []struct {
		name      string
		args      []string
		summary   []string
		stableKey string
	}{
		{name: "manifest", args: []string{"api", "manifest", "--vault", vault}, summary: []string{"Digest", "Capabilities", "Bindings"}, stableKey: "digest"},
		{name: "readiness", args: []string{"connection", "readiness", "--vault", vault}, summary: []string{"Contract", "Transport", "Mutation recovery", "Production"}, stableKey: "overall_status"},
		{name: "operation", args: []string{"operation", "show", operationID, "--vault", vault}, summary: []string{"Operation ID", operationID, "Succeeded"}, stableKey: "operation_id"},
	}
	modes := []struct {
		name string
		flag string
	}{
		{name: "summary"}, {name: "json", flag: "--json"}, {name: "agent", flag: "--agent"}, {name: "events", flag: "--events"}, {name: "explain", flag: "--explain"},
	}
	for _, command := range commands {
		for _, mode := range modes {
			t.Run(command.name+"/"+mode.name, func(t *testing.T) {
				args := append([]string(nil), command.args...)
				if mode.flag != "" {
					args = append(args, mode.flag)
				}
				stdout, stderr, err := runCLISeparate(args...)
				if err != nil || stderr != "" {
					t.Fatalf("err=%v stderr=%q stdout=%s", err, stderr, stdout)
				}
				for _, forbidden := range []string{"OUTPUT_BODY_SECRET", "OUTPUT_PRINCIPAL_SECRET", "OUTPUT_IDEMPOTENCY_SECRET", "/workspaces/private/output-vault", "Authorization: Bearer"} {
					if strings.Contains(stdout, forbidden) {
						t.Fatalf("%s leaked %q: %s", mode.name, forbidden, stdout)
					}
				}
				switch mode.name {
				case "summary":
					for _, want := range command.summary {
						if !strings.Contains(stdout, want) {
							t.Fatalf("summary missing %q: %s", want, stdout)
						}
					}
				case "json":
					var envelope map[string]any
					if json.Unmarshal([]byte(stdout), &envelope) != nil || envelope["mode"] != "json" {
						t.Fatalf("json output=%s", stdout)
					}
				case "agent":
					if !strings.Contains(stdout, "mode=agent") || !strings.Contains(stdout, "fact."+command.stableKey+"=") {
						t.Fatalf("agent output=%s", stdout)
					}
				case "events":
					events := parseNDJSONEvents(t, stdout)
					if len(events) != 2 || events[0]["type"] != "start" || events[1]["type"] != "end" {
						t.Fatalf("events=%#v", events)
					}
				case "explain":
					if !strings.Contains(stdout, "Conclusion:") || !strings.Contains(stdout, "Evidence:") || !strings.Contains(stdout, command.stableKey+"=") {
						t.Fatalf("explain output=%s", stdout)
					}
				}
			})
		}
	}
}

func seedOutputContractOperation(t *testing.T, root, operationID string) {
	t.Helper()
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	digest, err := operation.CanonicalDigest(map[string]any{"title": "output contract"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAccepted(context.Background(), operation.CreateRequest{
		OperationID: operationID, IdempotencyKey: "idem-" + operationID,
		CapabilityID: "inbox.capture", BindingID: "rpc.inbox.capture",
		PrincipalDigest: operation.IdentityDigest("principal", "output"),
		ScopeDigest:     operation.IdentityDigest("scope", "output"), RequestDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"path":"inbox/output.md","body":"OUTPUT_BODY_SECRET","principal_digest":"OUTPUT_PRINCIPAL_SECRET","idempotency_key":"OUTPUT_IDEMPOTENCY_SECRET","vault_path":"/workspaces/private/output-vault"}`)
	if _, err := store.StartApplyingWithOutcome(context.Background(), operationID, operation.Outcome{Result: result}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteSucceeded(context.Background(), operationID, operation.Outcome{
		Result: result, ReceiptRef: "record:output", ResourceRef: "pinax://note/output", RevisionAfter: "record:output",
	}); err != nil {
		t.Fatal(err)
	}
}
