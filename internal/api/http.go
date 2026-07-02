package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

type Server struct {
	service      *app.Service
	vault        string
	allowWrite   bool
	authMode     AuthMode
	tokenStore   TokenStore
	auditLogger  *AuditLogger
	logger       *zap.Logger
	exposeGroups []string
	hideGroups   []string
	tempSecret   string
}

type ServerOptions struct {
	AllowWrite   bool
	AuthMode     AuthMode
	TokenFile    string
	ExposeGroups []string
	HideGroups   []string
	Logger       *zap.Logger
}

type HTTPRPCRequest struct {
	ID     string         `json:"id,omitempty"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

const maxRPCBodyBytes int64 = 1 << 20

func NewServer(service *app.Service, vault string) *Server {
	return NewServerWithOptions(service, vault, ServerOptions{})
}

func NewServerWithOptions(service *app.Service, vault string, options ServerOptions) *Server {
	s := &Server{
		service:      service,
		vault:        vault,
		allowWrite:   options.AllowWrite,
		authMode:     options.AuthMode,
		exposeGroups: options.ExposeGroups,
		hideGroups:   options.HideGroups,
		logger:       options.Logger,
	}
	switch options.AuthMode {
	case AuthModeTemp:
		s.authMode = AuthModeTemp
		store := NewMemoryTokenStore()
		rec, secret := GenerateTokenRecord("temp", map[TokenScope]ScopeTarget{
			ScopeRead:  {},
			ScopeWrite: {},
		}, "", "auto")
		_ = store.Create(rec)
		s.tokenStore = store
		s.tempSecret = secret
	case AuthModeTokenFile:
		store, err := NewFileTokenStore(options.TokenFile)
		if err != nil {
			s.tokenStore = NewMemoryTokenStore()
			s.authMode = AuthModeTemp
		} else {
			s.tokenStore = store
		}
	case AuthModeNone:
		s.tokenStore = nil
	default:
		// Zero value: no auth middleware (used by tests via NewServer)
		s.authMode = 0
		s.tokenStore = nil
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	routes := []struct {
		pattern string
		handler http.HandlerFunc
		group   string
	}{
		{"/", s.handleRoot, "capabilities"},
		{"/workbench", s.handleWorkbenchPage, "capabilities"},
		{"/v1/capabilities", s.handleCapabilities, "capabilities"},
		{"/v1/workbench/status", s.handleWorkbenchStatus, "capabilities"},
		{"/v1/workbench/activity", s.handleWorkbenchActivity, "capabilities"},
		{"/v1/workbench/activity/", s.handleWorkbenchActivity, "capabilities"},
		{"/v1/monitor/runs", s.handleMonitorRuns, "capabilities"},
		{"/v1/monitor/runs/", s.handleMonitorRuns, "capabilities"},
		{"/v1/monitor/summary", s.handleMonitorSummary, "capabilities"},
		{"/v1/folders:repair-plan", s.handleFolderRepairPlan, "folders"},
		{"/v1/folders", s.handleFolders, "folders"},
		{"/v1/folders/", s.handleFolders, "folders"},
		{"/v1/inbox:capture", s.handleInboxCapture, "inbox"},
		{"/v1/inbox", s.handleInboxList, "inbox"},
		{"/v1/inbox/", s.handleInboxItem, "inbox"},
		{"/v1/drafts", s.handleDrafts, "drafts"},
		{"/v1/drafts/", s.handleDraftItem, "drafts"},
		{"/v1/notes/", s.handleNotes, "notes"},
		{"/v1/project-items/", s.handleProjectItems, "projects"},
		{"/v1/tasks/", s.handleTasks, "tasks"},
		{"/v1/database/views/", s.handleDatabaseViews, "database"},
		{"/v1/graph/summary", s.handleGraphSummary, "graph"},
		{"/v1/memory:capture", s.handleMemoryCapture, "memory"},
		{"/v1/memory:recall", s.handleMemoryRecall, "memory"},
		{"/v1/memory:context", s.handleMemoryContext, "memory"},
		{"/v1/memory:stats", s.handleMemoryStats, "memory"},
		{"/v1/memory", s.handleMemoryList, "memory"},
		{"/v1/projects", s.handleProjects, "projects"},
		{"/v1/projects/", s.handleProjects, "projects"},
	}
	mux.HandleFunc("/v1/rpc", s.handleRPC)

	for _, route := range routes {
		if !s.isGroupExposed(route.group) {
			continue
		}
		mux.HandleFunc(route.pattern, route.handler)
	}

	return s.accessLogMiddleware(s.authMiddleware(s.cacheMiddleware(mux)))
}

func (s *Server) isGroupExposed(group string) bool {
	if len(s.exposeGroups) > 0 {
		for _, g := range s.exposeGroups {
			if g == group {
				return true
			}
		}
		return false
	}
	for _, g := range s.hideGroups {
		if g == group {
			return false
		}
	}
	return true
}

// writeAudit logs an audit entry if the audit logger is configured.
func (s *Server) writeAudit(tokenID, method, path, scope, group string, status int) {
	if s.auditLogger == nil {
		return
	}
	_ = s.auditLogger.Log(AuditEntry{
		TokenID: tokenID,
		Method:  method,
		Path:    path,
		Scope:   scope,
		Group:   group,
		Status:  status,
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.handleRouteNotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.root", &domain.CommandError{Code: "method_not_allowed", Message: "API root only supports GET"}), http.StatusMethodNotAllowed)
		return
	}
	projection := domain.NewProjection("api.root", "Pinax local API is running.")
	projection.Facts["capabilities_url"] = "/v1/capabilities"
	projection.Facts["routes_command"] = "pinax api routes --vault <vault> --json"
	projection.Actions = []domain.Action{{Name: "list-routes", Command: "pinax api routes --vault <vault> --json"}}
	projection.Data = map[string]any{
		"service":          "pinax.local_api",
		"capabilities_url": "/v1/capabilities",
		"routes_command":   "pinax api routes --vault <vault> --json",
		"schema_command":   "pinax api schema export --format openapi --vault <vault> --json",
	}
	writeProjectionStatus(w, projection, http.StatusOK)
}

func (s *Server) handleWorkbenchPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/workbench" {
		s.handleRouteNotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	allowWrite := "false"
	if s.allowWrite {
		allowWrite = "true"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(strings.ReplaceAll(workbenchHTML, "{{ALLOW_WRITE}}", allowWrite)))
}

const workbenchHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Pinax Workbench</title>
<style>
:root{--bg:#f6f8fa;--surface:#fff;--surface-2:#f9fbfc;--border:#d9e1e8;--text:#1d2836;--muted:#627184;--primary:#1f6f8b;--primary-strong:#14566d;--danger:#b42318;--success:#127b4f;--warning:#a15c00}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:14px/1.5 Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}button,input,textarea,select{font:inherit}button{cursor:pointer}.workbench-shell{display:grid;grid-template-columns:248px minmax(0,1fr) 320px;min-height:100vh}.left-rail,.right-rail{background:var(--surface);border-color:var(--border);position:sticky;top:0;height:100vh;overflow:auto}.left-rail{border-right:1px solid var(--border);padding:16px}.right-rail{border-left:1px solid var(--border);padding:16px}.brand{display:flex;align-items:center;gap:10px;margin-bottom:18px}.brand-mark{display:grid;place-items:center;width:34px;height:34px;border-radius:8px;background:var(--primary);color:#fff;font-weight:700}.brand strong,.nav-title{display:block;font-size:13px}.brand span,.nav-title small,.muted{color:var(--muted);font-size:12px}.tree-section{border-top:1px solid var(--border);padding-top:14px;margin-top:14px}.tree-list,.project-list,.record-list,.capability-list{display:grid;gap:6px;margin:8px 0 0;padding:0;list-style:none}.tree-list li,.project-list li,.record-list li,.capability-list li{border:1px solid var(--border);border-radius:7px;background:var(--surface-2);padding:7px 8px;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.main{min-width:0;padding:20px 22px}.topbar{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:16px}.topbar h1{font-size:24px;line-height:1.2;margin:0 0 4px}.status-pill{display:inline-flex;align-items:center;border:1px solid var(--border);border-radius:999px;background:#eef6f8;color:var(--primary-strong);font-size:12px;font-weight:700;padding:4px 9px;white-space:nowrap}.tabs{display:flex;gap:6px;flex-wrap:wrap;border-bottom:1px solid var(--border);margin-bottom:14px}.tab{border:0;background:transparent;color:var(--muted);padding:9px 10px;border-radius:7px 7px 0 0}.tab.active{background:var(--surface);border:1px solid var(--border);border-bottom-color:var(--surface);color:var(--primary-strong);font-weight:700;margin-bottom:-1px}.panel{background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:16px;margin-bottom:14px}.panel h2{font-size:16px;margin:0 0 12px}.grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.field{display:grid;gap:5px}.field span{font-size:12px;color:var(--muted);font-weight:650}input,textarea,select{width:100%;border:1px solid var(--border);border-radius:7px;background:#fff;color:var(--text);padding:8px 9px}textarea{min-height:82px;resize:vertical}.actions{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}.btn{border:1px solid var(--primary);border-radius:7px;background:var(--primary);color:#fff;font-weight:700;padding:8px 11px}.btn.secondary{background:#fff;color:var(--text);border-color:var(--border)}.btn:disabled{cursor:not-allowed;opacity:.55}.output{border:1px solid var(--border);border-radius:8px;background:#101821;color:#d8e7f0;min-height:140px;overflow:auto;padding:12px;font:12px/1.5 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;white-space:pre-wrap}.hidden{display:none}.right-rail details{border:1px solid var(--border);border-radius:8px;background:var(--surface-2);margin-bottom:10px}.right-rail summary{cursor:pointer;font-weight:700;padding:9px 10px}.right-rail details>div{border-top:1px solid var(--border);padding:10px}.capability-list li{display:grid;gap:3px;white-space:normal}.capability-list code{font-size:11px}.error{color:var(--danger)}@media(max-width:1080px){.workbench-shell{grid-template-columns:220px minmax(0,1fr)}.right-rail{position:static;height:auto;grid-column:1/-1;border-left:0;border-top:1px solid var(--border)}}@media(max-width:760px){.workbench-shell{display:block}.left-rail,.right-rail{position:static;height:auto;border:0;border-bottom:1px solid var(--border)}.main{padding:16px}.grid{grid-template-columns:1fr}.topbar{display:block}.status-pill{margin-top:10px}.tabs{overflow-x:auto;flex-wrap:nowrap}.tab{white-space:nowrap}}
</style>
</head>
<body data-allow-write="{{ALLOW_WRITE}}">
<div class="workbench-shell">
<aside class="left-rail">
<div class="brand"><div class="brand-mark">P</div><div><strong>Pinax</strong><span>Local API Workbench</span></div></div>
<div class="tree-section"><div class="nav-title">Vault Tree<br><small>Folders from /v1/folders</small></div><ul id="vaultTree" class="tree-list"><li>Loading...</li></ul></div>
<div class="tree-section"><div class="nav-title">Projects<br><small>Project list</small></div><ul id="projectList" class="project-list"><li>Loading...</li></ul></div>
</aside>
<main class="main">
<div class="topbar"><div><h1>Workbench</h1><p class="muted">Memory and capability operations for the local vault.</p></div><span id="writeState" class="status-pill">Read-only</span></div>
<section class="panel" id="memory"><h2>Memory</h2><div class="tabs" role="tablist"><button class="tab active" data-tab="capture">Capture</button><button class="tab" data-tab="records">Records</button><button class="tab" data-tab="recall">Recall</button><button class="tab" data-tab="context">Context</button><button class="tab" data-tab="stats">Stats</button></div>
<div id="tab-capture"><div class="grid"><label class="field"><span>Type</span><select id="captureType"><option>fact</option><option>decision</option><option>event</option><option>task</option></select></label><label class="field"><span>Entity</span><input id="captureEntity" placeholder="pinax"></label><label class="field"><span>Subject</span><input id="captureSubject" placeholder="pinax"></label><label class="field"><span>Predicate</span><input id="capturePredicate" placeholder="memory_capture_usage"></label><label class="field"><span>Object</span><input id="captureObject" placeholder="Use --body or --subject and --object"></label><label class="field"><span>Source</span><input id="captureSource" placeholder="cli-help"></label></div><label class="field" style="margin-top:10px"><span>Body</span><textarea id="captureBody" placeholder="Narrative memory body"></textarea></label><div class="actions"><button class="btn secondary" id="dryRunCapture">Dry Run</button><button class="btn" id="confirmCapture">Capture</button></div></div>
<div id="tab-records" class="hidden"><div class="actions"><button class="btn secondary" id="loadRecords">Refresh Records</button></div><ul id="recordList" class="record-list"><li>No records loaded.</li></ul></div>
<div id="tab-recall" class="hidden"><div class="grid"><label class="field"><span>Query</span><input id="recallQuery" placeholder="memory capture"></label><label class="field"><span>Entity</span><input id="recallEntity" placeholder="pinax"></label></div><div class="actions"><button class="btn secondary" id="runRecall">Recall</button></div></div>
<div id="tab-context" class="hidden"><div class="grid"><label class="field"><span>Task</span><input id="contextTask" placeholder="pinax memory usage"></label><label class="field"><span>Limit</span><input id="contextLimit" value="12"></label></div><div class="actions"><button class="btn secondary" id="runContext">Context</button></div></div>
<div id="tab-stats" class="hidden"><div class="actions"><button class="btn secondary" id="loadStats">Refresh Stats</button></div></div></section>
<section class="panel" id="capabilities"><h2>Capability Explorer</h2><p class="muted">Registered local commands, REST paths, RPC methods, and write gates.</p><ul id="capabilityList" class="capability-list"><li>Loading...</li></ul></section>
<section class="panel"><h2>Output</h2><pre id="output" class="output">Ready.</pre></section>
</main>
<aside class="right-rail" id="inspector"><details id="inspector-memory" open><summary>Memory Inspector</summary><div><p class="muted">Use Capture for dry-run previews and confirmed local records. Recall and Context avoid full private memory bodies.</p><a href="#memory">Jump to Memory</a></div></details><details id="inspector-capabilities"><summary>Capability Inspector</summary><div><p class="muted">Capability metadata comes from /v1/capabilities and mirrors CLI route registration.</p><a href="#capabilities">Jump to Capabilities</a></div></details><details id="inspector-safety"><summary>Write Safety</summary><div><p class="muted">Real writes require allow-write mode and yes=true. Dry-run preview is available without persistence.</p></div></details></aside>
</div>
<script>
const allowWrite = document.body.dataset.allowWrite === 'true';
const output = document.querySelector('#output');
const writeState = document.querySelector('#writeState');
writeState.textContent = allowWrite ? 'Allow write' : 'Read-only';
document.querySelector('#confirmCapture').disabled = !allowWrite;
function show(value){ output.textContent = typeof value === 'string' ? value : JSON.stringify(value,null,2); }
async function fetchJSON(path, options){ const response = await fetch(path, options); const data = await response.json(); show(data); return data; }
function text(id){ return document.querySelector(id).value.trim(); }
function capturePayload(){ const entities = text('#captureEntity') ? [text('#captureEntity')] : []; return {type:text('#captureType')||'fact',subject:text('#captureSubject'),predicate:text('#capturePredicate'),object:text('#captureObject'),body:text('#captureBody'),source:text('#captureSource'),entities}; }
async function capture(dryRun){ const payload = capturePayload(); if(!payload.body && !(payload.subject && payload.object)){ show('Memory capture requires body, or subject and object.'); return; } const path = '/v1/memory:capture?' + new URLSearchParams(dryRun ? {dry_run:'true'} : {yes:'true'}); await fetchJSON(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)}); loadRecords(); loadStats(); }
function switchTab(name){ document.querySelectorAll('.tab').forEach(tab=>tab.classList.toggle('active',tab.dataset.tab===name)); ['capture','records','recall','context','stats'].forEach(id=>document.querySelector('#tab-'+id).classList.toggle('hidden',id!==name)); }
document.querySelectorAll('.tab').forEach(tab=>tab.addEventListener('click',()=>switchTab(tab.dataset.tab)));
document.querySelector('#dryRunCapture').addEventListener('click',()=>capture(true));
document.querySelector('#confirmCapture').addEventListener('click',()=>capture(false));
document.querySelector('#loadRecords').addEventListener('click',loadRecords);
document.querySelector('#runRecall').addEventListener('click',()=>fetchJSON('/v1/memory:recall?'+new URLSearchParams({query:text('#recallQuery'),entity:text('#recallEntity')})));
document.querySelector('#runContext').addEventListener('click',()=>fetchJSON('/v1/memory:context?'+new URLSearchParams({task:text('#contextTask'),limit:text('#contextLimit')||'12'})));
document.querySelector('#loadStats').addEventListener('click',loadStats);
async function loadVaultTree(){ try{ const data = await (await fetch('/v1/folders?include_empty=true&depth=3')).json(); const rows = data.data && data.data.folders ? data.data.folders : []; document.querySelector('#vaultTree').innerHTML = rows.length ? rows.map(row=>'<li>'+escapeHTML(row.path||row.name||'folder')+'</li>').join('') : '<li>No folders</li>'; }catch(err){ document.querySelector('#vaultTree').innerHTML='<li class="error">Unable to load</li>'; } }
async function loadProjects(){ try{ const data = await (await fetch('/v1/projects')).json(); const rows = data.data && data.data.projects ? data.data.projects : []; document.querySelector('#projectList').innerHTML = rows.length ? rows.map(row=>'<li>'+escapeHTML(row.slug||row.name||'project')+'</li>').join('') : '<li>No projects</li>'; }catch(err){ document.querySelector('#projectList').innerHTML='<li class="error">Unable to load</li>'; } }
async function loadRecords(){ const data = await fetchJSON('/v1/memory'); const records = data.data && data.data.records ? data.data.records : []; document.querySelector('#recordList').innerHTML = records.length ? records.map(record=>'<li><strong>'+escapeHTML(record.type||'memory')+'</strong> '+escapeHTML(record.subject||record.object||record.id||'record')+'</li>').join('') : '<li>No memory records</li>'; }
async function loadStats(){ await fetchJSON('/v1/memory:stats'); }
async function loadCapabilities(){ try{ const data = await (await fetch('/v1/capabilities')).json(); const routes = data.data && data.data.routes ? data.data.routes : []; document.querySelector('#capabilityList').innerHTML = routes.map(route=>'<li><strong>'+escapeHTML(route.capability_id||route.route_id)+'</strong><code>'+escapeHTML(route.method||route.rpc_method||'CALL')+' '+escapeHTML(route.path||route.rpc_method||'')+'</code><span class="muted">'+escapeHTML(route.command||'')+' · '+escapeHTML(route.write_gate||'readonly')+'</span></li>').join('') || '<li>No capabilities</li>'; }catch(err){ document.querySelector('#capabilityList').innerHTML='<li class="error">Unable to load</li>'; } }
function openDetailsForHash(){ const hash = location.hash.replace('#',''); if(!hash) return; document.querySelectorAll('.right-rail details').forEach(item=>{ item.open = false; }); if(hash === 'capabilities'){ document.querySelector('#inspector-capabilities').open = true; } else if(hash === 'memory'){ document.querySelector('#inspector-memory').open = true; } else { document.querySelector('#inspector-safety').open = true; } }
function escapeHTML(value){ return String(value||'').replace(/[&<>"]/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[ch])); }
window.addEventListener('hashchange', openDetailsForHash);
openDetailsForHash(); loadVaultTree(); loadProjects(); loadCapabilities(); loadRecords(); loadStats();
</script>
</body>
</html>`

func (s *Server) accessLogMiddleware(next http.Handler) http.Handler {
	if s.logger == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &auditStatusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		group := lookupRouteGroup(r.URL.Path)
		fields := []zap.Field{
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", status),
			zap.Duration("duration", time.Since(start)),
		}
		if group != "" {
			fields = append(fields, zap.String("group", group))
		}
		s.logger.Info("api.request", fields...)
	})
}

