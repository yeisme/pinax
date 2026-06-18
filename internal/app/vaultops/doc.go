// Package vaultops owns app-layer vault maintenance workflows behind the app.Service facade.
//
// Command family: init, validate, project, storage, stats, doctor, repair, and organize commands.
// Responsibility: vault setup, validation, maintenance, repair planning, repair apply, and organization workflows.
// Prohibited dependencies: internal/cli, internal/output, cloud sync protocol ownership, direct user-visible rendering.
// Focused tests: go test ./internal/app ./cmd/pinax -run 'Vault|Init|Validate|Project|Storage|Stats|Doctor|Repair|Organize' -count=1
package vaultops
