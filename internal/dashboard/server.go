package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>Pinax Dashboard</title><style>body{font-family:system-ui,sans-serif;margin:24px;color:#17202a}section{margin:18px 0}code{background:#f3f4f6;padding:2px 4px}</style></head><body>")
	fmt.Fprint(w, "<h1>Pinax Vault Dashboard</h1>")
	if statsErr != nil || doctorErr != nil {
		fmt.Fprintf(w, "<section><h2>状态</h2><p>%s %s</p></section>", html.EscapeString(fmt.Sprint(statsErr)), html.EscapeString(fmt.Sprint(doctorErr)))
		fmt.Fprint(w, "</body></html>")
		return
	}
	stats, _ := statsProjection.Data.(domain.VaultStats)
	report, _ := doctorProjection.Data.(domain.VaultDoctorReport)
	fmt.Fprintf(w, "<section><h2>统计</h2><p>笔记 %d，标签 %d，frontmatter 覆盖率 %d%%，索引状态 <code>%s</code></p></section>", stats.NoteCount, stats.TagCount, stats.FrontmatterCoverage, html.EscapeString(stats.IndexStatus))
	fmt.Fprintf(w, "<section><h2>健康</h2><p>问题 %d 个</p><ul>", len(report.Issues))
	for i, issue := range report.Issues {
		if i >= 20 {
			break
		}
		fmt.Fprintf(w, "<li><code>%s</code> %s %s</li>", html.EscapeString(issue.Code), html.EscapeString(issue.Severity), html.EscapeString(issue.Path))
	}
	fmt.Fprint(w, "</ul></section><section><h2>Repair plans</h2>")
	if repairErr != nil {
		fmt.Fprintf(w, "<p>%s</p>", html.EscapeString(fmt.Sprint(repairErr)))
	} else {
		repairData, _ := repairProjection.Data.(map[string]any)
		plans, _ := repairData["plans"].([]domain.RepairPlan)
		fmt.Fprintf(w, "<p>Saved plans %d 个</p><ul>", len(plans))
		for i, plan := range plans {
			if i >= 10 {
				break
			}
			applyCommand := fmt.Sprintf("pinax repair apply --vault %s --plan %s --yes", s.vault, plan.PlanID)
			fmt.Fprintf(w, "<li><code>%s</code> operations %d expires <code>%s</code><br><code>%s</code></li>", html.EscapeString(plan.PlanID), len(plan.Operations), html.EscapeString(plan.ExpiresAt), html.EscapeString(applyCommand))
		}
		fmt.Fprint(w, "</ul>")
	}
	fmt.Fprint(w, "</section><section><h2>数据</h2><p><a href=\"/api/overview\">/api/overview</a> · <a href=\"/api/repair-plans\">/api/repair-plans</a></p></section></body></html>")
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
	payload := domain.NewProjection("dashboard.overview", "Dashboard overview 已生成。")
	payload.Data = map[string]any{"stats": statsProjection.Data, "doctor": doctorProjection.Data}
	payload.Facts["stats_status"] = statsProjection.Status
	payload.Facts["doctor_status"] = doctorProjection.Status
	payload.Mode = "json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
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