func (s *Server) handleFolderRepairPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.RepairFolders(r.Context(), app.FolderRepairRequest{VaultPath: s.vault, Plan: true})
	writeProjection(w, projection, err)
}

func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/folders" || r.URL.Path == "/v1/folders/" {
		s.handleFolderCollection(w, r)
		return
	}
	ref := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/folders/"), "/")
	pathPart, action, _ := strings.Cut(ref, ":")
	folderPath, err := url.PathUnescape(pathPart)
	if err != nil {
		writeProjectionStatus(w, domain.NewErrorProjection("folder.show", &domain.CommandError{Code: "unsafe_folder_path", Message: "folder path cannot be resolved"}), http.StatusBadRequest)
		return
	}
	if action == "" && r.Method == http.MethodGet {
		projection, err := s.service.ShowFolder(r.Context(), app.FolderRequest{VaultPath: s.vault, Path: folderPath})
		writeProjection(w, projection, err)
		return
	}
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	command := folderActionCommand(action)
	if command == "" {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"}), http.StatusNotFound)
		return
	}
	if !s.ensureFolderWriteAllowed(w, r, command) {
		return
	}
	query := r.URL.Query()
	var projection domain.Projection
	var callErr error
	switch action {
	case "rename":
		projection, callErr = s.service.RenameFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: folderPath, TargetPath: query.Get("target_path"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes"), RequireSnapshot: true})
	case "move":
		projection, callErr = s.service.MoveFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: folderPath, TargetParent: query.Get("target_parent"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes"), RequireSnapshot: true})
	case "delete":
		projection, callErr = s.service.DeleteFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: folderPath, EmptyOnly: boolQuery(query, "empty_only"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes"), RequireSnapshot: true})
	case "adopt":
		projection, callErr = s.service.AdoptFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: folderPath, Purpose: query.Get("purpose"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes")})
	}
	writeProjection(w, projection, callErr)
}

func (s *Server) handleFolderCollection(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	switch r.Method {
	case http.MethodGet:
		projection, err := s.service.ListFolders(r.Context(), app.FolderListRequest{VaultPath: s.vault, Purpose: query.Get("purpose"), Under: query.Get("under"), IncludeEmpty: boolQuery(query, "include_empty"), Depth: intQuery(query, "depth")})
		writeProjection(w, projection, err)
	case http.MethodPost:
		if !s.ensureFolderWriteAllowed(w, r, "folder.create") {
			return
		}
		projection, err := s.service.CreateFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: query.Get("path"), Purpose: query.Get("purpose"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes")})
		writeProjection(w, projection, err)
	default:
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
	}
}

func (s *Server) ensureFolderWriteAllowed(w http.ResponseWriter, r *http.Request, command string) bool {
	if !s.allowWrite {
		err := &domain.CommandError{Code: "write_disabled", Message: "API server is currently read-only", Hint: "Start with pinax api serve --allow-write and retry"}
		writeProjectionStatus(w, domain.NewErrorProjection(command, err), http.StatusForbidden)
		return false
	}
	query := r.URL.Query()
	if !boolQuery(query, "dry_run") && !boolQuery(query, "yes") {
		err := &domain.CommandError{Code: "approval_required", Message: "Remote folder writes require yes=true", Hint: "Preview with dry_run=true first, then append yes=true to confirm"}
		writeProjectionStatus(w, domain.NewErrorProjection(command, err), http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.APIRoutes(r.Context(), app.APIRequest{VaultPath: s.vault})
	writeProjection(w, projection, err)
}

func (s *Server) handleWorkbenchStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	writeMode := "remote_readonly"
	if s.allowWrite {
		writeMode = "remote_allow_write"
	}
	projection, err := s.service.WorkbenchStatus(r.Context(), app.APIRequest{VaultPath: s.vault, WriteMode: writeMode})
	writeProjection(w, projection, err)
}

func (s *Server) handleWorkbenchActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/v1/workbench/activity" || r.URL.Path == "/v1/workbench/activity/" {
		q := r.URL.Query()
		projection, err := s.service.ActivityList(r.Context(), app.ActivityRequest{VaultPath: s.vault, Source: q.Get("source"), Query: q.Get("query"), Status: q.Get("status"), Object: q.Get("object"), Since: q.Get("since"), Until: q.Get("until"), Limit: intQuery(q, "limit")})
		writeProjection(w, projection, err)
		return
	}
	eventID, err := url.PathUnescape(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/workbench/activity/"), "/"))
	if err != nil || eventID == "" {
		writeProjectionStatus(w, domain.NewErrorProjection("activity.show", &domain.CommandError{Code: "activity_event_not_found", Message: "activity event was not found"}), http.StatusBadRequest)
		return
	}
	projection, err := s.service.ActivityShow(r.Context(), app.ActivityRequest{VaultPath: s.vault, EventID: eventID})
	writeProjection(w, projection, err)
}

func (s *Server) handleMonitorRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/v1/monitor/runs" || r.URL.Path == "/v1/monitor/runs/" {
		q := r.URL.Query()
		projection, err := s.service.MonitorList(r.Context(), app.MonitorRequest{VaultPath: s.vault, Command: q.Get("command"), Query: q.Get("query"), Status: q.Get("status"), Since: q.Get("since"), Until: q.Get("until"), Limit: intQuery(q, "limit")})
		writeProjection(w, projection, err)
		return
	}
	runID, err := url.PathUnescape(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/monitor/runs/"), "/"))
	if err != nil || runID == "" {
		writeProjectionStatus(w, domain.NewErrorProjection("monitor.show", &domain.CommandError{Code: "monitor_run_not_found", Message: "monitor run was not found"}), http.StatusBadRequest)
		return
	}
	projection, err := s.service.MonitorShow(r.Context(), app.MonitorRequest{VaultPath: s.vault, RunID: runID})
	writeProjection(w, projection, err)
}

func (s *Server) handleMonitorSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	projection, err := s.service.MonitorSummary(r.Context(), app.MonitorRequest{VaultPath: s.vault, Command: q.Get("command"), Query: q.Get("query"), Status: q.Get("status"), Since: q.Get("since"), Until: q.Get("until"), Limit: intQuery(q, "limit")})
	writeProjection(w, projection, err)
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	ref := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/notes/"), "/")
	display := r.URL.Query().Get("display")
	if display == "" {
		display = "card"
	}
	projection, err := s.service.ShowNoteProjection(r.Context(), app.ShowNoteRequest{VaultPath: s.vault, NoteRef: ref, Display: display})
	writeProjection(w, projection, err)
}

func (s *Server) handleProjectItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	target := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/project-items/"), "/")
	if r.Method == http.MethodGet {
		if strings.Contains(target, ":") {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		projection, err := s.service.ProjectItemShow(r.Context(), app.ProjectItemRequest{VaultPath: s.vault, ItemID: target})
		writeProjection(w, projection, err)
		return
	}
	action := ""
	ref := target
	if before, after, ok := strings.Cut(target, ":"); ok {
		ref = before
		action = after
	}
	if action == "" {
		action = r.URL.Query().Get("action")
	}
	projection, err := s.service.ProjectItemPlan(r.Context(), app.ProjectItemRequest{VaultPath: s.vault, ItemID: ref, Action: action, Column: r.URL.Query().Get("column"), Yes: r.URL.Query().Get("yes") == "true"})
	writeProjection(w, projection, err)
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	target := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/tasks/"), "/")
	itemID, action, ok := strings.Cut(target, ":")
	if !ok || action != "adopt-plan" || itemID == "" {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"}), http.StatusNotFound)
		return
	}
	itemID, err := url.PathUnescape(itemID)
	if err != nil {
		writeProjectionStatus(w, domain.NewErrorProjection("task.adopt", &domain.CommandError{Code: "argument_invalid", Message: "task id cannot be resolved"}), http.StatusBadRequest)
		return
	}
	projection, err := s.service.TaskAdopt(r.Context(), app.TaskAdoptRequest{VaultPath: s.vault, ItemID: itemID, Yes: false})
	writeProjection(w, projection, err)
}

func (s *Server) handleDatabaseViews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	target := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/database/views/"), "/")
	name, action, ok := strings.Cut(target, ":")
	if !ok || action != "render" || name == "" {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"}), http.StatusNotFound)
		return
	}
	name, err := url.PathUnescape(name)
	if err != nil {
		writeProjectionStatus(w, domain.NewErrorProjection("database.view.render", &domain.CommandError{Code: "invalid_view_name", Message: "database view name cannot be resolved"}), http.StatusBadRequest)
		return
	}
	projection, err := s.service.RenderDatabaseView(r.Context(), app.ViewRequest{VaultPath: s.vault, Name: name})
	writeProjection(w, projection, err)
}

func (s *Server) handleGraphSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.GraphSummaryProjection(r.Context(), s.vault)
	writeProjection(w, projection, err)
}

