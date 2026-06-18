// Package noteops owns app-layer note workflows behind the app.Service facade.
//
// Command family: note, folder, metadata, import, export, attachment commands.
// Responsibility: note CRUD, metadata, tags, folders, imports, exports, and attachments.
// Prohibited dependencies: internal/cli, internal/output, direct stdout/stderr writes, provider token handling.
// Focused tests: go test ./internal/app ./cmd/pinax -run 'Note|Folder|Metadata|Import|Export|Attachment|Record' -count=1
package noteops
