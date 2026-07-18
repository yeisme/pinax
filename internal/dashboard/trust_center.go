package dashboard

import (
	"encoding/json"
	"net/http"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
)

// dashboardScope is the workspace-level scope used by the read-only Trust
// Center endpoints.
func dashboardScope() agentprotocol.Scope {
	return agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}
}

// dashboardPrincipal is a read-only principal for the Trust Center dashboard.
func dashboardPrincipal() agentprotocol.Principal {
	return agentprotocol.DefaultAdapterPrincipal("dashboard", "pinax-dashboard")
}

// handleAgentContinuity serves the experimental agent continuity pack as JSON.
// The pack is bounded: full note bodies, transcripts, and provider payloads
// never enter. GET-only.
func (s *Server) handleAgentContinuity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	pack, err := s.memSvc.AgentContinuity(r.Context(), app.ContinuityRequest{
		VaultPath: s.vault,
		Principal: dashboardPrincipal(),
		Scope:     dashboardScope(),
	})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	writeTrustCenterJSON(w, pack)
}

// handleMemoryInbox serves the experimental memory inbox pack as JSON.
// Items are projections — they never contain proposal bodies or full memory
// content. GET-only.
func (s *Server) handleMemoryInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	pack, err := s.memSvc.MemoryInbox(r.Context(), app.InboxRequest{
		VaultPath: s.vault,
		Scope:     dashboardScope(),
		Limit:     100,
	})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	writeTrustCenterJSON(w, pack)
}

// handleTrustMetrics serves the experimental trust center projection as JSON.
// Includes local trust metrics (context_reuse, source_resolvability,
// proposal_acceptance, handoff_continuation, silent_promotion) and section
// summaries with isolated failure handling. GET-only.
func (s *Server) handleTrustMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	proj, err := s.memSvc.AgentTrustCenter(r.Context(), app.TrustCenterRequest{
		VaultPath: s.vault,
		Scope:     dashboardScope(),
	})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	writeTrustCenterJSON(w, proj)
}

// writeTrustCenterJSON encodes a Trust Center payload as JSON without HTML
// escaping, consistent with the other dashboard JSON endpoints.
func writeTrustCenterJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