func (s *Server) handleMemoryList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	projection, err := s.service.MemoryList(r.Context(), app.MemoryListRequest{VaultPath: s.vault, Type: q.Get("type"), Entity: q.Get("entity"), IncludeDraft: boolQuery(q, "include_draft") || boolQuery(q, "include-draft"), IncludeSuperseded: boolQuery(q, "include_superseded") || boolQuery(q, "include-superseded"), IncludeExpired: boolQuery(q, "include_expired") || boolQuery(q, "include-expired"), IncludeRejected: boolQuery(q, "include_rejected") || boolQuery(q, "include-rejected"), Limit: intQuery(q, "limit")})
	writeProjection(w, projection, err)
}

func (s *Server) handleMemoryCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	req, ok := s.decodeMemoryCaptureRequest(w, r)
	if !ok {
		return
	}
	if !s.ensureMemoryWriteAllowed(w, r, req.DryRun) {
		return
	}
	req.VaultPath = s.vault
	projection, err := s.service.MemoryCapture(r.Context(), req)
	writeProjection(w, projection, err)
}

func (s *Server) handleMemoryRecall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	projection, err := s.service.MemoryRecall(r.Context(), app.MemoryRecallRequest{VaultPath: s.vault, Query: q.Get("query"), Entity: q.Get("entity"), Type: q.Get("type"), Limit: intQuery(q, "limit")})
	writeProjection(w, projection, err)
}

