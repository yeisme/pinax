package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

type Server struct {
	service *app.Service
	vault   string
}

func NewServer(service *app.Service, vault string) *Server {
	return &Server{service: service, vault: vault}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/overview", s.handleOverview)
	mux.HandleFunc("/api/graph-summary", s.handleGraphSummary)
	mux.HandleFunc("/api/project-board/", s.handleProjectBoard)
	mux.HandleFunc("/api/note-display/", s.handleNoteDisplay)
	mux.HandleFunc("/api/database-tabs/", s.handleDatabaseTab)
	mux.HandleFunc("/api/repair-plans", s.handleRepairPlans)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	statsProjection, statsErr := s.service.VaultStats(r.Context(), app.VaultStatsRequest{VaultPath: s.vault})
	doctorProjection, doctorErr := s.service.VaultDoctor(r.Context(), app.VaultDoctorRequest{VaultPath: s.vault})
	repairProjection, repairErr := s.service.ListRepairPlans(r.Context(), app.VaultRequest{VaultPath: s.vault})
	graphProjection, graphErr := s.graphSummaryProjection(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	writeHTML(w, "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>Pinax Dashboard</title><style>%s</style></head><body>", dashboardCSS)
	writeHTML(w, "<div class=\"shell\"><aside class=\"sidebar\"><div class=\"brand\"><span class=\"brand-mark\">P</span><div><strong>Pinax</strong><span>Local vault</span></div></div><nav><a class=\"active\" href=\"#overview\">Overview</a><a href=\"#health\">Health</a><a href=\"#repair\">Repair</a><a href=\"#data\">Data</a></nav></aside><main class=\"main\">")
	writeHTML(w, "<header class=\"page-header\"><div><p class=\"eyebrow\">Read-only dashboard</p><h1>Pinax Vault Dashboard</h1><p>本地 Markdown vault 的状态、关系健康和维护入口。</p></div><div class=\"header-actions\"><a class=\"button secondary\" href=\"/api/overview\">JSON overview</a><a class=\"button\" href=\"/api/graph-summary\">Graph summary</a></div></header>")
	if statsErr != nil || doctorErr != nil {
		writeHTML(w, "<section class=\"panel\"><div class=\"panel-heading\"><h2>状态</h2></div><p class=\"error-text\">%s %s</p></section>", html.EscapeString(fmt.Sprint(statsErr)), html.EscapeString(fmt.Sprint(doctorErr)))
		writeHTML(w, "</main></div></body></html>")
		return
	}
	stats, _ := statsProjection.Data.(domain.VaultStats)
	report, _ := doctorProjection.Data.(domain.VaultDoctorReport)
	writeHTML(w, "<section id=\"overview\" class=\"status-strip\" aria-label=\"Vault status\">")
	writeDashboardMetric(w, "笔记", fmt.Sprint(stats.NoteCount), "Indexed markdown notes")
	writeDashboardMetric(w, "标签", fmt.Sprint(stats.TagCount), "Distinct vault tags")
	writeDashboardMetric(w, "frontmatter", fmt.Sprintf("%d%%", stats.FrontmatterCoverage), "Metadata coverage")
	writeHTML(w, "<article class=\"metric\"><span>索引状态</span><strong><span class=\"pill %s\">%s</span></strong><small>%s</small></article>", dashboardStatusClass(stats.IndexStatus), html.EscapeString(stats.IndexStatus), html.EscapeString(stats.IndexPath))
	writeHTML(w, "</section>")
	if graphErr != nil {
		writeHTML(w, "<section class=\"panel\"><div class=\"panel-heading\"><div><p class=\"eyebrow\">Link graph</p><h2>关系</h2></div></div><p class=\"error-text\">%s</p></section>", html.EscapeString(fmt.Sprint(graphErr)))
	} else {
		graph, _ := graphProjection.Data.(domain.NoteGraphProjection)
		writeHTML(w, "<section class=\"panel\"><div class=\"panel-heading\"><div><p class=\"eyebrow\">Link graph</p><h2>关系</h2></div><span class=\"pill %s\">engine %s</span></div>", dashboardStatusClass(graph.IndexStatus), html.EscapeString(graph.Engine))
		writeHTML(w, "<div class=\"graph-grid\">")
		writeDashboardMetric(w, "链接", fmt.Sprint(graph.TotalLinks), "Total relationships")
		writeDashboardMetric(w, "断链", fmt.Sprint(graph.Broken), "Broken wikilinks")
		writeDashboardMetric(w, "歧义", fmt.Sprint(graph.Ambiguous), "Ambiguous targets")
		writeDashboardMetric(w, "孤立", fmt.Sprint(graph.Orphans), "Notes with no links")
		writeHTML(w, "</div>")
		if len(graphProjection.Actions) > 0 {
			writeHTML(w, "<div class=\"next-action\"><span>推荐下一步</span><code>%s</code></div>", html.EscapeString(graphProjection.Actions[0].Command))
		}
		writeHTML(w, "</section>")
	}
	writeHTML(w, "<section id=\"health\" class=\"panel\"><div class=\"panel-heading\"><div><p class=\"eyebrow\">Diagnostics</p><h2>健康</h2></div><span class=\"pill info\">问题 %d 个</span></div>", len(report.Issues))
	if len(report.Issues) == 0 {
		writeHTML(w, "<div class=\"empty-state\">当前没有 dashboard 可见问题。</div>")
	} else {
		writeHTML(w, "<div class=\"issue-table\" role=\"table\" aria-label=\"Vault health issues\"><div class=\"issue-row head\" role=\"row\"><span>Issue</span><span>Severity</span><span>Path</span></div>")
	}
	for i, issue := range report.Issues {
		if i >= 20 {
			break
		}
		writeHTML(w, "<div class=\"issue-row\" role=\"row\"><span><code>%s</code></span><span><span class=\"pill %s\">%s</span></span><span class=\"path\" title=\"%s\">%s</span></div>", html.EscapeString(issue.Code), dashboardIssueClass(issue.Severity), html.EscapeString(issue.Severity), html.EscapeString(issue.Path), html.EscapeString(issue.Path))
	}
	if len(report.Issues) > 0 {
		writeHTML(w, "</div>")
	}
	writeHTML(w, "</section><section id=\"repair\" class=\"panel\"><div class=\"panel-heading\"><div><p class=\"eyebrow\">Maintenance</p><h2>Repair plans</h2></div></div>")
	if repairErr != nil {
		writeHTML(w, "<p class=\"error-text\">%s</p>", html.EscapeString(fmt.Sprint(repairErr)))
	} else {
		repairData, _ := repairProjection.Data.(map[string]any)
		plans, _ := repairData["plans"].([]domain.RepairPlan)
		writeHTML(w, "<p class=\"section-copy\">Saved plans <strong>%d</strong> 个</p>", len(plans))
		if len(plans) == 0 {
			writeHTML(w, "<div class=\"empty-state\">没有保存的 repair plan。需要维护时先运行 <code>pinax repair plan --vault %s --save</code>。</div>", html.EscapeString(s.vault))
		} else {
			writeHTML(w, "<div class=\"plan-list\">")
		}
		for i, plan := range plans {
			if i >= 10 {
				break
			}
			applyCommand := fmt.Sprintf("pinax repair apply --vault %s --plan %s --yes", s.vault, plan.PlanID)
			writeHTML(w, "<div class=\"plan-item\"><div><strong>%s</strong><span>operations %d · expires %s</span></div><code>%s</code></div>", html.EscapeString(plan.PlanID), len(plan.Operations), html.EscapeString(plan.ExpiresAt), html.EscapeString(applyCommand))
		}
		if len(plans) > 0 {
			writeHTML(w, "</div>")
		}
	}
	writeHTML(w, "</section><section id=\"data\" class=\"panel\"><div class=\"panel-heading\"><div><p class=\"eyebrow\">Read API</p><h2>数据</h2></div></div><div class=\"endpoint-grid\"><a href=\"/api/overview\">/api/overview</a><a href=\"/api/graph-summary\">/api/graph-summary</a><a href=\"/api/repair-plans\">/api/repair-plans</a><a href=\"/api/database-tabs/&lt;view&gt;\">/api/database-tabs/&lt;view&gt;</a></div></section></main></div></body></html>")
}

func writeDashboardMetric(w io.Writer, label, value, help string) {
	writeHTML(w, "<article class=\"metric\"><span>%s</span><strong>%s</strong><small>%s</small></article>", html.EscapeString(label), html.EscapeString(value), html.EscapeString(help))
}

func dashboardStatusClass(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fresh", "success", "ok", "resolved":
		return "success"
	case "partial", "stale", "warning":
		return "warning"
	case "failed", "error":
		return "danger"
	default:
		return "info"
	}
}

