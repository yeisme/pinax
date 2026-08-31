package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
	"github.com/yeisme/pinax/internal/transportmanifest"
)

type Request struct {
	JSONRPC string         `json:"jsonrpc,omitempty"`
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type Response struct {
	JSONRPC   string         `json:"jsonrpc,omitempty"`
	ID        any            `json:"id,omitempty"`
	Tools     []Tool         `json:"tools,omitempty"`
	Resources []Resource     `json:"resources,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
	Error     *MCPError      `json:"error,omitempty"`
}

type MCPError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

type Tool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	Readonly     bool           `json:"readonly,omitempty"`
	BodyExposure string         `json:"body_exposure,omitempty"`
	CostClass    string         `json:"cost_class,omitempty"`
	Scope        string         `json:"scope,omitempty"`
}

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Server struct {
	service           *app.Service
	vault             string
	agentMem          *app.AgentMemoryService
	manifest          func() (domain.Projection, error)
	strictLifecycle   bool
	legacyInitialized bool
}

func NewServer(service *app.Service, vault string) *Server {
	return NewServerWithOptions(service, vault, ServerOptions{})
}

type ServerOptions struct {
	Manifest        func() (domain.Projection, error)
	StrictLifecycle bool
	Diagnostics     io.Writer
}

func NewServerWithOptions(service *app.Service, vault string, options ServerOptions) *Server {
	manifest := options.Manifest
	if manifest == nil {
		manifest = transportmanifest.ProjectionProvider(TransportManifest)
	}
	return &Server{
		service:         service,
		vault:           vault,
		agentMem:        app.NewAgentMemoryService(),
		manifest:        manifest,
		strictLifecycle: options.StrictLifecycle,
	}
}

func (s *Server) Handle(ctx context.Context, req Request) (Response, error) {
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "server/discover":
		if err := validateModernProtocol(req); err != nil {
			return resp, err
		}
		resp.Result = modernDiscoveryResult()
		return resp, nil
	case "initialize":
		protocolVersion := mcpStringArg(req.Params, "protocolVersion")
		if protocolVersion == "" {
			protocolVersion = defaultLegacyProtocolVersion
		}
		if !containsProtocolVersion(legacyProtocolVersions, protocolVersion) {
			return resp, unsupportedLegacyProtocolVersion(protocolVersion)
		}
		s.legacyInitialized = true
		resp.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"subscribe": false, "listChanged": false},
			},
			"serverInfo": map[string]any{"name": "pinax", "version": "dev"},
			// Keep the original fields for pre-standard Pinax MCP consumers.
			"name":      "pinax",
			"read_only": true,
		}
		return resp, nil
	case "ping":
		if _, err := s.validateOperationalRequest(req); err != nil {
			return resp, err
		}
		resp.Result = map[string]any{}
		return resp, nil
	case "resources/list":
		modern, err := s.validateOperationalRequest(req)
		if err != nil {
			return resp, err
		}
		resources := ResourceInventory()
		if modern {
			resources = ConcreteResourceInventory()
			resp.Result = cacheableCompleteResult(map[string]any{"resources": resources}, 300000, "public")
		} else {
			resp.Resources = resources
			resp.Result = map[string]any{"resources": resp.Resources}
		}
		return resp, nil
	case "resources/templates/list":
		modern, err := s.validateOperationalRequest(req)
		if err != nil {
			return resp, err
		}
		templates := ResourceTemplateInventory()
		if modern {
			resp.Result = cacheableCompleteResult(map[string]any{"resourceTemplates": templates}, 300000, "public")
		} else {
			resp.Result = map[string]any{"resourceTemplates": templates}
		}
		return resp, nil
	case "resources/read":
		modern, err := s.validateOperationalRequest(req)
		if err != nil {
			return resp, err
		}
		resourceResp, readErr := s.readResource(ctx, req)
		if readErr == nil && modern {
			resourceResp.Result = cacheableCompleteResult(resourceResp.Result, 60000, "private")
		}
		return resourceResp, readErr
	case "tools/list":
		modern, err := s.validateOperationalRequest(req)
		if err != nil {
			return resp, err
		}
		tools := ToolInventory()
		if modern {
			resp.Result = cacheableCompleteResult(map[string]any{"tools": standardToolDefinitions(tools)}, 300000, "public")
		} else {
			resp.Tools = tools
			resp.Result = map[string]any{"tools": standardToolDefinitions(resp.Tools)}
		}
		return resp, nil
	case "tools/call":
		modern, err := s.validateOperationalRequest(req)
		if err != nil {
			return resp, err
		}
		toolResp, err := s.callTool(ctx, req)
		if err != nil {
			return toolResp, err
		}
		toolResp.Result = standardToolCallResult(toolResp.Result)
		if modern {
			toolResp.Result["resultType"] = "complete"
		}
		return toolResp, nil
	default:
		return resp, newMCPError(-32601, "method_not_found", "未知 MCP 方法")
	}
}

const (
	modernProtocolVersion        = "2026-07-28"
	defaultLegacyProtocolVersion = "2025-03-26"
	protocolVersionMetaKey       = "io.modelcontextprotocol/protocolVersion"
)

var legacyProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

func supportedProtocolVersions() []string {
	versions := make([]string, 0, 1+len(legacyProtocolVersions))
	versions = append(versions, modernProtocolVersion)
	versions = append(versions, legacyProtocolVersions...)
	return versions
}

func (s *Server) validateOperationalRequest(req Request) (bool, error) {
	version := modernRequestProtocolVersion(req)
	if version != "" {
		if version != modernProtocolVersion {
			return false, unsupportedModernProtocolVersion(version)
		}
		if err := validateModernClientCapabilities(req); err != nil {
			return false, err
		}
		return true, nil
	}
	if req.Method == "ping" {
		return false, nil
	}
	if s.strictLifecycle && !s.legacyInitialized {
		return false, newMCPError(-32002, "server_not_initialized", "MCP server must be initialized before legacy operations")
	}
	return false, nil
}

func validateModernProtocol(req Request) error {
	version := modernRequestProtocolVersion(req)
	if version == "" {
		return newMCPError(-32602, "protocol_version_required", "Modern MCP requests require protocol version metadata")
	}
	if version != modernProtocolVersion {
		return unsupportedModernProtocolVersion(version)
	}
	return validateModernClientCapabilities(req)
}

func modernRequestProtocolVersion(req Request) string {
	meta, _ := req.Params["_meta"].(map[string]any)
	version, _ := meta[protocolVersionMetaKey].(string)
	return strings.TrimSpace(version)
}

func validateModernClientCapabilities(req Request) error {
	meta, _ := req.Params["_meta"].(map[string]any)
	if _, ok := meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any); !ok {
		return newMCPError(-32602, "client_capabilities_required", "Modern MCP requests require client capabilities metadata")
	}
	return nil
}

func unsupportedModernProtocolVersion(requested string) *MCPError {
	err := newMCPError(-32022, "unsupported_protocol_version", "Unsupported protocol version")
	err.Data["requested"] = requested
	err.Data["supported"] = supportedProtocolVersions()
	return err
}

func unsupportedLegacyProtocolVersion(requested string) *MCPError {
	err := newMCPError(-32602, "unsupported_protocol_version", "Unsupported protocol version")
	err.Data["requested"] = requested
	err.Data["supported"] = append([]string(nil), legacyProtocolVersions...)
	return err
}

func containsProtocolVersion(versions []string, target string) bool {
	for _, version := range versions {
		if version == target {
			return true
		}
	}
	return false
}

func modernDiscoveryResult() map[string]any {
	return map[string]any{
		"resultType":        "complete",
		"supportedVersions": supportedProtocolVersions(),
		"capabilities": map[string]any{
			"tools":     map[string]any{"listChanged": false},
			"resources": map[string]any{"subscribe": false, "listChanged": false},
		},
		"_meta": map[string]any{
			"io.modelcontextprotocol/serverInfo": map[string]any{"name": "pinax", "version": "dev"},
		},
		"instructions": "Pinax exposes bounded read-only local-vault tools and resources.",
		"ttlMs":        3600000,
		"cacheScope":   "public",
	}
}

func cacheableCompleteResult(result map[string]any, ttlMS int, cacheScope string) map[string]any {
	result["resultType"] = "complete"
	result["ttlMs"] = ttlMS
	result["cacheScope"] = cacheScope
	return result
}

func (s *Server) callTool(ctx context.Context, req Request) (Response, error) {
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	name, _ := req.Params["name"].(string)
	args, _ := req.Params["arguments"].(map[string]any)
	if registration, ok := toolRegistrationByName(name); ok {
		if err := validateToolArguments(registration.Tool, args); err != nil {
			return resp, err
		}
	}
	switch name {
	case "pinax.brain.context":
		question := mcpBrainQuestion(args)
		projection, err := s.service.BrainAnswerPreview(ctx, app.BrainAnswerRequest{VaultPath: s.vault, Question: question})
		if err != nil {
			return resp, err
		}
		answer, _ := projection.Data.(domain.AgentBrainAnswer)
		resp.Result = projectionMap(projection.Status, "Bounded Agent Brain context bundle generated.", answer.ContextBundle)
		resp.Result["facts"] = projection.Facts
		resp.Result["command"] = "brain.context"
		resp.Result["body_exposure"] = "bounded_projection"
		return resp, nil
	case "pinax.brain.answer":
		question := mcpBrainQuestion(args)
		projection, err := s.service.BrainAnswerPreview(ctx, app.BrainAnswerRequest{VaultPath: s.vault, Question: question})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		resp.Result["facts"] = projection.Facts
		resp.Result["command"] = projection.Command
		return resp, nil
	case "pinax.brain.sources":
		question := mcpBrainQuestion(args)
		projection, err := s.service.BrainAnswerPreview(ctx, app.BrainAnswerRequest{VaultPath: s.vault, Question: question})
		if err != nil {
			return resp, err
		}
		answer, _ := projection.Data.(domain.AgentBrainAnswer)
		resp.Result = map[string]any{"status": projection.Status, "summary": "Bounded Agent Brain sources listed.", "command": "brain.sources", "sources": answer.Sources, "body_exposure": answer.BodyExposure, "cost": answer.Cost, "next_actions": answer.NextActions}
		return resp, nil
	case "pinax.brain.maintenance_plan":
		projection, err := s.service.BrainMaintenancePlan(ctx, app.BrainMaintenanceRequest{VaultPath: s.vault, DryRun: true})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		resp.Result["facts"] = projection.Facts
		resp.Result["command"] = projection.Command
		return resp, nil
	case "pinax.query.run":
		sql, _ := args["sql"].(string)
		projection, err := s.service.QueryRun(ctx, app.QueryRequest{VaultPath: s.vault, SQL: sql})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		resp.Result["facts"] = projection.Facts
		return resp, nil
	case "pinax.database.view.show":
		name, _ := args["name"].(string)
		projection, err := s.service.ShowView(ctx, app.ViewRequest{VaultPath: s.vault, Name: name})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		resp.Result["facts"] = projection.Facts
		return resp, nil
	case "pinax.database.view.render":
		name, _ := args["name"].(string)
		projection, err := s.service.RenderDatabaseView(ctx, app.ViewRequest{VaultPath: s.vault, Name: name})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		resp.Result["facts"] = projection.Facts
		resp.Result["command"] = projection.Command
		return resp, nil
	case "pinax.search":
		query, _ := args["query"].(string)
		projection, err := s.service.SearchProjection(ctx, app.SearchRequest{VaultPath: s.vault, Query: query})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		return resp, nil
	case "pinax.note.read":
		noteRef := mcpNoteRef(args)
		display, _ := args["display"].(string)
		if display == "" || display == string(domain.NoteDisplayBody) {
			display = string(domain.NoteDisplayCard)
		}
		projection, err := s.service.ShowNoteProjection(ctx, app.ShowNoteRequest{VaultPath: s.vault, NoteRef: noteRef, Display: display})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		return resp, nil
	case "pinax.note.links":
		noteRef := mcpNoteRef(args)
		projection, err := s.service.NoteLinks(ctx, app.NoteLinkRequest{VaultPath: s.vault, NoteRef: noteRef})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		return resp, nil
	case "pinax.note.backlinks":
		noteRef := mcpNoteRef(args)
		projection, err := s.service.NoteBacklinks(ctx, app.NoteLinkRequest{VaultPath: s.vault, NoteRef: noteRef})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		return resp, nil
	case "pinax.note.context":
		// note context 返回 links + backlinks 有界上下文，不包含 note body。
		noteRef := mcpNoteRef(args)
		linksProj, linksErr := s.service.NoteLinks(ctx, app.NoteLinkRequest{VaultPath: s.vault, NoteRef: noteRef})
		backProj, backErr := s.service.NoteBacklinks(ctx, app.NoteLinkRequest{VaultPath: s.vault, NoteRef: noteRef})
		if linksErr != nil {
			return resp, linksErr
		}
		if backErr != nil {
			return resp, backErr
		}
		facts := map[string]any{"truncated": "false"}
		linksData := boundedGraphProjectionData(linksProj.Data, "links", facts)
		backData := boundedGraphProjectionData(backProj.Data, "backlinks", facts)
		status := "success"
		if facts["truncated"] == "true" {
			status = "partial"
		}
		resp.Result = map[string]any{
			"status":      status,
			"summary":     "笔记图谱上下文已读取。",
			"facts":       facts,
			"links":       linksData,
			"backlinks":   backData,
			"next_action": fmt.Sprintf("pinax note links %s --vault %s --json", noteRef, s.vault),
		}
		return resp, nil
	case "pinax.vault.graph_summary":
		summary, err := s.service.GraphSummary(ctx, s.vault)
		if err != nil {
			return resp, err
		}
		resp.Result = map[string]any{
			"status":  "success",
			"summary": "Vault 链接图谱健康摘要已生成。",
			"data":    summary,
		}
		return resp, nil
	case "pinax.project.board":
		project, _ := args["project"].(string)
		if project == "" {
			project, _ = args["slug"].(string)
		}
		projection, err := s.service.ProjectBoardShow(ctx, app.ProjectBoardRequest{VaultPath: s.vault, Project: project, NoteDisplay: "card"})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		resp.Result["facts"] = projection.Facts
		return resp, nil
	case "pinax.task.adopt_plan":
		itemID, _ := args["item_id"].(string)
		if itemID == "" {
			itemID, _ = args["item"].(string)
		}
		projection, err := s.service.TaskAdopt(ctx, app.TaskAdoptRequest{VaultPath: s.vault, ItemID: itemID, Yes: false})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		resp.Result["facts"] = projection.Facts
		resp.Result["command"] = projection.Command
		return resp, nil
	case "pinax.organize.plan":
		projection, err := s.service.PlanOrganize(ctx, app.VaultRequest{VaultPath: s.vault})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		return resp, nil
	case "pinax.git.snapshot_plan":
		resp.Result = map[string]any{"status": "success", "command": fmt.Sprintf("pinax version snapshot --vault %s --message '整理前快照'", s.vault)}
		return resp, nil
	case "pinax.agent.context":
		projection, err := s.agentMem.AgentContextForPersonalAssistant(ctx, app.AgentContextRequest{
			VaultPath: s.vault,
			Principal: agentprotocol.DefaultAdapterPrincipal("mcp-client", "mcp"),
			Scope:     agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: mcpWorkspaceArg(args)},
			Entities:  mcpStringSliceArg(args, "entities"),
			MaxItems:  20,
			MaxChars:  8000,
		}, "")
		if err != nil {
			return resp, err
		}
		resp.Result = map[string]any{"status": "success", "command": "agent.context", "body_exposure": "bounded_projection", "pack": projection.Pack, "grounding": projection.Grounding}
		return resp, nil
	case "pinax.agent.memory_recall":
		projection, err := s.agentMem.AgentMemoryRecallForPersonalAssistant(ctx, s.vault, app.RecallQuery{
			Scope: agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: mcpWorkspaceArg(args)},
			Kinds: mcpKindSliceArg(args, "kinds"),
		}, "")
		if err != nil {
			return resp, err
		}
		resp.Result = map[string]any{"status": "success", "command": "agent.memory.recall", "body_exposure": "bounded_projection", "count": len(projection.Memories), "memories": projection.Memories, "grounding": projection.Grounding}
		return resp, nil
	case "pinax.agent.handoff_read":
		projection, err := s.agentMem.AgentHandoffReadForPersonalAssistant(ctx, s.vault, agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: mcpWorkspaceArg(args)}, "")
		if err != nil {
			return resp, err
		}
		resp.Result = map[string]any{"status": "success", "command": "agent.handoff.read", "body_exposure": "bounded_projection", "count": len(projection.Handoffs), "handoffs": projection.Handoffs, "grounding": projection.Grounding}
		return resp, nil
	default:
		return resp, newMCPError(-32001, "approval_required", "MVP MCP surface 只允许只读工具")
	}
}

// mcpStringArg 提取 string 参数，缺省返回空。
func mcpStringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func mcpWorkspaceArg(args map[string]any) string {
	workspace := strings.TrimSpace(mcpStringArg(args, "workspace"))
	if workspace == "" {
		return "default"
	}
	return workspace
}

// mcpStringSliceArg 提取 string 切片参数。
func mcpStringSliceArg(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// mcpKindSliceArg 提取 MemoryKind 切片参数。
func mcpKindSliceArg(args map[string]any, key string) []agentprotocol.MemoryKind {
	strs := mcpStringSliceArg(args, key)
	kinds := make([]agentprotocol.MemoryKind, 0, len(strs))
	for _, s := range strs {
		kinds = append(kinds, agentprotocol.MemoryKind(s))
	}
	return kinds
}

func standardToolDefinitions(tools []Tool) []map[string]any {
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		result = append(result, map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"inputSchema":  tool.InputSchema,
			"outputSchema": tool.OutputSchema,
			"annotations": map[string]any{
				"readOnlyHint":    true,
				"destructiveHint": false,
				"idempotentHint":  true,
				"openWorldHint":   false,
			},
		})
	}
	return result
}

func standardToolCallResult(result map[string]any) map[string]any {
	if result == nil {
		result = map[string]any{}
	}
	structured := make(map[string]any, len(result))
	for key, value := range result {
		structured[key] = value
	}
	payload, err := json.Marshal(structured)
	if err != nil {
		payload = []byte(`{"status":"failed","summary":"Pinax MCP result could not be encoded."}`)
	}
	status, _ := structured["status"].(string)
	if status == "" {
		status = "success"
	}
	summary, _ := structured["summary"].(string)
	if summary == "" {
		summary = "Pinax MCP tool completed."
	}
	structuredEnvelope := map[string]any{
		"schema_version": mcpToolResultSchemaVersion,
		"status":         status,
		"summary":        summary,
		"data":           structured,
	}
	if command, _ := structured["command"].(string); command != "" {
		structuredEnvelope["command"] = command
	}
	result["content"] = []map[string]any{{"type": "text", "text": string(payload)}}
	result["structuredContent"] = structuredEnvelope
	result["isError"] = false
	return result
}

const mcpToolResultSchemaVersion = catalogschema.MCPToolResultV1

func toolRegistrationByName(name string) (toolRegistration, bool) {
	for _, registration := range toolRegistrations() {
		if registration.Name == name {
			return registration, true
		}
	}
	return toolRegistration{}, false
}

func validateToolArguments(tool Tool, args map[string]any) error {
	if args == nil {
		args = map[string]any{}
	}
	properties, _ := tool.InputSchema["properties"].(map[string]any)
	for argument, value := range args {
		rawSchema, ok := properties[argument]
		if !ok {
			return invalidToolArgument(tool.Name, argument, "unknown_argument")
		}
		schema, _ := rawSchema.(map[string]any)
		if !matchesToolArgumentSchema(value, schema) {
			return invalidToolArgument(tool.Name, argument, "invalid_type_or_value")
		}
	}
	for _, required := range schemaStringList(tool.InputSchema["required"]) {
		value, exists := args[required]
		if !exists || strings.TrimSpace(fmt.Sprint(value)) == "" {
			return invalidToolArgument(tool.Name, required, "required_argument_missing")
		}
	}
	return nil
}

func matchesToolArgumentSchema(value any, schema map[string]any) bool {
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "string":
		stringValue, ok := value.(string)
		if !ok {
			return false
		}
		if enumValues := schemaStringList(schema["enum"]); len(enumValues) > 0 && !containsProtocolVersion(enumValues, stringValue) {
			return false
		}
		return true
	case "array":
		values, ok := value.([]any)
		if !ok {
			return false
		}
		itemSchema, _ := schema["items"].(map[string]any)
		for _, item := range values {
			if !matchesToolArgumentSchema(item, itemSchema) {
				return false
			}
		}
		return true
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		_, ok := value.(float64)
		return ok
	default:
		return false
	}
}

func schemaStringList(value any) []string {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if stringValue, ok := value.(string); ok {
				result = append(result, stringValue)
			}
		}
		return result
	default:
		return nil
	}
}

func invalidToolArgument(tool, argument, reason string) *MCPError {
	err := newMCPError(-32602, "invalid_tool_arguments", "Tool arguments do not match the input schema")
	err.Data["tool"] = tool
	err.Data["argument"] = argument
	err.Data["reason"] = reason
	return err
}

func mcpBrainQuestion(args map[string]any) string {
	if question, _ := args["question"].(string); strings.TrimSpace(question) != "" {
		return question
	}
	if task, _ := args["task"].(string); strings.TrimSpace(task) != "" {
		return task
	}
	return "agent brain context"
}

// mcpNoteRef 从 MCP arguments 中提取 note 引用。
func mcpNoteRef(args map[string]any) string {
	if ref, ok := args["note_ref"].(string); ok && ref != "" {
		return ref
	}
	if ref, ok := args["note_id"].(string); ok && ref != "" {
		return ref
	}
	if ref, ok := args["path"].(string); ok && ref != "" {
		return ref
	}
	return ""
}

func Serve(ctx context.Context, service *app.Service, vault string, in io.Reader, out io.Writer) error {
	return ServeWithOptions(ctx, service, vault, in, out, ServerOptions{})
}

func ServeWithOptions(ctx context.Context, service *app.Service, vault string, in io.Reader, out io.Writer, options ServerOptions) error {
	options.StrictLifecycle = true
	server := NewServerWithOptions(service, vault, options)
	diagnostics := options.Diagnostics
	if diagnostics == nil {
		diagnostics = io.Discard
	}
	enc := json.NewEncoder(out)
	type scannedFrame struct {
		data []byte
		err  error
	}
	frames := make(chan scannedFrame)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(in)
		for scanner.Scan() {
			data := append([]byte(nil), scanner.Bytes()...)
			select {
			case frames <- scannedFrame{data: data}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case frames <- scannedFrame{err: err}:
			case <-ctx.Done():
			}
		}
	}()

	for {
		var frame scannedFrame
		var ok bool
		select {
		case <-ctx.Done():
			return nil
		case frame, ok = <-frames:
			if !ok {
				return nil
			}
		}
		if frame.err != nil {
			return frame.err
		}
		var req Request
		if err := json.Unmarshal(frame.data, &req); err != nil {
			_ = enc.Encode(Response{JSONRPC: "2.0", Error: newMCPError(-32700, "parse_error", err.Error())})
			continue
		}
		// JSON-RPC notifications intentionally have no response. Standard MCP
		// clients send notifications/initialized immediately after initialize.
		if strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		resp, err := safeHandleMCPRequest(ctx, server, req, diagnostics)
		if err != nil {
			if mcpErr, ok := err.(*MCPError); ok {
				resp.Error = mcpErr
			} else {
				_, _ = fmt.Fprintf(diagnostics, "pinax mcp request failed method=%q error_type=%T\n", req.Method, err)
				resp.Error = newMCPError(-32603, "internal_error", "MCP request failed")
			}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
}

func safeHandleMCPRequest(ctx context.Context, server *Server, req Request, diagnostics io.Writer) (resp Response, err error) {
	defer func() {
		if recover() != nil {
			_, _ = fmt.Fprintf(diagnostics, "pinax mcp panic recovered method=%q\n", req.Method)
			resp = Response{JSONRPC: "2.0", ID: req.ID}
			err = newMCPError(-32603, "internal_error", "MCP request failed")
		}
	}()
	return server.Handle(ctx, req)
}

const (
	mcpGraphContextMaxEdges      = 20
	mcpGraphContextMaxCandidates = 3
	mcpGraphContextMaxEvidence   = 120
)

func boundedGraphProjectionData(data any, edgeKey string, facts map[string]any) map[string]any {
	payload, _ := data.(map[string]any)
	bounded := make(map[string]any, len(payload))
	for key, value := range payload {
		bounded[key] = value
	}
	links, _ := payload[edgeKey].([]domain.NoteLink)
	boundedLinks, truncationFacts := boundMCPNoteLinks(links)
	truncated := truncationFacts.truncated
	if len(links) > mcpGraphContextMaxEdges {
		truncated = true
	}
	if len(boundedLinks) > mcpGraphContextMaxEdges {
		boundedLinks = boundedLinks[:mcpGraphContextMaxEdges]
	}
	bounded[edgeKey] = boundedLinks
	facts[edgeKey+".total"] = fmt.Sprint(len(links))
	facts[edgeKey+".returned"] = fmt.Sprint(len(boundedLinks))
	if truncationFacts.candidates {
		facts["candidates.truncated"] = "true"
	}
	if truncationFacts.evidence {
		facts["evidence.truncated"] = "true"
	}
	if truncated {
		facts["truncated"] = "true"
	}
	return bounded
}

type graphContextTruncationFacts struct {
	truncated  bool
	candidates bool
	evidence   bool
}

func boundMCPNoteLinks(links []domain.NoteLink) ([]domain.NoteLink, graphContextTruncationFacts) {
	limit := len(links)
	if limit > mcpGraphContextMaxEdges {
		limit = mcpGraphContextMaxEdges
	}
	bounded := make([]domain.NoteLink, 0, limit)
	facts := graphContextTruncationFacts{truncated: len(links) > limit}
	for _, link := range links[:limit] {
		if link.Evidence != "" && len(link.Evidence) > mcpGraphContextMaxEvidence {
			link.Evidence = strings.TrimSpace(link.Evidence[:mcpGraphContextMaxEvidence]) + "..."
			facts.truncated = true
			facts.evidence = true
		}
		if len(link.Candidates) > mcpGraphContextMaxCandidates {
			link.Candidates = link.Candidates[:mcpGraphContextMaxCandidates]
			facts.truncated = true
			facts.candidates = true
		}
		bounded = append(bounded, link)
	}
	return bounded, facts
}

func projectionMap(status, summary string, data any) map[string]any {
	return map[string]any{"status": status, "summary": summary, "data": data}
}

func newMCPError(code int, legacyCode, message string) *MCPError {
	return &MCPError{Code: code, Message: message, Data: map[string]any{"legacy_code": legacyCode}}
}

func (e *MCPError) Error() string {
	legacyCode, _ := e.Data["legacy_code"].(string)
	if legacyCode == "" {
		legacyCode = fmt.Sprint(e.Code)
	}
	return legacyCode + ": " + e.Message
}
