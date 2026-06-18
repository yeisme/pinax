// Package templateops owns app-layer template and journal workflows behind the app.Service facade.
//
// Command family: template, journal, render run, and index page commands.
// Responsibility: template resolution, journal creation, render run orchestration, and index page generation.
// Prohibited dependencies: internal/cli, internal/output, version backend implementation, direct stdout/stderr writes.
// Focused tests: go test ./internal/app ./internal/templateengine ./cmd/pinax ./tests/e2e -run 'Template|Journal|Render|IndexPage' -count=1
package templateops
