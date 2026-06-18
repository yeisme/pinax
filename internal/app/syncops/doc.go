// Package syncops owns app-layer cloud and backend sync workflows behind the app.Service facade.
//
// Command family: cloud, sync, backend, conflict, and sync log commands.
// Responsibility: sync push/pull/diff orchestration, backend provider coordination, sync logs, and conflict workflows.
// Prohibited dependencies: internal/cli, internal/output, real credentials in tests, direct stdout/stderr writes.
// Focused tests: go test ./internal/app ./internal/cloudsync ./internal/cloudclient ./cmd/pinax ./tests/e2e -run 'Cloud|Sync|Conflict|Backend|Redaction' -count=1
package syncops
