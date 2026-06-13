package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
)

func TestResolveVaultObjectUsesIndexManifestAndScanFallback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Alpha Note", Slug: "alpha", Body: "alpha body"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "draft.md"), "# Draft\n")
	writeAppFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	manifest := pinaxassets.Manifest{Assets: []pinaxassets.Asset{{ID: "asset_diagram", Path: "assets/diagram.png", Filename: "diagram.png", Stem: "diagram", Extension: "png", MediaType: "image/png", SHA256: "abc123", ManagedStatus: domain.ManagedStatusManaged}}}
	if err := pinaxassets.Save(root, manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	indexed, err := svc.ResolveVaultObject(ctx, ResolverRequest{VaultPath: root, Query: "alpha", Scope: "registered", Kind: "note"})
	if err != nil {
		t.Fatalf("resolve indexed note: %v", err)
	}
	if indexed.Facts.IndexStatus != "fresh" || indexed.LedgerSeq == 0 || len(indexed.Candidates) != 1 || indexed.Candidates[0].Path != "alpha.md" || indexed.Candidates[0].ManagedStatus != domain.ManagedStatusRegistered {
		t.Fatalf("indexed resolver result = %#v", indexed)
	}

	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove index: %v", err)
	}
	asset, err := svc.ResolveVaultObject(ctx, ResolverRequest{VaultPath: root, Query: "diagram", Scope: "assets", Kind: "asset"})
	if err != nil {
		t.Fatalf("resolve manifest asset: %v", err)
	}
	if asset.Facts.IndexStatus != "missing" || len(asset.Candidates) != 1 || asset.Candidates[0].ObjectKind != domain.VaultObjectKindAsset || asset.Candidates[0].Path != "assets/diagram.png" || asset.Candidates[0].ManagedStatus != domain.ManagedStatusManaged {
		t.Fatalf("manifest fallback result = %#v", asset)
	}

	adoptable, err := svc.ResolveVaultObject(ctx, ResolverRequest{VaultPath: root, Query: "draft", Scope: "adoptable", Kind: "file"})
	if err != nil {
		t.Fatalf("resolve scanned adoptable: %v", err)
	}
	if adoptable.Facts.IndexStatus != "missing" || len(adoptable.Candidates) != 1 || adoptable.Candidates[0].ObjectKind != domain.VaultObjectKindFile || adoptable.Candidates[0].Path != "notes/draft.md" || adoptable.Candidates[0].ManagedStatus != domain.ManagedStatusAdoptable {
		t.Fatalf("scan fallback result = %#v", adoptable)
	}
}

func TestResolveVaultObjectScopes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Shared", Slug: "shared", Body: "registered"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "shared-draft.md"), "# Shared Draft\n")
	writeAppFixture(t, filepath.Join(root, "assets", "shared.png"), "png")
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	assertResolvedKinds(t, svc, root, "registered", "all", "shared", []domain.VaultObjectKind{domain.VaultObjectKindNote})
	assertResolvedKinds(t, svc, root, "adoptable", "all", "shared", []domain.VaultObjectKind{domain.VaultObjectKindFile})
	assertResolvedKinds(t, svc, root, "assets", "all", "shared", []domain.VaultObjectKind{domain.VaultObjectKindAsset})
	assertResolvedKinds(t, svc, root, "registered_or_adoptable", "all", "shared", []domain.VaultObjectKind{domain.VaultObjectKindNote, domain.VaultObjectKindFile})
	assertResolvedKinds(t, svc, root, "all", "all", "shared", []domain.VaultObjectKind{domain.VaultObjectKindAsset, domain.VaultObjectKindNote, domain.VaultObjectKindFile})
	assertResolvedKinds(t, svc, root, "all", "note", "shared", []domain.VaultObjectKind{domain.VaultObjectKindNote})

	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	registeredFallback, err := svc.ResolveVaultObject(ctx, ResolverRequest{VaultPath: root, Query: "shared", Scope: "registered", Kind: "note"})
	if err != nil {
		t.Fatalf("resolve registered fallback: %v", err)
	}
	if registeredFallback.Facts.IndexStatus != "missing" || len(registeredFallback.Candidates) != 1 || registeredFallback.Candidates[0].Path != "shared.md" {
		t.Fatalf("registered fallback = %#v", registeredFallback)
	}
}

func assertResolvedKinds(t *testing.T, svc *Service, root, scope, kind, query string, want []domain.VaultObjectKind) {
	t.Helper()
	result, err := svc.ResolveVaultObject(context.Background(), ResolverRequest{VaultPath: root, Query: query, Scope: scope, Kind: kind})
	if err != nil {
		t.Fatalf("resolve scope=%s kind=%s: %v", scope, kind, err)
	}
	if len(result.Candidates) != len(want) {
		t.Fatalf("scope=%s kind=%s candidates=%#v want kinds=%#v", scope, kind, result.Candidates, want)
	}
	got := map[domain.VaultObjectKind]int{}
	for _, candidate := range result.Candidates {
		got[candidate.ObjectKind]++
	}
	for _, kind := range want {
		if got[kind] == 0 {
			t.Fatalf("scope=%s kind=%s missing kind %s in %#v", scope, kind, kind, result.Candidates)
		}
		got[kind]--
	}
	for kind, count := range got {
		if count != 0 {
			t.Fatalf("scope=%s kind=%s unexpected kind %s in %#v", scope, kind, kind, result.Candidates)
		}
	}
}

