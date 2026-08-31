# Security policy

请勿在公开 issue 中提交凭据、签名 URL、Prompt 正文或可复现的敏感 catalog。请通过
GitHub Security Advisories 私密报告安全问题，并提供受影响版本、最小复现步骤和影响。

Do not submit credentials, signed URLs, prompt bodies, or sensitive catalogs in
public issues. Report vulnerabilities privately through GitHub Security
Advisories with the affected version, a minimal reproduction, and impact.

Supported releases are the latest stable release and the current release
candidate. We acknowledge reports within five business days and coordinate a
fix and disclosure timeline privately.

The SDK intentionally keeps credential values out of repository profiles and
rejects source URI credentials in built-in adapters. Reports involving source
path traversal, Git argument handling, state locking, digest validation, or
secret exposure are in scope.
