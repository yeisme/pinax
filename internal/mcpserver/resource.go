package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/transportcatalog"
)

const (
	resourceContentSchemaVersion = "pinax.mcp.resource.v1"
	maxResourceContentBytes      = 64 << 10
	maxResourceArrayItems        = 20
	maxResourceStringBytes       = 512
	maxResourceDepth             = 8
	maxManifestResourceBytes     = 256 << 10
)

func (s *Server) readResource(ctx context.Context, req Request) (Response, error) {
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	uri := strings.TrimSpace(mcpStringArg(req.Params, "uri"))
	if uri == "" {
		return resp, newMCPError(-32602, "resource_uri_required", "Resource URI is required")
	}

	projection, err := s.resourceProjection(ctx, uri)
	if err != nil {
		return resp, err
	}
	var text string
	if uri == "pinax://manifest" {
		text, err = encodeManifestResourceProjection(uri, projection, s.vault)
	} else {
		text, err = encodeResourceProjection(uri, projection, s.vault)
	}
	if err != nil {
		return resp, newMCPError(-32603, "resource_encoding_failed", "Resource projection could not be encoded")
	}
	resp.Result = map[string]any{
		"contents": []map[string]any{{
			"uri":      uri,
			"mimeType": "application/json",
			"text":     text,
		}},
	}
	return resp, nil
}

func (s *Server) resourceProjection(ctx context.Context, uri string) (domain.Projection, error) {
	switch uri {
	case "pinax://manifest":
		projection, err := s.manifest()
		if err != nil {
			return projection, newMCPError(-32603, "transport_manifest_invalid", "Transport manifest could not be compiled")
		}
		return projection, nil
	case "pinax://readiness":
		projection := app.ConnectionReadinessProjection(app.ConnectionReadinessOptions{
			Mode:           "local-vault",
			Transport:      "stdio",
			OwnerAvailable: s.service != nil,
		})
		// lifecycle 事实（pinax-local-async-substrate-v1）：gateway supervise
		// 语义要求的退出/重启行为与远程写边界，与六层 readiness 一并投影。
		projection.Facts["lifecycle_exit"] = "stdin_eof_drain_exit"
		projection.Facts["lifecycle_restart_projection"] = "vault_state_consistent"
		projection.Facts["lifecycle_remote_writes"] = "gateway_approval_only"
		return projection, nil
	case "pinax://vault/current":
		return s.service.VaultStats(ctx, app.VaultStatsRequest{VaultPath: s.vault})
	case "pinax://organize/plan":
		return s.service.PlanOrganize(ctx, app.VaultRequest{VaultPath: s.vault})
	case "pinax://vault/graph":
		return s.service.GraphSummaryProjection(ctx, s.vault)
	}

	if value, ok := resourceURIValue(uri, "pinax://note/"); ok {
		return s.service.ShowNoteProjection(ctx, app.ShowNoteRequest{
			VaultPath: s.vault,
			NoteRef:   value,
			Display:   string(domain.NoteDisplayCard),
		})
	}
	if value, ok := resourceURIValue(uri, "pinax://search/"); ok {
		return s.service.SearchProjection(ctx, app.SearchRequest{VaultPath: s.vault, Query: value, Limit: maxResourceArrayItems})
	}
	if strings.HasPrefix(uri, "pinax://project/") && strings.HasSuffix(uri, "/board") {
		encodedSlug := strings.TrimSuffix(strings.TrimPrefix(uri, "pinax://project/"), "/board")
		slug, err := url.PathUnescape(encodedSlug)
		if err == nil && strings.TrimSpace(slug) != "" && !strings.Contains(slug, "/") {
			return s.service.ProjectBoardShow(ctx, app.ProjectBoardRequest{
				VaultPath:   s.vault,
				Project:     slug,
				NoteDisplay: string(domain.NoteDisplayCard),
				Compact:     true,
			})
		}
	}

	return domain.Projection{}, newMCPError(-32002, "resource_not_found", "Resource URI is not registered")
}

