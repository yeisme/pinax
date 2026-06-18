// Package planningops owns app-layer planning workflows behind the app.Service facade.
//
// Command family: plan and planning workflow commands.
// Responsibility: planning workflow orchestration, plan state transitions, and facade-facing planning results.
// Prohibited dependencies: internal/cli, internal/output, root OpenSpec governance ownership, direct stdout/stderr writes.
// Focused tests: go test ./internal/app ./cmd/pinax -run 'Plan|Planning|Task|Decision' -count=1
package planningops
