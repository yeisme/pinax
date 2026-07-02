# Pinax Claude Instructions

Follow [AGENTS.md](./AGENTS.md).

This subproject owns its product, design, interface, runtime, guide, review, and implementation documents under `docs/`. Do not write or require root project-doc mirrors for Pinax tasks.

Use repository-root `.skills/profiles/targets/cli/pinax.txt` as the skill assignment source. Runtime skill copies under `.agents/skills/` and `.claude/skills/` are generated; do not edit them as source. After changing skill source or profiles, sync from the repository root with `scripts/skills.sh sync-root` and `scripts/skills.sh sync-subprojects`.

Implementation plans, validation evidence, and closeout belong under `openspec/changes/pinax-*`.

## Health Stack

- typecheck: go vet ./...
- lint: golangci-lint run
- test: go test ./...
- deadcode: golangci-lint run --enable-only unused
- openspec: openspec validate --all