func TestResolveVaultObjectProjectionReturnsCandidatesAndNextActions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Shared", Slug: "shared", Body: "registered"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "shared-draft.md"), "# Shared Draft\n")
	writeAppFixture(t, filepath.Join(root, "assets", "shared.png"), "png")
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	projection, err := svc.ResolveVaultObjectProjection(ctx, ResolverRequest{VaultPath: root, Query: "shared", Scope: "all", Kind: "all"})
	if err != nil {
		t.Fatalf("resolver projection: %v", err)
	}
	if projection.Command != "resolver.lookup" || projection.Status != "partial" || projection.Facts["candidates"] != "3" || projection.Facts["ambiguous"] != "true" || projection.Facts["index_status"] != "fresh" {
		t.Fatalf("resolver projection facts = %#v", projection)
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("resolver projection data = %#v", projection.Data)
	}
	candidates, ok := data["candidates"].([]domain.VaultObjectCandidate)
	if !ok || len(candidates) != 3 {
		t.Fatalf("resolver candidates data = %#v", data["candidates"])
	}
	actions := resolverActionCommands(projection.Actions)
	for _, want := range []string{"pinax note show", "pinax asset show", "pinax record adopt"} {
		if !actionContains(actions, want) {
			t.Fatalf("resolver actions missing %q: %#v", want, projection.Actions)
		}
	}
}

func TestResolveVaultObjectIntegrationDisambiguatesNoteAssetAndUnmanagedConflicts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Shared", Slug: "shared-note", Body: "registered"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "shared-draft.md"), "# Shared Draft\n")
	writeAppFixture(t, filepath.Join(root, "assets", "shared.png"), "png")
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	all, err := svc.ResolveVaultObjectProjection(ctx, ResolverRequest{VaultPath: root, Query: "shared", Scope: "all", Kind: "all"})
	if err != nil {
		t.Fatalf("resolve all conflicts: %v", err)
	}
	if all.Status != "partial" || all.Facts["candidates"] != "3" || all.Facts["ambiguous"] != "true" {
		t.Fatalf("all conflict projection = %#v", all)
	}
	data := all.Data.(map[string]any)
	assertCandidateKinds(t, data["candidates"].([]domain.VaultObjectCandidate), []domain.VaultObjectKind{domain.VaultObjectKindAsset, domain.VaultObjectKindFile, domain.VaultObjectKindNote})

	assertResolvedKinds(t, svc, root, "registered", "note", "shared", []domain.VaultObjectKind{domain.VaultObjectKindNote})
	assertResolvedKinds(t, svc, root, "assets", "asset", "shared", []domain.VaultObjectKind{domain.VaultObjectKindAsset})
	assertResolvedKinds(t, svc, root, "adoptable", "file", "shared", []domain.VaultObjectKind{domain.VaultObjectKindFile})
	if _, err := svc.ResolveVaultObjectForWrite(ctx, ResolverRequest{VaultPath: root, Query: "shared", Scope: "registered_or_adoptable", Kind: "all"}); err == nil || !hasCommandCode(err, domain.ErrorCodeVaultObjectRefAmbiguous) {
		t.Fatalf("write guard conflict err = %v", err)
	}
}

func TestResolveVaultObjectForWriteFailsBeforeWriteOnAmbiguousCandidates(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "target-a.md"), "# Target A\n")
	writeAppFixture(t, filepath.Join(root, "notes", "target-b.md"), "# Target B\n")

	result, err := svc.ResolveVaultObjectForWrite(ctx, ResolverRequest{VaultPath: root, Query: "target", Scope: "registered_or_adoptable", Kind: "all"})
	if err == nil || !hasCommandCode(err, domain.ErrorCodeVaultObjectRefAmbiguous) {
		t.Fatalf("write resolver guard err = %v", err)
	}
	if len(result.Candidates) != 2 || !result.Facts.Ambiguous {
		t.Fatalf("write resolver guard result = %#v", result)
	}
	projection := resolverWriteGuardErrorProjection("record.adopt", result, err)
	if projection.Command != "record.adopt" || projection.Status != "failed" || projection.Facts["candidates"] != "2" {
		t.Fatalf("write resolver guard projection = %#v", projection)
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("write resolver guard data = %#v", projection.Data)
	}
	candidates, ok := data["candidates"].([]domain.VaultObjectCandidate)

	if !ok || len(candidates) != 2 {
		t.Fatalf("write resolver guard candidates data = %#v", data["candidates"])
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "records", "events.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("write resolver guard wrote record ledger: %v", err)
	}
}

func resolverActionCommands(actions []domain.Action) []string {
	commands := make([]string, 0, len(actions))
	for _, action := range actions {
		commands = append(commands, action.Command)
	}
	return commands
}

func actionContains(actions []string, want string) bool {
	for _, action := range actions {
		if strings.Contains(action, want) {
			return true
		}
	}
	return false
}

func assertCandidateKinds(t *testing.T, candidates []domain.VaultObjectCandidate, want []domain.VaultObjectKind) {
	t.Helper()
	if len(candidates) != len(want) {
		t.Fatalf("candidate kinds = %#v want %#v", candidates, want)
	}
	got := map[domain.VaultObjectKind]int{}
	for _, candidate := range candidates {
		got[candidate.ObjectKind]++
	}
	for _, kind := range want {
		if got[kind] == 0 {
			t.Fatalf("missing kind %s in %#v", kind, candidates)
		}
		got[kind]--
	}
	for kind, count := range got {
		if count != 0 {
			t.Fatalf("unexpected kind %s in %#v", kind, candidates)
		}
	}
}
