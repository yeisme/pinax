// Package publishops owns pure rules for the publish command family.
//
// Command family: publish profile, publish plan, publish build, publish deploy, publish doctor, publish theme.
// Responsibility: validate publish profiles, classify note and asset eligibility, enforce target policy, classify publish safety violations, and shape manifest-ready domain data.
// Prohibited dependencies: internal/cli, internal/output, Cobra/pflag, provider clients, Git subprocesses, Hugo subprocesses, and direct filesystem writes.
// Focused tests: go test ./internal/app/publishops -run Publish -count=1.
package publishops
