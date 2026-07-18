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
	memoryCapture := byID["memory.capture"].(domain.RemoteCapability)
	if memoryCapture.UIGroup != "agent.memory" || memoryCapture.BodyExposureDefault != "explicit" || memoryCapture.WriteGate != "approval" || !memoryCapture.ApprovalRequired || memoryCapture.Readonly {
		t.Fatalf("memory.capture web metadata = %#v", memoryCapture)
	}
	if memoryCapture.CopyCommand != "pinax memory capture --type fact --subject <subject> --object <object> --vault <vault> --json" {
		t.Fatalf("memory.capture copy command = %#v", memoryCapture.CopyCommand)
	}
	memoryContext := byID["memory.context"].(domain.RemoteCapability)
	if memoryContext.UIGroup != "agent.memory" || memoryContext.WriteGate != "readonly" || !memoryContext.Readonly {
		t.Fatalf("memory.context web metadata = %#v", memoryContext)
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

func TestRemoteCapabilitiesExposePlannedBrainDiscoveryWithoutRoutes(t *testing.T) {
	caps := RemoteCapabilities()
	byID := map[string]domain.RemoteCapability{}
	for _, cap := range caps {
		byID[cap.ID] = cap
	}

	want := map[string]string{
		"brain.context.bundle":       "pinax brain context <task> --vault <vault> --json",
		"brain.answer.preview":       "pinax brain answer <question> --vault <vault> --json",
		"brain.maintenance.plan":     "pinax brain maintain --vault <vault> --dry-run --json",
		"brain.sources.list":         "pinax brain sources <question> --vault <vault> --json",
		"brain.provider.cost_status": "pinax brain provider status --vault <vault> --json",
	}
	for id, copyCommand := range want {
		cap, ok := byID[id]
		if !ok {
			t.Fatalf("missing planned brain capability %s", id)
		}
		if !cap.Readonly || cap.BodyAllowed || cap.BodyExposureDefault != "none" || cap.WriteGate != "readonly" {
			t.Fatalf("brain capability %s boundary = %#v", id, cap)
		}
		if cap.LocalOnlyReason != "future-contract" || cap.UIGroup != "agent.brain" || cap.CopyCommand != copyCommand {
			t.Fatalf("brain capability %s discovery metadata = %#v", id, cap)
		}
		if len(cap.Surfaces) != 1 || cap.Surfaces[0] != "cli" {
			t.Fatalf("brain capability %s surfaces = %#v", id, cap.Surfaces)
		}
	}

	for _, route := range RemoteRoutes() {
		if strings.HasPrefix(route.CapabilityID, "brain.") {
			t.Fatalf("planned brain capability must not register fake route: %#v", route)
		}
	}

	projection, err := NewService().APISchemaExport(context.Background(), APIRequest{Format: "openapi"})
	if err != nil {
		t.Fatalf("export schema: %v", err)
	}
	schema := projection.Data.(map[string]any)["schema"].(map[string]any)
	paths := schema["paths"].(map[string]any)
	for path := range paths {
		if strings.Contains(path, "/brain") {
			t.Fatalf("planned brain capability must not export OpenAPI path %s", path)
		}
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
			"x-pinax-release-core":      route.ReleaseCore,
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

// TestReleaseCoreCapabilitiesCoverProofLoop verifies the release convergence
// guarantee: every proof-loop scenario (bootstrap, capture, retrieve, diagnose,
// plan, apply safely, discover) has at least one release_core capability that
// agents can discover, and that CLI-local proof-loop capabilities remain
// discoverable metadata without fabricated REST paths.
func TestReleaseCoreCapabilitiesCoverProofLoop(t *testing.T) {
	caps := RemoteCapabilities()
	capByID := map[string]domain.RemoteCapability{}
	for _, cap := range caps {
		capByID[cap.ID] = cap
		if cap.ReleaseCore && cap.CopyCommand == "" {
			t.Fatalf("release core capability %s missing copy_command", cap.ID)
		}
	}

	// Every proof-loop scenario must be represented by at least one release core
	// capability, so an agent can discover the whole loop from one registry.
	requiredScenarios := map[string][]string{
		"vault bootstrap": {"vault.init", "vault.validate", "vault.stats"},
		"capture":         {"note.add", "inbox.capture", "journal.daily.append"},
		"retrieve":        {"note.search", "note.read", "memory.context", "graph.summary", "database.view.render"},
		"diagnose":        {"vault.doctor", "asset.doctor", "proof.loop.run"},
		"plan":            {"repair.plan", "organize.plan", "project.item.plan"},
		"apply safely":    {"version.snapshot", "repair.apply", "version.restore"},
		"discover":        {"api.routes", "api.schema.export", "mcp.serve"},
	}
	for scenario, ids := range requiredScenarios {
		found := false
		for _, id := range ids {
			cap, ok := capByID[id]
			if !ok {
				t.Fatalf("release core capability %s (%s) missing from registry", id, scenario)
			}
			if !cap.ReleaseCore {
				t.Fatalf("capability %s (%s) must be marked release_core", id, scenario)
			}
			found = true
		}
		if !found {
			t.Fatalf("scenario %s has no release core capability", scenario)
		}
	}

	// CLI-local proof-loop capabilities (no REST/RPC route) must carry a
	// local_only_reason so the agent knows they are CLI-gated, not remote.
	cliLocalRequired := []string{
		"vault.init", "vault.validate", "vault.doctor", "note.add",
		"repair.plan", "repair.apply", "version.snapshot", "version.restore",
		"proof.loop.run", "api.routes",
	}
	for _, id := range cliLocalRequired {
		cap := capByID[id]
		if cap.LocalOnlyReason == "" {
			t.Fatalf("CLI-local release core capability %s must carry local_only_reason", id)
		}
	}

	// OpenAPI export must not fabricate REST paths for CLI-local capabilities.
	projection, err := NewService().APISchemaExport(context.Background(), APIRequest{Format: "openapi"})
	if err != nil {
		t.Fatalf("export schema: %v", err)
	}
	paths := exportedOpenAPIPaths(t, projection.Data)
	for _, id := range cliLocalRequired {
		for path := range paths {
			if strings.Contains(path, id) {
				t.Fatalf("CLI-local capability %s must not export OpenAPI path %s", id, path)
			}
		}
	}
}

// TestReleaseCoreRoutesPropagatedToRegistry ensures REST/RPC routes inherit the
// release_core flag from their backing capability, so the registry stays the
// single source of release surface truth across CLI, API, and schema export.
func TestReleaseCoreRoutesPropagatedToRegistry(t *testing.T) {
	releaseCoreCap := map[string]bool{}
	for _, cap := range RemoteCapabilities() {
		if cap.ReleaseCore {
			if cap.ID == "" {
				t.Fatalf("release core capability has empty ID: %#v", cap)
			}
			releaseCoreCap[cap.ID] = true
		}
	}
	if len(releaseCoreCap) == 0 {
		t.Fatalf("no release core capabilities registered")
	}
	for _, route := range RemoteRoutes() {
		capIsReleaseCore := releaseCoreCap[route.CapabilityID]
		if route.ReleaseCore != capIsReleaseCore {
			t.Fatalf("route %s release_core=%v but capability %s release_core=%v", route.RouteID, route.ReleaseCore, route.CapabilityID, capIsReleaseCore)
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
