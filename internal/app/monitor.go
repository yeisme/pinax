package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yeisme/pinax/internal/app/syncops"
	"github.com/yeisme/pinax/internal/domain"
)

const (
	monitorRunSchemaVersion   = "pinax.monitor_run.v1"
	monitorEventSchemaVersion = "pinax.monitor_event.v1"
	monitorDefaultLimit       = 50
	monitorMaxLimit           = 200
	monitorSampleInterval     = 250 * time.Millisecond
)

type MonitorRequest struct {
	VaultPath string
	RunID     string
	Command   string
	Status    string
	Query     string
	Since     string
	Until     string
	Limit     int
}

type MonitorRun struct {
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Command       string            `json:"command"`
	Status        string            `json:"status"`
	StartedAt     string            `json:"started_at"`
	FinishedAt    string            `json:"finished_at"`
	DurationMS    int64             `json:"duration_ms"`
	Facts         map[string]string `json:"facts,omitempty"`
	Metrics       MonitorMetrics    `json:"metrics"`
	Steps         []MonitorStep     `json:"steps"`
	Evidence      []string          `json:"evidence,omitempty"`
	ErrorCode     string            `json:"error_code,omitempty"`
	ErrorMessage  string            `json:"error_message,omitempty"`
}

type MonitorStep struct {
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	StartedAt    string            `json:"started_at"`
	FinishedAt   string            `json:"finished_at"`
	DurationMS   int64             `json:"duration_ms"`
	Facts        map[string]string `json:"facts,omitempty"`
	Metrics      MonitorMetrics    `json:"metrics"`
	ErrorCode    string            `json:"error_code,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
}

type MonitorMetrics struct {
	WallMS               int64  `json:"wall_ms"`
	HeapAllocBytes       uint64 `json:"heap_alloc_bytes"`
	HeapAllocDeltaBytes  int64  `json:"heap_alloc_delta_bytes"`
	TotalAllocDeltaBytes uint64 `json:"total_alloc_delta_bytes"`
	SysBytes             uint64 `json:"sys_bytes"`
	HeapInuseBytes       uint64 `json:"heap_inuse_bytes"`
	StackInuseBytes      uint64 `json:"stack_inuse_bytes"`
	GCDelta              uint32 `json:"gc_delta"`
	Goroutines           int    `json:"goroutines"`
	PeakHeapAllocBytes   uint64 `json:"peak_heap_alloc_bytes"`
	PeakGoroutines       int    `json:"peak_goroutines"`
	CPUUserMicrosDelta   int64  `json:"cpu_user_micros_delta"`
	CPUSystemMicrosDelta int64  `json:"cpu_system_micros_delta"`
	CPUSupported         bool   `json:"cpu_supported"`
	RSSBytes             uint64 `json:"rss_bytes,omitempty"`
	PeakRSSBytes         uint64 `json:"peak_rss_bytes,omitempty"`
	RSSSupported         bool   `json:"rss_supported"`
}

type monitorResourceSnapshot struct {
	time            time.Time
	heapAllocBytes  uint64
	totalAllocBytes uint64
	sysBytes        uint64
	heapInuseBytes  uint64
	stackInuseBytes uint64
	numGC           uint32
	goroutines      int
	cpuUserMicros   int64
	cpuSystemMicros int64
	cpuSupported    bool
	rssBytes        uint64
	peakRSSBytes    uint64
	rssSupported    bool
}

type monitorRecorder struct {
	root     string
	run      MonitorRun
	start    monitorResourceSnapshot
	peak     monitorResourceSnapshot
	mu       sync.Mutex
	current  *monitorStepTracker
	stop     chan struct{}
	done     chan struct{}
	finished bool
}

type monitorStepTracker struct {
	name  string
	facts map[string]string
	start monitorResourceSnapshot
	peak  monitorResourceSnapshot
}

type MonitorEvent struct {
	SchemaVersion string            `json:"schema_version"`
	Type          string            `json:"type"`
	RunID         string            `json:"run_id"`
	Command       string            `json:"command"`
	Status        string            `json:"status"`
	StartedAt     string            `json:"started_at"`
	FinishedAt    string            `json:"finished_at"`
	DurationMS    int64             `json:"duration_ms"`
	Steps         int               `json:"steps"`
	Facts         map[string]string `json:"facts,omitempty"`
	Metrics       MonitorMetrics    `json:"metrics"`
	Evidence      []string          `json:"evidence,omitempty"`
	ErrorCode     string            `json:"error_code,omitempty"`
}

type monitorQueryResult struct {
	Root     string
	Runs     []MonitorRun
	Events   []MonitorEvent
	Warnings []activityWarning
	Filters  map[string]string
}

func startMonitorRun(root, command string, facts map[string]string) *monitorRecorder {
	now := time.Now().UTC()
	start := captureMonitorResource(now)
	runID := newMonitorRunID(command, now)
	rec := &monitorRecorder{
		root:  root,
		start: start,
		peak:  start,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
		run:   MonitorRun{SchemaVersion: monitorRunSchemaVersion, RunID: runID, Command: syncops.SanitizeString(command), Status: "running", StartedAt: now.Format(time.RFC3339Nano), Facts: sanitizeMonitorFacts(facts)},
	}
	go rec.sample()
	return rec
}

func (r *monitorRecorder) BeginStep(name string, facts map[string]string) func(error) {
	if r == nil {
		return func(error) {}
	}
	start := captureMonitorResource(time.Now().UTC())
	r.mu.Lock()
	r.current = &monitorStepTracker{name: syncops.SanitizeString(name), facts: sanitizeMonitorFacts(facts), start: start, peak: start}
	r.mu.Unlock()
	return func(err error) { r.endStep(err) }
}

func (r *monitorRecorder) endStep(err error) {
	if r == nil {
		return
	}
	end := captureMonitorResource(time.Now().UTC())
	r.mu.Lock()
	tracker := r.current
	r.current = nil
	r.mu.Unlock()
	if tracker == nil {
		return
	}
	status := "success"
	code, message := "", ""
	if err != nil {
		status = "failed"
		code, message = monitorError(err)
	}
	step := MonitorStep{Name: tracker.name, Status: status, StartedAt: tracker.start.time.Format(time.RFC3339Nano), FinishedAt: end.time.Format(time.RFC3339Nano), DurationMS: durationMillis(tracker.start.time, end.time), Facts: tracker.facts, Metrics: monitorMetricsDelta(tracker.start, end, tracker.peak), ErrorCode: code, ErrorMessage: message}
	r.mu.Lock()
	r.run.Steps = append(r.run.Steps, step)
	r.mu.Unlock()
}

func (r *monitorRecorder) Finish(status string, err error) (string, string) {
	if r == nil {
		return "", ""
	}
	end := captureMonitorResource(time.Now().UTC())
	r.mu.Lock()
	if r.finished {
		runID := r.run.RunID
		evidence := firstActivityNonEmpty(r.run.Evidence...)
		r.mu.Unlock()
		return runID, evidence
	}
	r.finished = true
	r.mu.Unlock()
	close(r.stop)
	<-r.done
	if strings.TrimSpace(status) == "" || status == "running" {
		status = "success"
	}
	if err != nil {
		status = "failed"
	}
	code, message := monitorError(err)
	r.mu.Lock()
	r.run.Status = syncops.SanitizeString(status)
	r.run.FinishedAt = end.time.Format(time.RFC3339Nano)
	r.run.DurationMS = durationMillis(r.start.time, end.time)
	r.run.Metrics = monitorMetricsDelta(r.start, end, r.peak)
	r.run.ErrorCode = code
	r.run.ErrorMessage = message
	evidence := monitorRunEvidence(r.run.RunID, r.start.time)
	r.run.Evidence = []string{evidence, filepath.ToSlash(filepath.Join(".pinax", "monitor", "events.jsonl"))}
	run := r.run
	r.mu.Unlock()
	_ = writeMonitorRun(r.root, run)
	_ = appendMonitorEvent(r.root, run)
	return run.RunID, evidence
}

func (r *monitorRecorder) sample() {
	ticker := time.NewTicker(monitorSampleInterval)
	defer func() {
		ticker.Stop()
		close(r.done)
	}()
	for {
		select {
		case <-ticker.C:
			snap := captureMonitorResource(time.Now().UTC())
			r.mu.Lock()
			r.peak = maxMonitorSnapshot(r.peak, snap)
			if r.current != nil {
				r.current.peak = maxMonitorSnapshot(r.current.peak, snap)
			}
			r.mu.Unlock()
		case <-r.stop:
			return
		}
	}
}

func (s *Service) MonitorList(_ context.Context, req MonitorRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("monitor.runs", err), err
	}
	result, err := filterMonitorRuns(root, req)
	if err != nil {
		return errorProjection("monitor.runs", err), err
	}
	projection := monitorProjection("monitor.runs", "Monitor runs listed.", result)
	projection.Data = map[string]any{"schema_version": monitorRunSchemaVersion, "runs": result.Runs, "filters": result.Filters, "warnings": result.Warnings}
	if len(result.Runs) > 0 {
		projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax monitor show %s --vault %s --json", result.Runs[0].RunID, shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) MonitorTail(ctx context.Context, req MonitorRequest) (domain.Projection, error) {
	projection, err := s.MonitorList(ctx, req)
	projection.Command = "monitor.tail"
	if projection.Status == "success" {
		projection.Summary = "Recent monitor runs read."
	}
	return projection, err
}

func (s *Service) MonitorShow(_ context.Context, req MonitorRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("monitor.show", err), err
	}
	runID := strings.TrimSpace(req.RunID)
	run, err := readMonitorRunByID(root, runID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			commandErr := &domain.CommandError{Code: "monitor_run_not_found", Message: "monitor run was not found", Hint: "Run pinax monitor runs --vault <vault> --json"}
			return domain.NewErrorProjection("monitor.show", commandErr), commandErr
		}
		return errorProjection("monitor.show", err), err
	}
	projection := domain.NewProjection("monitor.show", "Monitor run read.")
	projection.Facts["run_id"] = run.RunID
	projection.Facts["command"] = run.Command
	projection.Facts["status"] = run.Status
	projection.Facts["steps"] = fmt.Sprint(len(run.Steps))
	projection.Facts["duration_ms"] = fmt.Sprint(run.DurationMS)
	projection.Facts["schema_version"] = monitorRunSchemaVersion
	projection.Evidence = run.Evidence
	projection.Data = map[string]any{"schema_version": monitorRunSchemaVersion, "run": run}
	return projection, nil
}

func (s *Service) MonitorSummary(_ context.Context, req MonitorRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("monitor.summary", err), err
	}
	result, err := filterMonitorRuns(root, req)
	if err != nil {
		return errorProjection("monitor.summary", err), err
	}
	byCommand := map[string]int{}
	byStatus := map[string]int{}
	var totalDuration int64
	var maxHeap uint64
	for _, run := range result.Runs {
		byCommand[run.Command]++
		byStatus[run.Status]++
		totalDuration += run.DurationMS
		if run.Metrics.PeakHeapAllocBytes > maxHeap {
			maxHeap = run.Metrics.PeakHeapAllocBytes
		}
	}
	avg := int64(0)
	if len(result.Runs) > 0 {
		avg = totalDuration / int64(len(result.Runs))
	}
	projection := monitorProjection("monitor.summary", "Monitor summary generated.", result)
	projection.Facts["avg_duration_ms"] = fmt.Sprint(avg)
	projection.Facts["peak_heap_alloc_bytes"] = fmt.Sprint(maxHeap)
	projection.Data = map[string]any{"schema_version": monitorRunSchemaVersion, "runs": len(result.Runs), "by_command": byCommand, "by_status": byStatus, "avg_duration_ms": avg, "peak_heap_alloc_bytes": maxHeap, "filters": result.Filters, "warnings": result.Warnings}
	return projection, nil
}

func (s *Service) MonitorManage(_ context.Context, req MonitorRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("monitor.manage", err), err
	}
	result, err := readMonitorRuns(root)
	if err != nil {
		return errorProjection("monitor.manage", err), err
	}
	projection := monitorProjection("monitor.manage", "Monitor log management summary generated.", result)
	projection.Data = map[string]any{"schema_version": monitorRunSchemaVersion, "runs": len(result.Runs), "warnings": result.Warnings, "readonly": true, "paths": []string{filepath.ToSlash(filepath.Join(".pinax", "monitor", "runs")), filepath.ToSlash(filepath.Join(".pinax", "monitor", "events.jsonl"))}}
	return projection, nil
}

func filterMonitorRuns(root string, req MonitorRequest) (monitorQueryResult, error) {
	result, err := readMonitorRuns(root)
	if err != nil {
		return result, err
	}
	since, err := parseActivityTime(req.Since)
	if err != nil {
		return result, err
	}
	until, err := parseActivityTime(req.Until)
	if err != nil {
		return result, err
	}
	command := strings.ToLower(strings.TrimSpace(req.Command))
	status := strings.ToLower(strings.TrimSpace(req.Status))
	query := strings.ToLower(strings.TrimSpace(req.Query))
	filtered := make([]MonitorRun, 0, len(result.Runs))
	for _, run := range result.Runs {
		if command != "" && strings.ToLower(run.Command) != command {
			continue
		}
		if status != "" && strings.ToLower(run.Status) != status {
			continue
		}
		started := monitorRunTime(run)
		if !since.IsZero() && started.Before(since) {
			continue
		}
		if !until.IsZero() && started.After(until) {
			continue
		}
		if query != "" && !monitorRunMatchesQuery(run, query) {
			continue
		}
		filtered = append(filtered, run)
	}
	limit := monitorLimit(req.Limit)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	result.Runs = filtered
	result.Filters = map[string]string{"limit": strconv.Itoa(limit)}
	for key, value := range map[string]string{"command": req.Command, "query": req.Query, "status": req.Status, "since": req.Since, "until": req.Until} {
		if strings.TrimSpace(value) != "" {
			result.Filters[key] = strings.TrimSpace(value)
		}
	}
	return result, nil
}

func readMonitorRuns(root string) (monitorQueryResult, error) {
	result := monitorQueryResult{Root: root, Filters: map[string]string{}}
	base := filepath.Join(root, ".pinax", "monitor", "runs")
	if _, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			result.Warnings = append(result.Warnings, activityWarning{Source: "monitor_runs", Path: relativeEvidence(root, path), Message: syncops.SanitizeString(err.Error())})
			return nil
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			result.Warnings = append(result.Warnings, activityWarning{Source: "monitor_runs", Path: relativeEvidence(root, path), Message: syncops.SanitizeString(readErr.Error())})
			return nil
		}
		var run MonitorRun
		if unmarshalErr := json.Unmarshal(b, &run); unmarshalErr != nil {
			result.Warnings = append(result.Warnings, activityWarning{Source: "monitor_runs", Path: relativeEvidence(root, path), Message: syncops.SanitizeString(unmarshalErr.Error())})
			return nil
		}
		if run.SchemaVersion != monitorRunSchemaVersion || strings.TrimSpace(run.RunID) == "" {
			return nil
		}
		result.Runs = append(result.Runs, run)
		return nil
	})
	sort.SliceStable(result.Runs, func(i, j int) bool {
		ti := monitorRunTime(result.Runs[i])
		tj := monitorRunTime(result.Runs[j])
		if ti.Equal(tj) {
			return result.Runs[i].RunID > result.Runs[j].RunID
		}
		return ti.After(tj)
	})
	return result, nil
}

func readMonitorRunByID(root, runID string) (MonitorRun, error) {
	if strings.TrimSpace(runID) == "" {
		return MonitorRun{}, os.ErrNotExist
	}
	result, err := readMonitorRuns(root)
	if err != nil {
		return MonitorRun{}, err
	}
	for _, run := range result.Runs {
		if run.RunID == runID {
			return run, nil
		}
	}
	return MonitorRun{}, os.ErrNotExist
}

func readMonitorEventsActivity(root string, result *activityQueryResult) {
	path := filepath.Join(root, ".pinax", "monitor", "events.jsonl")
	readJSONLActivity(root, path, "monitor_runs", result, func(line []byte, lineNo int) (ActivityEntry, bool, error) {
		var event MonitorEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return ActivityEntry{}, false, err
		}
		if event.SchemaVersion != monitorEventSchemaVersion || strings.TrimSpace(event.RunID) == "" {
			return ActivityEntry{}, false, nil
		}
		entry := newActivityEntry("monitor_runs", event.Command, event.Status, event.FinishedAt, line)
		entry.RunID = event.RunID
		entry.ObjectRef = event.RunID
		entry.DurationMS = event.DurationMS
		entry.Summary = activitySummary(event.Command, event.Status, event.RunID)
		entry.Facts = sanitizeActivityFacts(map[string]string{"command": event.Command, "run_id": event.RunID, "steps": fmt.Sprint(event.Steps), "duration_ms": fmt.Sprint(event.DurationMS), "peak_heap_alloc_bytes": fmt.Sprint(event.Metrics.PeakHeapAllocBytes), "cpu_supported": fmt.Sprint(event.Metrics.CPUSupported), "rss_supported": fmt.Sprint(event.Metrics.RSSSupported), "error_code": event.ErrorCode})
		entry.Evidence = event.Evidence
		entry.Actions = []domain.Action{{Name: "show-monitor", Command: fmt.Sprintf("pinax monitor show %s --vault %s --json", event.RunID, shellQuote(root))}}
		return entry, true, nil
	})
}

func writeMonitorRun(root string, run MonitorRun) error {
	path := filepath.Join(root, filepath.FromSlash(monitorRunEvidence(run.RunID, monitorRunTime(run))))
	return writeJSONAsset(path, run)
}

func appendMonitorEvent(root string, run MonitorRun) error {
	path := filepath.Join(root, ".pinax", "monitor", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	event := MonitorEvent{SchemaVersion: monitorEventSchemaVersion, Type: "monitor.run", RunID: run.RunID, Command: run.Command, Status: run.Status, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, DurationMS: run.DurationMS, Steps: len(run.Steps), Facts: run.Facts, Metrics: run.Metrics, Evidence: run.Evidence, ErrorCode: run.ErrorCode}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(b, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func monitorProjection(command, summary string, result monitorQueryResult) domain.Projection {
	projection := domain.NewProjection(command, summary)
	if len(result.Warnings) > 0 {
		projection.Status = "partial"
	}
	projection.Facts["runs"] = fmt.Sprint(len(result.Runs))
	projection.Facts["warnings"] = fmt.Sprint(len(result.Warnings))
	projection.Facts["schema_version"] = monitorRunSchemaVersion
	addProjectionFilterFacts(&projection, result.Filters)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "monitor", "runs")), filepath.ToSlash(filepath.Join(".pinax", "monitor", "events.jsonl"))}
	return projection
}

func monitorRunEvidence(runID string, started time.Time) string {
	if started.IsZero() {
		started = time.Now().UTC()
	}
	return filepath.ToSlash(filepath.Join(".pinax", "monitor", "runs", started.Format("2006"), started.Format("01"), runID+".json"))
}

func newMonitorRunID(command string, now time.Time) string {
	seed := fmt.Sprintf("%s:%d:%d", command, now.UnixNano(), os.Getpid())
	sum := sha256.Sum256([]byte(seed))
	return "mon_" + now.Format("20060102T150405") + "_" + hex.EncodeToString(sum[:])[:12]
}

func captureMonitorResource(now time.Time) monitorResourceSnapshot {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	snap := monitorResourceSnapshot{time: now, heapAllocBytes: mem.HeapAlloc, totalAllocBytes: mem.TotalAlloc, sysBytes: mem.Sys, heapInuseBytes: mem.HeapInuse, stackInuseBytes: mem.StackInuse, numGC: mem.NumGC, goroutines: runtime.NumGoroutine()}
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err == nil {
		snap.cpuSupported = true
		snap.cpuUserMicros = timevalMicros(usage.Utime)
		snap.cpuSystemMicros = timevalMicros(usage.Stime)
		if usage.Maxrss > 0 {
			snap.peakRSSBytes = uint64(usage.Maxrss) * 1024
			snap.rssSupported = true
		}
	}
	if rss, ok := readLinuxRSSBytes(); ok {
		snap.rssBytes = rss
		snap.rssSupported = true
		if snap.peakRSSBytes < rss {
			snap.peakRSSBytes = rss
		}
	}
	return snap
}

func timevalMicros(tv syscall.Timeval) int64 {
	return int64(tv.Sec)*1_000_000 + int64(tv.Usec)
}

func readLinuxRSSBytes() (uint64, bool) {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, false
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}

func maxMonitorSnapshot(a, b monitorResourceSnapshot) monitorResourceSnapshot {
	if b.heapAllocBytes > a.heapAllocBytes {
		a.heapAllocBytes = b.heapAllocBytes
	}
	if b.goroutines > a.goroutines {
		a.goroutines = b.goroutines
	}
	if b.rssBytes > a.rssBytes {
		a.rssBytes = b.rssBytes
	}
	if b.peakRSSBytes > a.peakRSSBytes {
		a.peakRSSBytes = b.peakRSSBytes
	}
	if b.rssSupported {
		a.rssSupported = true
	}
	if b.cpuSupported {
		a.cpuSupported = true
	}
	return a
}

func monitorMetricsDelta(start, end, peak monitorResourceSnapshot) MonitorMetrics {
	heapDelta := int64(end.heapAllocBytes) - int64(start.heapAllocBytes)
	cpuUserDelta := int64(0)
	cpuSystemDelta := int64(0)
	if start.cpuSupported && end.cpuSupported {
		cpuUserDelta = end.cpuUserMicros - start.cpuUserMicros
		cpuSystemDelta = end.cpuSystemMicros - start.cpuSystemMicros
	}
	peakHeap := peak.heapAllocBytes
	if peakHeap < end.heapAllocBytes {
		peakHeap = end.heapAllocBytes
	}
	peakGoroutines := peak.goroutines
	if peakGoroutines < end.goroutines {
		peakGoroutines = end.goroutines
	}
	return MonitorMetrics{WallMS: durationMillis(start.time, end.time), HeapAllocBytes: end.heapAllocBytes, HeapAllocDeltaBytes: heapDelta, TotalAllocDeltaBytes: end.totalAllocBytes - start.totalAllocBytes, SysBytes: end.sysBytes, HeapInuseBytes: end.heapInuseBytes, StackInuseBytes: end.stackInuseBytes, GCDelta: end.numGC - start.numGC, Goroutines: end.goroutines, PeakHeapAllocBytes: peakHeap, PeakGoroutines: peakGoroutines, CPUUserMicrosDelta: cpuUserDelta, CPUSystemMicrosDelta: cpuSystemDelta, CPUSupported: start.cpuSupported && end.cpuSupported, RSSBytes: end.rssBytes, PeakRSSBytes: maxUint64(peak.peakRSSBytes, end.peakRSSBytes), RSSSupported: start.rssSupported || end.rssSupported || peak.rssSupported}
}

func durationMillis(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func monitorError(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return commandErr.Code, syncops.SanitizeString(commandErr.Message)
	}
	return "internal_error", syncops.SanitizeString(err.Error())
}

func monitorLimit(limit int) int {
	if limit <= 0 {
		return monitorDefaultLimit
	}
	if limit > monitorMaxLimit {
		return monitorMaxLimit
	}
	return limit
}

func monitorRunTime(run MonitorRun) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, run.StartedAt)
	if t.IsZero() {
		t, _ = time.Parse(time.RFC3339, run.StartedAt)
	}
	return t
}

func monitorRunMatchesQuery(run MonitorRun, query string) bool {
	fields := []string{run.RunID, run.Command, run.Status, run.ErrorCode}
	for _, value := range fields {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	for key, value := range run.Facts {
		if strings.Contains(strings.ToLower(key), query) || strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	for _, step := range run.Steps {
		if strings.Contains(strings.ToLower(step.Name), query) || strings.Contains(strings.ToLower(step.Status), query) || strings.Contains(strings.ToLower(step.ErrorCode), query) {
			return true
		}
	}
	return false
}

func sanitizeMonitorFacts(facts map[string]string) map[string]string {
	clean := sanitizeActivityFacts(facts)
	if len(clean) == 0 {
		return nil
	}
	return clean
}

func monitorQueryFacts(prefix, value string) map[string]string {
	value = strings.TrimSpace(value)
	facts := map[string]string{prefix + "_length": fmt.Sprint(len(value))}
	if value != "" {
		sum := sha256.Sum256([]byte(value))
		facts[prefix+"_sha256"] = hex.EncodeToString(sum[:])[:16]
	}
	return facts
}

func addMonitorProjectionFacts(projection *domain.Projection, runID, evidence string) {
	if projection == nil || runID == "" {
		return
	}
	if projection.Facts == nil {
		projection.Facts = map[string]string{}
	}
	projection.Facts["monitor_run_id"] = runID
	if evidence != "" {
		projection.Evidence = append(projection.Evidence, evidence)
	}
}
