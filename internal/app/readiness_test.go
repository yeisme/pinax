package app

import "testing"

func TestReadinessAlwaysContainsSixStableLayers(t *testing.T) {
	t.Parallel()

	readiness := BuildConnectionReadiness(ConnectionReadinessOptions{
		Mode:           "local-vault",
		Transport:      "loopback-http",
		AuthMode:       "token-store",
		OwnerAvailable: true,
	})
	if readiness.SchemaVersion != ConnectionReadinessSchemaV1 || len(readiness.Layers) != 6 {
		t.Fatalf("readiness = %#v", readiness)
	}
	for _, name := range requiredReadinessLayers {
		layer, ok := readiness.Layers[name]
		if !ok || layer.Status == "" || layer.Maturity == "" || layer.Blockers == nil || layer.NextActions == nil || layer.EvidenceRefs == nil {
			t.Fatalf("layer %q = %#v", name, layer)
		}
	}
	if readiness.Layers[ReadinessLayerMutationRecovery].Status != "blocked" || readiness.Layers[ReadinessLayerProduction].Status != "not_applicable" || readiness.Overall.Status != "degraded" {
		t.Fatalf("local API readiness overclaim = %#v", readiness)
	}
}

func TestProductionReadinessIsNeverSynthesizedFromLocalFixture(t *testing.T) {
	t.Parallel()

	for _, transport := range []string{"embedded", "stdio", "loopback-http"} {
		readiness := BuildConnectionReadiness(ConnectionReadinessOptions{Mode: "local-vault", Transport: transport, AuthMode: "none", OwnerAvailable: true, MutationRecoveryReady: true})
		if readiness.Layers[ReadinessLayerProduction].Status == "ready" {
			t.Fatalf("transport %s synthesized production ready: %#v", transport, readiness)
		}
	}
}

func TestUnknownStatusIsPreservedAndFailsSafe(t *testing.T) {
	t.Parallel()

	readiness := BuildConnectionReadiness(ConnectionReadinessOptions{Mode: "local-vault", Transport: "embedded", OwnerAvailable: true})
	contract := readiness.Layers[ReadinessLayerContract]
	contract.Status = "future_ready_extension"
	readiness.Layers[ReadinessLayerContract] = contract
	overall := SummarizeConnectionReadiness(readiness.Layers)
	if readiness.Layers[ReadinessLayerContract].Status != "future_ready_extension" || overall.Status != "blocked" {
		t.Fatalf("unknown status was not preserved fail-safe: layer=%#v overall=%#v", readiness.Layers[ReadinessLayerContract], overall)
	}
}