func (s *Server) handleMemoryContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	task := q.Get("task")
	if task == "" {
		task = q.Get("query")
	}
	projection, err := s.service.MemoryContext(r.Context(), app.MemoryRecallRequest{VaultPath: s.vault, Query: task, Entity: q.Get("entity"), Type: q.Get("type"), Limit: intQuery(q, "limit")})
	writeProjection(w, projection, err)
}

func (s *Server) handleMemoryStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.MemoryStats(r.Context(), app.MemoryListRequest{VaultPath: s.vault})
	writeProjection(w, projection, err)
}

func (s *Server) decodeMemoryCaptureRequest(w http.ResponseWriter, r *http.Request) (app.MemoryCaptureRequest, bool) {
	req := app.MemoryCaptureRequest{}
	if r.Body != nil {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRPCBodyBytes))
		if err := decoder.Decode(&req); err != nil && err != io.EOF {
			writeProjectionStatus(w, domain.NewErrorProjection("memory.capture", &domain.CommandError{Code: "invalid_memory_capture_request", Message: "memory capture body must be valid JSON"}), http.StatusBadRequest)
			return app.MemoryCaptureRequest{}, false
		}
	}
	q := r.URL.Query()
	if value := q.Get("type"); value != "" {
		req.Type = value
	}
	if value := q.Get("subject"); value != "" {
		req.Subject = value
	}
	if value := q.Get("predicate"); value != "" {
		req.Predicate = value
	}
	if value := q.Get("object"); value != "" {
		req.Object = value
	}
	if value := q.Get("body"); value != "" {
		req.Body = value
	}
	if value := q.Get("status"); value != "" {
		req.Status = value
	}
	if value := q.Get("confidence"); value != "" {
		req.Confidence = value
	}
	if value := q.Get("source"); value != "" {
		req.Source = value
	}
	if value := q.Get("source_span"); value != "" {
		req.SourceSpan = value
	}
	if value := q.Get("source-span"); value != "" {
		req.SourceSpan = value
	}
	if entities := q["entity"]; len(entities) > 0 {
		req.Entities = append(req.Entities, entities...)
	}
	if entities := q["entities"]; len(entities) > 0 {
		req.Entities = append(req.Entities, entities...)
	}
	if boolQuery(q, "dry_run") || boolQuery(q, "dry-run") {
		req.DryRun = true
	}
	return req, true
}

