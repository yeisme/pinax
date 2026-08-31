package app

import (
	"sort"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	ConnectionReadinessSchemaV1 = "pinax.connection_readiness.v1"

	ReadinessLayerContract         = "contract"
	ReadinessLayerTransport        = "transport"
	ReadinessLayerAuth             = "auth"
	ReadinessLayerOwner            = "owner"
	ReadinessLayerMutationRecovery = "mutation_recovery"
	ReadinessLayerProduction       = "production"
)

var requiredReadinessLayers = []string{
	ReadinessLayerContract,
	ReadinessLayerTransport,
	ReadinessLayerAuth,
	ReadinessLayerOwner,
	ReadinessLayerMutationRecovery,
	ReadinessLayerProduction,
}

type Readiness struct {
	Status       string   `json:"status"`
	Maturity     string   `json:"maturity"`
	Blockers     []string `json:"blockers"`
	NextActions  []string `json:"next_actions"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type ConnectionReadiness struct {
	SchemaVersion string               `json:"schema_version"`
	Overall       Readiness            `json:"overall"`
	Layers        map[string]Readiness `json:"layers"`
}

type ConnectionReadinessOptions struct {
	Mode                  string
	Transport             string
	AuthMode              string
	OwnerAvailable        bool
	MutationRecoveryReady bool
	ProductionStatus      string
}

func BuildConnectionReadiness(options ConnectionReadinessOptions) ConnectionReadiness {
	layers := map[string]Readiness{
		ReadinessLayerContract: layer(
			"ready", "first-support", nil, nil,
			[]string{"transport_manifest", "openapi_3_1", "projection_v1"},
		),
		ReadinessLayerTransport: transportReadiness(options.Transport),
		ReadinessLayerAuth:      authReadiness(options.Transport, options.AuthMode),
		ReadinessLayerOwner:     ownerReadiness(options.OwnerAvailable),
		ReadinessLayerMutationRecovery: mutationRecoveryReadiness(
			options.Transport,
			options.MutationRecoveryReady,
		),
		ReadinessLayerProduction: productionReadiness(options.Mode, options.ProductionStatus),
	}
	readiness := ConnectionReadiness{SchemaVersion: ConnectionReadinessSchemaV1, Layers: layers}
	readiness.Overall = SummarizeConnectionReadiness(layers)
	return readiness
}

func ConnectionReadinessProjection(options ConnectionReadinessOptions) domain.Projection {
	return ConnectionReadinessValueProjection(BuildConnectionReadiness(options))
}

func ConnectionReadinessValueProjection(readiness ConnectionReadiness) domain.Projection {
	projection := domain.NewProjection("connection.readiness", "Pinax connection readiness is reported by independent layers.")
	projection.Facts["schema_version"] = readiness.SchemaVersion
	projection.Facts["overall_status"] = readiness.Overall.Status
	projection.Facts["overall_maturity"] = readiness.Overall.Maturity
	for _, name := range requiredReadinessLayers {
		projection.Facts[name+"_status"] = readiness.Layers[name].Status
		projection.Facts[name+"_maturity"] = readiness.Layers[name].Maturity
	}
	projection.Data = map[string]any{"readiness": readiness}
	return projection
}

func SummarizeConnectionReadiness(layers map[string]Readiness) Readiness {
	overall := layer("ready", "mature", nil, nil, nil)
	critical := map[string]bool{
		ReadinessLayerContract:  true,
		ReadinessLayerTransport: true,
		ReadinessLayerAuth:      true,
		ReadinessLayerOwner:     true,
	}
	for _, name := range requiredReadinessLayers {
		value, ok := layers[name]
		if !ok {
			overall.Status = "blocked"
			overall.Blockers = append(overall.Blockers, "missing_layer:"+name)
			continue
		}
		if value.Maturity == "exploratory" {
			overall.Maturity = "exploratory"
		} else if value.Maturity == "first-support" && overall.Maturity == "mature" {
			overall.Maturity = "first-support"
		}
		switch value.Status {
		case "blocked":
			if critical[name] {
				overall.Status = "blocked"
			} else if overall.Status == "ready" {
				overall.Status = "degraded"
			}
		case "degraded":
			if overall.Status == "ready" {
				overall.Status = "degraded"
			}
		case "ready", "not_applicable":
		case "not_configured":
			if name != ReadinessLayerProduction && overall.Status == "ready" {
				overall.Status = "degraded"
			}
		default:
			// Unknown extensions are preserved in the layer but can never make
			// the summary ready.
			if critical[name] {
				overall.Status = "blocked"
			} else if overall.Status == "ready" {
				overall.Status = "degraded"
			}
			overall.Blockers = append(overall.Blockers, "unknown_status:"+name+":"+value.Status)
		}
		overall.Blockers = append(overall.Blockers, value.Blockers...)
		overall.NextActions = append(overall.NextActions, value.NextActions...)
		overall.EvidenceRefs = append(overall.EvidenceRefs, value.EvidenceRefs...)
	}
	overall.Blockers = sortedUniqueReadiness(overall.Blockers)
	overall.NextActions = sortedUniqueReadiness(overall.NextActions)
	overall.EvidenceRefs = sortedUniqueReadiness(overall.EvidenceRefs)
	return overall
}

func transportReadiness(transport string) Readiness {
	switch transport {
	case "embedded":
		return layer("ready", "mature", nil, nil, []string{"application_service_embedded"})
	case "stdio":
		return layer("ready", "first-support", nil, nil, []string{"mcp_stdio_lifecycle"})
	case "loopback-http":
		return layer("ready", "first-support", nil, nil, []string{"loopback_listener"})
	case "https":
		return layer("ready", "first-support", nil, nil, []string{"https_owner_endpoint"})
	default:
		return layer("blocked", "exploratory", []string{"transport_unknown"}, []string{"Select embedded, stdio, loopback-http, or https transport"}, nil)
	}
}

func authReadiness(transport, authMode string) Readiness {
	if transport == "embedded" || transport == "stdio" {
		return layer("not_applicable", "mature", nil, nil, []string{"local_process_boundary"})
	}
	switch authMode {
	case "none", "temp", "temp-token", "token-store":
		return layer("ready", "first-support", nil, nil, []string{"auth_middleware"})
	case "", "unset":
		return layer("not_configured", "exploratory", []string{"auth_not_configured"}, []string{"Configure the owner authentication mode"}, nil)
	default:
		return layer("blocked", "exploratory", []string{"auth_mode_unknown"}, []string{"Use temp, token-store, or explicit loopback no-auth mode"}, nil)
	}
}

func ownerReadiness(available bool) Readiness {
	if available {
		return layer("ready", "first-support", nil, nil, []string{"application_service_backing"})
	}
	return layer("blocked", "exploratory", []string{"owner_unavailable"}, []string{"Start or configure the Pinax owner application service"}, nil)
}

func mutationRecoveryReadiness(transport string, ready bool) Readiness {
	if transport == "embedded" || transport == "stdio" {
		return layer("not_applicable", "exploratory", nil, nil, []string{"no_remote_mutation_transport"})
	}
	if ready {
		return layer("ready", "first-support", nil, nil, []string{"operation_ledger", "idempotency_binding", "reconcile"})
	}
	return layer("blocked", "exploratory", []string{"operation_ledger_not_configured"}, []string{"Use only readonly remote calls until operation recovery is available"}, []string{"remote_write_routes_registered"})
}

func productionReadiness(mode, explicitStatus string) Readiness {
	if explicitStatus != "" {
		switch explicitStatus {
		case "ready", "degraded", "blocked", "not_configured", "not_applicable":
			return layer(explicitStatus, "exploratory", nil, nil, []string{"owner_reported_production_status"})
		default:
			return layer("blocked", "exploratory", []string{"production_status_unknown"}, []string{"Use a stable production readiness status"}, nil)
		}
	}
	if mode == "local-vault" || mode == "" {
		return layer("not_applicable", "exploratory", nil, nil, []string{"local_owner_only"})
	}
	return layer("not_configured", "exploratory", []string{"production_evidence_not_configured"}, []string{"Require owner-provided deployment, backup, monitoring and operations evidence"}, nil)
}

func layer(status, maturity string, blockers, actions, evidence []string) Readiness {
	return Readiness{
		Status:       status,
		Maturity:     maturity,
		Blockers:     append([]string{}, blockers...),
		NextActions:  append([]string{}, actions...),
		EvidenceRefs: append([]string{}, evidence...),
	}
}

func sortedUniqueReadiness(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
