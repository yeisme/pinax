package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/yeisme/pinax/internal/app"
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
	Name        string `json:"name"`
	Description string `json:"description"`
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
		}
		return resp, nil
	case "tools/list":
		resp.Tools = []Tool{
			{Name: "pinax.search", Description: "Search local notes"},
			{Name: "pinax.note.read", Description: "Read one local note"},
			{Name: "pinax.note.links", Description: "Read outgoing links for a note"},
			{Name: "pinax.note.backlinks", Description: "Read backlinks for a note"},
			{Name: "pinax.note.context", Description: "Read bounded graph context around a note"},
			{Name: "pinax.vault.graph_summary", Description: "Read vault link graph health summary"},
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
		projection, err := s.service.ShowNoteProjection(ctx, app.ShowNoteRequest{VaultPath: s.vault, NoteRef: noteRef})
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
		resp.Result = map[string]any{
			"status":      "success",
			"summary":     "笔记图谱上下文已读取。",
			"links":       linksProj.Data,
			"backlinks":   backProj.Data,
			"next_action": fmt.Sprintf("pinax note show %s --vault %s", noteRef, s.vault),
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
	case "pinax.organize.plan":
		projection, err := s.service.PlanOrganize(ctx, app.VaultRequest{VaultPath: s.vault})
		if err != nil {
			return resp, err
		}
		resp.Result = projectionMap(projection.Status, projection.Summary, projection.Data)
		return resp, nil
	case "pinax.git.snapshot_plan":
		resp.Result = map[string]any{"status": "success", "command": fmt.Sprintf("pinax git snapshot --vault %s --message '整理前快照'", s.vault)}
		return resp, nil
	default:
		return resp, &MCPError{Code: "approval_required", Message: "MVP MCP surface 只允许只读工具"}
	}
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

func projectionMap(status, summary string, data any) map[string]any {
	return map[string]any{"status": status, "summary": summary, "data": data}
}

func (e *MCPError) Error() string { return e.Code + ": " + e.Message }