func (s *Server) ensureMemoryWriteAllowed(w http.ResponseWriter, r *http.Request, dryRun bool) bool {
	if dryRun {
		return true
	}
	if !s.allowWrite {
		writeProjectionStatus(w, domain.NewErrorProjection("memory.capture", &domain.CommandError{Code: "write_disabled", Message: "Remote memory writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
		return false
	}
	if !boolQuery(r.URL.Query(), "yes") {
		writeProjectionStatus(w, domain.NewErrorProjection("memory.capture", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/projects" || r.URL.Path == "/v1/projects/" {
		if r.Method != http.MethodGet {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		projection, err := s.service.ListProjects(r.Context(), app.VaultRequest{VaultPath: s.vault})
		writeProjection(w, projection, err)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/projects/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"}), http.StatusNotFound)
		return
	}
	project := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		projection, err := s.service.ProjectShow(r.Context(), app.ProjectRequest{VaultPath: s.vault, Slug: project})
		writeProjection(w, projection, err)
		return
	}
	if len(parts) == 2 && parts[1] == "board" {
		if r.Method != http.MethodGet {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		display := r.URL.Query().Get("note_display")
		if display == "" {
			display = "card"
		}
		projection, err := s.service.ProjectBoardShow(r.Context(), app.ProjectBoardRequest{VaultPath: s.vault, Project: project, Subproject: r.URL.Query().Get("subproject"), NoteDisplay: display})
		writeProjection(w, projection, err)
		return
	}
	if len(parts) >= 2 && parts[1] == "subprojects" {
		s.handleProjectSubprojects(w, r, project, parts[2:])
		return
	}
	writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"}), http.StatusNotFound)
}

func (s *Server) handleProjectSubprojects(w http.ResponseWriter, r *http.Request, project string, rest []string) {
	query := r.URL.Query()
	if len(rest) == 0 {
		switch r.Method {
		case http.MethodGet:
			projection, err := s.service.ProjectSubprojectList(r.Context(), app.ProjectWorkspaceRequest{VaultPath: s.vault, Project: project})
			writeProjection(w, projection, err)
		case http.MethodPost:
			if !s.ensureProjectWriteAllowed(w, r, "project.subproject.create") {
				return
			}
			projection, err := s.service.ProjectSubprojectCreate(r.Context(), app.ProjectWorkspaceRequest{VaultPath: s.vault, Project: project, Subproject: query.Get("subproject"), Title: query.Get("title"), Template: query.Get("template"), DryRun: boolQuery(query, "dry_run")})
			writeProjection(w, projection, err)
		default:
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		}
		return
	}
	if len(rest) == 1 && r.Method == http.MethodGet {
		projection, err := s.service.ProjectSubprojectShow(r.Context(), app.ProjectWorkspaceRequest{VaultPath: s.vault, Project: project, Subproject: rest[0]})
		writeProjection(w, projection, err)
		return
	}
	writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
}

func (s *Server) ensureProjectWriteAllowed(w http.ResponseWriter, r *http.Request, command string) bool {
	if !s.allowWrite {
		err := &domain.CommandError{Code: "write_disabled", Message: "API server is currently read-only", Hint: "Start with pinax api serve --allow-write and retry"}
		writeProjectionStatus(w, domain.NewErrorProjection(command, err), http.StatusForbidden)
		return false
	}
	query := r.URL.Query()
	if !boolQuery(query, "dry_run") && !boolQuery(query, "yes") {
		err := &domain.CommandError{Code: "approval_required", Message: "Remote project writes require yes=true", Hint: "Preview with dry_run=true first, then append yes=true to confirm"}
		writeProjectionStatus(w, domain.NewErrorProjection(command, err), http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) handleInboxList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.InboxList(r.Context(), app.VaultRequest{VaultPath: s.vault})
	writeProjection(w, projection, err)
}

func (s *Server) handleInboxCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	if !s.allowWrite {
		writeProjectionStatus(w, domain.NewErrorProjection("inbox.capture", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
		return
	}
	if !boolQuery(r.URL.Query(), "yes") {
		writeProjectionStatus(w, domain.NewErrorProjection("inbox.capture", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
		return
	}
	projection, err := s.service.InboxCapture(r.Context(), app.CreateNoteRequest{VaultPath: s.vault, Title: r.URL.Query().Get("title"), Body: r.URL.Query().Get("body")})
	writeProjection(w, projection, err)
}

func (s *Server) handleInboxItem(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimPrefix(r.URL.Path, "/v1/inbox/")
	action := ""
	if idx := strings.Index(ref, ":"); idx >= 0 {
		action = ref[idx+1:]
		ref = ref[:idx]
	}
	if action == "promote" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("inbox.promote", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("inbox.promote", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		to := r.URL.Query().Get("to")
		if to == "" {
			to = "active"
		}
		projection, err := s.service.InboxPromote(r.Context(), app.InboxPromoteRequest{VaultPath: s.vault, NoteRef: ref, To: to, Group: r.URL.Query().Get("group"), Folder: r.URL.Query().Get("folder"), Kind: r.URL.Query().Get("kind"), Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	if action == "discard" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("inbox.discard", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("inbox.discard", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		projection, err := s.service.InboxDiscard(r.Context(), app.NoteMutationRequest{VaultPath: s.vault, NoteRef: ref, Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	// Default: show inbox note
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.InboxShow(r.Context(), app.ShowNoteRequest{VaultPath: s.vault, NoteRef: ref})
	writeProjection(w, projection, err)
}

func (s *Server) handleDrafts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		projection, err := s.service.DraftList(r.Context(), app.VaultRequest{VaultPath: s.vault})
		writeProjection(w, projection, err)
		return
	}
	if r.Method == http.MethodPost {
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.create", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.create", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		projection, err := s.service.DraftCreate(r.Context(), app.CreateNoteRequest{VaultPath: s.vault, Title: r.URL.Query().Get("title"), Body: r.URL.Query().Get("body")})
		writeProjection(w, projection, err)
		return
	}
	writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
}

func (s *Server) handleDraftItem(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimPrefix(r.URL.Path, "/v1/drafts/")
	action := ""
	if idx := strings.Index(ref, ":"); idx >= 0 {
		action = ref[idx+1:]
		ref = ref[:idx]
	}
	if action == "promote" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.promote", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.promote", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "active"
		}
		projection, err := s.service.DraftPromote(r.Context(), app.DraftPromoteRequest{VaultPath: s.vault, NoteRef: ref, Status: status, Folder: r.URL.Query().Get("folder"), Kind: r.URL.Query().Get("kind"), Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	if action == "archive" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.archive", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.archive", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		projection, err := s.service.DraftArchive(r.Context(), app.NoteMutationRequest{VaultPath: s.vault, NoteRef: ref, Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	if action == "discard" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.discard", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.discard", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		projection, err := s.service.DraftDiscard(r.Context(), app.NoteMutationRequest{VaultPath: s.vault, NoteRef: ref, Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	// Default: show draft note
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.DraftShow(r.Context(), app.ShowNoteRequest{VaultPath: s.vault, NoteRef: ref})
	writeProjection(w, projection, err)
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if r.Method != http.MethodPost {
		projection := domain.NewErrorProjection("api.rpc", &domain.CommandError{Code: "method_not_allowed", Message: "RPC endpoint only supports POST"})
		writeProjectionStatus(w, projection, http.StatusMethodNotAllowed)
		s.logRPCRequest(start, HTTPRPCRequest{}, domain.RemoteRoute{}, http.StatusMethodNotAllowed, projection)
		return
	}

	var req HTTPRPCRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRPCBodyBytes))
	if err := decoder.Decode(&req); err != nil {
		projection := domain.NewErrorProjection("api.rpc", &domain.CommandError{Code: "invalid_rpc_request", Message: "RPC request body must be valid JSON"})
		writeProjectionStatus(w, projection, http.StatusBadRequest)
		s.logRPCRequest(start, req, domain.RemoteRoute{}, http.StatusBadRequest, projection)
		return
	}
	if strings.TrimSpace(req.Method) == "" {
		projection := domain.NewErrorProjection("api.rpc", &domain.CommandError{Code: "rpc_method_required", Message: "RPC method is required"})
		writeProjectionStatus(w, projection, http.StatusBadRequest)
		s.logRPCRequest(start, req, domain.RemoteRoute{}, http.StatusBadRequest, projection)
		return
	}

	route, ok := app.FindRemoteRPCMethod(req.Method)
	if !ok {
		err := &domain.CommandError{Code: "rpc_method_not_found", Message: "RPC method not found", Hint: fmt.Sprintf("Check whether pinax api routes includes %s", req.Method)}
		projection := domain.NewErrorProjection("api.rpc", err)
		writeProjectionStatus(w, projection, http.StatusNotFound)
		s.logRPCRequest(start, req, domain.RemoteRoute{}, http.StatusNotFound, projection)
		return
	}
	group := rpcRouteGroup(route)
	if !s.isGroupExposed(group) {
		projection := domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"})
		writeProjectionStatus(w, projection, http.StatusNotFound)
		s.logRPCRequest(start, req, route, http.StatusNotFound, projection)
		return
	}
	if !route.Readonly {
		if boolParam(req.Params, "dry_run") {
			// Dry-run routes validate and preview without persistence, so they are safe on read-only API servers.
		} else if !s.allowWrite {
			err := &domain.CommandError{Code: "write_disabled", Message: "API server is currently read-only", Hint: "Start with pinax api serve --allow-write and retry"}
			projection := domain.NewErrorProjection(route.Command, err)
			writeProjectionStatus(w, projection, http.StatusForbidden)
			s.logRPCRequest(start, req, route, http.StatusForbidden, projection)
			return
		} else if !boolParam(req.Params, "yes") {
			err := &domain.CommandError{Code: "approval_required", Message: "Remote RPC writes require yes=true", Hint: "Preview with dry_run=true first, then include yes=true to confirm"}
			projection := domain.NewErrorProjection(route.Command, err)
			writeProjectionStatus(w, projection, http.StatusBadRequest)
			s.logRPCRequest(start, req, route, http.StatusBadRequest, projection)
			return
		}
	}

	projection, err := NewRPCDispatcherWithOptions(s.service, s.vault, DispatcherOptions{AllowWrite: s.allowWrite}).Call(r.Context(), RPCRequest{Method: req.Method, Params: req.Params})
	status := projectionHTTPStatus(projection, err)
	writeProjectionStatus(w, projection, status)
	s.logRPCRequest(start, req, route, status, projection)
}

func (s *Server) logRPCRequest(start time.Time, req HTTPRPCRequest, route domain.RemoteRoute, status int, projection domain.Projection) {
	if s.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("rpc_method", req.Method),
		zap.Int("status", status),
		zap.Duration("duration", time.Since(start)),
	}
	if req.ID != "" {
		fields = append(fields, zap.String("rpc_id", req.ID))
	}
	if route.Command != "" {
		fields = append(fields,
			zap.String("command", route.Command),
			zap.String("group", rpcRouteGroup(route)),
			zap.Bool("readonly", route.Readonly),
		)
	} else if projection.Command != "" {
		fields = append(fields, zap.String("command", projection.Command))
	}
	if projection.Error != nil {
		fields = append(fields, zap.String("error_code", projection.Error.Code))
	}
	s.logger.Info("api.rpc", fields...)
}

func projectionHTTPStatus(projection domain.Projection, err error) int {
	if err == nil {
		return http.StatusOK
	}
	if projection.Error == nil {
		return http.StatusBadRequest
	}
	switch projection.Error.Code {
	case "write_disabled", "insufficient_scope":
		return http.StatusForbidden
	case "rpc_method_not_found", "route_not_found", "note_not_found", "folder_not_found":
		return http.StatusNotFound
	case "revision_conflict", "lock_held", "folder_path_conflict", "note_path_conflict":
		return http.StatusConflict
	case "backend_unavailable", "transport_unavailable", "cloud_backend_unavailable":
		return http.StatusServiceUnavailable
	case "internal_error":
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}

func rpcRouteGroup(route domain.RemoteRoute) string {
	parts := strings.Split(route.CapabilityID, ".")
	if len(parts) == 0 {
		return ""
	}
	switch parts[0] {
	case "project":
		return "projects"
	case "folder":
		return "folders"
	case "note":
		return "notes"
	case "draft":
		return "drafts"
	default:
		return parts[0]
	}
}

func (s *Server) handleRouteNotFound(w http.ResponseWriter, r *http.Request) {
	writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"}), http.StatusNotFound)
}

func writeProjection(w http.ResponseWriter, projection domain.Projection, err error) {
	writeProjectionStatus(w, projection, projectionHTTPStatus(projection, err))
}

func writeProjectionStatus(w http.ResponseWriter, projection domain.Projection, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	projection.Mode = "json"
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(projection)
}

func ListenAndServe(ctx context.Context, service *app.Service, vault string, port int, logf func(string, ...any), options ...ServerOptions) error {
	serverOptions := ServerOptions{}
	if len(options) > 0 {
		serverOptions = options[0]
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := NewServerWithOptions(service, vault, serverOptions)

	// Init audit logger
	if srv.authMode != AuthModeNone {
		auditPath := filepath.Join(vault, ".pinax", "events", "api-audit.jsonl")
		auditLogger, auditErr := NewAuditLogger(auditPath)
		if auditErr == nil {
			srv.auditLogger = auditLogger
			defer func() { _ = auditLogger.Close() }()
		}
	}

	apiURL := fmt.Sprintf("http://%s", listener.Addr().String())
	if srv.logger != nil {
		srv.logger.Info("pinax api ready", zap.String("url", apiURL), zap.String("vault", vault), zap.Bool("allow_write", srv.allowWrite), zap.String("auth_mode", authModeLabel(srv.authMode)))
		if srv.tempSecret != "" {
			srv.logger.Warn("pinax api temp token issued", zap.String("token", srv.tempSecret), zap.String("scope", "read,write"))
		}
	} else if logf != nil {
		logf("Pinax local API: %s", apiURL)
		if srv.tempSecret != "" {
			logf("Temp token: %s", srv.tempSecret)
		}
	}

	httpServer := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	err = httpServer.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func authModeLabel(mode AuthMode) string {
	switch mode {
	case AuthModeTemp:
		return "temp-token"
	case AuthModeTokenFile:
		return "token-file"
	case AuthModeNone:
		return "none"
	default:
		return "unset"
	}
}

func folderActionCommand(action string) string {
	switch action {
	case "rename", "move", "delete", "adopt":
		return "folder." + action
	default:
		return ""
	}
}

func boolQuery(values url.Values, key string) bool {
	value := strings.ToLower(strings.TrimSpace(values.Get(key)))
	return value == "true" || value == "1" || value == "yes"
}

func intQuery(values url.Values, key string) int {
	value := strings.TrimSpace(values.Get(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}
