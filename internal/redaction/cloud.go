package redaction

import "regexp"

var (
	authorizationPattern = regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+[^\s]+`)
	tokenPattern         = regexp.MustCompile(`(?i)(token=)[^\s&]+`)
	pathPattern          = regexp.MustCompile(`(?i)(path=)[^\s]+\.md`)
	secretRefPattern     = regexp.MustCompile(`(?i)(secret_ref=)(op://|keychain://|env://)[^\s]+`)
)

func Cloud(input string) string {
	out := authorizationPattern.ReplaceAllString(input, "Authorization: Bearer [REDACTED]")
	out = tokenPattern.ReplaceAllString(out, "${1}[REDACTED]")
	out = pathPattern.ReplaceAllString(out, "${1}[REDACTED_PATH]")
	out = secretRefPattern.ReplaceAllString(out, "${1}[REDACTED_SECRET_REF]")
	return out
}
