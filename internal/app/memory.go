package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/memory"
)

type MemoryCaptureRequest struct {
	VaultPath  string
	Type       string
	Subject    string
	Predicate  string
	Object     string
	Body       string
	Status     string
	Confidence string
	Source     string
	SourceSpan string
	Entities   []string
	DryRun     bool
}

type MemoryListRequest struct {
	VaultPath         string
	Type              string
	Entity            string
	IncludeDraft      bool
	IncludeSuperseded bool
	IncludeExpired    bool
	IncludeRejected   bool
	Limit             int
}

type MemoryRecallRequest struct {
	VaultPath string
	Query     string
	Entity    string
	Type      string
	Limit     int
}

func (s *Service) MemoryCapture(ctx context.Context, req MemoryCaptureRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("memory.capture", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("memory.capture", err), err
	}
	captureReq := memory.CaptureRequest{Type: req.Type, Subject: req.Subject, Predicate: req.Predicate, Object: req.Object, Body: req.Body, Status: req.Status, Confidence: req.Confidence, SourceURI: req.Source, SourceSpan: req.SourceSpan, Entities: req.Entities, DryRun: req.DryRun}
	if req.DryRun {
		record, err := memory.BuildRecord(captureReq)
		if err != nil {
			return memoryCommandErrorProjection("memory.capture", err)
		}
		projection := memoryProjection("memory.capture", "Memory capture plan generated.", []memory.Record{record})
		projection.Facts["dry_run"] = "true"
		projection.Data = map[string]any{"record": memoryRecordData(record), "dry_run": true}
		return projection, nil
	}
	store, err := memory.Open(root)
	if err != nil {
		return memoryCommandErrorProjection("memory.capture", err)
	}
	record, err := store.Capture(ctx, captureReq)
	if err != nil {
		return memoryCommandErrorProjection("memory.capture", err)
	}
	projection := memoryProjection("memory.capture", "Memory record captured.", []memory.Record{record})
	projection.Facts["dry_run"] = "false"
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "memory", "ledger.sqlite"))}
	projection.Data = map[string]any{"record": memoryRecordData(record), "dry_run": false}
	return projection, nil
}

func (s *Service) MemoryList(ctx context.Context, req MemoryListRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("memory.list", err), err
	}
	store, err := memory.Open(root)
	if err != nil {
		return memoryCommandErrorProjection("memory.list", err)
	}
	records, err := store.List(ctx, memory.ListFilter{Type: req.Type, Entity: req.Entity, IncludeDraft: req.IncludeDraft, IncludeSuperseded: req.IncludeSuperseded, IncludeExpired: req.IncludeExpired, IncludeRejected: req.IncludeRejected, Limit: req.Limit})
	if err != nil {
		return memoryCommandErrorProjection("memory.list", err)
	}
	projection := memoryProjection("memory.list", "Memory records listed.", records)
	projection.Data = map[string]any{"records": memoryRecordsData(records)}
	return projection, nil
}

func (s *Service) MemoryRecall(ctx context.Context, req MemoryRecallRequest) (domain.Projection, error) {
	return s.memoryRecallProjection(ctx, "memory.recall", "Memory recall completed.", req)
}

func (s *Service) MemoryContext(ctx context.Context, req MemoryRecallRequest) (domain.Projection, error) {
	if req.Limit == 0 {
		req.Limit = 12
	}
	return s.memoryRecallProjection(ctx, "memory.context", "Memory context generated.", req)
}

func (s *Service) MemoryStats(ctx context.Context, req MemoryListRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("memory.stats", err), err
	}
	store, err := memory.Open(root)
	if err != nil {
		return memoryCommandErrorProjection("memory.stats", err)
	}
	stats, err := store.Stats(ctx)
	if err != nil {
		return memoryCommandErrorProjection("memory.stats", err)
	}
	projection := domain.NewProjection("memory.stats", "Memory ledger stats generated.")
	projection.Facts["memory.records"] = fmt.Sprint(stats.Records)
	projection.Facts["memory.confirmed"] = fmt.Sprint(stats.Confirmed)
	projection.Facts["memory.drafts"] = fmt.Sprint(stats.Drafts)
	projection.Facts["memory.scope"] = root
	projection.Data = stats
	return projection, nil
}

func (s *Service) memoryRecallProjection(ctx context.Context, command, summary string, req MemoryRecallRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	store, err := memory.Open(root)
	if err != nil {
		return memoryCommandErrorProjection(command, err)
	}
	hits, err := store.Recall(ctx, memory.RecallFilter{Query: req.Query, Entity: req.Entity, Type: req.Type, Limit: req.Limit})
	if err != nil {
		return memoryCommandErrorProjection(command, err)
	}
	records := make([]memory.Record, 0, len(hits))
	for _, hit := range hits {
		records = append(records, hit.Record)
	}
	projection := memoryProjection(command, summary, records)
	projection.Facts["memory.matches"] = fmt.Sprint(len(hits))
	projection.Facts["memory.scope"] = root
	if strings.TrimSpace(req.Entity) != "" {
		projection.Facts["memory.entity"] = strings.ToLower(strings.TrimSpace(req.Entity))
	}
	projection.Data = map[string]any{"query": req.Query, "entity": req.Entity, "matches": memoryHitsData(hits)}
	return projection, nil
}

func memoryProjection(command, summary string, records []memory.Record) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["records"] = fmt.Sprint(len(records))
	projection.Facts["memory.records"] = fmt.Sprint(len(records))
	if len(records) > 0 {
		projection.Facts["record_id"] = records[0].ID
		projection.Facts["type"] = records[0].Type
		projection.Facts["status"] = records[0].Status
		projection.Facts["source"] = records[0].SourceURI
	}
	projection.Facts["memory.types"] = strings.Join(memoryTypes(records), ",")
	return projection
}

func memoryRecordData(record memory.Record) map[string]any {
	return map[string]any{"id": record.ID, "type": record.Type, "subject": record.Subject, "predicate": record.Predicate, "object": record.Object, "body": record.Body, "status": record.Status, "confidence": record.Confidence, "source_uri": record.SourceURI, "source_span": record.SourceSpan, "created_at": record.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), "updated_at": record.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")}
}

func memoryRecordsData(records []memory.Record) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, memoryRecordData(record))
	}
	return out
}

func memoryHitsData(hits []memory.RecallHit) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		item := memoryRecordData(hit.Record)
		item["recall_reason"] = hit.RecallReason
		item["score"] = hit.Score
		out = append(out, item)
	}
	return out
}

func memoryTypes(records []memory.Record) []string {
	seen := map[string]bool{}
	var types []string
	for _, record := range records {
		if record.Type == "" || seen[record.Type] {
			continue
		}
		seen[record.Type] = true
		types = append(types, record.Type)
	}
	sort.Strings(types)
	return types
}

func memoryCommandErrorProjection(command string, err error) (domain.Projection, error) {
	if cmdErr, ok := err.(*domain.CommandError); ok {
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	return errorProjection(command, err), err
}
