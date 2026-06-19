## MODIFIED Requirements

### Requirement: Publish profiles define static publishing policy
Pinax SHALL manage static publishing policy through CLI-authored publish profiles stored as structured vault metadata.

#### Scenario: Initialize a publish profile
- **WHEN** a user runs `pinax publish profile init public --target github-pages --renderer hugo --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update `.pinax/publish/profiles/public.yaml` through the application service
- **AND** the profile SHALL include `schema_version`, `name`, `target`, `renderer`, selection rules, body policy, asset policy and deploy policy
- **AND** stdout SHALL contain exactly one JSON projection without human prose outside the envelope.

#### Scenario: Initialize a Gist or HTTP sharing profile
- **WHEN** a user runs `pinax publish profile init gist --target github-gist --renderer none --vault ./my-notes --json` or `pinax publish profile init http --target http --renderer none --vault ./my-notes --json`
- **THEN** Pinax SHALL create a publish profile using the same selection, asset, body, safety and theme defaults as other publish targets
- **AND** validation SHALL accept `github-gist` and `http` as delivery targets, not as vault sources of truth.

### Requirement: Publish deploy only writes to explicit publish targets
Pinax SHALL deploy static publish output only after explicit confirmation and only to the configured publishing target, never to the private vault source.

#### Scenario: Deploy to GitHub Pages branch
- **WHEN** a user runs `pinax publish deploy --profile public --target github-pages --out ./dist/site --repo <repo> --branch gh-pages --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL verify the output manifest and scan result before deploying
- **AND** it SHALL commit and push only the generated publish output to the target repository branch
- **AND** it SHALL not modify Markdown note source files or private vault metadata except for CLI-authored publish receipts.

#### Scenario: Deploy to GitHub Gist through gh CLI
- **WHEN** a user runs `pinax publish deploy --profile gist --target github-gist --out ./dist/gist --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL verify the output receipt, hash and scan result before invoking `gh gist create` or `gh gist edit`
- **AND** it SHALL NOT store GitHub tokens, cookies, Authorization headers or `gh` configuration in the vault
- **AND** stdout SHALL report stable facts such as `mode=gist`, `target=github-gist`, `files` and optional redacted URL.

#### Scenario: Deploy to an HTTP sharing endpoint
- **WHEN** a user runs `pinax publish deploy --profile http --target http --out ./dist/http --endpoint https://share.example.test/publish --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL verify the output receipt, hash and scan result before sending the manifest and content bundle
- **AND** it SHALL use only configured safe endpoint schemes and optional `env:` secret references
- **AND** stdout SHALL report stable facts such as `mode=http`, `target=http`, `http_status` and optional redacted URL without echoing secrets.

### Requirement: Local publish preview is loopback-only
Pinax SHALL provide a local preview command for already-built publish output without turning Pinax into a required daemon.

#### Scenario: Serve one preview smoke request
- **WHEN** a user runs `pinax publish serve --profile wiki --out ./dist/wiki --host 127.0.0.1 --port 0 --once --vault ./my-notes --json`
- **THEN** Pinax SHALL serve the output directory on a loopback address, perform one local request, and exit
- **AND** stdout SHALL include a `publish.serve` projection with `served=true`, host, port and URL facts
- **AND** it SHALL NOT expose the private vault root, `.pinax/**`, provider credentials or private note bodies.

### Requirement: Publish artifacts are auditable and reproducible
Pinax SHALL write publish manifests, receipts and evidence that describe what was generated without leaking private source content.

#### Scenario: Markdown sharing bundle is auditable
- **WHEN** `publish build` completes successfully for `github-gist` or `http`
- **THEN** Pinax SHALL write `pinax-gist.md`, `pinax-publish-manifest.json` and a CLI-authored publish receipt
- **AND** the manifest and receipt SHALL use the same scan, hash, selected-count and redaction summary rules as Pages/Wiki builds.
