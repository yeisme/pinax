# pinax-supply-chain-scan

## ADDED Requirements

### Requirement: Pinax SHALL run weekly OSV-Scanner without failing pull requests
`.github/workflows/security.yml` SHALL 每周运行 OSV-Scanner v2.5.1，第一波 MUST `continue-on-error`。

#### Scenario: 定时任务触发
- **WHEN** 周一 cron 运行
- **THEN** workflow SHALL 产出扫描 artifact
- **AND** pull_request 的 fast CI SHALL 不依赖该 job 成功

### Requirement: Pinax CI SHALL run pinned govulncheck
`task ci` SHALL 运行 `govulncheck@v1.6.0`。

#### Scenario: 引入可达漏洞
- **WHEN** govulncheck 报告调用图上的漏洞
- **THEN** `task ci` SHALL 失败
