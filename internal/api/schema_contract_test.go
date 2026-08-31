package api

import (
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestHTTPRouteSchemaContractsMatchRuntimeRegistry(t *testing.T) {
	t.Parallel()

	contracts, err := app.RemoteRESTContracts()
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]app.RemoteRESTContract, len(contracts))
	for _, contract := range contracts {
		byID[contract.RouteID] = contract
	}
	for _, route := range app.RemoteRoutes() {
		if route.Surface != "rest" || route.Path == "" {
			continue
		}
		contract, ok := byID[route.RouteID]
		if !ok {
			t.Errorf("registered REST route %s has no typed contract", route.RouteID)
			continue
		}
		if contract.Method != route.Method || contract.Path != route.Path || contract.CapabilityID != route.CapabilityID {
			t.Errorf("registered REST route %s drifted from typed contract: route=%#v contract=%#v", route.RouteID, route, contract)
		}
	}

	memoryCapture := byID["rest.memory.capture"]
	if memoryCapture.RequestBody == nil || memoryCapture.RequestBody.MaxBytes != maxRPCBodyBytes || memoryCapture.RequestBody.ContentType != "application/json" {
		t.Fatalf("memory capture body contract does not match HTTP decoder limit: %#v", memoryCapture.RequestBody)
	}
}
