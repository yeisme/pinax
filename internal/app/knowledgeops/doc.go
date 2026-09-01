// Package knowledgeops owns allowlisted knowledge projection export rules
// behind the app.Service facade.
//
// Command family: knowledge export-projection.
// Responsibility: dual-condition allowlist matching, refs-only projection
// entries, tombstone revocation, and digest-diff incremental selection.
// Prohibited dependencies: internal/cli, internal/output, direct stdout/stderr
// writes, provider token handling, Inferrum/txtai clients, vault or index writes.
// Focused tests: go test ./internal/app/knowledgeops ./internal/app ./cmd/pinax -run 'Knowledge' -count=1
package knowledgeops