func dashboardIssueClass(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error":
		return "danger"
	case "warning":
		return "warning"
	case "info":
		return "info"
	default:
		return "muted"
	}
}

const dashboardCSS = `
:root{color-scheme:light;--bg:#f6f8fb;--surface:#ffffff;--surface-2:#f9fafb;--text:#17202a;--muted:#657386;--border:#d9e1ea;--primary:#176b87;--primary-strong:#0f5268;--success-bg:#e8f6ef;--success:#176b45;--warning-bg:#fff5dc;--warning:#8a5a00;--danger-bg:#fdecec;--danger:#a43b3b;--info-bg:#eaf4ff;--info:#245a8d;--shadow:0 10px 30px rgba(20,37,55,.07)}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:14px/1.5 Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}a{color:var(--primary);text-decoration:none}a:hover{text-decoration:underline}.shell{display:grid;grid-template-columns:232px minmax(0,1fr);min-height:100vh}.sidebar{border-right:1px solid var(--border);background:#fdfefe;padding:20px 16px;position:sticky;top:0;height:100vh}.brand{display:flex;align-items:center;gap:10px;margin-bottom:28px}.brand-mark{display:grid;place-items:center;width:34px;height:34px;border-radius:8px;background:var(--primary);color:#fff;font-weight:700}.brand strong{display:block;font-size:15px}.brand span:last-child{display:block;color:var(--muted);font-size:12px}nav{display:grid;gap:4px}nav a{border-radius:7px;color:#425063;padding:9px 10px}nav a.active,nav a:hover{background:#edf4f7;color:var(--primary-strong);text-decoration:none}.main{min-width:0;padding:24px;max-width:1280px}.page-header{display:flex;justify-content:space-between;gap:18px;align-items:flex-start;margin-bottom:20px}.page-header h1{font-size:26px;line-height:1.2;margin:2px 0 6px}.page-header p{color:var(--muted);margin:0}.eyebrow{color:var(--primary);font-size:12px;font-weight:700;letter-spacing:0;margin:0 0 4px;text-transform:uppercase}.header-actions{display:flex;gap:8px;flex-wrap:wrap}.button{display:inline-flex;align-items:center;justify-content:center;min-height:36px;border-radius:7px;background:var(--primary);color:#fff;font-weight:650;padding:8px 12px}.button.secondary{background:#fff;border:1px solid var(--border);color:var(--text)}.button:hover{text-decoration:none}.status-strip,.graph-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}.status-strip{margin-bottom:16px}.metric,.panel{background:var(--surface);border:1px solid var(--border);border-radius:8px;box-shadow:var(--shadow)}.metric{padding:14px}.metric span{display:block;color:var(--muted);font-size:12px}.metric strong{display:block;font-size:24px;line-height:1.2;margin:4px 0}.metric small{display:block;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.panel{padding:18px;margin-bottom:16px}.panel-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;margin-bottom:14px}.panel-heading h2{font-size:18px;margin:0}.pill{display:inline-flex;align-items:center;max-width:100%;border-radius:999px;padding:3px 8px;font-size:12px;font-weight:700;white-space:nowrap}.pill.success{background:var(--success-bg);color:var(--success)}.pill.warning{background:var(--warning-bg);color:var(--warning)}.pill.danger{background:var(--danger-bg);color:var(--danger)}.pill.info{background:var(--info-bg);color:var(--info)}.pill.muted{background:#eef1f4;color:#536172}.next-action,.empty-state{border:1px solid var(--border);border-radius:8px;background:var(--surface-2);padding:12px;margin-top:14px}.next-action span{display:block;color:var(--muted);font-size:12px;margin-bottom:4px}.section-copy{color:var(--muted);margin:0 0 12px}.issue-table{border:1px solid var(--border);border-radius:8px;overflow:hidden}.issue-row{display:grid;grid-template-columns:minmax(170px,.8fr) 96px minmax(0,2fr);gap:12px;align-items:center;padding:10px 12px;border-top:1px solid var(--border);background:#fff}.issue-row:first-child{border-top:0}.issue-row.head{background:#f3f6f9;color:var(--muted);font-size:12px;font-weight:700;text-transform:uppercase}.path{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#314154}code{border-radius:6px;background:#edf1f5;color:#25364a;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px;padding:2px 5px}.plan-list{display:grid;gap:10px}.plan-item{display:grid;grid-template-columns:minmax(0,1fr) minmax(280px,.9fr);gap:12px;align-items:center;border:1px solid var(--border);border-radius:8px;padding:12px;background:var(--surface-2)}.plan-item span{display:block;color:var(--muted);font-size:12px}.endpoint-grid{display:flex;gap:8px;flex-wrap:wrap}.endpoint-grid a{border:1px solid var(--border);border-radius:7px;background:#fff;padding:8px 10px;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px}.error-text{color:var(--danger);margin:0}@media (max-width:900px){.shell{display:block}.sidebar{position:static;height:auto;border-right:0;border-bottom:1px solid var(--border)}nav{display:flex;overflow-x:auto}.main{padding:18px}.page-header{display:block}.header-actions{margin-top:12px}.status-strip,.graph-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.issue-row{grid-template-columns:1fr}.issue-row.head{display:none}.path{white-space:normal}.plan-item{grid-template-columns:1fr}}@media (max-width:520px){.status-strip,.graph-grid{grid-template-columns:1fr}.metric strong{font-size:22px}.panel{padding:14px}.page-header h1{font-size:23px}}
`

