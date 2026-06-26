package app

import (
	"context"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestRemoteCapabilitiesExposeProjectionCommandsAndGates(t *testing.T) {
	caps := RemoteCapabilities()
	byID := map[string]string{}
	for _, cap := range caps {
		byID[cap.ID] = cap.Command
		if cap.UIGroup == "" || cap.BodyExposureDefault == "" || cap.WriteGate == "" || cap.CopyCommand == "" {
			t.Fatalf("capability %s missing web-facing metadata: %#v", cap.ID, cap)
		}
		if cap.ID == "project.item.plan" && (!cap.ApprovalRequired || !cap.SnapshotRequired) {
			t.Fatalf("project.item.plan gates missing: %#v", cap)
		}
	}
	if byID["note.read"] != "note.show" || byID["project.board.show"] != "project.board.show" {
		t.Fatalf("capability commands = %#v", byID)
	}
}

func TestRemoteCapabilitiesExposeWebFacingMetadata(t *testing.T) {
	caps := RemoteCapabilities()
	byID := map[string]any{}
	for _, cap := range caps {
		byID[cap.ID] = cap
	}
	noteRead := byID["note.read"].(domain.RemoteCapability)
	if noteRead.UIGroup != "editor.note" || noteRead.BodyExposureDefault != "explicit" || noteRead.WriteGate != "readonly" {
		t.Fatalf("note.read web metadata = %#v", noteRead)
	}
	projectPlan := byID["project.item.plan"].(domain.RemoteCapability)
	if projectPlan.UIGroup != "proof.gate" || projectPlan.WriteGate != "approval_and_snapshot" {
		t.Fatalf("project.item.plan web metadata = %#v", projectPlan)
	}
	databaseView := byID["database.view.render"].(domain.RemoteCapability)
	if databaseView.UIGroup != "search.view" || databaseView.BodyExposureDefault != "none" {
		t.Fatalf("database.view.render web metadata = %#v", databaseView)
	}
	configDoctor := byID["config.doctor"].(domain.RemoteCapability)
	if configDoctor.UIGroup != "settings.control" || configDoctor.WriteGate != "readonly" || configDoctor.CopyCommand != "pinax config doctor --vault <vault> --json" {
		t.Fatalf("config.doctor web metadata = %#v", configDoctor)
	}
	configSet := byID["config.set"].(domain.RemoteCapability)
	if configSet.UIGroup != "settings.control" || configSet.WriteGate != "explicit_scope" || configSet.Readonly {
		t.Fatalf("config.set web metadata = %#v", configSet)
	}
	canvasLayout := byID["canvas.layout.metadata"].(domain.RemoteCapability)
	if canvasLayout.UIGroup != "canvas.view" || !canvasLayout.Readonly || canvasLayout.BodyAllowed || canvasLayout.BodyExposureDefault != "none" {
		t.Fatalf("canvas.layout.metadata web metadata = %#v", canvasLayout)
	}
	if canvasLayout.LocalOnlyReason != "future-client-only" || canvasLayout.CopyCommand != "pinax api routes --vault <vault> --json" {
		t.Fatalf("canvas.layout.metadata future-client boundary = %#v", canvasLayout)
	}
}

func TestFindRemoteRPCMethodReturnsRegisteredRPCOnly(t *testing.T) {
	route, ok := FindRemoteRPCMethod("Pinax.Folder.List")
	if !ok {
		t.Fatalf("expected registered RPC method")
	}
	if route.Surface != "rpc" || route.RouteID != "rpc.folder.list" || route.Command != "folder.list" || !route.Readonly {
		t.Fatalf("unexpected RPC route metadata: %#v", route)
	}

	if _, ok := FindRemoteRPCMethod("GET"); ok {
		t.Fatalf("REST method names must not match RPC metadata")
	}
	if _, ok := FindRemoteRPCMethod("Pinax.Unknown"); ok {
		t.Fatalf("unknown RPC method should not match")
	}
}

func TestAPISchemaExportUsesRegisteredRESTMethods(t *testing.T) {
	projection, err := NewService().APISchemaExport(context.Background(), APIRequest{Format: "openapi"})
	if err != nil {
		t.Fatalf("export schema: %v", err)
	}
	paths := exportedOpenAPIPaths(t, projection.Data)

	projectItemPlan := paths["/v1/project-items/{ref}:{action}"]
	if projectItemPlan == nil {
		t.Fatalf("missing project item plan path: %#v", paths)
	}
	if _, ok := projectItemPlan["post"]; !ok {
		t.Fatalf("project item plan should export post operation, got %#v", projectItemPlan)
	}
	if _, ok := projectItemPlan["get"]; ok {
		t.Fatalf("project item plan must not export get operation: %#v", projectItemPlan)
	}
}

func TestAPISchemaExportMatchesRemoteRouteRegistry(t *testing.T) {
	projection, err := NewService().APISchemaExport(context.Background(), APIRequest{Format: "openapi"})
	if err != nil {
		t.Fatalf("export schema: %v", err)
	}
	paths := exportedOpenAPIPaths(t, projection.Data)

	for _, route := range RemoteRoutes() {
		if route.Surface != "rest" || route.Path == "" {
			continue
		}
		pathItem := paths[route.Path]
		if pathItem == nil {
			t.Fatalf("missing OpenAPI path for route %s (%s)", route.RouteID, route.Path)
		}
		method := strings.ToLower(route.Method)
		operation, ok := pathItem[method].(map[string]any)
		if !ok {
			t.Fatalf("route %s should export method %s under %s, got %#v", route.RouteID, method, route.Path, pathItem)
		}
		wantExtensions := map[string]any{
			"operationId":               route.RouteID,
			"x-pinax-command":           route.Command,
			"x-pinax-capability":        route.CapabilityID,
			"x-pinax-readonly":          route.Readonly,
			"x-pinax-body-allowed":      route.BodyAllowed,
			"x-pinax-approval-required": route.ApprovalRequired,
			"x-pinax-snapshot-required": route.SnapshotRequired,
			"x-pinax-ui-group":          route.UIGroup,
			"x-pinax-body-exposure":     route.BodyExposureDefault,
			"x-pinax-write-gate":        route.WriteGate,
		}
		for key, want := range wantExtensions {
			if got := operation[key]; got != want {
				t.Fatalf("route %s extension %s = %#v, want %#v (operation %#v)", route.RouteID, key, got, want, operation)
			}
		}
	}
}

func exportedOpenAPIPaths(t *testing.T, rawData any) map[string]map[string]any {
	t.Helper()
	data, ok := rawData.(map[string]any)
	if !ok {
		t.Fatalf("projection data is not an object: %#v", rawData)
	}
	schema, ok := data["schema"].(map[string]any)
	if !ok {
		t.Fatalf("projection data missing schema: %#v", data)
	}
	rawPaths, ok := schema["paths"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing paths: %#v", schema)
	}
	paths := map[string]map[string]any{}
	for path, rawPathItem := range rawPaths {
		pathItem, ok := rawPathItem.(map[string]any)
		if !ok {
			t.Fatalf("path %s has non-object item: %#v", path, rawPathItem)
		}
		paths[path] = pathItem
	}
	return paths
}
