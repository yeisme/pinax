package app

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/continuitybinding"
	"github.com/yeisme/pinax/internal/domain"
	"gopkg.in/yaml.v3"
)

// bindWorkbenchFixture 绑定 fixture worktree 并返回 binding_id。
func bindWorkbenchFixture(t *testing.T, configDir, repoRoot string) string {
	t.Helper()
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()
	result, err := svc.ContinuityBind(context.Background(), ContinuityBindingRequest{
		RepoPath: repoRoot, VaultRef: "fixture-vault", ScopeKind: "project", ScopeID: "pinax", ConfigDir: configDir,
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	return result.BindingID
}

func TestWorkbenchProjectionReadyContract(t *testing.T) {
	t.Parallel()
	configDir, repoRoot, vaultRoot := setupContinuityBindingFixture(t, "pinax")
	projectRef := bindWorkbenchFixture(t, configDir, repoRoot)
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	envelope, err := svc.WorkbenchContinuityProjection(context.Background(), WorkbenchProjectionRequest{
		ProjectRef: projectRef, ConfigDir: configDir, TTLSeconds: 600, Now: now,
	})
	if err != nil {
		t.Fatalf("projection: %v", err)
	}
	if envelope.SchemaVersion != "pinax.workbench.continuity_projection.v1" {
		t.Fatalf("schema = %q", envelope.SchemaVersion)
	}
	if envelope.Binding.Status != "ready" || !envelope.Binding.Ready || !envelope.Binding.VaultResolved || !envelope.Binding.ScopeValid {
		t.Fatalf("binding = %#v", envelope.Binding)
	}
	if envelope.Recovery != nil {
		t.Fatalf("ready projection must not carry recovery: %#v", envelope.Recovery)
	}
	if envelope.Contract.Digest == "" || !strings.HasPrefix(envelope.Contract.Digest, "sha256:") {
		t.Fatalf("digest = %q", envelope.Contract.Digest)
	}
	if envelope.Contract.Digest != WorkbenchProviderPacket().Contract.Digest {
		t.Fatalf("envelope and packet digests diverge")
	}
	if envelope.Freshness.Basis != "evidence_observed_at" || envelope.Freshness.TTLSeconds != 600 {
		t.Fatalf("freshness = %#v", envelope.Freshness)
	}
	// basis 是 evidence 观测时间：fixture vault 无证据 → 回落当次调用时间。
	if envelope.Freshness.ObservedAt != now.Format(time.RFC3339) {
		t.Fatalf("observed_at = %q want now fallback", envelope.Freshness.ObservedAt)
	}
	wantExpiry := now.Add(600 * time.Second).Format(time.RFC3339)
	if envelope.Freshness.ExpiresAt != wantExpiry {
		t.Fatalf("expires_at = %q want %q", envelope.Freshness.ExpiresAt, wantExpiry)
	}

	// leak 扫描：bounded envelope 不含绝对路径。
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{vaultRoot, repoRoot, configDir} {
		if forbidden != "" && strings.Contains(string(encoded), forbidden) {
			t.Fatalf("envelope leaked absolute path %q", forbidden)
		}
	}
}

func TestWorkbenchProjectionErrorStatesFailClosed(t *testing.T) {
	t.Parallel()
	configDir, repoRoot, _ := setupContinuityBindingFixture(t, "pinax")
	projectRef := bindWorkbenchFixture(t, configDir, repoRoot)
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	// not_found：未知 ref，不得猜测 scope。
	missing, err := svc.WorkbenchContinuityProjection(context.Background(), WorkbenchProjectionRequest{ProjectRef: "bind_unknown", ConfigDir: configDir, Now: now})
	if err != nil {
		t.Fatalf("not_found must be an envelope, not an error: %v", err)
	}
	if missing.Binding.Status != "not_found" || missing.Recovery == nil || missing.Recovery.Code != "binding_not_found" {
		t.Fatalf("missing = %#v recovery=%#v", missing.Binding, missing.Recovery)
	}
	if missing.ResumeCard != nil {
		t.Fatalf("error state must not synthesize a resume card")
	}

	// disabled：改写 registry Enabled=false。
	disableWorkbenchBinding(t, configDir, projectRef)
	disabled, err := svc.WorkbenchContinuityProjection(context.Background(), WorkbenchProjectionRequest{ProjectRef: projectRef, ConfigDir: configDir, Now: now})
	if err != nil {
		t.Fatalf("disabled projection: %v", err)
	}
	if disabled.Binding.Status != "disabled" || disabled.Recovery == nil || disabled.Recovery.Code != "binding_disabled" {
		t.Fatalf("disabled = %#v recovery=%#v", disabled.Binding, disabled.Recovery)
	}
	if !strings.Contains(disabled.Recovery.Action, projectRef) {
		t.Fatalf("recovery action must reference projectRef: %q", disabled.Recovery.Action)
	}
}

func disableWorkbenchBinding(t *testing.T, configDir, bindingID string) {
	t.Helper()
	registry, err := continuitybinding.LoadRegistry(configDir)
	if err != nil {
		t.Fatal(err)
	}
	for index := range registry.Bindings {
		if registry.Bindings[index].BindingID == bindingID {
			registry.Bindings[index].Enabled = false
		}
	}
	data, err := yaml.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(continuitybinding.RegistryPath(configDir), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkbenchProjectionTTLClockAndDigestStability(t *testing.T) {
	t.Parallel()
	// TTL 下限钳制。
	freshness := workbenchTTLProbe(30)
	if freshness != 60 {
		t.Fatalf("ttl must clamp to min 60: %d", freshness)
	}
	// digest 跨调用稳定（canonical 域无时间戳）。
	first := WorkbenchContract().Digest
	for i := 0; i < 3; i++ {
		if got := WorkbenchContract().Digest; got != first {
			t.Fatalf("digest unstable: %q vs %q", got, first)
		}
	}
}

func workbenchTTLProbe(ttl int) int {
	freshness := domain.WorkbenchFreshnessFromEvidence(time.Time{}, ttl, time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC))
	return freshness.TTLSeconds
}
