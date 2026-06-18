// Package versionops owns app-layer version-control workflows behind the app.Service facade.
//
// Command family: version and record history commands that coordinate version backends.
// Responsibility: version-control use case orchestration and facade-facing version operation results.
// Prohibited dependencies: internal/cli, internal/output, Git porcelain parsing in command code, direct renderer calls.
// Focused tests: go test ./internal/app ./internal/version ./cmd/pinax -run 'Version|Record|History|Rollback' -count=1
package versionops
