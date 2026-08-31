package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestRemoteContractOutputModesRemainStableAndRedacted(t *testing.T) {
	projection := domain.NewProjection("operation.show", "Operation status is succeeded.")
	projection.Facts = map[string]string{
		"operation_id": "op_output_contract", "operation_status": "succeeded",
		"retryable": "false", "replay_safe": "false", "reconcile_required": "false",
		"receipt_ref": "record:event-1", "resource_ref": "pinax://note/note-1",
	}
	projection.Data = map[string]any{"operation": map[string]any{
		"schema_version": "pinax.operation.v1", "operation_id": "op_output_contract",
		"capability_id": "inbox.capture", "binding_id": "rpc.inbox.capture", "status": "succeeded",
		"body": "REMOTE_BODY_SECRET", "principal_digest": "REMOTE_PRINCIPAL_SECRET",
		"idempotency_key": "REMOTE_IDEMPOTENCY_SECRET", "vault_path": "/workspaces/private/vault",
	}}

	for _, mode := range []Mode{ModeSummary, ModeJSON, ModeAgent, ModeEvents, ModeExplain} {
		var output bytes.Buffer
		if err := RenderWithOptions(&output, mode, projection, RenderOptions{ColorMode: "never"}); err != nil {
			t.Fatalf("render %s: %v", mode, err)
		}
		text := output.String()
		for _, forbidden := range []string{"REMOTE_BODY_SECRET", "REMOTE_PRINCIPAL_SECRET", "REMOTE_IDEMPOTENCY_SECRET", "/workspaces/private/vault"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s leaked %q: %s", mode, forbidden, text)
			}
		}
		switch mode {
		case ModeSummary:
			for _, want := range []string{"Operation ID", "Operation status", "op_output_contract", "Succeeded"} {
				if !strings.Contains(text, want) {
					t.Fatalf("summary missing %q: %s", want, text)
				}
			}
		case ModeJSON:
			var envelope map[string]any
			if json.Unmarshal(output.Bytes(), &envelope) != nil || envelope["mode"] != "json" || envelope["command"] != "operation.show" {
				t.Fatalf("json envelope=%s", text)
			}
		case ModeAgent:
			if !strings.Contains(text, "fact.operation_id=op_output_contract") || !strings.Contains(text, "fact.operation_status=succeeded") {
				t.Fatalf("agent output=%s", text)
			}
		case ModeEvents:
			lines := strings.Split(strings.TrimSpace(text), "\n")
			if len(lines) != 2 || !strings.Contains(lines[0], `"type":"start"`) || !strings.Contains(lines[1], `"type":"end"`) || !strings.Contains(lines[1], `"operation_id":"op_output_contract"`) {
				t.Fatalf("events output=%s", text)
			}
		case ModeExplain:
			if !strings.Contains(text, "Conclusion:") || !strings.Contains(text, "Evidence:") || !strings.Contains(text, "operation_id=op_output_contract") {
				t.Fatalf("explain output=%s", text)
			}
		}
	}
}

func TestConnectionReadinessSummaryRendersSixIndependentLayers(t *testing.T) {
	projection := domain.NewProjection("connection.readiness", "Pinax connection readiness is reported by independent layers.")
	projection.Facts["overall_status"] = "degraded"
	layers := map[string]any{}
	for _, name := range []string{"contract", "transport", "auth", "owner", "mutation_recovery", "production"} {
		layers[name] = map[string]any{"status": "ready", "maturity": "first-support", "blockers": []any{}, "next_actions": []any{}}
	}
	layers["production"] = map[string]any{"status": "not_configured", "maturity": "exploratory", "blockers": []any{"production_evidence_not_configured"}, "next_actions": []any{"Provide owner operations evidence"}}
	projection.Data = map[string]any{"readiness": map[string]any{"schema_version": "pinax.connection_readiness.v1", "layers": layers}}
	var output bytes.Buffer
	if err := RenderWithOptions(&output, ModeSummary, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Layer", "Contract", "Transport", "Auth", "Owner", "Mutation recovery", "Production", "Not configured", "production_evidence_not_configured"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("readiness summary missing %q: %s", want, output.String())
		}
	}
}
