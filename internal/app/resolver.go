package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
)

type ResolverRequest struct {
	VaultPath string
	Query     string
	Scope     string
	Kind      string
}

type ResolverResult struct {
	Facts      domain.ResolverFacts          `json:"facts"`
	LedgerSeq  uint64                        `json:"ledger_seq,omitempty"`
	Candidates []domain.VaultObjectCandidate `json:"candidates"`
}

func (s *Service) ResolveVaultObjectProjection(ctx context.Context, req ResolverRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("resolver.lookup", err), err
	}
	result, err := s.ResolveVaultObject(ctx, req)
	if err != nil {
		return errorProjection("resolver.lookup", err), err
	}
	projection := domain.NewProjection("resolver.lookup", "Resolver candidates generated.")
	projection.Facts["query"] = result.Facts.Query
	projection.Facts["scope"] = result.Facts.Scope
	projection.Facts["kind"] = result.Facts.Kind
	projection.Facts["candidates"] = fmt.Sprint(result.Facts.Candidates)
	projection.Facts["ambiguous"] = fmt.Sprint(result.Facts.Ambiguous)
	projection.Facts["index_status"] = result.Facts.IndexStatus
	if result.Facts.MatchField != "" {
		projection.Facts["match_field"] = result.Facts.MatchField
	}
	if result.LedgerSeq > 0 {
		projection.Facts["ledger_seq"] = fmt.Sprint(result.LedgerSeq)
	}
	if result.Facts.Ambiguous {
		projection.Status = "partial"
	}
	projection.Actions = resolverCandidateActions(root, result.Candidates, result.Facts.IndexStatus)
	projection.Evidence = resolverProjectionEvidence(result)
	projection.Data = map[string]any{"facts": result.Facts, "candidates": result.Candidates}
	return projection, nil
}

func (s *Service) ResolveVaultObjectForWrite(ctx context.Context, req ResolverRequest) (ResolverResult, error) {
	result, err := s.ResolveVaultObject(ctx, req)
	if err != nil {
		return result, err
	}
	if len(result.Candidates) > 1 {
		return result, &domain.CommandError{Code: domain.ErrorCodeVaultObjectRefAmbiguous, Message: "resolver query matched multiple candidates", Hint: "Retry with a more specific note_id, asset_id, filename, or full path"}
	}
	return result, nil
}

func resolverWriteGuardErrorProjection(command string, result ResolverResult, err error) domain.Projection {
	projection := errorProjection(command, err)
	projection.Facts["query"] = result.Facts.Query
	projection.Facts["scope"] = result.Facts.Scope
	projection.Facts["kind"] = result.Facts.Kind
	projection.Facts["candidates"] = fmt.Sprint(len(result.Candidates))
	projection.Facts["ambiguous"] = fmt.Sprint(result.Facts.Ambiguous)
	if result.Facts.IndexStatus != "" {
		projection.Facts["index_status"] = result.Facts.IndexStatus
	}
	if result.Facts.MatchField != "" {
		projection.Facts["match_field"] = result.Facts.MatchField
	}
	projection.Data = map[string]any{"facts": result.Facts, "candidates": result.Candidates}
	return projection
}
func (s *Service) ResolveVaultObject(_ context.Context, req ResolverRequest) (ResolverResult, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return ResolverResult{}, err
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return ResolverResult{}, &domain.CommandError{Code: "argument_required", Message: "resolver requires a query", Hint: "Provide a note id, filename, path, title, or asset ref"}
	}
	scope := resolverDefault(req.Scope, "registered")
	kind := resolverDefault(req.Kind, "all")
	result := ResolverResult{Facts: domain.ResolverFacts{Query: query, Scope: scope, Kind: kind}, LedgerSeq: readResolverLedgerSeq(root)}

	lookup, err := noteindex.Lookup(root, noteindex.LookupRequest{Query: query, Scope: scope, Kind: kind})
	if err != nil {
		return ResolverResult{}, err
	}
	result.Facts.IndexStatus = lookup.Status.Status
	result.Candidates = append(result.Candidates, lookup.Candidates...)
	if lookup.Status.Status != "fresh" || len(result.Candidates) == 0 {
		fallback, err := s.resolveVaultObjectFallback(root, query, scope, kind, lookup.Status.Status)
		if err != nil {
			return ResolverResult{}, err
		}
		result.Candidates = mergeResolverCandidates(result.Candidates, fallback)
	}
	sortResolverCandidates(result.Candidates)
	result.Facts.Candidates = len(result.Candidates)
	if len(result.Candidates) > 1 {
		result.Facts.Ambiguous = true
	}
	if len(result.Candidates) > 0 && len(result.Candidates[0].MatchFields) > 0 {
		result.Facts.MatchField = result.Candidates[0].MatchFields[0]
	}
	return result, nil
}