func writeHTML(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	statsProjection, err := s.service.VaultStats(r.Context(), app.VaultStatsRequest{VaultPath: s.vault})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	doctorProjection, err := s.service.VaultDoctor(r.Context(), app.VaultDoctorRequest{VaultPath: s.vault})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	graphProjection, err := s.graphSummaryProjection(r.Context())
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	payload := domain.NewProjection("dashboard.overview", "Dashboard overview 已生成。")
	payload.Data = map[string]any{"stats": statsProjection.Data, "doctor": doctorProjection.Data, "link_graph": graphProjection.Data, "link_graph_command": graphProjection.Command}
	payload.Actions = graphProjection.Actions
	payload.Facts["stats_status"] = statsProjection.Status
	payload.Facts["doctor_status"] = doctorProjection.Status
	payload.Facts["link_graph_status"] = graphProjection.Status
	payload.Mode = "json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func (s *Server) handleGraphSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	payload, err := s.graphSummaryProjection(r.Context())
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	payload.Mode = "json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func (s *Server) graphSummaryProjection(ctx context.Context) (domain.Projection, error) {
	summary, err := s.service.GraphSummary(ctx, s.vault)
	if err != nil {
		return domain.Projection{}, err
	}
	payload := domain.NewProjection("dashboard.graph_summary", "Dashboard link graph summary 已生成。")
	payload.Facts["engine"] = summary.Engine
	payload.Facts["index_status"] = summary.IndexStatus
	payload.Facts["total_links"] = fmt.Sprint(summary.TotalLinks)
	payload.Facts["resolved"] = fmt.Sprint(summary.Resolved)
	payload.Facts["broken"] = fmt.Sprint(summary.Broken)
	payload.Facts["ambiguous"] = fmt.Sprint(summary.Ambiguous)
	payload.Facts["orphans"] = fmt.Sprint(summary.Orphans)
	payload.Data = summary
	payload.Actions = summary.NextActions
	if summary.Broken > 0 || summary.Ambiguous > 0 || summary.Orphans > 0 {
		payload.Status = "partial"
	}
	return payload, nil
}

