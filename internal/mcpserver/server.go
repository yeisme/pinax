package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

type Request struct {
	JSONRPC string         `json:"jsonrpc,omitempty"`
	ID      int            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type Response struct {
	JSONRPC   string         `json:"jsonrpc,omitempty"`
	ID        int            `json:"id,omitempty"`
	Tools     []Tool         `json:"tools,omitempty"`
	Resources []Resource     `json:"resources,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
	Error     *MCPError      `json:"error,omitempty"`
}

type MCPError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Tool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
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
	service *app.Service
	vault   string
}

func NewServer(service *app.Service, vault string) *Server {
	return &Server{service: service, vault: vault}
}

func (s *Server) Handle(ctx context.Context, req Request) (Response, error) {
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{"name": "pinax", "read_only": true}
		return resp, nil
	case "resources/list":
		resp.Resources = []Resource{
			{URI: "pinax://vault/current", Name: "current vault"},
			{URI: "pinax://note/{note_id}", Name: "note by id"},
			{URI: "pinax://search/{query}", Name: "search notes"},
			{URI: "pinax://organize/plan", Name: "organize plan"},
			{URI: "pinax://vault/graph", Name: "vault link graph"},
			{URI: "pinax://project/{slug}/board", Name: "project board", Description: "bounded readonly project board"},
		}
		return resp, nil
	case "tools/list":
		resp.Tools = []Tool{
			{Name: "pinax.search", Description: "Search local notes"},
			brainTool("pinax.brain.context", "Read bounded Agent Brain context bundle"),
			brainTool("pinax.brain.answer", "Preview a citation-first bounded answer"),
			brainTool("pinax.brain.sources", "List bounded Agent Brain evidence sources"),
			brainTool("pinax.brain.maintenance_plan", "Preview Agent Brain maintenance next steps without writing"),
			{Name: "pinax.query.run", Description: "Run bounded readonly Pinax SQL query"},
			{Name: "pinax.database.view.show", Description: "Show saved readonly database view"},
			{Name: "pinax.database.view.render", Description: "Render saved readonly database view as a bounded tab projection"},
			{Name: "pinax.note.read", Description: "Read one local note"},
			{Name: "pinax.note.links", Description: "Read outgoing links for a note"},
			{Name: "pinax.note.backlinks", Description: "Read backlinks for a note"},
			{Name: "pinax.note.context", Description: "Read bounded graph context around a note"},
			{Name: "pinax.vault.graph_summary", Description: "Read vault link graph health summary"},
			{Name: "pinax.project.board", Description: "Read bounded project board facts"},
			{Name: "pinax.task.adopt_plan", Description: "Preview inferred task adoption without writing"},
			{Name: "pinax.organize.plan", Description: "Preview organize operations"},
			{Name: "pinax.git.snapshot_plan", Description: "Show snapshot command"},
		}
		return resp, nil
	case "tools/call":
		return s.callTool(ctx, req)
	default:
		return resp, &MCPError{Code: "method_not_found", Message: "未知 MCP 方法"}
	}
}

func (s *Server) callTool(ctx context.Context, req Request) (Response, error) {
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	name, _ := req.Params["name"].(string)
	args, _ := req.Params["arguments"].(map[string]any)
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
	default:
		return resp, &MCPError{Code: "approval_required", Message: "MVP MCP surface 只允许只读工具"}
	}
}

func brainTool(name, description string) Tool {
	return Tool{Name: name, Description: description, Readonly: true, BodyExposure: "bounded_projection", CostClass: "none", Scope: "local_vault", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"question": map[string]any{"type": "string"}, "task": map[string]any{"type": "string"}, "body_exposure": map[string]any{"type": "string", "enum": []string{"bounded_projection"}}}}}
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
	server := NewServer(service, vault)
	scanner := bufio.NewScanner(in)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = enc.Encode(Response{JSONRPC: "2.0", Error: &MCPError{Code: "parse_error", Message: err.Error()}})
			continue
		}
		resp, err := server.Handle(ctx, req)
		if err != nil {
			if mcpErr, ok := err.(*MCPError); ok {
				resp.Error = mcpErr
			} else {
				resp.Error = &MCPError{Code: "internal_error", Message: err.Error()}
			}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
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

func (e *MCPError) Error() string { return e.Code + ": " + e.Message }
