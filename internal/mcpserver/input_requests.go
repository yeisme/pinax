package mcpserver

import (
	"context"
	"encoding/json"
	"strings"

	intake "github.com/yeisme/runtime-plane/pkg/inputintake"
)

// InputToolInventory exposes the enabled input intake tools so alternate
// protocol runtimes can register the same tools.
func InputToolInventory() []Tool { return inputTools() }

func inputTools() []Tool {
	result := []Tool{}
	for _, op := range []string{"prepare", "status", "renew", "abort", "preview", "apply"} {
		props := map[string]any{"input_request_id": map[string]any{"type": "string"}}
		required := []string{"input_request_id"}
		if op == "prepare" {
			delete(props, "input_request_id")
			required = []string{"purpose", "idempotency_key"}
			props["purpose"] = map[string]any{"type": "string", "enum": []string{"markdown", "attachment"}}
			props["idempotency_key"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 256}
			props["file"] = map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "mime": map[string]any{"type": "string"}, "size": map[string]any{"type": "integer", "minimum": 1, "maximum": 2 << 20}, "sha256": map[string]any{"type": "string"}}, "required": []string{"name", "mime", "size"}, "additionalProperties": false}
		}
		if op == "preview" || op == "apply" {
			props["conflict"] = map[string]any{"type": "string", "enum": []string{"skip", "rename", "overwrite"}, "default": "skip"}
			props["note_ref"] = map[string]any{"type": "string"}
		}
		if op == "apply" {
			props["preview_digest"] = map[string]any{"type": "string"}
			props["confirm"] = map[string]any{"type": "boolean", "enum": []any{true}}
			required = append(required, "preview_digest", "confirm")
		}
		result = append(result, Tool{Name: "pinax.input." + op, Description: "Explicitly enabled file input. Upload returns a staged reference; preview and confirm separately before importing a note or attaching a file.", Readonly: op == "status", InputSchema: map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}})
	}
	return result
}
func (s *Server) inputResource(req Request) Response {
	var caps map[string]any
	if s.input == nil {
		caps = map[string]any{"schema_version": intake.Schema, "owner": "pinax", "enabled": false, "reason": "input_not_configured"}
	} else {
		caps = s.input.Capabilities()
		caps["actions"] = map[string]string{"prepare": "pinax.input.prepare", "status": "pinax.input.status", "renew": "pinax.input.renew", "abort": "pinax.input.abort", "preview": "pinax.input.preview", "apply": "pinax.input.apply"}
		caps["tool_schemas"] = inputTools()
	}
	raw, _ := json.Marshal(caps)
	return Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"contents": []map[string]any{{"uri": "pinax://input/capabilities", "mimeType": "application/json", "text": string(raw)}}}}
}
func (s *Server) inputCall(ctx context.Context, req Request) (Response, error) {
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	if s.input == nil {
		return resp, newMCPError(-32601, "method_not_found", "Input intake is not enabled")
	}
	name, _ := req.Params["name"].(string)
	args, _ := req.Params["arguments"].(map[string]any)
	registered := false
	for _, tool := range inputTools() {
		if tool.Name == name {
			registered = true
			if err := validateToolArguments(tool, args); err != nil {
				return resp, err
			}
			break
		}
	}
	if !registered {
		return resp, newMCPError(-32601, "method_not_found", "Unknown input action")
	}
	raw, _ := json.Marshal(args)
	var input struct {
		Purpose  string       `json:"purpose"`
		Key      string       `json:"idempotency_key"`
		File     *intake.File `json:"file"`
		ID       string       `json:"input_request_id"`
		Conflict string       `json:"conflict"`
		Note     string       `json:"note_ref"`
		Digest   string       `json:"preview_digest"`
		Confirm  bool         `json:"confirm"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return resp, intake.ErrInvalid
	}
	who := intake.Identity{Actor: "stdio-owner", Project: "vault"}
	var value any
	var access intake.Access
	var err error
	switch strings.TrimPrefix(name, "pinax.input.") {
	case "prepare":
		access, err = s.input.Prepare(ctx, who, intake.Prepare{Purpose: input.Purpose, IdempotencyKey: input.Key, File: input.File})
		value = access.Request
	case "renew":
		access, err = s.input.Renew(ctx, who, input.ID)
		value = access.Request
	case "status":
		value, err = s.input.Status(ctx, who, input.ID)
	case "abort":
		value, err = s.input.Abort(ctx, who, input.ID)
	case "preview", "apply":
		if input.Conflict == "" {
			input.Conflict = "skip"
		}
		value, err = s.input.Preview(ctx, who, input.ID, input.Conflict, input.Note, strings.HasSuffix(name, ".apply") && input.Confirm, input.Digest)
	}
	if err != nil {
		return resp, newMCPError(-32000, "input_rejected", "Input rejected; query the original request, check its metadata or renew through MCP")
	}
	// Safe projection is encoded before adding transient links. Never copy links into receipts.
	encoded, _ := json.Marshal(value)
	_ = json.Unmarshal(encoded, &value)
	value = sanitizeResourceValue(value, s.vault, 0)
	body, _ := json.Marshal(value)
	content := []map[string]any{{"type": "text", "text": string(body)}}
	for _, link := range []struct{ name, url string }{{"Choose a file", access.PageURL}, {"Native HTTP transfer", access.TransferURL}} {
		if intake.CheckTransientLink(link.url) {
			content = append(content, map[string]any{"type": "resource_link", "name": link.name, "uri": link.url})
		}
	}
	resp.Result = map[string]any{"content": content, "structuredContent": value, "isError": false}
	return resp, nil
}