func (s *Server) handleRepairPlans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	repairProjection, err := s.service.ListRepairPlans(r.Context(), app.VaultRequest{VaultPath: s.vault})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	plansData, _ := repairProjection.Data.(map[string]any)
	plans, _ := plansData["plans"].([]domain.RepairPlan)
	applyCommand := "pinax repair plan --vault " + s.vault + " --save"
	if len(repairProjection.Actions) > 0 {
		applyCommand = repairProjection.Actions[0].Command
	}
	payload := domain.NewProjection("dashboard.repair_plans", "Dashboard repair plans 已生成。")
	payload.Facts["plans"] = fmt.Sprint(len(plans))
	payload.Data = map[string]any{"plans": plans, "apply_command": applyCommand}
	payload.Actions = repairProjection.Actions
	payload.Mode = "json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func (s *Server) handleProjectBoard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	slug := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/project-board/"), "/")
	if slug == "" {
		writeDashboardError(w, &domain.CommandError{Code: "project_required", Message: "project board endpoint 需要 project slug"})
		return
	}
	projection, err := s.service.ProjectBoardShow(r.Context(), app.ProjectBoardRequest{VaultPath: s.vault, Project: slug, NoteDisplay: "card"})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	projection.Command = "dashboard.project_board"
	projection.Mode = "json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(projection)
}

