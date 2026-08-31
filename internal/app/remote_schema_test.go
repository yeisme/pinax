package app

import (
	"reflect"
	"sort"
	"testing"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestRemoteRESTRouteSchemaCompleteness(t *testing.T) {
	t.Parallel()

	contracts, err := RemoteRESTContracts()
	if err != nil {
		t.Fatal(err)
	}
	wantRoutes := 0
	for _, route := range RemoteRoutes() {
		if route.Surface == "rest" && route.Path != "" {
			wantRoutes++
		}
	}
	if len(contracts) != wantRoutes {
		t.Fatalf("contracts = %d, registered REST routes = %d", len(contracts), wantRoutes)
	}

	registry, err := RemoteSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range contracts {
		for _, schemaID := range []string{contract.RequestSchema, contract.SuccessSchema, contract.SuccessDataSchema, contract.ErrorSchema, contract.ErrorDetailSchema} {
			if !registry.Has(schemaID) {
				t.Errorf("route %s schema %q is not registered", contract.RouteID, schemaID)
			}
		}
		request, _ := registry.Lookup(contract.RequestSchema)
		properties, _ := request["properties"].(map[string]any)
		if len(contract.Parameters) > 0 && len(properties) == 0 {
			t.Errorf("route %s uses an empty request schema for typed parameters", contract.RouteID)
		}
		if contract.RequestBody != nil {
			body, ok := registry.Lookup(contract.RequestBody.SchemaID)
			if !ok {
				t.Errorf("route %s body schema %q is not registered", contract.RouteID, contract.RequestBody.SchemaID)
			} else if body["additionalProperties"] != false {
				t.Errorf("route %s body schema is not closed: %#v", contract.RouteID, body)
			}
		}
	}
}

func TestRemoteRESTPathParametersMatchRegisteredPaths(t *testing.T) {
	t.Parallel()

	contracts, err := RemoteRESTContracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range contracts {
		want := make([]string, 0)
		for _, match := range pathParameterPattern.FindAllStringSubmatch(contract.Path, -1) {
			want = append(want, match[1])
		}
		got := make([]string, 0)
		for _, parameter := range contract.Parameters {
			if parameter.In == HTTPParameterPath {
				got = append(got, parameter.Name)
			}
		}
		sort.Strings(want)
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("route %s path params = %#v, want %#v", contract.RouteID, got, want)
		}
	}
}

func TestRemoteRESTContractsExposeActualEnvelopeSchemas(t *testing.T) {
	t.Parallel()

	contracts, err := RemoteRESTContracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range contracts {
		if contract.SuccessSchema != catalogschema.ProjectionV1 || contract.ErrorSchema != catalogschema.ProjectionV1 || contract.ErrorDetailSchema != catalogschema.ErrorV1 {
			t.Errorf("route %s envelope refs = success:%s error:%s detail:%s", contract.RouteID, contract.SuccessSchema, contract.ErrorSchema, contract.ErrorDetailSchema)
		}
		if contract.BodyExposureDefault == "" || contract.WriteGate == "" || contract.Stability == "" {
			t.Errorf("route %s lacks capability metadata: %#v", contract.RouteID, contract)
		}
	}
}
