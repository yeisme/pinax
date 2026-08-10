# `pinax backup`

`pinax backup` is the personal local safety entry point. It uses the existing local version/Git snapshot and restore services; it does not start a daemon, contact a remote provider, or create a second backup format.

## Everyday commands

Check whether the vault can create local snapshots:

```bash
pinax backup --vault ./my-notes
pinax backup status --vault ./my-notes --json
```

Create and inspect a checkpoint:

```bash
pinax backup create --message "daily checkpoint" --vault ./my-notes
pinax backup history --vault ./my-notes --json
```

Restore remains plan-first and explicitly approved:

```bash
pinax backup restore notes/example.md --revision <snapshot_id> --plan --vault ./my-notes --json
pinax backup restore apply --plan <restore_plan_id> --yes --vault ./my-notes --json
```

The plan command is read-only. Only `restore apply --yes` writes Markdown, and a successful apply reports `local_write=true` and `remote_write=false`.

## Relationship to advanced commands

- `pinax version` remains available for existing scripts and lower-level `diff`, `show`, `changed`, and backend inspection.
- `pinax sync` and `pinax capsa` remain advanced multi-device or remote transport workflows.
- `pinax backup` does not imply that S3, rclone, or a Capsa server contains a durable copy. Use the remote workflow's own commit and read-back evidence before treating it as off-device backup.

Run `pinax commands` to inspect every advanced command without adding them back to the default help.