func (s *Server) handleNoteDisplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ref := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/note-display/"), "/")
	if ref == "" {
		writeDashboardError(w, &domain.CommandError{Code: "note_ref_required", Message: "note display endpoint 需要 note ref"})
		return
	}
	display := r.URL.Query().Get("display")
	if display == "" {
		display = "card"
	}
	if display == string(domain.NoteDisplayBody) {
		writeDashboardError(w, &domain.CommandError{Code: "dashboard_body_display_unsupported", Message: "dashboard note display 不返回完整正文"})
		return
	}
	projection, err := s.service.ShowNoteProjection(r.Context(), app.ShowNoteRequest{VaultPath: s.vault, NoteRef: ref, Display: display})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	projection.Command = "dashboard.note_display"
	projection.Mode = "json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(projection)
}

func (s *Server) handleDatabaseTab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/database-tabs/"), "/")
	if name == "" {
		writeDashboardError(w, &domain.CommandError{Code: "database_view_required", Message: "database tab endpoint 需要 view name"})
		return
	}
	projection, err := s.service.RenderDatabaseView(r.Context(), app.ViewRequest{VaultPath: s.vault, Name: name})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	projection.Command = "dashboard.database_tab"
	projection.Mode = "json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(projection)
}

func ListenAndServe(ctx context.Context, service *app.Service, vault string, port int, logf func(string, ...any)) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: NewServer(service, vault).Handler(), ReadHeaderTimeout: 5 * time.Second}
	if logf != nil {
		logf("Pinax dashboard: http://%s", listener.Addr().String())
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func writeDashboardError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	projection := domain.NewErrorProjection("dashboard.overview", &domain.CommandError{Code: "dashboard_error", Message: err.Error()})
	projection.Mode = "json"
	_ = json.NewEncoder(w).Encode(projection)
}
