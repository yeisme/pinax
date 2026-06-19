# publish

`pinax publish` builds reviewable static publishing surfaces from a local Pinax vault. The vault remains the source of truth. GitHub Pages, GitHub Wiki, GitHub Gist, HTTP endpoints, and local preview are delivery surfaces only, and Cloud Sync remains a separate encrypted sync workflow.

## Minimal Flow

```bash
pinax publish profile init public --target github-pages --renderer hugo --title "Knowledge" --base-url https://example.github.io/kb/ --vault ./my-notes --json
pinax publish plan --profile public --target github-pages --vault ./my-notes --json
pinax publish build --profile public --target github-pages --out ./dist/site --vault ./my-notes --json
pinax publish deploy --profile public --target github-pages --out ./dist/site --repo ../kb-pages --yes --vault ./my-notes --json
```

For GitHub Wiki output, use `--target github-wiki --renderer none` and deploy to a separate Wiki repository path:

```bash
pinax publish profile init wiki --target github-wiki --renderer none --vault ./my-notes --json
pinax publish plan --profile wiki --target github-wiki --vault ./my-notes --json
pinax publish build --profile wiki --target github-wiki --out ./dist/wiki --vault ./my-notes --json
pinax publish deploy --profile wiki --target github-wiki --out ./dist/wiki --repo ../kb.wiki --yes --vault ./my-notes --json
```

For a single Markdown bundle shared through GitHub Gist, use `github-gist` and deploy through the local `gh` CLI:

```bash
pinax publish profile init gist --target github-gist --renderer none --vault ./my-notes --json
pinax publish build --profile gist --target github-gist --out ./dist/gist --vault ./my-notes --json
pinax publish deploy --profile gist --target github-gist --out ./dist/gist --yes --vault ./my-notes --json
```

For a controlled HTTP delivery surface, send the scanned manifest and Markdown bundle to an HTTPS endpoint. Authentication uses secret references, not literal tokens in profiles or command output:

```bash
pinax publish profile init share --target http --renderer none --vault ./my-notes --json
pinax publish build --profile share --target http --out ./dist/share --vault ./my-notes --json
pinax publish deploy --profile share --target http --out ./dist/share --endpoint https://share.example.test/publish --secret-ref env:PINAX_SHARE_TOKEN --yes --vault ./my-notes --json
```

For local review, serve an already-built output directory on loopback:

```bash
pinax publish serve --profile wiki --out ./dist/wiki --host 127.0.0.1 --port 4173 --vault ./my-notes
```

## Safety Boundaries

- Do not publish the private vault repository directly. Build into `dist/` and deploy only the generated output to a separate Pages/Wiki repository, Gist, HTTP endpoint, or local preview.
- `publish plan` is read-only and reports selected, skipped and blocking items without dumping private note bodies.
- `publish build` scans staging and final output for secrets, Authorization/Cookie headers, provider payloads, absolute paths, `.pinax` internals and private-body leaks before writing a success receipt.
- `publish deploy` requires `--yes`, validates the latest receipt and output hash, scans output again, and rejects deploy targets at the vault root or inside `.pinax/**`.
- Gist deploy uses the system `gh` CLI and does not store GitHub credentials. HTTP deploy accepts HTTPS or loopback HTTP endpoints and optional `env:` secret references only.
- `publish serve` is a loopback preview command, not a required daemon or hosted notebook service.
- Machine output modes stay protocol-only: `--json`, `--agent`, `--events` and `--explain` do not change business behavior.

## Hugo and Themes

GitHub Pages uses Hugo through the local `hugo` executable. If Hugo is unavailable, Wiki build remains available because it emits Markdown directly.

The built-in theme is `builtin:pinax-encyclopedia` and follows `pinax.publish_theme.v1`. Theme commands:

```bash
pinax publish theme list --vault ./my-notes --json
pinax publish theme eject pinax-encyclopedia --out ./review/theme --vault ./my-notes --json
```

Profiles may use `local:<path>` theme sources. The path must stay inside the vault, must not point into `.pinax/**`, and is copied into Hugo staging before the same scans run. The built-in theme uses local CSS/JS only; it does not include external CDN assets, remote fonts, analytics or remote images.

## CI Recommendation

A conservative CI job should run `publish plan`, `publish build`, inspect the receipt, and only then `publish deploy --yes` against a clean/orphan publishing repository. GitHub private Pages availability and permissions are controlled by GitHub settings, not by Pinax.