func encodeManifestResourceProjection(uri string, projection domain.Projection, vaultPath string) (string, error) {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return "", fmt.Errorf("manifest projection data is invalid")
	}
	rawManifest, ok := data["manifest"]
	if !ok {
		return "", fmt.Errorf("manifest projection is missing manifest data")
	}
	encodedManifest, err := json.Marshal(rawManifest)
	if err != nil {
		return "", err
	}
	var manifest transportcatalog.Manifest
	if err := json.Unmarshal(encodedManifest, &manifest); err != nil {
		return "", fmt.Errorf("decode transport manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return "", err
	}

	payload := map[string]any{
		"schema_version": resourceContentSchemaVersion,
		"uri":            uri,
		"command":        projection.Command,
		"status":         projection.Status,
		"summary":        projection.Summary,
		"facts":          sanitizeResourceStringMap(projection.Facts, vaultPath),
		"data":           map[string]any{"manifest": manifest},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if vaultPath != "" && strings.Contains(string(encoded), vaultPath) {
		return "", fmt.Errorf("manifest resource contains vault path")
	}
	if len(encoded) > maxManifestResourceBytes {
		return "", fmt.Errorf("manifest resource exceeds %d bytes", maxManifestResourceBytes)
	}
	return string(encoded), nil
}

func resourceURIValue(uri, prefix string) (string, bool) {
	if !strings.HasPrefix(uri, prefix) {
		return "", false
	}
	encoded := strings.TrimPrefix(uri, prefix)
	if encoded == "" || strings.Contains(encoded, "/") {
		return "", false
	}
	value, err := url.PathUnescape(encoded)
	if err != nil || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func encodeResourceProjection(uri string, projection domain.Projection, vaultPath string) (string, error) {
	payload := map[string]any{
		"schema_version": resourceContentSchemaVersion,
		"uri":            uri,
		"command":        projection.Command,
		"status":         projection.Status,
		"summary":        projection.Summary,
		"facts":          sanitizeResourceStringMap(projection.Facts, vaultPath),
		"data":           sanitizeResourceData(projection.Data, vaultPath),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(encoded) <= maxResourceContentBytes {
		return string(encoded), nil
	}

	// Large vaults fall back to the bounded identity/facts envelope rather than
	// failing resources/read or returning an unbounded partial payload.
	delete(payload, "data")
	payload["truncated"] = true
	encoded, err = json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(encoded) > maxResourceContentBytes {
		return "", fmt.Errorf("bounded resource payload is still too large")
	}
	return string(encoded), nil
}

func sanitizeResourceStringMap(values map[string]string, vaultPath string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if !resourceSensitiveKey(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > maxResourceArrayItems {
		keys = keys[:maxResourceArrayItems]
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = sanitizeResourceString(values[key], vaultPath)
	}
	return out
}

func sanitizeResourceData(value any, vaultPath string) any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil
	}
	return sanitizeResourceValue(decoded, vaultPath, 0)
}

func sanitizeResourceValue(value any, vaultPath string, depth int) any {
	if depth >= maxResourceDepth {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			if !resourceSensitiveKey(key) {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		if len(keys) > maxResourceArrayItems {
			keys = keys[:maxResourceArrayItems]
		}
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			if sanitized := sanitizeResourceValue(typed[key], vaultPath, depth+1); sanitized != nil {
				out[key] = sanitized
			}
		}
		return out
	case []any:
		limit := len(typed)
		if limit > maxResourceArrayItems {
			limit = maxResourceArrayItems
		}
		out := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			if sanitized := sanitizeResourceValue(item, vaultPath, depth+1); sanitized != nil {
				out = append(out, sanitized)
			}
		}
		return out
	case string:
		return sanitizeResourceString(typed, vaultPath)
	case float64, bool:
		return typed
	default:
		return nil
	}
}

func resourceSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "body" ||
		normalized == "excerpt" ||
		normalized == "snippet" ||
		normalized == "snippets" ||
		normalized == "source" ||
		normalized == "actions" ||
		normalized == "evidence" ||
		normalized == "evidence_refs" ||
		strings.Contains(normalized, "path") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "authorization") ||
		normalized == "vault" ||
		normalized == "vault_root"
}

func sanitizeResourceString(value, vaultPath string) string {
	if vaultPath != "" {
		value = strings.ReplaceAll(value, vaultPath, "<vault>")
	}
	if len(value) > maxResourceStringBytes {
		value = value[:maxResourceStringBytes] + "..."
	}
	return value
}
