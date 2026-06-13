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
	writeHTML(w, "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>Pinax Dashboard</title><style>body{font-family:system-ui,sans-serif;margin:24px;color:#17202a}section{margin:18px 0}code{background:#f3f4f6;padding:2px 4px}</style></head><body>")
	writeHTML(w, "<h1>Pinax Vault Dashboard</h1>")
	if statsErr != nil || doctorErr != nil {
		writeHTML(w, "<section><h2>状态</h2><p>%s %s</p></section>", html.EscapeString(fmt.Sprint(statsErr)), html.EscapeString(fmt.Sprint(doctorErr)))
		writeHTML(w, "</body></html>")
		return
	}
	stats, _ := statsProjection.Data.(domain.VaultStats)
	report, _ := doctorProjection.Data.(domain.VaultDoctorReport)
	writeHTML(w, "<section><h2>统计</h2><p>笔记 %d，标签 %d，frontmatter 覆盖率 %d%%，索引状态 <code>%s</code></p></section>", stats.NoteCount, stats.TagCount, stats.FrontmatterCoverage, html.EscapeString(stats.IndexStatus))
	if graphErr != nil {
		writeHTML(w, "<section><h2>关系</h2><p>%s</p></section>", html.EscapeString(fmt.Sprint(graphErr)))
	} else {
		graph, _ := graphProjection.Data.(domain.NoteGraphProjection)
		writeHTML(w, "<section><h2>关系</h2><p>链接 %d，断链 %d，歧义 %d，孤立 %d，engine <code>%s</code></p>", graph.TotalLinks, graph.Broken, graph.Ambiguous, graph.Orphans, html.EscapeString(graph.Engine))
		if len(graphProjection.Actions) > 0 {
			writeHTML(w, "<p>推荐下一步 <code>%s</code></p>", html.EscapeString(graphProjection.Actions[0].Command))
		}
		writeHTML(w, "</section>")
	}
	writeHTML(w, "<section><h2>健康</h2><p>问题 %d 个</p><ul>", len(report.Issues))
	for i, issue := range report.Issues {
		if i >= 20 {
			break
		}
		writeHTML(w, "<li><code>%s</code> %s %s</li>", html.EscapeString(issue.Code), html.EscapeString(issue.Severity), html.EscapeString(issue.Path))
	}
	writeHTML(w, "</ul></section><section><h2>Repair plans</h2>")
	if repairErr != nil {
		writeHTML(w, "<p>%s</p>", html.EscapeString(fmt.Sprint(repairErr)))
	} else {
		repairData, _ := repairProjection.Data.(map[string]any)
		plans, _ := repairData["plans"].([]domain.RepairPlan)
		writeHTML(w, "<p>Saved plans %d 个</p><ul>", len(plans))
		for i, plan := range plans {
			if i >= 10 {
				break
			}
			applyCommand := fmt.Sprintf("pinax repair apply --vault %s --plan %s --yes", s.vault, plan.PlanID)
			writeHTML(w, "<li><code>%s</code> operations %d expires <code>%s</code><br><code>%s</code></li>", html.EscapeString(plan.PlanID), len(plan.Operations), html.EscapeString(plan.ExpiresAt), html.EscapeString(applyCommand))
		}
		writeHTML(w, "</ul>")
	}
	writeHTML(w, "</section><section><h2>数据</h2><p><a href=\"/api/overview\">/api/overview</a> · <a href=\"/api/repair-plans\">/api/repair-plans</a></p></section></body></html>")
}

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
