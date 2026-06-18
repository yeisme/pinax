// Package searchops owns app-layer search and query workflows behind the app.Service facade.
//
// Command family: list, search, query, and database view commands.
// Responsibility: app-level list/search/query orchestration and database view use cases.
// Prohibited dependencies: internal/cli, internal/output, Cobra command parsing, direct renderer calls.
// Focused tests: go test ./internal/app ./internal/index ./cmd/pinax -run 'Search|Query|Database|List' -count=1
package searchops
