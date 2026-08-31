package redaction

import (
	"regexp"
	"strings"
)

var (
	authorizationPattern = regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+[^\s]+`)
	tokenPattern         = regexp.MustCompile(`(?i)(token=)[^\s&]+`)
	pathPattern          = regexp.MustCompile(`(?i)(path=)[^\s]+\.md`)
	secretRefPattern     = regexp.MustCompile(`(?i)(secret_ref=)(op://|keychain://|env://)[^\s]+`)
	// S3/COS-compatible CLIs print auth failures in these forms; rclone
	// specifically surfaces access keys and passwords on stderr.
	accessKeyIDPattern     = regexp.MustCompile(`(?i)(access_key_id=)[^\s&]+`)
	secretAccessKeyPattern = regexp.MustCompile(`(?i)(secret_access_key=)[^\s&]+`)
	passwordPattern        = regexp.MustCompile(`(?i)(password=)[^\s&]+`)
	apiKeyPattern          = regexp.MustCompile(`(?i)(api_key=)[^\s&]+`)
)

func Cloud(input string) string {
	out := authorizationPattern.ReplaceAllString(input, "Authorization: Bearer [REDACTED]")
	out = tokenPattern.ReplaceAllString(out, "${1}[REDACTED]")
	out = pathPattern.ReplaceAllString(out, "${1}[REDACTED_PATH]")
	out = secretRefPattern.ReplaceAllString(out, "${1}[REDACTED_SECRET_REF]")
	out = accessKeyIDPattern.ReplaceAllString(out, "${1}[REDACTED]")
	out = secretAccessKeyPattern.ReplaceAllString(out, "${1}[REDACTED]")
	out = passwordPattern.ReplaceAllString(out, "${1}[REDACTED]")
	out = apiKeyPattern.ReplaceAllString(out, "${1}[REDACTED]")
	return out
}

// Credentials redacts standard credential shapes and any exact secret values
// already known to the caller. It is intended for diagnostics and evidence,
// never as a substitute for keeping secrets out of those surfaces entirely.
func Credentials(input string, secrets ...string) string {
	out := Cloud(input)
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		out = strings.ReplaceAll(out, secret, "[REDACTED]")
	}
	return out
}
