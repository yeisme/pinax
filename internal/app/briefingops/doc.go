// Package briefingops owns app-layer briefing workflows behind the app.Service facade.
//
// Command family: briefing and provider-backed research summary commands.
// Responsibility: briefing use case orchestration, provider adapter dispatch, and redacted evidence handoff.
// Prohibited dependencies: internal/cli, internal/output, raw provider payload output, provider token persistence.
// Focused tests: go test ./internal/app ./cmd/pinax -run 'Briefing|Provider|Research|Redaction' -count=1
package briefingops