func (s *Service) resolveVaultObjectFallback(root, query, scope, kind, indexStatus string) ([]domain.VaultObjectCandidate, error) {
	candidates := []domain.VaultObjectCandidate{}
	if resolverScopeAllows(scope, "registered") && resolverKindAllows(kind, "note") {
		notes, err := scanNotes(root)
		if err != nil {
			return nil, err
		}
		for _, note := range notes {
			if isSystemJournalNote(note) && !journalNoteIdentityMatches(note, query) {
				continue
			}
			fields, score := noteCandidateMatch(note, query)
			if score == 0 {
				continue
			}
			candidates = append(candidates, domain.VaultObjectCandidate{ObjectKind: domain.VaultObjectKindNote, Path: note.Path, Title: note.Title, NoteID: note.ID, ManagedStatus: domain.ManagedStatusRegistered, MatchFields: fields, Score: score, IndexStatus: indexStatus})
		}
	}
	if resolverScopeAllows(scope, "assets") && resolverKindAllows(kind, "asset") {
		manifest, err := pinaxassets.Load(root)
		if err != nil {
			return nil, err
		}
		for _, asset := range manifest.Assets {
			fields, score := assetCandidateMatch(asset, query)
			if score == 0 {
				continue
			}
			candidates = append(candidates, domain.VaultObjectCandidate{ObjectKind: domain.VaultObjectKindAsset, Path: asset.Path, AssetID: asset.ID, ManagedStatus: asset.ManagedStatus, MatchFields: fields, Score: score, MediaType: asset.MediaType, IndexStatus: indexStatus})
		}
	}
	if resolverScopeAllows(scope, "adoptable") && resolverKindAllows(kind, "file") {
		notes, err := scanNotes(root)
		if err != nil {
			return nil, err
		}
		files, err := adoptableMarkdownCandidates(root, notes, query, indexStatus)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, files...)
	}
	return candidates, nil
}

func journalNoteIdentityMatches(note domain.Note, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return false
	}
	path := filepath.ToSlash(note.Path)
	needle := filepath.ToSlash(strings.TrimPrefix(query, "notes/"))
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return note.ID == query || path == query || strings.TrimPrefix(path, "notes/") == needle || stem == query || note.Title == query || journalNoteShellFriendlyAlias(note) == query
}

func readResolverLedgerSeq(root string) uint64 {
	payload, err := os.ReadFile(filepath.Join(root, ".pinax", "records", "version.json"))
	if err != nil {
		return 0
	}
	var version domain.LedgerVersion
	if err := json.Unmarshal(payload, &version); err != nil {
		return 0
	}
	return version.LastSeq
}

func mergeResolverCandidates(base, extra []domain.VaultObjectCandidate) []domain.VaultObjectCandidate {
	seen := map[string]bool{}
	merged := make([]domain.VaultObjectCandidate, 0, len(base)+len(extra))
	for _, candidate := range append(base, extra...) {
		key := string(candidate.ObjectKind) + "\x00" + candidate.Path + "\x00" + string(candidate.ManagedStatus)
		if candidate.Path == "" || seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, candidate)
	}
	return merged
}

func sortResolverCandidates(candidates []domain.VaultObjectCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Path < candidates[j].Path
	})
}

func resolverDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func resolverScopeAllows(scope, target string) bool {
	return scopeAllows(scope, target)
}

func resolverKindAllows(kind, target string) bool {
	return kindAllows(kind, target)
}

func resolverCandidateActions(root string, candidates []domain.VaultObjectCandidate, indexStatus string) []domain.Action {
	actions := []domain.Action{}
	seen := map[string]bool{}
	add := func(name, command string) {
		key := name + "\x00" + command
		if seen[key] {
			return
		}
		seen[key] = true
		actions = append(actions, domain.Action{Name: name, Command: command})
	}
	if indexStatus != "" && indexStatus != "fresh" {
		add("refresh_index", fmt.Sprintf("pinax index refresh --vault %s --json", shellQuote(root)))
	}
	for _, candidate := range candidates {
		switch candidate.ObjectKind {
		case domain.VaultObjectKindNote:
			add("show_note", fmt.Sprintf("pinax note show %s --vault %s --json", shellQuote(candidate.Path), shellQuote(root)))
		case domain.VaultObjectKindAsset:
			add("show_asset", fmt.Sprintf("pinax asset show %s --vault %s --json", shellQuote(candidate.Path), shellQuote(root)))
		case domain.VaultObjectKindFile:
			add("adopt_file", fmt.Sprintf("pinax record adopt %s --plan --vault %s --json", shellQuote(candidate.Path), shellQuote(root)))
		}
	}
	return actions
}

func resolverProjectionEvidence(result ResolverResult) []string {
	evidence := []string{}
	if result.Facts.IndexStatus != "" {
		evidence = append(evidence, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite")))
	}
	if result.LedgerSeq > 0 {
		evidence = append(evidence, filepath.ToSlash(filepath.Join(".pinax", "records", "version.json")))
	}
	return evidence
}
