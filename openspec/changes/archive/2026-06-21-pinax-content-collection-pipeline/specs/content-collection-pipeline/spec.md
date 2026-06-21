## ADDED Requirements

### Requirement: Pinax SHALL import content bundles into vault-owned assets

Pinax SHALL accept `pinax.content_bundle.v1` files and turn them into local Markdown notes and prompt assets through CLI-owned services.

#### Scenario: Preview collection import
- **WHEN** the user runs `pinax collection import --from <bundle> --dry-run --json`
- **THEN** Pinax SHALL report item counts, complete prompt counts, missing prompt counts, and planned note/prompt writes
- **AND** it SHALL NOT write notes, `.pinax/`, Git state, providers, or remote services.

#### Scenario: Confirm collection import
- **WHEN** the user runs `pinax collection import --from <bundle> --yes --json`
- **THEN** Pinax SHALL write collection notes, complete prompt assets, index updates, receipt evidence, and safe event evidence.

### Requirement: Pinax SHALL diagnose and export content collections

Pinax SHALL expose read-only collection diff and doctor commands, and SHALL export local prompt assets to `eikona.prompt_bundle.v1` when the user requests an output file.

#### Scenario: Diagnose missing prompt text
- **WHEN** a bundle item has empty prompt text
- **THEN** `pinax collection doctor --from <bundle> --json` SHALL report missing prompt item counts without inventing prompt content.

#### Scenario: Export Eikona prompt bundle
- **WHEN** the user runs `pinax collection export --to <file> --format eikona.prompt_bundle.v1 --json`
- **THEN** Pinax SHALL write a prompt bundle file from local prompt assets
- **AND** command output SHALL remain a bounded projection.

### Requirement: Pinax SHALL expose a rebuildable prompt knowledge graph

Pinax SHALL derive a local prompt graph projection from prompt asset source refs and dimension tags while keeping vault assets as the source records.

#### Scenario: Rebuild prompt graph
- **WHEN** the user runs `pinax graph rebuild --json`
- **THEN** Pinax SHALL write `.pinax/graph/prompt_graph.json` with prompt/source/category/technique/style/subject nodes and edges.

#### Scenario: Query prompt graph safely
- **WHEN** the user runs `pinax graph query --kind technique --match storyboard --agent`
- **THEN** Pinax SHALL return bounded prompt asset facts
- **AND** it SHALL NOT emit full prompt bodies or local filesystem paths.
