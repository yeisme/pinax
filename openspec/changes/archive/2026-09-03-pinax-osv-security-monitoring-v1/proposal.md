# pinax-osv-security-monitoring-v1

## Why

Pinax 已发版并进入 yeisme-dist，但 CI 没有 govulncheck，也没有跨锁文件定时扫描。

## What Changes

- 增加每周 OSV-Scanner（第一波不挡 PR）。
- `task ci` 增加 pinned `govulncheck@v1.6.0`。
- `osv-scanner.toml` 与发版文档。

## Impact

根 handoff：`openspec/changes/supply-chain-security-monitoring-v1/`。
